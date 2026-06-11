package query

import (
	"fmt"

	"github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/model"
)

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

func PlanDistributedQuery(_ *logical.Plan, _ *model.QueryHints) (*DistributedPlan, error) {
	return &DistributedPlan{Strategy: MergeConcat}, nil
}
