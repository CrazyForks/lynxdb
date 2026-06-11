package query

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/lynxbase/lynxdb/pkg/cluster"
	"github.com/lynxbase/lynxdb/pkg/cluster/sharding"
	"github.com/lynxbase/lynxdb/pkg/config"
	"github.com/lynxbase/lynxdb/pkg/model"
	"github.com/lynxbase/lynxdb/pkg/storage/sources"
)

func newTestShardPruner(t *testing.T, indexes []string, assignments map[string]*sharding.ShardAssignment, addrs map[sharding.NodeID]string, partCount int) *ShardPruner {
	t.Helper()

	reg := sources.NewRegistry()
	for _, idx := range indexes {
		reg.Register(idx)
	}

	cache := cluster.NewShardMapCache()
	sm := sharding.NewShardMap()
	for _, a := range assignments {
		sm.SetAssignment(a)
	}
	cache.Update(sm)

	return NewShardPruner(
		cache,
		reg,
		config.ClusterConfig{
			VirtualPartitionCount: partCount,
			TimeBucketSize:        config.Duration(24 * time.Hour),
		},
		func() map[sharding.NodeID]string { return addrs },
		slog.Default(),
	)
}

func TestShardPruner_EmptyIndexes(t *testing.T) {
	pruner := newTestShardPruner(t, nil, nil, nil, 2)
	hints := &model.QueryHints{SourceScopeType: model.SourceScopeAll}

	targets, err := pruner.FindRelevantShards(context.Background(), hints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 0 {
		t.Errorf("expected 0 targets for empty registry, got %d", len(targets))
	}
}

func TestShardPruner_SingleIndex_TimeBounds(t *testing.T) {
	now := time.Now().UTC()
	bucket := now.Truncate(24 * time.Hour)

	sid := sharding.ShardID{
		Index:      "main",
		TimeBucket: bucket,
		Partition:  0,
	}

	assignments := map[string]*sharding.ShardAssignment{
		sid.String(): {
			ShardID: sid,
			Primary: "node-1",
		},
	}
	addrs := map[sharding.NodeID]string{
		"node-1": "localhost:9400",
	}

	pruner := newTestShardPruner(t, []string{"main"}, assignments, addrs, 1)

	hints := &model.QueryHints{
		SourceScopeType:    model.SourceScopeSingle,
		SourceScopeSources: []string{"main"},
		TimeBounds: &model.TimeBounds{
			Earliest: now.Add(-1 * time.Hour),
			Latest:   now,
		},
	}

	targets, err := pruner.FindRelevantShards(context.Background(), hints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].NodeAddr != "localhost:9400" {
		t.Errorf("expected localhost:9400, got %s", targets[0].NodeAddr)
	}
	if targets[0].NodeID != "node-1" {
		t.Errorf("expected node-1, got %s", targets[0].NodeID)
	}
}

func TestShardPruner_MultiIndex(t *testing.T) {
	now := time.Now().UTC()
	bucket := now.Truncate(24 * time.Hour)

	assignments := make(map[string]*sharding.ShardAssignment)
	for _, idx := range []string{"web", "api"} {
		sid := sharding.ShardID{
			Index:      idx,
			TimeBucket: bucket,
			Partition:  0,
		}
		assignments[sid.String()] = &sharding.ShardAssignment{
			ShardID: sid,
			Primary: "node-1",
		}
	}

	addrs := map[sharding.NodeID]string{"node-1": "localhost:9400"}
	pruner := newTestShardPruner(t, []string{"web", "api"}, assignments, addrs, 1)

	hints := &model.QueryHints{
		SourceScopeType: model.SourceScopeAll,
		TimeBounds: &model.TimeBounds{
			Earliest: now.Add(-1 * time.Hour),
			Latest:   now,
		},
	}

	targets, err := pruner.FindRelevantShards(context.Background(), hints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets (web+api), got %d", len(targets))
	}
}

func TestShardPruner_NoAssignment_Skipped(t *testing.T) {
	now := time.Now().UTC()

	// Register an index but don't add any shard assignment.
	pruner := newTestShardPruner(t, []string{"main"}, nil, nil, 1)

	hints := &model.QueryHints{
		SourceScopeType:    model.SourceScopeSingle,
		SourceScopeSources: []string{"main"},
		TimeBounds: &model.TimeBounds{
			Earliest: now.Add(-1 * time.Hour),
			Latest:   now,
		},
	}

	targets, err := pruner.FindRelevantShards(context.Background(), hints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 0 {
		t.Errorf("expected 0 targets (no assignment), got %d", len(targets))
	}
}

func TestShardPruner_FallbackToReplica(t *testing.T) {
	now := time.Now().UTC()
	bucket := now.Truncate(24 * time.Hour)

	sid := sharding.ShardID{
		Index:      "main",
		TimeBucket: bucket,
		Partition:  0,
	}

	assignments := map[string]*sharding.ShardAssignment{
		sid.String(): {
			ShardID:  sid,
			Primary:  "node-1",
			Replicas: []sharding.NodeID{"node-2"},
		},
	}
	// Primary has no address — should fall back to replica.
	addrs := map[sharding.NodeID]string{
		"node-2": "localhost:9402",
	}

	pruner := newTestShardPruner(t, []string{"main"}, assignments, addrs, 1)

	hints := &model.QueryHints{
		SourceScopeType:    model.SourceScopeSingle,
		SourceScopeSources: []string{"main"},
		TimeBounds: &model.TimeBounds{
			Earliest: now.Add(-1 * time.Hour),
			Latest:   now,
		},
	}

	targets, err := pruner.FindRelevantShards(context.Background(), hints)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].NodeID != "node-2" {
		t.Errorf("expected fallback to node-2, got %s", targets[0].NodeID)
	}
	if targets[0].NodeAddr != "localhost:9402" {
		t.Errorf("expected localhost:9402, got %s", targets[0].NodeAddr)
	}
}

func TestComputeTimeBuckets_NilBounds(t *testing.T) {
	buckets := computeTimeBuckets(nil, 24*time.Hour)
	// With 24h bucket size, nil bounds should return maxUnboundedBuckets+1
	// buckets (365 days back + current day).
	expected := maxUnboundedBuckets + 1
	if len(buckets) != expected {
		t.Fatalf("expected %d buckets for nil bounds, got %d", expected, len(buckets))
	}
}

func TestComputeTimeBuckets_ThreeDays(t *testing.T) {
	now := time.Now().UTC()
	bounds := &model.TimeBounds{
		Earliest: now.Add(-3 * 24 * time.Hour),
		Latest:   now,
	}
	buckets := computeTimeBuckets(bounds, 24*time.Hour)
	// 3 full days + current day = 4 buckets.
	if len(buckets) < 3 || len(buckets) > 5 {
		t.Errorf("expected 3-5 buckets for 3-day range, got %d", len(buckets))
	}
}

func TestComputeTimeBuckets_MaxCap(t *testing.T) {
	now := time.Now().UTC()
	bounds := &model.TimeBounds{
		Earliest: now.Add(-1000 * 24 * time.Hour),
		Latest:   now,
	}
	buckets := computeTimeBuckets(bounds, 24*time.Hour)
	if len(buckets) > 365 {
		t.Errorf("expected at most 365 buckets, got %d", len(buckets))
	}
}
