package query

import (
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
)

// JoinStrategy describes how to execute a distributed join.
type JoinStrategy struct {
	// Type is one of "broadcast", "shuffle", or "colocated".
	Type string
	// Broadcast is true when the right side is small enough to replicate
	// to all shards (subsearch/CTE-backed). When false, shuffle join is used.
	Broadcast bool
}

// PlanDistributedJoin determines the execution strategy for a distributed
// join based on the join node's properties. It inspects the right-side
// pipeline to decide:
//
//   - broadcast: when the right side is backed by a CTE ($variable) or a
//     small subsearch (no explicit large source), the right side is
//     replicated to all shards. This is the default for CTE-backed joins
//     which are the common case in log analytics (e.g., threat lists).
//
//   - shuffle: when the right side reads from a named source (non-CTE),
//     both sides are hash-partitioned on the join key and co-located.
//     This avoids full replication of large datasets.
//
// The strategy is recorded on IRDistributedPlan.JoinStrategy for
// observability (EXPLAIN/tracing can display it).
func PlanDistributedJoin(join *logical.Join) *JoinStrategy {
	if join == nil || join.Right == nil {
		return &JoinStrategy{Type: "broadcast", Broadcast: true}
	}

	// Walk the right-side pipeline to find the leaf (Scan node).
	// If the right side reads from a CTE or has no explicit source,
	// the data is materialized on the coordinator -> broadcast is safe.
	rightLeaf := findLeafNode(join.Right)
	if rightLeaf == nil {
		return &JoinStrategy{Type: "broadcast", Broadcast: true}
	}

	scan, ok := rightLeaf.(*logical.Scan)
	if !ok {
		// Non-scan leaf (e.g., Empty): broadcast.
		return &JoinStrategy{Type: "broadcast", Broadcast: true}
	}

	// CTE source: data is coordinator-materialized, broadcast to shards.
	for _, src := range scan.Sources {
		if src.Kind == ast.SourceCTE {
			return &JoinStrategy{Type: "broadcast", Broadcast: true}
		}
	}

	// Named source on the right side: shuffle join.
	return &JoinStrategy{Type: "shuffle", Broadcast: false}
}

// findLeafNode walks the node tree to find the leaf (a node with no children).
func findLeafNode(n logical.Node) logical.Node {
	if n == nil {
		return nil
	}
	for {
		children := n.Children()
		if len(children) == 0 {
			return n
		}
		n = children[0]
	}
}
