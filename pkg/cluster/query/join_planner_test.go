package query

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
)

func TestPlanDistributedJoin_NilJoin(t *testing.T) {
	strategy := PlanDistributedJoin(nil)
	if strategy.Type != "broadcast" {
		t.Errorf("expected broadcast, got %s", strategy.Type)
	}
	if !strategy.Broadcast {
		t.Error("expected Broadcast=true")
	}
}

func TestPlanDistributedJoin_NilRight(t *testing.T) {
	join := &logical.Join{Type: "inner"}
	strategy := PlanDistributedJoin(join)
	if strategy.Type != "broadcast" {
		t.Errorf("expected broadcast, got %s", strategy.Type)
	}
}

func TestPlanDistributedJoin_CTERight(t *testing.T) {
	// Right side reads from a CTE ($threats) -> broadcast.
	right := &logical.Scan{
		Sources: []logical.SourcePattern{
			{Kind: ast.SourceCTE, Name: "threats"},
		},
	}
	join := &logical.Join{
		Type:  "inner",
		On:    []string{"client_ip"},
		Right: right,
	}

	strategy := PlanDistributedJoin(join)
	if strategy.Type != "broadcast" {
		t.Errorf("expected broadcast for CTE right side, got %s", strategy.Type)
	}
	if !strategy.Broadcast {
		t.Error("expected Broadcast=true for CTE right side")
	}
}

func TestPlanDistributedJoin_NamedSourceRight(t *testing.T) {
	// Right side reads from a named source (backend) -> shuffle.
	right := &logical.Scan{
		Sources: []logical.SourcePattern{
			{Kind: ast.SourceName, Name: "backend"},
		},
	}
	join := &logical.Join{
		Type:  "inner",
		On:    []string{"host"},
		Right: right,
	}

	strategy := PlanDistributedJoin(join)
	if strategy.Type != "shuffle" {
		t.Errorf("expected shuffle for named source right side, got %s", strategy.Type)
	}
	if strategy.Broadcast {
		t.Error("expected Broadcast=false for named source right side")
	}
}

func TestPlanDistributedJoin_RightWithPipeline(t *testing.T) {
	// Right side is: Scan(backend) -> Filter -> Aggregate
	// The leaf is a Scan with a named source -> shuffle.
	scan := &logical.Scan{
		Sources: []logical.SourcePattern{
			{Kind: ast.SourceName, Name: "backend"},
		},
	}
	filter := &logical.Filter{Expr: &ast.Ident{Name: "x"}}
	filter.SetChildren([]logical.Node{scan})
	agg := &logical.Aggregate{
		Aggs: []logical.Agg{{Func: &ast.Call{Callee: "count"}, Alias: "cnt"}},
		Keys: []logical.Key{{Name: "host"}},
	}
	agg.SetChildren([]logical.Node{filter})

	join := &logical.Join{
		Type:  "inner",
		On:    []string{"host"},
		Right: agg,
	}

	strategy := PlanDistributedJoin(join)
	if strategy.Type != "shuffle" {
		t.Errorf("expected shuffle for named source in pipeline, got %s", strategy.Type)
	}
}
