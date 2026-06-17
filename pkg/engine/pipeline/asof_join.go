package pipeline

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/lynxbase/lynxdb/pkg/event"
)

type asofRightRow struct {
	ts  time.Time
	row map[string]event.Value
}

// AsofJoinIterator joins each left row to the latest right row with the same
// key whose _time is less than or equal to the left _time.
type AsofJoinIterator struct {
	left      Iterator
	right     Iterator
	keys      []string
	tolerance *time.Duration
	batchSize int
	index     map[string][]asofRightRow
	built     bool
	leftBatch *Batch
	leftOff   int
}

func NewAsofJoinIterator(
	left Iterator,
	right Iterator,
	keys []string,
	tolerance *time.Duration,
	batchSize int,
) *AsofJoinIterator {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	return &AsofJoinIterator{
		left:      left,
		right:     right,
		keys:      append([]string(nil), keys...),
		tolerance: tolerance,
		batchSize: batchSize,
		index:     make(map[string][]asofRightRow),
	}
}

func (a *AsofJoinIterator) Init(ctx context.Context) error {
	if err := a.left.Init(ctx); err != nil {
		return err
	}
	if err := a.right.Init(ctx); err != nil {
		return err
	}
	if err := a.buildRightIndex(ctx); err != nil {
		return err
	}
	a.built = true
	return nil
}

func (a *AsofJoinIterator) Next(ctx context.Context) (*Batch, error) {
	if !a.built {
		if err := a.buildRightIndex(ctx); err != nil {
			return nil, err
		}
		a.built = true
	}

	out := NewBatch(a.batchSize)
	for out.Len < a.batchSize {
		if a.leftBatch == nil || a.leftOff >= a.leftBatch.Len {
			batch, err := a.left.Next(ctx)
			if err != nil {
				return nil, err
			}
			if batch == nil {
				if out.Len == 0 {
					return nil, nil
				}
				return out, nil
			}
			a.leftBatch = batch
			a.leftOff = 0
		}

		row := a.leftBatch.Row(a.leftOff)
		a.leftOff++
		match := a.match(row)
		if match == nil {
			continue
		}
		out.AddRow(mergeRows(row, match))
	}

	return out, nil
}

func (a *AsofJoinIterator) Close() error {
	return errors.Join(a.left.Close(), a.right.Close())
}

func (a *AsofJoinIterator) Schema() []FieldInfo { return nil }

func (a *AsofJoinIterator) buildRightIndex(ctx context.Context) error {
	for {
		batch, err := a.right.Next(ctx)
		if err != nil {
			return err
		}
		if batch == nil {
			break
		}
		for i := 0; i < batch.Len; i++ {
			row := batch.Row(i)
			ts, ok := asofRowTimestamp(row)
			if !ok {
				continue
			}
			key := asofJoinKey(row, a.keys)
			a.index[key] = append(a.index[key], asofRightRow{ts: ts, row: row})
		}
	}
	for key := range a.index {
		sort.SliceStable(a.index[key], func(i, j int) bool {
			return a.index[key][i].ts.Before(a.index[key][j].ts)
		})
	}
	return nil
}

func (a *AsofJoinIterator) match(row map[string]event.Value) map[string]event.Value {
	leftTS, ok := asofRowTimestamp(row)
	if !ok {
		return nil
	}
	candidates := a.index[asofJoinKey(row, a.keys)]
	if len(candidates) == 0 {
		return nil
	}
	idx := sort.Search(len(candidates), func(i int) bool {
		return candidates[i].ts.After(leftTS)
	})
	if idx == 0 {
		return nil
	}
	candidate := candidates[idx-1]
	if a.tolerance != nil && leftTS.Sub(candidate.ts) > *a.tolerance {
		return nil
	}
	return candidate.row
}

func asofRowTimestamp(row map[string]event.Value) (time.Time, bool) {
	v, ok := row["_time"]
	if !ok {
		return time.Time{}, false
	}
	return v.TryAsTimestamp()
}

func asofJoinKey(row map[string]event.Value, keys []string) string {
	parts := make([]string, len(keys))
	for i, key := range keys {
		v := row[key]
		parts[i] = v.Type().String() + ":" + v.String()
	}
	return strings.Join(parts, "\x00")
}
