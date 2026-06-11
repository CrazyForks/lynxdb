package query

import (
	"context"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
)

// makeRows builds test rows from key-value pairs.
func makeRows(rows ...map[string]event.Value) []map[string]event.Value {
	return rows
}

func row(pairs ...interface{}) map[string]event.Value {
	m := make(map[string]event.Value, len(pairs)/2)
	for i := 0; i < len(pairs)-1; i += 2 {
		k := pairs[i].(string)
		switch v := pairs[i+1].(type) {
		case string:
			m[k] = event.StringValue(v)
		case int:
			m[k] = event.IntValue(int64(v))
		case int64:
			m[k] = event.IntValue(v)
		case float64:
			m[k] = event.FloatValue(v)
		}
	}
	return m
}

func TestApplyCoordCommands_Empty(t *testing.T) {
	rows := makeRows(
		row("host", "a", "count", 10),
		row("host", "b", "count", 20),
	)

	result, err := applyCoordCommands(context.Background(), rows, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result))
	}
}

func TestApplyCoordCommands_SortDescAndLimit(t *testing.T) {
	rows := makeRows(
		row("host", "a", "count", int64(10)),
		row("host", "b", "count", int64(30)),
		row("host", "c", "count", int64(20)),
		row("host", "d", "count", int64(5)),
		row("host", "e", "count", int64(40)),
	)

	// CoordNodes: Sort(desc by count) -> Limit(3)
	sortNode := &logical.Sort{
		Keys: []logical.SortKey{
			{Expr: &ast.Ident{Name: "count"}, Desc: true},
		},
	}
	limitNode := &logical.Limit{N: 3}

	coordNodes := []logical.Node{sortNode, limitNode}

	result, err := applyCoordCommands(context.Background(), rows, coordNodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 rows, got %d", len(result))
	}

	// Verify ordering: 40, 30, 20
	expected := []int64{40, 30, 20}
	for i, exp := range expected {
		if i >= len(result) {
			break
		}
		v, ok := result[i]["count"]
		if !ok {
			t.Errorf("row %d: missing 'count'", i)
			continue
		}
		got, ok := v.TryAsInt()
		if !ok {
			t.Errorf("row %d: count is not int: %v", i, v)
			continue
		}
		if got != exp {
			t.Errorf("row %d: count = %d, want %d", i, got, exp)
		}
	}
}

func TestApplyCoordCommands_Dedup(t *testing.T) {
	rows := makeRows(
		row("host", "a", "count", int64(10)),
		row("host", "a", "count", int64(20)),
		row("host", "b", "count", int64(30)),
		row("host", "b", "count", int64(40)),
		row("host", "c", "count", int64(50)),
	)

	// CoordNodes: Dedup(1, host)
	dedupNode := &logical.Dedup{
		N:      1,
		Fields: []string{"host"},
	}

	coordNodes := []logical.Node{dedupNode}

	result, err := applyCoordCommands(context.Background(), rows, coordNodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 rows (one per unique host), got %d", len(result))
	}

	// Verify each host appears exactly once.
	hosts := make(map[string]int)
	for _, r := range result {
		v, ok := r["host"]
		if !ok {
			t.Error("row missing 'host'")
			continue
		}
		s, _ := v.TryAsString()
		hosts[s]++
	}
	for h, c := range hosts {
		if c != 1 {
			t.Errorf("host %q appears %d times, want 1", h, c)
		}
	}
}

func TestApplyCoordCommands_EmptyRows(t *testing.T) {
	// Empty input rows should return empty.
	result, err := applyCoordCommands(context.Background(), nil, []logical.Node{
		&logical.Sort{Keys: []logical.SortKey{{Expr: &ast.Ident{Name: "x"}, Desc: true}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result))
	}
}

func TestApplyCoordCommands_Head(t *testing.T) {
	rows := makeRows(
		row("x", int64(1)),
		row("x", int64(2)),
		row("x", int64(3)),
		row("x", int64(4)),
		row("x", int64(5)),
	)

	// CoordNodes: Limit(2)
	coordNodes := []logical.Node{&logical.Limit{N: 2}}

	result, err := applyCoordCommands(context.Background(), rows, coordNodes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 rows, got %d", len(result))
	}
}

func TestCloneCoordNode_DoesNotMutateOriginal(t *testing.T) {
	original := &logical.Sort{
		Keys: []logical.SortKey{
			{Expr: &ast.Ident{Name: "count"}, Desc: true},
		},
	}
	// Set a fake input to verify cloning doesn't affect the original.
	original.SetChildren([]logical.Node{&logical.Scan{}})

	cloned := cloneCoordNode(original)

	// Mutate the clone's children.
	cloned.SetChildren([]logical.Node{&logical.Scan{
		Sources: []logical.SourcePattern{{Name: "_merged"}},
	}})

	// Verify original is unchanged.
	origChildren := original.Children()
	if len(origChildren) != 1 {
		t.Fatal("original children modified")
	}
	origScan, ok := origChildren[0].(*logical.Scan)
	if !ok {
		t.Fatal("original child is not Scan")
	}
	if len(origScan.Sources) != 0 {
		t.Error("original Scan sources were modified by clone mutation")
	}
}

// TestPlanDistributedQueryIR_JoinInCoordNodes verifies that a query with a
// join places the join in CoordNodes (not shard nodes).
func TestPlanDistributedQueryIR_JoinInCoordNodes(t *testing.T) {
	plan := parseLowerOptIR(t, `from main | join on host with [from backend | stats count() by host]`)
	result, err := PlanDistributedQueryIR(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Join must NOT be in shard nodes.
	for _, n := range result.ShardNodes {
		if _, ok := n.(*logical.Join); ok {
			t.Error("Join should not be in shard nodes")
		}
	}

	// Join must be in coord nodes.
	foundJoin := false
	for _, n := range result.CoordNodes {
		if _, ok := n.(*logical.Join); ok {
			foundJoin = true
			break
		}
	}
	if !foundJoin {
		t.Error("Join should be in coord nodes")
	}

	// JoinStrategy should be set.
	if result.JoinStrategy == nil {
		t.Error("JoinStrategy should be non-nil for a query with join")
	}
}
