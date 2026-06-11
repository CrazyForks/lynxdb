package server

import (
	"context"
	"sort"
	"time"

	enginepipeline "github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/physical"
	"github.com/lynxbase/lynxdb/pkg/memgov"
	"github.com/lynxbase/lynxdb/pkg/model"
	"github.com/lynxbase/lynxdb/pkg/storage"
)

// StreamingStats holds pre-pipeline scan statistics.
type StreamingStats struct {
	RowsScanned         int64
	ProcessedBytes      int64
	IndexesUsed         []string
	SegmentsTotal       int
	SegmentsScanned     int
	SegmentsSkippedTime int
	SegmentsSkippedBF   int
	BufferedEvents      int
}

// BuildStreamingPipeline builds the query pipeline and returns the raw Iterator
// instead of collecting all results. The caller MUST call iter.Close().
// The returned iterator holds a reference to a per-query memory budget (either
// BudgetAdapter). The budget is released when the
// caller closes the iterator; callers that abandon the iterator will leak.
func (e *Engine) BuildStreamingPipeline(ctx context.Context, prog *logical.Plan,
	externalTimeBounds *model.TimeBounds) (enginepipeline.Iterator, StreamingStats, error) {
	// RFC-002: hints must be pre-extracted by caller.
	// TODO(RFC-002): BuildStreamingPipeline should accept *model.QueryHints directly.
	hints := &model.QueryHints{}
	if externalTimeBounds != nil {
		if hints.TimeBounds == nil {
			hints.TimeBounds = externalTimeBounds
		} else {
			if !externalTimeBounds.Earliest.IsZero() &&
				(hints.TimeBounds.Earliest.IsZero() || externalTimeBounds.Earliest.After(hints.TimeBounds.Earliest)) {
				hints.TimeBounds.Earliest = externalTimeBounds.Earliest
			}
			if !externalTimeBounds.Latest.IsZero() &&
				(hints.TimeBounds.Latest.IsZero() || externalTimeBounds.Latest.Before(hints.TimeBounds.Latest)) {
				hints.TimeBounds.Latest = externalTimeBounds.Latest
			}
		}
	}

	return e.buildStreamingPipelineWithGovernor(ctx, prog, hints)
}

// buildStreamingPipelineWithGovernor uses the governor v2 for memory accounting.
func (e *Engine) buildStreamingPipelineWithGovernor(ctx context.Context, prog *logical.Plan,
	hints *model.QueryHints) (enginepipeline.Iterator, StreamingStats, error) {

	eventStore, ss, memErr := e.buildEventStore(ctx, hints, nil)
	if memErr != nil {
		return nil, StreamingStats{}, memErr
	}
	streamStats := buildStreamingStats(eventStore, ss)

	// RFC-002 Phase 10: use physical.Build with an ephemeral source backed by
	// the materialized event store, replacing the removed BuildProgramWithGovernor.
	source := physical.NewStorageSourceFromMap(eventStore, DefaultIndexName)
	iter, err := physical.Build(prog, physical.BuildOptions{
		Source: source,
		Now:    time.Now(),
	})
	if err != nil {
		return nil, streamStats, err
	}

	if err := iter.Init(ctx); err != nil {
		_ = iter.Close()
		return nil, streamStats, err
	}

	return &govClosingIterator{Iterator: iter, budget: nil}, streamStats, nil
}

// buildStreamingStats constructs StreamingStats from an event store and scan stats.
func buildStreamingStats(eventStore map[string][]*event.Event, ss storeStats) StreamingStats {
	var streamStats StreamingStats
	for name, idxEvents := range eventStore {
		streamStats.RowsScanned += int64(len(idxEvents))
		streamStats.IndexesUsed = append(streamStats.IndexesUsed, name)
	}
	sort.Strings(streamStats.IndexesUsed)
	streamStats.SegmentsTotal = ss.SegmentsTotal
	streamStats.SegmentsScanned = ss.SegmentsScanned
	streamStats.SegmentsSkippedTime = ss.SegmentsSkippedTime
	streamStats.SegmentsSkippedBF = ss.SegmentsSkippedBF
	streamStats.BufferedEvents = ss.BufferedEvents
	streamStats.ProcessedBytes = ss.TotalBytesRead
	return streamStats
}

// govClosingIterator wraps an Iterator and closes the governor BudgetAdapter
// when the iterator is closed, ensuring governor reservations are released.
type govClosingIterator struct {
	enginepipeline.Iterator
	budget *memgov.BudgetAdapter
	closed bool
}

func (g *govClosingIterator) Close() error {
	err := g.Iterator.Close()
	if !g.closed {
		if g.budget != nil {
			g.budget.Close()
		}
		g.closed = true
	}
	return err
}

// EventBus returns the engine's event bus for live subscriptions.
func (e *Engine) EventBus() *storage.EventBus {
	return e.eventBus
}
