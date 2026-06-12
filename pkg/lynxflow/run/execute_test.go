package run

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lynxbase/lynxdb/pkg/event"
)

// Helpers

func makeRawEvents(rawLines ...string) map[string][]*event.Event {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := make([]*event.Event, len(rawLines))
	for i, line := range rawLines {
		ev := event.NewEvent(base.Add(time.Duration(i)*time.Second), line)
		ev.Index = "main"
		events[i] = ev
	}
	return map[string][]*event.Event{"main": events}
}

// Test: parse json on_error=propagate (default)

func TestExecute_ParseJSON_MixedValidity_Propagate(t *testing.T) {
	events := makeRawEvents(
		`{"name":"alice","age":30}`,
		`this is not json`,
		`{"name":"bob","age":25}`,
		`{"broken`,
	)

	rows, err := Execute(context.Background(),
		`from main | parse json`,
		events, Options{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// All 4 rows survive in propagate mode.
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	// Row 0: valid JSON, no _error.
	assertNoError(t, rows[0], 0)
	assertStringField(t, rows[0], "name", "alice", 0)

	// Row 1: invalid JSON, _error and _error_detail present.
	assertHasError(t, rows[1], 1)
	assertHasErrorDetail(t, rows[1], 1)

	// Row 2: valid JSON, no _error.
	assertNoError(t, rows[2], 2)
	assertStringField(t, rows[2], "name", "bob", 2)

	// Row 3: broken JSON, _error and _error_detail present.
	assertHasError(t, rows[3], 3)
	assertHasErrorDetail(t, rows[3], 3)
}

// Test: parse json on_error=propagate — _error string starts with "parse:json:"

func TestExecute_ParseJSON_Propagate_ErrorFormat(t *testing.T) {
	events := makeRawEvents(`not json`)

	rows, err := Execute(context.Background(),
		`from main | parse json`,
		events, Options{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	errVal, ok := rows[0]["_error"]
	if !ok || errVal.IsNull() {
		t.Fatal("row 0: expected non-null _error")
	}

	errStr := errVal.String()
	if !strings.HasPrefix(errStr, "parse:json:") {
		t.Errorf("_error should start with 'parse:json:', got %q", errStr)
	}
}

// Test: parse json on_error=propagate — _error_detail is an object with
// stage, format, code, message keys per spec (RFC-002 7.3).

func TestExecute_ParseJSON_Propagate_ErrorDetailShape(t *testing.T) {
	events := makeRawEvents(`not json at all`)

	rows, err := Execute(context.Background(),
		`from main | parse json on_error propagate`,
		events, Options{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	detail, ok := rows[0]["_error_detail"]
	if !ok || detail.IsNull() {
		t.Fatal("row 0: expected non-null _error_detail")
	}
	if detail.Type() != event.FieldTypeObject {
		t.Fatalf("_error_detail type: got %s, want object", detail.Type())
	}

	obj, _ := detail.TryAsObject()
	for _, key := range []string{"stage", "format", "code", "message"} {
		v, exists := obj[key]
		if !exists || v.IsNull() {
			t.Errorf("_error_detail missing or null key %q", key)
		}
	}

	// format should be "json".
	if fmtVal, ok := obj["format"]; ok {
		if fmtVal.String() != "json" {
			t.Errorf("_error_detail.format: got %q, want %q", fmtVal.String(), "json")
		}
	}
}

// Test: parse json on_error=drop

func TestExecute_ParseJSON_MixedValidity_Drop(t *testing.T) {
	events := makeRawEvents(
		`{"name":"alice","age":30}`,
		`not json`,
		`{"name":"bob","age":25}`,
		`also not json`,
	)

	rows, err := Execute(context.Background(),
		`from main | parse json on_error drop`,
		events, Options{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Only 2 valid JSON rows survive.
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	assertStringField(t, rows[0], "name", "alice", 0)
	assertStringField(t, rows[1], "name", "bob", 1)
}

// Test: parse json on_error=null

func TestExecute_ParseJSON_MixedValidity_Null(t *testing.T) {
	events := makeRawEvents(
		`{"name":"alice"}`,
		`garbage`,
		`{"name":"bob"}`,
	)

	rows, err := Execute(context.Background(),
		`from main | parse json on_error null`,
		events, Options{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// All 3 rows survive.
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Row 0: valid JSON.
	assertStringField(t, rows[0], "name", "alice", 0)

	// Row 1: no _error, no _error_detail, no extracted fields.
	if v, ok := rows[1]["_error"]; ok && !v.IsNull() {
		t.Errorf("row 1: on_error=null should NOT set _error, got %v", v)
	}
	if v, ok := rows[1]["_error_detail"]; ok && !v.IsNull() {
		t.Errorf("row 1: on_error=null should NOT set _error_detail, got %v", v)
	}
	if v, ok := rows[1]["name"]; ok && !v.IsNull() {
		t.Errorf("row 1: on_error=null should NOT populate extracted fields, got name=%v", v)
	}

	// Row 2: valid JSON.
	assertStringField(t, rows[2], "name", "bob", 2)
}

// Test: parse json on_error=strict

func TestExecute_ParseJSON_MixedValidity_Strict(t *testing.T) {
	events := makeRawEvents(
		`{"name":"alice"}`,
		`not json`,
	)

	_, err := Execute(context.Background(),
		`from main | parse json on_error strict`,
		events, Options{})
	if err == nil {
		t.Fatal("expected error from on_error=strict with malformed row, got nil")
	}

	// Error should mention "parse:json:" per the contract.
	if !strings.Contains(err.Error(), "parse:json:") {
		t.Errorf("strict error should contain 'parse:json:', got %q", err.Error())
	}
}

// Test: parse json with downstream where on _error

func TestExecute_ParseJSON_FilterOnError(t *testing.T) {
	events := makeRawEvents(
		`{"name":"alice"}`,
		`not json`,
		`{"name":"bob"}`,
		`broken json`,
	)

	rows, err := Execute(context.Background(),
		`from main | parse json | where exists(_error) | keep _error, _error_detail, _raw`,
		events, Options{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Only the 2 invalid rows should pass through the where exists(_error) filter.
	if len(rows) != 2 {
		t.Fatalf("expected 2 error rows, got %d", len(rows))
	}

	for i, row := range rows {
		assertHasError(t, row, i)
		if _, ok := row["_raw"]; !ok {
			t.Errorf("row %d: expected _raw to be kept", i)
		}
	}
}

// Test: parse first_of(json, logfmt) on_error=propagate

func TestExecute_ParseFirstOf_MixedFormats(t *testing.T) {
	events := makeRawEvents(
		`{"name":"alice"}`,                // JSON succeeds
		`level=info msg="hello" count=42`, // logfmt succeeds
		`total garbage !@#$%`,             // both fail
	)

	rows, err := Execute(context.Background(),
		`from main | parse first_of(json, logfmt)`,
		events, Options{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Row 0: JSON succeeded.
	assertNoError(t, rows[0], 0)
	assertStringField(t, rows[0], "name", "alice", 0)

	// Row 1: JSON failed, logfmt succeeded.
	assertNoError(t, rows[1], 1)
	assertStringField(t, rows[1], "level", "info", 1)

	// Row 2: both failed, _error with first_of.
	assertHasError(t, rows[2], 2)
	errStr := rows[2]["_error"].String()
	if !strings.Contains(errStr, "first_of") {
		t.Errorf("row 2: _error should mention first_of, got %q", errStr)
	}

	// _error_detail should have a stages array.
	assertHasErrorDetail(t, rows[2], 2)
	detail, _ := rows[2]["_error_detail"].TryAsObject()
	if _, ok := detail["stages"]; !ok {
		t.Error("row 2: _error_detail should have 'stages' key for first_of chain")
	}
}

// Assertion helpers

func assertNoError(t *testing.T, row map[string]event.Value, rowIdx int) {
	t.Helper()
	if v, ok := row["_error"]; ok && !v.IsNull() {
		t.Errorf("row %d: unexpected non-null _error: %v", rowIdx, v)
	}
}

func assertHasError(t *testing.T, row map[string]event.Value, rowIdx int) {
	t.Helper()
	v, ok := row["_error"]
	if !ok || v.IsNull() {
		t.Fatalf("row %d: expected non-null _error, got absent/null", rowIdx)
	}
}

func assertHasErrorDetail(t *testing.T, row map[string]event.Value, rowIdx int) {
	t.Helper()
	v, ok := row["_error_detail"]
	if !ok || v.IsNull() {
		t.Fatalf("row %d: expected non-null _error_detail, got absent/null", rowIdx)
	}
	if v.Type() != event.FieldTypeObject {
		t.Fatalf("row %d: _error_detail type: got %s, want object", rowIdx, v.Type())
	}
}

func assertStringField(t *testing.T, row map[string]event.Value, field, want string, rowIdx int) {
	t.Helper()
	v, ok := row[field]
	if !ok || v.IsNull() {
		t.Fatalf("row %d: expected non-null %q field", rowIdx, field)
	}
	if v.String() != want {
		t.Errorf("row %d: %s: got %q, want %q", rowIdx, field, v.String(), want)
	}
}

// Test: materialize in ephemeral mode returns a clear error

func TestExecute_Materialize_EphemeralError(t *testing.T) {
	events := makeRawEvents(`{"level":"error"}`, `{"level":"info"}`)

	_, err := Execute(context.Background(),
		`from main | stats count() by level | materialize "mv_test" retention=90d`,
		events, Options{})
	if err == nil {
		t.Fatal("expected error for materialize in ephemeral mode, got nil")
	}

	errMsg := err.Error()
	// The error should mention the view name and suggest lynxdb mv create.
	if !strings.Contains(errMsg, "mv_test") {
		t.Errorf("error should mention view name, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "lynxdb mv create") {
		t.Errorf("error should suggest lynxdb mv create, got: %s", errMsg)
	}
}

// Test: compare requires aggregated pipeline

func TestExecute_Compare_RequiresAggregation(t *testing.T) {
	events := makeRawEvents(`{"level":"error"}`)

	_, err := Execute(context.Background(),
		`from main | compare previous 1h`,
		events, Options{})
	if err == nil {
		t.Fatal("expected error for compare without aggregation, got nil")
	}
	if !strings.Contains(err.Error(), "compare requires an aggregated pipeline") {
		t.Errorf("unexpected error: %s", err.Error())
	}
}

// Test: compare with aggregation executes end-to-end

func TestExecute_Compare_WithAggregation(t *testing.T) {
	// - "current" window: 2h ago to 1h ago (2 events)
	// - We query from main (all events) | stats count() | compare previous 2h
	// The compare should produce previous_count() and change_count() columns.
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

	events := make([]*event.Event, 4)
	// Events at t-90m and t-80m (in the "current" 0..now window)
	events[0] = event.NewEvent(now.Add(-90*time.Minute), `{"level":"error"}`)
	events[0].Index = "main"
	events[1] = event.NewEvent(now.Add(-80*time.Minute), `{"level":"info"}`)
	events[1].Index = "main"
	// Events at t-3h (in the "previous" window shifted back by 2h)
	events[2] = event.NewEvent(now.Add(-3*time.Hour), `{"level":"warn"}`)
	events[2].Index = "main"
	events[3] = event.NewEvent(now.Add(-4*time.Hour), `{"level":"debug"}`)
	events[3].Index = "main"

	store := map[string][]*event.Event{"main": events}

	rows, err := Execute(context.Background(),
		`from main | stats count() | compare previous 2h`,
		store, Options{Now: now})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Should produce at least 1 row with count(), previous_count(), change_count().
	if len(rows) == 0 {
		t.Fatal("expected at least 1 row, got 0")
	}

	row := rows[0]
	// Check that previous_count() and change_count() columns exist.
	if _, ok := row["previous_count()"]; !ok {
		t.Errorf("missing previous_count() column; row keys: %v", rowKeys(row))
	}
	if _, ok := row["change_count()"]; !ok {
		t.Errorf("missing change_count() column; row keys: %v", rowKeys(row))
	}
}

// TestExecute_Compare_WithGroupBy tests compare with a group-by variant.
func TestExecute_Compare_WithGroupBy(t *testing.T) {
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)

	mkEvent := func(offset time.Duration, raw string, fields map[string]event.Value) *event.Event {
		ev := event.NewEvent(now.Add(offset), raw)
		ev.Index = "main"
		for k, v := range fields {
			ev.Fields[k] = v
		}
		return ev
	}

	events := []*event.Event{
		mkEvent(-30*time.Minute, `{"level":"error"}`, map[string]event.Value{"level": event.StringValue("error")}),
		mkEvent(-20*time.Minute, `{"level":"error"}`, map[string]event.Value{"level": event.StringValue("error")}),
		mkEvent(-10*time.Minute, `{"level":"info"}`, map[string]event.Value{"level": event.StringValue("info")}),
		mkEvent(-150*time.Minute, `{"level":"error"}`, map[string]event.Value{"level": event.StringValue("error")}),
		mkEvent(-140*time.Minute, `{"level":"info"}`, map[string]event.Value{"level": event.StringValue("info")}),
	}
	store := map[string][]*event.Event{"main": events}

	rows, err := Execute(context.Background(),
		`from main | stats count() by level | compare previous 2h`,
		store, Options{Now: now})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Should produce rows with level, count(), previous_count(), change_count().
	if len(rows) == 0 {
		t.Fatal("expected rows, got 0")
	}

	for i, row := range rows {
		if _, ok := row["level"]; !ok {
			t.Errorf("row %d: missing level column", i)
		}
		if _, ok := row["previous_count()"]; !ok {
			t.Errorf("row %d: missing previous_count() column; keys: %v", i, rowKeys(row))
		}
		if _, ok := row["change_count()"]; !ok {
			t.Errorf("row %d: missing change_count() column; keys: %v", i, rowKeys(row))
		}
	}
}

func rowKeys(row map[string]event.Value) []string {
	var keys []string
	for k := range row {
		keys = append(keys, k)
	}
	return keys
}
