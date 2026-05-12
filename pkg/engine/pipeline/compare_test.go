package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/vm"
)

func TestCompareIteratorUsesAbsoluteChange(t *testing.T) {
	current := []map[string]event.Value{
		{
			"service": event.StringValue("api"),
			"n":       event.IntValue(15),
		},
		{
			"service": event.StringValue("worker"),
			"n":       event.IntValue(5),
		},
	}
	previous := []map[string]event.Value{
		{
			"service": event.StringValue("api"),
			"n":       event.IntValue(10),
		},
		{
			"service": event.StringValue("worker"),
			"n":       event.IntValue(0),
		},
	}

	iter := NewCompareIterator(
		NewRowScanIterator(current, 2),
		time.Hour,
		func(ctx context.Context) (Iterator, error) {
			return NewRowScanIterator(previous, 2), nil
		},
		2,
	)

	rows, err := CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}

	byService := make(map[string]map[string]event.Value, len(rows))
	for _, row := range rows {
		byService[row["service"].AsString()] = row
	}

	assertCompareValue(t, byService["api"], "previous_n", 10)
	assertCompareValue(t, byService["api"], "change_n", 5)
	assertCompareValue(t, byService["worker"], "previous_n", 0)
	assertCompareValue(t, byService["worker"], "change_n", 5)
}

func assertCompareValue(t *testing.T, row map[string]event.Value, field string, want float64) {
	t.Helper()
	if row == nil {
		t.Fatalf("missing row")
	}
	got := row[field]
	if got.IsNull() {
		t.Fatalf("%s is null, want %v", field, want)
	}
	gotF, ok := vm.ValueToFloat(got)
	if !ok {
		t.Fatalf("%s is not numeric: %v", field, got)
	}
	if gotF != want {
		t.Fatalf("%s: got %v, want %v", field, gotF, want)
	}
}
