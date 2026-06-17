package pipeline

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/lynxbase/lynxdb/pkg/event"
)

const maxGapfillRows = 1_000_000

// GapfillIterator inserts missing _time buckets between query or observed bounds per group.
type GapfillIterator struct {
	child     Iterator
	span      time.Duration
	fill      event.Value
	by        []string
	batchSize int
	start     *time.Time
	end       *time.Time

	output []map[string]event.Value
	offset int
}

func NewGapfillIterator(child Iterator, span time.Duration, fill event.Value, by []string, batchSize int) *GapfillIterator {
	return NewGapfillIteratorWithBounds(child, span, fill, by, batchSize, nil, nil)
}

func NewGapfillIteratorWithBounds(
	child Iterator,
	span time.Duration,
	fill event.Value,
	by []string,
	batchSize int,
	start *time.Time,
	end *time.Time,
) *GapfillIterator {
	return &GapfillIterator{
		child:     child,
		span:      span,
		fill:      fill,
		by:        append([]string(nil), by...),
		batchSize: batchSize,
		start:     cloneTimePtr(start),
		end:       cloneTimePtr(end),
	}
}

func (g *GapfillIterator) Init(ctx context.Context) error {
	return g.child.Init(ctx)
}

func (g *GapfillIterator) Next(ctx context.Context) (*Batch, error) {
	if g.output == nil {
		if err := g.materialize(ctx); err != nil {
			return nil, err
		}
	}
	if g.offset >= len(g.output) {
		return nil, nil
	}

	limit := g.batchSize
	if limit <= 0 {
		limit = DefaultBatchSize
	}
	batch := NewBatch(limit)
	for batch.Len < limit && g.offset < len(g.output) {
		batch.AddRow(g.output[g.offset])
		g.offset++
	}
	return batch, nil
}

func (g *GapfillIterator) Close() error {
	return g.child.Close()
}

func (g *GapfillIterator) Schema() []FieldInfo {
	return g.child.Schema()
}

type gapfillGroup struct {
	key       string
	values    map[string]event.Value
	rows      []map[string]event.Value
	present   map[int64]bool
	minTime   time.Time
	maxTime   time.Time
	haveRange bool
}

func (g *GapfillIterator) materialize(ctx context.Context) error {
	groups := make(map[string]*gapfillGroup)
	columns := g.schemaColumns()

	for {
		batch, err := g.child.Next(ctx)
		if err != nil || batch == nil {
			if err != nil {
				return err
			}
			break
		}
		for i := 0; i < batch.Len; i++ {
			row := batch.Row(i)
			for name := range row {
				if !containsString(columns, name) {
					columns = append(columns, name)
				}
			}
			t, ok := row["_time"].TryAsTimestamp()
			if !ok {
				continue
			}
			key, values := g.groupKey(row)
			group := groups[key]
			if group == nil {
				group = &gapfillGroup{
					key:     key,
					values:  values,
					present: make(map[int64]bool),
				}
				groups[key] = group
			}
			group.rows = append(group.rows, copyRow(row))
			group.present[t.UnixNano()] = true
			if !group.haveRange || t.Before(group.minTime) {
				group.minTime = t
			}
			if !group.haveRange || t.After(group.maxTime) {
				group.maxTime = t
			}
			group.haveRange = true
		}
	}

	sort.Strings(columns)
	g.output = g.buildRows(groups, columns)
	return nil
}

func (g *GapfillIterator) schemaColumns() []string {
	schema := g.child.Schema()
	columns := make([]string, 0, len(schema))
	for _, f := range schema {
		columns = append(columns, f.Name)
	}
	return columns
}

func (g *GapfillIterator) groupKey(row map[string]event.Value) (string, map[string]event.Value) {
	if len(g.by) == 0 {
		return "", nil
	}
	parts := make([]string, len(g.by))
	values := make(map[string]event.Value, len(g.by))
	for i, field := range g.by {
		v := row[field]
		values[field] = v
		parts[i] = field + "=" + v.String()
	}
	return strings.Join(parts, "\x00"), values
}

func (g *GapfillIterator) buildRows(groups map[string]*gapfillGroup, columns []string) []map[string]event.Value {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	rows := make([]map[string]event.Value, 0)
	for _, key := range keys {
		group := groups[key]
		start, end := g.groupBounds(group)
		for t := start; !t.After(end); t = t.Add(g.span) {
			if len(rows) >= maxGapfillRows {
				return nil
			}
			if !group.present[t.UnixNano()] {
				rows = append(rows, g.syntheticRow(group, columns, t))
			}
		}
		rows = append(rows, group.rows...)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		gi, _ := g.groupKey(rows[i])
		gj, _ := g.groupKey(rows[j])
		if gi != gj {
			return gi < gj
		}
		ti, _ := rows[i]["_time"].TryAsTimestamp()
		tj, _ := rows[j]["_time"].TryAsTimestamp()
		return ti.Before(tj)
	})
	return rows
}

func (g *GapfillIterator) groupBounds(group *gapfillGroup) (time.Time, time.Time) {
	start := group.minTime
	end := group.maxTime
	if g.start != nil {
		start = *g.start
	}
	if g.end != nil {
		end = *g.end
	}
	return start, end
}

func (g *GapfillIterator) syntheticRow(group *gapfillGroup, columns []string, t time.Time) map[string]event.Value {
	row := make(map[string]event.Value, len(columns))
	for _, column := range columns {
		switch column {
		case "_time":
			row[column] = event.TimestampValue(t)
		default:
			if v, ok := group.values[column]; ok {
				row[column] = v
			} else {
				row[column] = g.fill
			}
		}
	}
	return row
}

func copyRow(row map[string]event.Value) map[string]event.Value {
	out := make(map[string]event.Value, len(row))
	for k, v := range row {
		out[k] = v
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	out := *t
	return &out
}
