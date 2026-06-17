package pipeline

import (
	"context"
	"math/rand"
	"sort"
	"time"

	"github.com/lynxbase/lynxdb/pkg/event"
)

type sampledRow struct {
	index int64
	row   map[string]event.Value
}

func sampleRand(seed *int64) *rand.Rand {
	if seed != nil {
		return rand.New(rand.NewSource(*seed))
	}
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// BernoulliSampleIterator keeps each row independently with probability p.
type BernoulliSampleIterator struct {
	child   Iterator
	percent float64
	rng     *rand.Rand
	seed    *int64
}

func NewBernoulliSampleIterator(child Iterator, percent float64, seed *int64) *BernoulliSampleIterator {
	return &BernoulliSampleIterator{child: child, percent: percent, seed: seed}
}

func (s *BernoulliSampleIterator) Init(ctx context.Context) error {
	s.rng = sampleRand(s.seed)
	return s.child.Init(ctx)
}

func (s *BernoulliSampleIterator) Next(ctx context.Context) (*Batch, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch, err := s.child.Next(ctx)
		if batch == nil || err != nil {
			return nil, err
		}
		out := NewBatch(batch.Len)
		for i := 0; i < batch.Len; i++ {
			if s.rng.Float64()*100 < s.percent {
				out.AddRow(batch.Row(i))
			}
		}
		if out.Len > 0 {
			return out, nil
		}
	}
}

func (s *BernoulliSampleIterator) Close() error        { return s.child.Close() }
func (s *BernoulliSampleIterator) Schema() []FieldInfo { return s.child.Schema() }

// ReservoirSampleIterator keeps a fixed-size random sample over the full input.
type ReservoirSampleIterator struct {
	child     Iterator
	count     int
	batchSize int
	rng       *rand.Rand
	seed      *int64
	output    []sampledRow
	offset    int
}

func NewReservoirSampleIterator(child Iterator, count, batchSize int, seed *int64) *ReservoirSampleIterator {
	return &ReservoirSampleIterator{
		child:     child,
		count:     count,
		batchSize: batchSize,
		seed:      seed,
	}
}

func (s *ReservoirSampleIterator) Init(ctx context.Context) error {
	s.rng = sampleRand(s.seed)
	if err := s.child.Init(ctx); err != nil {
		return err
	}
	var seen int64
	reservoir := make([]sampledRow, 0, s.count)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch, err := s.child.Next(ctx)
		if err != nil {
			return err
		}
		if batch == nil {
			break
		}
		for i := 0; i < batch.Len; i++ {
			seen++
			item := sampledRow{index: seen - 1, row: batch.Row(i)}
			if len(reservoir) < s.count {
				reservoir = append(reservoir, item)
				continue
			}
			j := s.rng.Int63n(seen)
			if j < int64(s.count) {
				reservoir[j] = item
			}
		}
	}
	sort.Slice(reservoir, func(i, j int) bool {
		return reservoir[i].index < reservoir[j].index
	})
	s.output = reservoir

	return nil
}

func (s *ReservoirSampleIterator) Next(ctx context.Context) (*Batch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.offset >= len(s.output) {
		return nil, nil
	}
	size := s.batchSize
	if size <= 0 {
		size = DefaultBatchSize
	}
	end := s.offset + size
	if end > len(s.output) {
		end = len(s.output)
	}
	out := NewBatch(end - s.offset)
	for _, item := range s.output[s.offset:end] {
		out.AddRow(item.row)
	}
	s.offset = end

	return out, nil
}

func (s *ReservoirSampleIterator) Close() error {
	s.output = nil
	return s.child.Close()
}

func (s *ReservoirSampleIterator) Schema() []FieldInfo { return s.child.Schema() }
