package query

import (
	"context"
	"log/slog"
	"time"

	"github.com/lynxbase/lynxdb/pkg/cluster"
	"github.com/lynxbase/lynxdb/pkg/cluster/sharding"
	"github.com/lynxbase/lynxdb/pkg/config"
	"github.com/lynxbase/lynxdb/pkg/model"
	"github.com/lynxbase/lynxdb/pkg/storage/sources"
)

// ShardTarget identifies a specific shard and the node that owns it.
type ShardTarget struct {
	ShardID  sharding.ShardID
	NodeAddr string
	NodeID   sharding.NodeID
}

// ShardPruner resolves which shards are relevant for a query based on
// time bounds, index scope, and shard map assignments.
type ShardPruner struct {
	shardMapCache *cluster.ShardMapCache
	sourceReg     *sources.Registry
	clusterCfg    config.ClusterConfig
	nodeAddrs     func() map[sharding.NodeID]string
	logger        *slog.Logger
}

// NewShardPruner creates a new ShardPruner.
func NewShardPruner(
	shardMapCache *cluster.ShardMapCache,
	sourceReg *sources.Registry,
	clusterCfg config.ClusterConfig,
	nodeAddrs func() map[sharding.NodeID]string,
	logger *slog.Logger,
) *ShardPruner {
	return &ShardPruner{
		shardMapCache: shardMapCache,
		sourceReg:     sourceReg,
		clusterCfg:    clusterCfg,
		nodeAddrs:     nodeAddrs,
		logger:        logger,
	}
}

// FindRelevantShards returns all shard targets that may contain data matching
// the query hints. It generates candidate shards from the cross product of
// (indexes × timeBuckets × partitions) and filters by shard map assignment.
func (p *ShardPruner) FindRelevantShards(
	_ context.Context, hints *model.QueryHints,
) ([]ShardTarget, error) {
	// 1. Resolve index list.
	indexes := p.resolveIndexes(hints)
	if len(indexes) == 0 {
		// No indexes matched — return empty result (no shards).
		return nil, nil
	}

	// 2. Compute time bucket range.
	bucketSize := p.clusterCfg.TimeBucketSize.Duration()
	if bucketSize == 0 {
		bucketSize = 24 * time.Hour
	}
	buckets := computeTimeBuckets(hints.TimeBounds, bucketSize)

	// 3. Get shard map and node addresses.
	sm := p.shardMapCache.Get()
	addrs := p.nodeAddrs()
	partCount := uint32(p.clusterCfg.VirtualPartitionCount)
	if partCount == 0 {
		partCount = 1024
	}

	// 4. Generate all candidate shards and filter by assignment.
	// Use a map to deduplicate by shard string ID.
	seen := make(map[string]struct{})
	var targets []ShardTarget

	for _, idx := range indexes {
		for _, bucket := range buckets {
			for part := uint32(0); part < partCount; part++ {
				sid := sharding.ShardID{
					Index:      idx,
					TimeBucket: bucket,
					Partition:  part,
				}

				sidStr := sid.String()
				if _, dup := seen[sidStr]; dup {
					continue
				}
				seen[sidStr] = struct{}{}

				assignment := sm.Assignment(sid)
				if assignment == nil {
					continue // shard not assigned — skip
				}

				nodeID := assignment.Primary
				addr, ok := addrs[nodeID]
				if !ok {
					// Primary node address unknown — try first replica.
					for _, replica := range assignment.Replicas {
						if a, found := addrs[replica]; found {
							nodeID = replica
							addr = a
							ok = true

							break
						}
					}
					if !ok {
						p.logger.Warn("shard has no reachable node",
							"shard_id", sidStr,
							"primary", assignment.Primary)

						continue
					}
				}

				targets = append(targets, ShardTarget{
					ShardID:  sid,
					NodeAddr: addr,
					NodeID:   nodeID,
				})
			}
		}
	}

	return targets, nil
}

// resolveIndexes returns the list of concrete index names to query.
func (p *ShardPruner) resolveIndexes(hints *model.QueryHints) []string {
	switch hints.SourceScopeType {
	case model.SourceScopeAll:
		return p.sourceReg.List()
	case model.SourceScopeSingle:
		return hints.SourceScopeSources
	case model.SourceScopeList:
		return hints.SourceScopeSources
	case model.SourceScopeGlob:
		if hints.SourceScopePattern != "" {
			return p.sourceReg.Match(hints.SourceScopePattern)
		}
		if hints.SourceGlob != "" {
			return p.sourceReg.Match(hints.SourceGlob)
		}

		return p.sourceReg.List()
	default:
		// No explicit scope — use index hint or all.
		if hints.IndexName != "" {
			return []string{hints.IndexName}
		}

		return p.sourceReg.List()
	}
}

// maxUnboundedBuckets is the safety limit for unbounded queries. When no time
// constraints are specified, we scan at most this many buckets back in time
// to avoid runaway shard enumeration.
const maxUnboundedBuckets = 365

// computeTimeBuckets generates all time bucket boundaries that overlap
// with the given time bounds. If bounds is nil, returns all buckets from
// maxUnboundedBuckets back to now so that unbounded queries include
// historical data.
func computeTimeBuckets(bounds *model.TimeBounds, bucketSize time.Duration) []time.Time {
	now := time.Now().UTC()

	if bounds == nil {
		// No time bounds — scan all buckets up to a safety limit.
		// This ensures unbounded queries like "| stats count" return
		// historical data rather than only the current bucket.
		start := now.Add(-time.Duration(maxUnboundedBuckets) * bucketSize).Truncate(bucketSize).UTC()
		end := now.Truncate(bucketSize).UTC()
		var buckets []time.Time
		for t := start; !t.After(end); t = t.Add(bucketSize) {
			buckets = append(buckets, t)
		}
		return buckets
	}

	earliest := bounds.Earliest
	latest := bounds.Latest

	if earliest.IsZero() {
		// Default to 7 days ago.
		earliest = now.Add(-7 * 24 * time.Hour)
	}
	if latest.IsZero() {
		latest = now
	}

	start := earliest.Truncate(bucketSize).UTC()
	end := latest.Truncate(bucketSize).UTC()

	// Safety cap: limit to 365 buckets to prevent runaway queries.
	const maxBuckets = 365
	var buckets []time.Time
	for t := start; !t.After(end) && len(buckets) < maxBuckets; t = t.Add(bucketSize) {
		buckets = append(buckets, t)
	}

	return buckets
}
