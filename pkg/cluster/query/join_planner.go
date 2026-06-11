package query

import "github.com/lynxbase/lynxdb/pkg/model"

// JoinStrategy describes how to execute a distributed join.
type JoinStrategy struct {
	Type      string // "broadcast", "shuffle", "colocated"
	Broadcast bool
}

// PlanDistributedJoin is a stub. RFC-002: spl2 AST join planning removed.
// TODO(RFC-002): reimplement on logical.Join nodes.
func PlanDistributedJoin(_ interface{}, _ interface{}, _ *model.QueryHints) JoinStrategy {
	return JoinStrategy{Type: "broadcast", Broadcast: true}
}
