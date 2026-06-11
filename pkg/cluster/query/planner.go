package query

import (
	"fmt"

	"github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/logical"
)

// MergeStrategy describes how shard results are combined.
type MergeStrategy int

const (
	MergeConcat MergeStrategy = iota
	MergePartialAgg
	MergeTopK
)

func (s MergeStrategy) String() string {
	switch s {
	case MergeConcat:
		return "concat"
	case MergePartialAgg:
		return "partial_agg"
	case MergeTopK:
		return "topk"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// DistributedPlan describes how a query is split between shards and
// coordinator. It is used as the fan-out bridge by both the IR planner
// and the executePartialAgg/executeConcat methods.
type DistributedPlan struct {
	ShardQuery     string
	Strategy       MergeStrategy
	PartialAgg     *pipeline.PartialAggSpec
	PartialAggSpec *pipeline.PartialAggSpec
	CoordCommands  []logical.Node
	Pushable       bool
	TopK           int
	TopKSortFields []pipeline.SortField
	SplitIndex     int
}
