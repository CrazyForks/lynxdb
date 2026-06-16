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
