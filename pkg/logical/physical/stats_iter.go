package physical

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/explain"
)

// StatsIterator wraps a child iterator and collects per-node runtime
// statistics (row count, batch count, wall time) for EXPLAIN ANALYZE.
//
// StatsIterator is safe for single-goroutine pull-model use (the standard
// Volcano model). The atomic fields are used so that stats can be read
// concurrently (e.g. for progress reporting) without a mutex.
type StatsIterator struct {
	child pipeline.Iterator
	stats *explain.NodeStats
	// wall tracks cumulative time spent in this node's Next calls.
	wall atomic.Int64
	rows atomic.Int64
	bat  atomic.Int64
}

// NewStatsIterator wraps child with instrumentation that records into ns.
func NewStatsIterator(child pipeline.Iterator, ns *explain.NodeStats) *StatsIterator {
	return &StatsIterator{child: child, stats: ns}
}

// Init delegates to the child iterator.
func (s *StatsIterator) Init(ctx context.Context) error {
	return s.child.Init(ctx)
}

// Next delegates to the child and records timing/row statistics.
func (s *StatsIterator) Next(ctx context.Context) (*pipeline.Batch, error) {
	start := time.Now()
	batch, err := s.child.Next(ctx)
	elapsed := time.Since(start)

	s.wall.Add(int64(elapsed))

	if batch != nil {
		s.bat.Add(1)
		s.rows.Add(int64(batch.Len))
	}

	// Flush atomics into the shared NodeStats.
	s.stats.Rows = s.rows.Load()
	s.stats.Batches = s.bat.Load()
	s.stats.WallTime = time.Duration(s.wall.Load())

	return batch, err
}

// Close delegates to the child and finalizes stats.
func (s *StatsIterator) Close() error {
	s.stats.Rows = s.rows.Load()
	s.stats.Batches = s.bat.Load()
	s.stats.WallTime = time.Duration(s.wall.Load())
	return s.child.Close()
}

// Schema delegates to the child.
func (s *StatsIterator) Schema() []pipeline.FieldInfo {
	return s.child.Schema()
}

// wrapCollect wraps iter with a StatsIterator recording into collect[n],
// if collect is non-nil. Otherwise returns iter unchanged.
func wrapCollect(iter pipeline.Iterator, n logical.Node, collect map[logical.Node]*explain.NodeStats) pipeline.Iterator {
	if collect == nil {
		return iter
	}
	ns := &explain.NodeStats{}
	collect[n] = ns
	return NewStatsIterator(iter, ns)
}
