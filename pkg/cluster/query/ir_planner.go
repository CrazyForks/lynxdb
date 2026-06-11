package query

import (
	"fmt"
	"strings"

	"github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
)

// IRDistributedPlan describes how a LynxFlow query is split between shards
// and coordinator using the logical IR.
type IRDistributedPlan struct {
	// ShardQuery is the LynxFlow text to execute on each shard.
	ShardQuery string
	// ShardNodes are the logical nodes pushed to shards (pipeline order).
	ShardNodes []logical.Node
	// CoordNodes are the logical nodes executed on the coordinator after merge.
	CoordNodes []logical.Node
	// Strategy determines how shard results are combined.
	Strategy MergeStrategy
	// PartialAggSpec describes the partial aggregation (non-nil for MergePartialAgg/MergeTopK).
	PartialAggSpec *pipeline.PartialAggSpec
	// TopK is the number of top results to keep (only for MergeTopK).
	TopK int
	// TopKSortFields are the sort fields for TopK selection.
	TopKSortFields []pipeline.SortField
	// SplitIndex is the position in the linearized node list where the split occurs.
	SplitIndex int
}

// PlanDistributedQueryIR splits a logical plan into shard-level and
// coordinator-level nodes. It mirrors the semantics of PlanDistributedQuery
// for the SPL2 path but operates on logical.Node types.
func PlanDistributedQueryIR(plan *logical.Plan) (*IRDistributedPlan, error) {
	if plan == nil || plan.Root == nil {
		return &IRDistributedPlan{Strategy: MergeConcat}, nil
	}

	nodes := linearizeIR(plan.Root)
	if len(nodes) == 0 {
		return &IRDistributedPlan{Strategy: MergeConcat}, nil
	}

	splitIdx := findSplitPointIR(nodes)

	result := &IRDistributedPlan{
		SplitIndex: splitIdx,
	}

	if splitIdx == 0 {
		// No pushable nodes -- stream all rows, run full pipeline on coordinator.
		result.Strategy = MergeConcat
		result.CoordNodes = nodes
		result.ShardQuery = renderShardQueryIR(plan, nil)
		return result, nil
	}

	result.ShardNodes = nodes[:splitIdx]
	result.CoordNodes = nodes[splitIdx:]

	// Check if the last shard node is Aggregate -- use partial aggregation.
	lastShard := result.ShardNodes[len(result.ShardNodes)-1]
	if agg, ok := lastShard.(*logical.Aggregate); ok {
		spec := extractPartialAggSpecFromAggregate(agg)
		if spec != nil && allPushable(spec) {
			result.PartialAggSpec = spec
			result.Strategy = MergePartialAgg

			// Check for TopK pattern: Aggregate followed by TopK node.
			if topK, sortFields := detectTopKIR(result.CoordNodes); topK > 0 {
				result.Strategy = MergeTopK
				result.TopK = topK
				result.TopKSortFields = sortFields
				// Remove the TopK node from coord commands since TopK merge handles it.
				result.CoordNodes = result.CoordNodes[1:]
			} else if topK, sortFields := detectSortHeadIR(result.CoordNodes); topK > 0 {
				// Also check for Sort + Limit (head) pattern.
				result.Strategy = MergeTopK
				result.TopK = topK
				result.TopKSortFields = sortFields
				result.CoordNodes = result.CoordNodes[2:]
			}
		} else {
			result.Strategy = MergeConcat
		}
	} else if _, ok := lastShard.(*logical.TopK); ok {
		// TopK itself is not pushable in the split (handled above).
		result.Strategy = MergeConcat
	} else {
		result.Strategy = MergeConcat
	}

	result.ShardQuery = renderShardQueryIR(plan, result.ShardNodes)

	return result, nil
}

// linearizeIR walks from root to leaf and returns nodes in pipeline order
// (leaf first). Same as logical.linearize but exported for this package.
func linearizeIR(root logical.Node) []logical.Node {
	if root == nil {
		return nil
	}
	var chain []logical.Node
	cur := root
	for cur != nil {
		chain = append(chain, cur)
		children := cur.Children()
		if len(children) == 0 {
			break
		}
		cur = children[0]
	}
	// Reverse to get pipeline order (leaf = Scan first).
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// findSplitPointIR walks nodes forward and returns the index of the first
// non-pushable node. If all nodes are pushable, returns len(nodes).
func findSplitPointIR(nodes []logical.Node) int {
	for i, n := range nodes {
		if !isPushableIR(n) {
			return i
		}
	}
	return len(nodes)
}

// isPushableIR returns true if a logical node can be executed on individual
// shards. This mirrors the SPL2 isPushable decisions exactly:
//
//	Pushable: Scan, Filter, Extend, Parse, Project(keep-mode), Aggregate, Top(implicit), Rare(implicit)
//	Not pushable: Sort, TopK, Limit, Dedup, Join, Union, StreamStats, EventStats, etc.
func isPushableIR(n logical.Node) bool {
	switch n := n.(type) {
	case *logical.Scan:
		return true
	case *logical.Filter:
		return true
	case *logical.Extend:
		return true
	case *logical.Parse:
		return true
	case *logical.Aggregate:
		// Plain stats is pushable. Eventstats/streamstats are not.
		return n.Window == nil
	case *logical.Project:
		// Only keep-mode is safe to push. Drop-mode (has any ProjectDrop)
		// could remove fields needed by coordinator. Check: if all cols are
		// keeps or renames, it is safe.
		for _, c := range n.Cols {
			if c.Action == logical.ProjectDrop {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// extractPartialAggSpecFromAggregate builds a PartialAggSpec from a logical
// Aggregate node. This mirrors extractPartialAggSpecFromStats for SPL2.
func extractPartialAggSpecFromAggregate(agg *logical.Aggregate) *pipeline.PartialAggSpec {
	groupBy := make([]string, len(agg.Keys))
	for i, k := range agg.Keys {
		groupBy[i] = k.Name
	}

	funcs := make([]pipeline.PartialAggFunc, len(agg.Aggs))
	for i, a := range agg.Aggs {
		funcName := ""
		field := ""

		if call, ok := a.Func.(*ast.Call); ok {
			funcName = strings.ToLower(call.Callee)
			if len(call.Args) > 0 {
				if ident, ok := call.Args[0].(*ast.Ident); ok {
					field = ident.Name
				}
			}
		}

		alias := a.Alias
		if alias == "" {
			if field != "" {
				alias = fmt.Sprintf("%s(%s)", funcName, field)
			} else {
				alias = funcName
			}
		}

		funcs[i] = pipeline.PartialAggFunc{
			Name:  funcName,
			Field: field,
			Alias: alias,
		}
	}

	return &pipeline.PartialAggSpec{
		GroupBy: groupBy,
		Funcs:   funcs,
	}
}

// detectTopKIR checks if the first coordinator node is a TopK node.
func detectTopKIR(coordNodes []logical.Node) (int, []pipeline.SortField) {
	if len(coordNodes) < 1 {
		return 0, nil
	}
	topk, ok := coordNodes[0].(*logical.TopK)
	if !ok {
		return 0, nil
	}
	sortFields := make([]pipeline.SortField, len(topk.SortKeys))
	for i, k := range topk.SortKeys {
		name := ""
		if ident, ok := k.Expr.(*ast.Ident); ok {
			name = ident.Name
		} else {
			name = k.Expr.String()
		}
		sortFields[i] = pipeline.SortField{Name: name, Desc: k.Desc}
	}
	return int(topk.K), sortFields
}

// detectSortHeadIR checks if coordinator nodes start with Sort + Limit(head).
func detectSortHeadIR(coordNodes []logical.Node) (int, []pipeline.SortField) {
	if len(coordNodes) < 2 {
		return 0, nil
	}
	sortN, ok := coordNodes[0].(*logical.Sort)
	if !ok {
		return 0, nil
	}
	limit, ok := coordNodes[1].(*logical.Limit)
	if !ok || limit.Tail {
		return 0, nil
	}
	sortFields := make([]pipeline.SortField, len(sortN.Keys))
	for i, k := range sortN.Keys {
		name := ""
		if ident, ok := k.Expr.(*ast.Ident); ok {
			name = ident.Name
		} else {
			name = k.Expr.String()
		}
		sortFields[i] = pipeline.SortField{Name: name, Desc: k.Desc}
	}
	return int(limit.N), sortFields
}

// renderShardQueryIR reconstructs LynxFlow text for the shard query.
// If shardNodes is nil, returns just the source clause for full scan.
func renderShardQueryIR(plan *logical.Plan, shardNodes []logical.Node) string {
	if shardNodes == nil {
		// Full scan: render just the scan node from the plan.
		nodes := linearizeIR(plan.Root)
		for _, n := range nodes {
			if scan, ok := n.(*logical.Scan); ok {
				return logical.RenderPipeline(scan)
			}
		}
		return "from *"
	}
	return logical.RenderPipeline(shardNodes...)
}

func allPushable(_ interface{}) bool {
	// RFC-002: stub - all nodes pushable by default
	return true
}
