package pipeline

import (
	"context"
	"fmt"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/memgov"
)

func TestAggregateArgFunctionsWithSpill(t *testing.T) {
	const groups = 100
	const perGroup = 50

	rows := make([]map[string]event.Value, 0, groups*perGroup)
	for score := 0; score < perGroup; score++ {
		for group := 0; group < groups; group++ {
			groupName := fmt.Sprintf("g%d", group)
			rows = append(rows, map[string]event.Value{
				"group": event.StringValue(groupName),
				"value": event.StringValue(fmt.Sprintf("%s-%02d", groupName, score)),
				"score": event.IntValue(int64(score)),
				"route": event.StringValue(fmt.Sprintf("route-%d", score%3)),
				"team":  event.StringValue(fmt.Sprintf("team-%d", group)),
			})
		}
	}

	child := NewRowScanIterator(rows, 128)
	mgr, err := NewSpillManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.CleanupAll()

	acct := memgov.NewTestBudget("test", 8*1024).NewAccount("agg")
	aggs := []AggFunc{
		{Name: "arg_max", Field: "value", OrderField: "score", Alias: "max_value"},
		{Name: "arg_min", Field: "value", OrderField: "score", Alias: "min_value"},
		{Name: "any_value", Field: "team", Alias: "some_team"},
		{Name: "top_k", Field: "route", Limit: 2, Alias: "top_routes"},
		{Name: "value_counts", Field: "route", Alias: "route_counts"},
	}
	iter := NewAggregateIteratorWithSpill(child, aggs, []string{"group"}, acct, mgr)

	ctx := context.Background()
	if err := iter.Init(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := CollectAll(ctx, iter)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != groups {
		t.Fatalf("expected %d groups, got %d", groups, len(result))
	}

	byGroup := make(map[string]map[string]event.Value, len(result))
	for _, row := range result {
		byGroup[row["group"].AsString()] = row
	}

	for _, group := range []string{"g0", "g42", "g99"} {
		row := byGroup[group]
		if row == nil {
			t.Fatalf("missing group %s", group)
		}
		assertEventString(t, row["max_value"], fmt.Sprintf("%s-49", group), group+" max")
		assertEventString(t, row["min_value"], fmt.Sprintf("%s-00", group), group+" min")
		if row["some_team"].IsNull() {
			t.Fatalf("%s any_value should be non-null", group)
		}
		assertEventTopKEntry(t, row["top_routes"], 0, "route-0", 17, group+" top 0")
		assertEventTopKEntry(t, row["top_routes"], 1, "route-1", 17, group+" top 1")
		assertEventTopKEntry(t, row["route_counts"], 0, "route-0", 17, group+" counts 0")
		assertEventTopKEntry(t, row["route_counts"], 1, "route-1", 17, group+" counts 1")
		assertEventTopKEntry(t, row["route_counts"], 2, "route-2", 16, group+" counts 2")
	}
}

func assertEventString(t *testing.T, v event.Value, expected string, label string) {
	t.Helper()
	if v.Type() != event.FieldTypeString {
		t.Fatalf("%s: expected string, got %s", label, v.Type())
	}
	if v.AsString() != expected {
		t.Fatalf("%s: got %q, want %q", label, v.AsString(), expected)
	}
}

func assertEventTopKEntry(t *testing.T, v event.Value, idx int, expected string, count int64, label string) {
	t.Helper()
	if v.Type() != event.FieldTypeArray {
		t.Fatalf("%s: expected array, got %s", label, v.Type())
	}
	arr := v.AsArray()
	if len(arr) <= idx {
		t.Fatalf("%s: expected entry %d, got len %d", label, idx, len(arr))
	}
	obj, ok := arr[idx].TryAsObject()
	if !ok {
		t.Fatalf("%s: expected object, got %s", label, arr[idx].Type())
	}
	assertEventString(t, obj["value"], expected, label+" value")
	got, ok := obj["count"].TryAsInt()
	if !ok || got != count {
		t.Fatalf("%s count: got %v, want %d", label, obj["count"], count)
	}
}
