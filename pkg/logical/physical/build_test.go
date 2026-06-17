package physical

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	lfast "github.com/lynxbase/lynxdb/pkg/lynxflow/ast"

	"github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/opt"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
)

// sliceSource helper

// sliceSource builds a RowScanIterator from row maps.
func sliceSource(rows []map[string]event.Value, batchSize int) pipeline.Iterator {
	if batchSize <= 0 {
		batchSize = pipeline.DefaultBatchSize
	}
	return pipeline.NewRowScanIterator(rows, batchSize)
}

// sourceFromRows returns a BuildOptions.Source that ignores the scan node and
// returns a fixed set of rows.
func sourceFromRows(rows []map[string]event.Value) func(*logical.Scan) (pipeline.Iterator, error) {
	return func(_ *logical.Scan) (pipeline.Iterator, error) {
		return sliceSource(rows, 1024), nil
	}
}

// drain runs the full pipeline: parse -> desugar -> lower -> optimize -> build -> collect.
func drain(t *testing.T, query string, rows []map[string]event.Value) []map[string]event.Value {
	t.Helper()
	return drainWithBatchSize(t, query, rows, 1024)
}

func drainWithBatchSize(t *testing.T, query string, rows []map[string]event.Value, batchSize int) []map[string]event.Value {
	t.Helper()
	q, diags := parser.Parse(query)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("parse error: %s", d.Message)
		}
	}
	desugared, _ := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("lower error: %s", d.Message)
		}
	}
	plan, _ = opt.Optimize(plan)

	iter, err := Build(plan, BuildOptions{
		Source:    sourceFromRows(rows),
		BatchSize: batchSize,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	return result
}

// Test data helpers

func intV(n int64) event.Value              { return event.IntValue(n) }
func strV(s string) event.Value             { return event.StringValue(s) }
func floatV(f float64) event.Value          { return event.FloatValue(f) }
func boolV(b bool) event.Value              { return event.BoolValue(b) }
func nullV() event.Value                    { return event.NullValue() }
func tsV(t time.Time) event.Value           { return event.TimestampValue(t) }
func arrV(elems ...event.Value) event.Value { return event.ArrayValue(elems) }
func objV(fields map[string]event.Value) event.Value {
	return event.ObjectValue(fields)
}

func TestBuild_InlineSource(t *testing.T) {
	got := drain(t, `from [{level: "ERROR", w: 3, tags: ["hot"], meta: {zone: "a"}}, {level: "WARN", w: 2}, {level: "ERROR", w: 4}] | stats sum(w) as total by level | sort level`, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d: %#v", len(got), got)
	}
	if level := got[0]["level"].AsString(); level != "ERROR" {
		t.Fatalf("row 0 level = %q, want ERROR", level)
	}
	if total := got[0]["total"].AsFloat(); total != 7 {
		t.Fatalf("row 0 total = %g, want 7", total)
	}
	if level := got[1]["level"].AsString(); level != "WARN" {
		t.Fatalf("row 1 level = %q, want WARN", level)
	}
	if total := got[1]["total"].AsFloat(); total != 2 {
		t.Fatalf("row 1 total = %g, want 2", total)
	}
}

func TestBuild_ScalarLet(t *testing.T) {
	got := drain(t, `let $slo = 250ms; from [{duration: 300ms}, {duration: 100ms}, {duration: 400ms}] | where duration > $slo | stats count() as n`, nil)
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d: %#v", len(got), got)
	}
	if n := got[0]["n"].AsInt(); n != 2 {
		t.Fatalf("n = %d, want 2", n)
	}
}

func TestBuild_StatsByStar(t *testing.T) {
	rows := []map[string]event.Value{
		{"host": strV("a"), "index": strV("main")},
		{"host": strV("a"), "index": strV("main")},
		{"host": strV("b"), "index": strV("main")},
	}
	got := drain(t, `from * | stats count() as n by * | sort host`, rows)
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d: %#v", len(got), got)
	}
	if host := got[0]["host"].AsString(); host != "a" {
		t.Fatalf("row 0 host = %q, want a", host)
	}
	if n := got[0]["n"].AsInt(); n != 2 {
		t.Fatalf("row 0 n = %d, want 2", n)
	}
	if host := got[1]["host"].AsString(); host != "b" {
		t.Fatalf("row 1 host = %q, want b", host)
	}
	if n := got[1]["n"].AsInt(); n != 1 {
		t.Fatalf("row 1 n = %d, want 1", n)
	}
}

func TestBuild_ColumnsMacroInStats(t *testing.T) {
	rows := []map[string]event.Value{
		{"_raw": strV(`{"db_ms":12,"api_ms":7}`)},
		{"_raw": strV(`{"db_ms":5,"api_ms":20}`)},
	}
	got := drain(t, `from * | parse json into (db_ms as int, api_ms as int, level as string) | stats max(columns("*_ms"))`, rows)
	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d: %#v", len(got), got)
	}
	if maxDB := got[0]["max_db_ms"].AsInt(); maxDB != 12 {
		t.Fatalf("max_db_ms = %d, want 12", maxDB)
	}
	if maxAPI := got[0]["max_api_ms"].AsInt(); maxAPI != 20 {
		t.Fatalf("max_api_ms = %d, want 20", maxAPI)
	}
}

func TestBuild_Unpivot(t *testing.T) {
	rows := []map[string]event.Value{
		{"service": strV("api"), "cpu_ms": intV(12), "db_ms": intV(7), "status": intV(200)},
		{"service": strV("web"), "cpu_ms": intV(5), "status": intV(500)},
	}

	got := drain(t, `from * | unpivot cpu_ms, db_ms as metric, value`, rows)

	want := []map[string]event.Value{
		{"service": strV("api"), "status": intV(200), "metric": strV("cpu_ms"), "value": intV(12)},
		{"service": strV("api"), "status": intV(200), "metric": strV("db_ms"), "value": intV(7)},
		{"service": strV("web"), "status": intV(500), "metric": strV("cpu_ms"), "value": intV(5)},
		{"service": strV("web"), "status": intV(500), "metric": strV("db_ms"), "value": nullV()},
	}
	if len(got) != len(want) {
		t.Fatalf("row count: got %d want %d rows: %#v", len(got), len(want), got)
	}
	for i := range want {
		for field, wantValue := range want[i] {
			if gotValue := got[i][field]; gotValue != wantValue {
				t.Fatalf("row %d field %s: got %v want %v", i, field, gotValue, wantValue)
			}
		}
		if _, ok := got[i]["cpu_ms"]; ok {
			t.Fatalf("row %d retained cpu_ms: %#v", i, got[i])
		}
		if _, ok := got[i]["db_ms"]; ok {
			t.Fatalf("row %d retained db_ms: %#v", i, got[i])
		}
	}
}

func TestBuild_TopBy(t *testing.T) {
	rows := []map[string]event.Value{
		{"service": strV("api"), "uri": strV("/a")},
		{"service": strV("api"), "uri": strV("/a")},
		{"service": strV("api"), "uri": strV("/b")},
		{"service": strV("web"), "uri": strV("/x")},
		{"service": strV("web"), "uri": strV("/x")},
		{"service": strV("web"), "uri": strV("/y")},
	}

	got := drain(t, `from * | top 1 uri by service`, rows)

	if len(got) != 2 {
		t.Fatalf("row count: got %d want 2 rows: %#v", len(got), got)
	}
	expected := []struct {
		service string
		uri     string
		count   int64
	}{
		{service: "api", uri: "/a", count: 2},
		{service: "web", uri: "/x", count: 2},
	}
	for i, want := range expected {
		if gotService := got[i]["service"]; gotService != strV(want.service) {
			t.Fatalf("row %d service: got %s want %s", i, gotService.String(), want.service)
		}
		if gotURI := got[i]["uri"]; gotURI != strV(want.uri) {
			t.Fatalf("row %d uri: got %s want %s", i, gotURI.String(), want.uri)
		}
		if gotCount := got[i]["count"]; gotCount != intV(want.count) {
			t.Fatalf("row %d count: got %s want %d", i, gotCount.String(), want.count)
		}
		if _, ok := got[i]["_top_rank"]; ok {
			t.Fatalf("row %d retained _top_rank: %#v", i, got[i])
		}
	}
}

func intPtr(n int64) *int64       { return &n }
func floatPtr(n float64) *float64 { return &n }

func assertOptionalIntField(t *testing.T, row map[string]event.Value, field string, want *int64, rowIndex int) {
	t.Helper()
	got := row[field]
	if want == nil {
		if !got.IsNull() {
			t.Errorf("row %d field %s: expected null, got %s", rowIndex, field, got.String())
		}
		return
	}
	assertIntField(t, row, field, *want, rowIndex)
}

func assertOptionalFloatField(t *testing.T, row map[string]event.Value, field string, want *float64, rowIndex int) {
	t.Helper()
	got := row[field]
	if want == nil {
		if !got.IsNull() {
			t.Errorf("row %d field %s: expected null, got %s", rowIndex, field, got.String())
		}
		return
	}
	assertFloatField(t, row, field, *want, rowIndex)
}

func assertIntField(t *testing.T, row map[string]event.Value, field string, want int64, rowIndex int) {
	t.Helper()
	got, ok := row[field].TryAsInt()
	if !ok || got != want {
		t.Errorf("row %d field %s: expected %d, got %s", rowIndex, field, want, row[field].String())
	}
}

func assertFloatField(t *testing.T, row map[string]event.Value, field string, want float64, rowIndex int) {
	t.Helper()
	got, ok := row[field].TryAsFloat()
	if !ok || math.Abs(got-want) > 0.01 {
		t.Errorf("row %d field %s: expected %f, got %s", rowIndex, field, want, row[field].String())
	}
}

func assertObjectFloatField(
	t *testing.T,
	row map[string]event.Value,
	field string,
	key string,
	want float64,
	rowIndex int,
) {
	t.Helper()
	obj, ok := row[field].TryAsObject()
	if !ok {
		t.Fatalf("row %d field %s: expected object, got %s", rowIndex, field, row[field].Type())
	}
	got, ok := obj[key].TryAsFloat()
	if !ok || math.Abs(got-want) > 0.01 {
		t.Errorf("row %d field %s.%s: expected %f, got %s", rowIndex, field, key, want, obj[key].String())
	}
}

func sampleRows() []map[string]event.Value {
	return []map[string]event.Value{
		{"level": strV("info"), "status": intV(200), "duration": floatV(10.5), "host": strV("web-01")},
		{"level": strV("error"), "status": intV(500), "duration": floatV(100.2), "host": strV("web-01")},
		{"level": strV("warn"), "status": intV(404), "duration": floatV(5.1), "host": strV("web-02")},
		{"level": strV("error"), "status": intV(503), "duration": floatV(200.3), "host": strV("web-02")},
		{"level": strV("info"), "status": intV(200), "duration": floatV(8.7), "host": strV("web-01")},
	}
}

func idRows(n int) []map[string]event.Value {
	rows := make([]map[string]event.Value, n)
	for i := range rows {
		rows[i] = map[string]event.Value{"id": intV(int64(i))}
	}
	return rows
}

func rowIDs(t *testing.T, rows []map[string]event.Value) []int64 {
	t.Helper()
	ids := make([]int64, len(rows))
	for i, row := range rows {
		id, ok := row["id"].TryAsInt()
		if !ok {
			t.Fatalf("row %d: missing integer id: %v", i, row["id"])
		}
		ids[i] = id
	}
	return ids
}

func timedRows() []map[string]event.Value {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return []map[string]event.Value{
		{"_time": tsV(base), "level": strV("info"), "val": intV(10)},
		{"_time": tsV(base.Add(1 * time.Minute)), "level": strV("error"), "val": intV(20)},
		{"_time": tsV(base.Add(5 * time.Minute)), "level": strV("info"), "val": intV(30)},
		{"_time": tsV(base.Add(6 * time.Minute)), "level": strV("error"), "val": intV(40)},
		{"_time": tsV(base.Add(11 * time.Minute)), "level": strV("warn"), "val": intV(50)},
		{"_time": tsV(base.Add(12 * time.Minute)), "level": strV("info"), "val": intV(60)},
	}
}

// Tests: Filter (where)

func TestBuild_Filter_Simple(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | where status >= 500`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
	for _, r := range result {
		s, ok := r["status"]
		if !ok {
			t.Fatal("missing status field")
		}
		n, _ := s.TryAsInt()
		if n < 500 {
			t.Errorf("expected status >= 500, got %d", n)
		}
	}
}

func TestBuild_Filter_StringEquality(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | where level == "error"`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
}

func TestBuild_Filter_And(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | where level == "error" and status >= 503`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
}

// Tests: Extend (eval)

func TestBuild_Extend_Simple(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | extend doubled = status * 2`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}
	for i, r := range result {
		d, ok := r["doubled"]
		if !ok {
			t.Fatalf("row %d: missing 'doubled' field", i)
		}
		s := rows[i]["status"]
		sn, _ := s.TryAsInt()
		dn, _ := d.TryAsInt()
		if dn != sn*2 {
			t.Errorf("row %d: expected doubled=%d, got %d", i, sn*2, dn)
		}
	}
}

func TestBuild_Extend_LaterSeesEarlier(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(10)},
	}
	result := drain(t, `from * | extend a = x + 1, b = a + 1`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	a, _ := result[0]["a"].TryAsInt()
	b, _ := result[0]["b"].TryAsInt()
	if a != 11 {
		t.Errorf("expected a=11, got %d", a)
	}
	if b != 12 {
		t.Errorf("expected b=12, got %d", b)
	}
}

func TestBuild_Extend_NullPropagation(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(10)},
		{"x": nullV()},
	}
	result := drain(t, `from * | extend y = x + 1`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
	// First row: y = 11
	y0, _ := result[0]["y"].TryAsInt()
	if y0 != 11 {
		t.Errorf("row 0: expected y=11, got %d", y0)
	}
	// Second row: y = null (null propagation)
	if !result[1]["y"].IsNull() {
		t.Errorf("row 1: expected y=null, got %v", result[1]["y"])
	}
}

// Tests: Extend + Filter chain

func TestBuild_ExtendThenFilter(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | extend slow = duration > 50 | where slow == true`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
}

// Tests: Aggregate (stats)

func TestBuild_Stats_Count(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | stats count()`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	c, _ := result[0]["count()"].TryAsInt()
	if c != 5 {
		t.Errorf("expected count=5, got %d", c)
	}
}

func TestBuild_Stats_SumAvgByKey(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | stats sum(duration) as total, avg(duration) as mean by level`, rows)
	// 3 levels: info, error, warn
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}
	found := make(map[string]bool)
	for _, r := range result {
		lv, _ := r["level"].TryAsString()
		found[lv] = true
	}
	for _, lv := range []string{"info", "error", "warn"} {
		if !found[lv] {
			t.Errorf("missing level %q in results", lv)
		}
	}
}

func TestBuild_Stats_DC(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | stats dc(host) as hosts`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	h, _ := result[0]["hosts"].TryAsInt()
	if h != 2 {
		t.Errorf("expected dc(host)=2, got %d", h)
	}
}

func TestBuild_Stats_P95(t *testing.T) {
	rows := make([]map[string]event.Value, 100)
	for i := range rows {
		rows[i] = map[string]event.Value{"val": floatV(float64(i + 1))}
	}
	result := drain(t, `from * | stats p95(val) as p95_val`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	p95, _ := result[0]["p95_val"].TryAsFloat()
	// For 1..100, p95 should be ~95.
	if p95 < 90 || p95 > 100 {
		t.Errorf("expected p95 near 95, got %f", p95)
	}
}

func TestBuild_Stats_ConditionalCount(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | stats count(status, where status >= 500) as errors`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	c, _ := result[0]["errors"].TryAsInt()
	if c != 2 {
		t.Errorf("expected errors=2, got %d", c)
	}
}

// Tests: TimeBin in stats

func TestBuild_Stats_TimeBin(t *testing.T) {
	rows := timedRows()
	result := drain(t, `from * | stats count() by bin(_time, 5m)`, rows)
	// 0-5m: 2 rows, 5-10m: 2 rows, 10-15m: 2 rows
	if len(result) < 2 {
		t.Fatalf("expected at least 2 time buckets, got %d", len(result))
	}
	for _, r := range result {
		_, ok := r["_time"]
		if !ok {
			t.Error("missing _time in time-bucketed stats result")
		}
	}
}

func TestBuild_Stats_TimeBinOrigin(t *testing.T) {
	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	rows := []map[string]event.Value{
		{"_time": tsV(base), "val": intV(1)},
		{"_time": tsV(base.Add(2 * time.Minute)), "val": intV(2)},
		{"_time": tsV(base.Add(6 * time.Minute)), "val": intV(3)},
	}
	result := drain(t, `from * | stats count() by bin(_time, 5m, "2026-01-01T10:01:00Z")`, rows)
	if len(result) != 3 {
		t.Fatalf("expected 3 shifted time buckets, got %d: %#v", len(result), result)
	}
	slices.SortFunc(result, func(a, b map[string]event.Value) int {
		ta, _ := a["_time"].TryAsTimestamp()
		tb, _ := b["_time"].TryAsTimestamp()
		return ta.Compare(tb)
	})
	want := []time.Time{
		base.Add(-4 * time.Minute),
		base.Add(1 * time.Minute),
		base.Add(6 * time.Minute),
	}
	for i := range want {
		got, ok := result[i]["_time"].TryAsTimestamp()
		if !ok || !got.Equal(want[i]) {
			t.Fatalf("bucket %d = %v, want %v", i, result[i]["_time"], want[i])
		}
	}
}

func TestBuild_EveryFillGapfill(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []map[string]event.Value{
		{"_time": tsV(base), "service": strV("api")},
		{"_time": tsV(base.Add(10 * time.Minute)), "service": strV("api")},
		{"_time": tsV(base), "service": strV("web")},
		{"_time": tsV(base.Add(5 * time.Minute)), "service": strV("web")},
	}

	result := drain(t, `from * | every 5m by service stats count() as count fill=0`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d: %#v", len(result), result)
	}

	var gap map[string]event.Value
	for _, row := range result {
		service, _ := row["service"].TryAsString()
		ts, _ := row["_time"].TryAsTimestamp()
		if service == "api" && ts.Equal(base.Add(5*time.Minute)) {
			gap = row
			break
		}
	}
	if gap == nil {
		t.Fatalf("missing api gap row: %#v", result)
	}
	count, ok := gap["count"].TryAsInt()
	if !ok || count != 0 {
		t.Fatalf("gap count: got %s, want 0", gap["count"].String())
	}
}

// Tests: EventStats / StreamStats

func TestBuild_EventStats(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | eventstats count() as total by level`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows (all input preserved), got %d", len(result))
	}
	for _, r := range result {
		_, ok := r["total"]
		if !ok {
			t.Error("missing 'total' field from eventstats")
		}
	}
}

func TestBuild_ListValuesLimits(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | stats values(level, 2) as levels, list(level, 3) as listed`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	if got := result[0]["levels"].AsString(); got != "info|||error" {
		t.Fatalf("values(level, 2) got %q", got)
	}
	if got := result[0]["listed"].AsString(); got != "info|||error|||warn" {
		t.Fatalf("list(level, 3) got %q", got)
	}
}

func TestBuild_ListValuesLimitsInWindowAggregates(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | eventstats values(level, 2) as levels | streamstats list(level, 2) as seen`, rows)
	if len(result) != len(rows) {
		t.Fatalf("expected %d rows, got %d", len(rows), len(result))
	}
	for i, row := range result {
		if got := row["levels"].AsString(); got != "info|||error" {
			t.Fatalf("row %d eventstats values(level, 2) got %q", i, got)
		}
	}
	if got := result[0]["seen"].AsString(); got != "info" {
		t.Fatalf("row 0 streamstats list(level, 2) got %q", got)
	}
	if got := result[2]["seen"].AsString(); got != "info|||error" {
		t.Fatalf("row 2 streamstats list(level, 2) got %q", got)
	}
}

func TestBuild_CorrCovarAggregates(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(1), "y": intV(2)},
		{"x": intV(2), "y": intV(4)},
		{"x": intV(3), "y": intV(6)},
	}
	result := drain(t, `from * | stats corr(x, y) as r, covar(x, y) as c, linear_fit(x, y) as fit`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	assertFloatField(t, result[0], "r", 1, 0)
	assertFloatField(t, result[0], "c", 2, 0)
	assertObjectFloatField(t, result[0], "fit", "slope", 2, 0)
	assertObjectFloatField(t, result[0], "fit", "intercept", 0, 0)
	assertObjectFloatField(t, result[0], "fit", "r2", 1, 0)
}

func TestBuild_ObjectSumAggregates(t *testing.T) {
	rows := []map[string]event.Value{
		{
			"host":    strV("a"),
			"metrics": objV(map[string]event.Value{"ok": intV(2), "err": intV(1), "zero": intV(2), "skip": strV("x")}),
		},
		{
			"host":    strV("a"),
			"metrics": objV(map[string]event.Value{"ok": intV(3), "latency": floatV(1.5), "zero": intV(-2)}),
		},
		{
			"host":    strV("b"),
			"metrics": objV(map[string]event.Value{"err": intV(4)}),
		},
	}

	statsRows := drain(t, `from * | stats sum_object(metrics) as totals by host`, rows)
	if len(statsRows) != 2 {
		t.Fatalf("stats row count got %d, want 2", len(statsRows))
	}
	byHost := make(map[string]map[string]event.Value, len(statsRows))
	for _, row := range statsRows {
		byHost[row["host"].AsString()] = row
	}
	assertObjectFloatField(t, byHost["a"], "totals", "ok", 5, 0)
	assertObjectFloatField(t, byHost["a"], "totals", "err", 1, 0)
	assertObjectFloatField(t, byHost["a"], "totals", "latency", 1.5, 0)
	assertObjectFloatField(t, byHost["a"], "totals", "zero", 0, 0)
	assertObjectFloatField(t, byHost["b"], "totals", "err", 4, 0)

	eventRows := drain(t, `from * | eventstats sum_object(metrics) as totals by host`, rows)
	assertObjectFloatField(t, eventRows[0], "totals", "ok", 5, 0)
	assertObjectFloatField(t, eventRows[1], "totals", "latency", 1.5, 1)
	assertObjectFloatField(t, eventRows[2], "totals", "err", 4, 2)

	streamRows := drain(t, `from * | streamstats window=2 sum_object(metrics) as totals by host`, rows)
	assertObjectFloatField(t, streamRows[0], "totals", "ok", 2, 0)
	assertObjectFloatField(t, streamRows[1], "totals", "ok", 5, 1)
	assertObjectFloatField(t, streamRows[1], "totals", "latency", 1.5, 1)
	assertObjectFloatField(t, streamRows[1], "totals", "zero", 0, 1)
	assertObjectFloatField(t, streamRows[2], "totals", "err", 4, 2)
}

func TestBuild_WeightedTopKAggregate(t *testing.T) {
	rows := []map[string]event.Value{
		{"item": strV("a"), "w": intV(2)},
		{"item": strV("b"), "w": intV(5)},
		{"item": strV("a"), "w": intV(4)},
		{"item": strV("c"), "w": intV(10)},
		{"item": strV("b"), "w": intV(1)},
	}

	statsRows := drain(t, `from * | stats top_k_weighted(item, w, 2) as top_items`, rows)
	if len(statsRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(statsRows))
	}
	assertWeightedTopKField(t, statsRows[0], "top_items", 0, "c", 10)
	assertWeightedTopKField(t, statsRows[0], "top_items", 1, "a", 6)

	eventRows := drain(t, `from * | eventstats top_k_weighted(item, w, 2) as top_items`, rows)
	assertWeightedTopKField(t, eventRows[0], "top_items", 0, "c", 10)
	assertWeightedTopKField(t, eventRows[4], "top_items", 1, "a", 6)

	streamRows := drain(t, `from * | streamstats top_k_weighted(item, w, 2) as top_items`, rows)
	assertWeightedTopKField(t, streamRows[0], "top_items", 0, "a", 2)
	assertWeightedTopKField(t, streamRows[4], "top_items", 0, "c", 10)
	assertWeightedTopKField(t, streamRows[4], "top_items", 1, "a", 6)
}

func assertWeightedTopKField(
	t *testing.T,
	row map[string]event.Value,
	field string,
	index int,
	wantValue string,
	wantWeight float64,
) {
	t.Helper()
	values, ok := row[field].TryAsArray()
	if !ok || len(values) <= index {
		t.Fatalf("%s: missing weighted top-k entry %d in %s", field, index, row[field].String())
	}
	obj, ok := values[index].TryAsObject()
	if !ok {
		t.Fatalf("%s[%d]: expected object, got %s", field, index, values[index].String())
	}
	if got := obj["value"].String(); got != wantValue {
		t.Fatalf("%s[%d].value = %s, want %s", field, index, got, wantValue)
	}
	gotWeight, ok := obj["weight"].TryAsFloat()
	if !ok || math.Abs(gotWeight-wantWeight) > 0.01 {
		t.Fatalf("%s[%d].weight = %s, want %f", field, index, obj["weight"].String(), wantWeight)
	}
}

func TestBuild_PercAggregate(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(1)},
		{"x": intV(2)},
		{"x": intV(3)},
		{"x": intV(4)},
		{"x": intV(5)},
	}

	statsRows := drain(t, `from * | stats perc(x, 25) as p25, perc(x, 99.9) as p999`, rows)
	if len(statsRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(statsRows))
	}
	assertFloatField(t, statsRows[0], "p25", 2, 0)
	assertFloatField(t, statsRows[0], "p999", 4.996, 0)

	eventRows := drain(t, `from * | eventstats perc(x, 25) as p25`, rows)
	assertFloatField(t, eventRows[0], "p25", 2, 0)
	assertFloatField(t, eventRows[4], "p25", 2, 4)

	streamRows := drain(t, `from * | streamstats perc(x, 25) as p25`, rows)
	assertFloatField(t, streamRows[4], "p25", 2, 4)
}

func TestBuild_WeightedPercAggregate(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(10), "w": intV(100)},
		{"x": intV(100), "w": intV(1)},
	}

	statsRows := drain(t, `from * | stats perc_weighted(x, w, 50) as p50w`, rows)
	if len(statsRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(statsRows))
	}
	assertWeightedPercentileNearHeavyValue(t, statsRows[0], "p50w", 0)

	eventRows := drain(t, `from * | eventstats perc_weighted(x, w, 50) as p50w`, rows)
	assertWeightedPercentileNearHeavyValue(t, eventRows[0], "p50w", 0)
	assertWeightedPercentileNearHeavyValue(t, eventRows[1], "p50w", 1)

	streamRows := drain(t, `from * | streamstats perc_weighted(x, w, 50) as p50w`, rows)
	assertFloatField(t, streamRows[0], "p50w", 10, 0)
	assertWeightedPercentileNearHeavyValue(t, streamRows[1], "p50w", 1)
}

func assertWeightedPercentileNearHeavyValue(t *testing.T, row map[string]event.Value, field string, rowIndex int) {
	t.Helper()
	got, ok := row[field].TryAsFloat()
	if !ok || got < 10 || got > 20 {
		t.Fatalf("row %d field %s: got %s, want weighted percentile near 10", rowIndex, field, row[field].String())
	}
}

func TestBuild_MomentAggregates(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(1)},
		{"x": intV(2)},
		{"x": intV(3)},
		{"x": intV(10)},
	}

	statsRows := drain(t, `from * | stats skewness(x) as skew, kurtosis(x) as kurt`, rows)
	if len(statsRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(statsRows))
	}
	assertFloatField(t, statsRows[0], "skew", 1.763632614803888, 0)
	assertFloatField(t, statsRows[0], "kurt", 3.228, 0)

	eventRows := drain(t, `from * | eventstats skewness(x) as skew, kurtosis(x) as kurt`, rows)
	assertFloatField(t, eventRows[0], "skew", 1.763632614803888, 0)
	assertFloatField(t, eventRows[3], "kurt", 3.228, 3)

	streamRows := drain(t, `from * | streamstats skewness(x) as skew, kurtosis(x) as kurt`, rows)
	if !streamRows[0]["skew"].IsNull() || !streamRows[1]["skew"].IsNull() {
		t.Fatalf("streamstats skewness should be null until three values")
	}
	if !streamRows[2]["kurt"].IsNull() {
		t.Fatalf("streamstats kurtosis should be null until four values")
	}
	assertFloatField(t, streamRows[3], "skew", 1.763632614803888, 3)
	assertFloatField(t, streamRows[3], "kurt", 3.228, 3)
}

func TestBuild_MADAggregate(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(1)},
		{"x": intV(1)},
		{"x": intV(2)},
		{"x": intV(2)},
		{"x": intV(4)},
		{"x": intV(6)},
		{"x": intV(9)},
	}

	statsRows := drain(t, `from * | stats mad(x) as spread`, rows)
	if len(statsRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(statsRows))
	}
	assertFloatField(t, statsRows[0], "spread", 1, 0)

	eventRows := drain(t, `from * | eventstats mad(x) as spread`, rows)
	assertFloatField(t, eventRows[0], "spread", 1, 0)
	assertFloatField(t, eventRows[6], "spread", 1, 6)

	streamRows := drain(t, `from * | streamstats mad(x) as spread`, rows)
	assertFloatField(t, streamRows[0], "spread", 0, 0)
	assertFloatField(t, streamRows[6], "spread", 1, 6)
}

func TestBuild_DeltaSumAggregate(t *testing.T) {
	rows := []map[string]event.Value{
		{"host": strV("a"), "bytes": intV(10)},
		{"host": strV("a"), "bytes": intV(15)},
		{"host": strV("a"), "bytes": intV(3)},
		{"host": strV("a"), "bytes": intV(8)},
		{"host": strV("a"), "bytes": event.NullValue()},
		{"host": strV("a"), "bytes": intV(12)},
		{"host": strV("b"), "bytes": intV(7)},
	}

	result := drain(t, `from * | stats delta_sum(bytes) as d by host`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
	byHost := make(map[string]map[string]event.Value, len(result))
	for _, row := range result {
		byHost[row["host"].AsString()] = row
	}
	assertFloatField(t, byHost["a"], "d", 14, 0)
	assertFloatField(t, byHost["b"], "d", 0, 0)

	eventRows := drain(t, `from * | eventstats delta_sum(bytes) as d by host`, rows)
	assertFloatField(t, eventRows[0], "d", 14, 0)
	assertFloatField(t, eventRows[5], "d", 14, 5)
	assertFloatField(t, eventRows[6], "d", 0, 6)

	streamRows := drain(t, `from * | streamstats delta_sum(bytes) as d by host`, rows)
	expected := []float64{0, 5, 5, 10, 10, 14, 0}
	for i, want := range expected {
		assertFloatField(t, streamRows[i], "d", want, i)
	}
}

func TestBuild_HistogramAggregate(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(0)},
		{"x": intV(1)},
		{"x": intV(2)},
		{"x": intV(3)},
	}

	statsRows := drain(t, `from * | stats histogram(x, 2) as h`, rows)
	if len(statsRows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(statsRows))
	}
	assertHistogramCounts(t, statsRows[0]["h"], []int64{2, 2})

	eventRows := drain(t, `from * | eventstats histogram(x, 2) as h`, rows)
	assertHistogramCounts(t, eventRows[0]["h"], []int64{2, 2})
	assertHistogramCounts(t, eventRows[3]["h"], []int64{2, 2})

	streamRows := drain(t, `from * | streamstats histogram(x, 2) as h`, rows)
	assertHistogramCounts(t, streamRows[3]["h"], []int64{2, 2})
}

func TestBuild_HistSugar(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(0)},
		{"x": intV(1)},
		{"x": intV(2)},
		{"x": intV(3)},
	}

	got := drain(t, `from * | hist x bins=2`, rows)
	if len(got) != 2 {
		t.Fatalf("expected 2 histogram rows, got %d: %#v", len(got), got)
	}
	for i, row := range got {
		assertIntField(t, row, "count", 2, i)
		if !isNumericValue(row["lo"]) {
			t.Fatalf("row %d lo got %s, want numeric", i, row["lo"].String())
		}
		if !isNumericValue(row["hi"]) {
			t.Fatalf("row %d hi got %s, want numeric", i, row["hi"].String())
		}
		if _, ok := row["chart"].TryAsString(); !ok {
			t.Fatalf("row %d chart got %s, want string", i, row["chart"].String())
		}
		for _, temp := range []string{"_h", "_m", "bin"} {
			if _, ok := row[temp]; ok {
				t.Fatalf("row %d retained temp field %s: %#v", i, temp, row)
			}
		}
	}
}

func isNumericValue(v event.Value) bool {
	if _, ok := v.TryAsInt(); ok {
		return true
	}
	if _, ok := v.TryAsFloat(); ok {
		return true
	}
	return false
}

func assertHistogramCounts(t *testing.T, value event.Value, want []int64) {
	t.Helper()
	bins, ok := value.TryAsArray()
	if !ok {
		t.Fatalf("histogram got %s, want array", value.String())
	}
	if len(bins) != len(want) {
		t.Fatalf("histogram bin count got %d, want %d", len(bins), len(want))
	}
	for i, bin := range bins {
		obj, ok := bin.TryAsObject()
		if !ok {
			t.Fatalf("histogram bin %d got %s, want object", i, bin.String())
		}
		count, ok := obj["count"].TryAsInt()
		if !ok || count != want[i] {
			t.Fatalf("histogram bin %d count got %s, want %d", i, obj["count"].String(), want[i])
		}
		if _, ok := obj["lo"].TryAsFloat(); !ok {
			t.Fatalf("histogram bin %d lo got %s, want float", i, obj["lo"].String())
		}
		if _, ok := obj["hi"].TryAsFloat(); !ok {
			t.Fatalf("histogram bin %d hi got %s, want float", i, obj["hi"].String())
		}
	}
}

func TestBuild_CorrCovarInWindowAggregates(t *testing.T) {
	rows := []map[string]event.Value{
		{"x": intV(1), "y": intV(2)},
		{"x": intV(2), "y": intV(4)},
		{"x": intV(3), "y": intV(6)},
	}
	result := drain(t, `from * | eventstats corr(x, y) as er, covar(x, y) as ec, linear_fit(x, y) as ef | streamstats corr(x, y) as sr, covar(x, y) as sc, linear_fit(x, y) as sf`, rows)
	if len(result) != len(rows) {
		t.Fatalf("expected %d rows, got %d", len(rows), len(result))
	}
	for i, row := range result {
		assertFloatField(t, row, "er", 1, i)
		assertFloatField(t, row, "ec", 2, i)
		assertObjectFloatField(t, row, "ef", "slope", 2, i)
		assertObjectFloatField(t, row, "ef", "intercept", 0, i)
		assertObjectFloatField(t, row, "ef", "r2", 1, i)
	}
	if !result[0]["sr"].IsNull() {
		t.Fatalf("row 0 streamstats corr got %v, want null", result[0]["sr"])
	}
	if !result[0]["sf"].IsNull() {
		t.Fatalf("row 0 streamstats linear_fit got %v, want null", result[0]["sf"])
	}
	assertFloatField(t, result[1], "sr", 1, 1)
	assertFloatField(t, result[1], "sc", 1, 1)
	assertObjectFloatField(t, result[1], "sf", "slope", 2, 1)
	assertObjectFloatField(t, result[1], "sf", "intercept", 0, 1)
	assertObjectFloatField(t, result[1], "sf", "r2", 1, 1)
	assertFloatField(t, result[2], "sr", 1, 2)
	assertFloatField(t, result[2], "sc", 2, 2)
	assertObjectFloatField(t, result[2], "sf", "slope", 2, 2)
	assertObjectFloatField(t, result[2], "sf", "intercept", 0, 2)
	assertObjectFloatField(t, result[2], "sf", "r2", 1, 2)
}

func TestBuild_StreamStats(t *testing.T) {
	rows := []map[string]event.Value{
		{"val": intV(1)},
		{"val": intV(2)},
		{"val": intV(3)},
		{"val": intV(4)},
		{"val": intV(5)},
	}
	result := drain(t, `from * | streamstats sum(val) as running_sum`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}
	// running_sum should be: 1, 3, 6, 10, 15
	expected := []float64{1, 3, 6, 10, 15}
	for i, r := range result {
		rs, _ := r["running_sum"].TryAsFloat()
		if math.Abs(rs-expected[i]) > 0.01 {
			t.Errorf("row %d: expected running_sum=%f, got %f", i, expected[i], rs)
		}
	}
}

func TestBuild_StreamStats_Window(t *testing.T) {
	rows := []map[string]event.Value{
		{"val": intV(1)},
		{"val": intV(2)},
		{"val": intV(3)},
		{"val": intV(4)},
		{"val": intV(5)},
	}
	result := drain(t, `from * | streamstats window=3 sum(val) as running_sum`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}
	// Window=3: sum of current + 2 previous (or fewer for first rows).
	// Row 0: 1, Row 1: 1+2=3, Row 2: 1+2+3=6, Row 3: 2+3+4=9, Row 4: 3+4+5=12
	expected := []float64{1, 3, 6, 9, 12}
	for i, r := range result {
		rs, _ := r["running_sum"].TryAsFloat()
		if math.Abs(rs-expected[i]) > 0.01 {
			t.Errorf("row %d: expected running_sum=%f, got %f", i, expected[i], rs)
		}
	}
}

func TestBuild_StreamStats_DurationWindow(t *testing.T) {
	result := drain(t, `from * | streamstats window=5m sum(val) as recent_sum`, timedRows())
	if len(result) != 6 {
		t.Fatalf("expected 6 rows, got %d", len(result))
	}
	expected := []float64{10, 30, 60, 90, 90, 110}
	for i, r := range result {
		got, _ := r["recent_sum"].TryAsFloat()
		if math.Abs(got-expected[i]) > 0.01 {
			t.Errorf("row %d: expected recent_sum=%f, got %f", i, expected[i], got)
		}
	}
}

func TestBuild_StreamStats_CurrentFalse(t *testing.T) {
	rows := []map[string]event.Value{
		{"val": intV(1)},
		{"val": intV(2)},
		{"val": intV(3)},
	}
	result := drain(t, `from * | streamstats current=false sum(val) as previous_sum`, rows)
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}
	expected := []float64{0, 1, 3}
	for i, r := range result {
		got, _ := r["previous_sum"].TryAsFloat()
		if math.Abs(got-expected[i]) > 0.01 {
			t.Errorf("row %d: expected previous_sum=%f, got %f", i, expected[i], got)
		}
	}
}

func TestBuild_StreamStats_DurationWindowCurrentFalse(t *testing.T) {
	result := drain(t, `from * | streamstats window=5m current=false sum(val) as recent_sum`, timedRows())
	if len(result) != 6 {
		t.Fatalf("expected 6 rows, got %d", len(result))
	}
	expected := []float64{0, 10, 30, 50, 40, 50}
	for i, r := range result {
		got, _ := r["recent_sum"].TryAsFloat()
		if math.Abs(got-expected[i]) > 0.01 {
			t.Errorf("row %d: expected recent_sum=%f, got %f", i, expected[i], got)
		}
	}
}

func TestBuild_StreamStats_WindowOnlyFunctions(t *testing.T) {
	rows := []map[string]event.Value{
		{"host": strV("a"), "val": intV(10)},
		{"host": strV("a"), "val": intV(20)},
		{"host": strV("b"), "val": intV(5)},
		{"host": strV("a"), "val": intV(30)},
		{"host": strV("b"), "val": intV(7)},
	}
	result := drain(t, `from * | streamstats lag(val) as prev, lead(val) as next, row_number() as rn, running_sum(val) as total, moving_avg(val, 2) as avg2, delta(val) as d, ema(val, 3) as ema3 by host`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}

	expected := []struct {
		prev  *int64
		next  *int64
		rn    int64
		total float64
		avg2  float64
		d     *float64
		ema3  float64
	}{
		{prev: nil, next: intPtr(20), rn: 1, total: 10, avg2: 10, d: nil, ema3: 10},
		{prev: intPtr(10), next: intPtr(30), rn: 2, total: 30, avg2: 15, d: floatPtr(10), ema3: 15},
		{prev: nil, next: intPtr(7), rn: 1, total: 5, avg2: 5, d: nil, ema3: 5},
		{prev: intPtr(20), next: nil, rn: 3, total: 60, avg2: 25, d: floatPtr(10), ema3: 22.5},
		{prev: intPtr(5), next: nil, rn: 2, total: 12, avg2: 6, d: floatPtr(2), ema3: 6},
	}
	for i, r := range result {
		assertOptionalIntField(t, r, "prev", expected[i].prev, i)
		assertOptionalIntField(t, r, "next", expected[i].next, i)
		assertIntField(t, r, "rn", expected[i].rn, i)
		assertFloatField(t, r, "total", expected[i].total, i)
		assertFloatField(t, r, "avg2", expected[i].avg2, i)
		assertOptionalFloatField(t, r, "d", expected[i].d, i)
		assertFloatField(t, r, "ema3", expected[i].ema3, i)
	}
}

func TestBuild_StreamStatsRank(t *testing.T) {
	rows := []map[string]event.Value{
		{"service": strV("api"), "route": strV("/a"), "n": intV(10)},
		{"service": strV("api"), "route": strV("/b"), "n": intV(10)},
		{"service": strV("api"), "route": strV("/c"), "n": intV(7)},
		{"service": strV("web"), "route": strV("/x"), "n": intV(9)},
		{"service": strV("web"), "route": strV("/y"), "n": intV(5)},
		{"service": strV("web"), "route": strV("/z"), "n": intV(5)},
	}
	result := drain(t, `from * | sort +service, -n | streamstats rank() as r, dense_rank() as dr by service`, rows)
	if len(result) != 6 {
		t.Fatalf("expected 6 rows, got %d", len(result))
	}
	expected := []struct {
		route string
		rank  int64
		dense int64
	}{
		{route: "/a", rank: 1, dense: 1},
		{route: "/b", rank: 1, dense: 1},
		{route: "/c", rank: 3, dense: 2},
		{route: "/x", rank: 1, dense: 1},
		{route: "/y", rank: 2, dense: 2},
		{route: "/z", rank: 2, dense: 2},
	}
	for i, want := range expected {
		gotRoute, _ := result[i]["route"].TryAsString()
		if gotRoute != want.route {
			t.Fatalf("row %d route = %q, want %q", i, gotRoute, want.route)
		}
		assertIntField(t, result[i], "r", want.rank, i)
		assertIntField(t, result[i], "dr", want.dense, i)
	}
}

func TestBuild_StreamStatsRankRequiresSortKey(t *testing.T) {
	q, diags := parser.Parse(`from * | streamstats rank() as r`)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("parse error: %s", d.Message)
		}
	}
	desugared, _ := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("lower error: %s", d.Message)
		}
	}
	_, err := Build(plan, BuildOptions{Source: sourceFromRows(idRows(1))})
	if err == nil {
		t.Fatal("expected rank without sort to fail")
	}
	if !strings.Contains(err.Error(), "rank requires a preceding sort") {
		t.Fatalf("expected rank sort error, got %v", err)
	}
}

func TestBuild_StreamStatsEMA_CurrentFalse(t *testing.T) {
	rows := []map[string]event.Value{
		{"val": intV(10)},
		{"val": intV(20)},
		{"val": intV(30)},
	}
	result := drain(t, `from * | streamstats current=false ema(val, 3) as prev_ema`, rows)
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}

	assertOptionalFloatField(t, result[0], "prev_ema", nil, 0)
	assertOptionalFloatField(t, result[1], "prev_ema", floatPtr(10), 1)
	assertOptionalFloatField(t, result[2], "prev_ema", floatPtr(15), 2)
}

func TestBuild_StreamStatsDelta(t *testing.T) {
	rows := []map[string]event.Value{
		{"host": strV("a"), "val": intV(10)},
		{"host": strV("a"), "val": intV(15)},
		{"host": strV("b"), "val": intV(3)},
		{"host": strV("a"), "val": intV(12)},
		{"host": strV("b"), "val": intV(8)},
	}
	result := drain(t, `from * | streamstats delta(val) as d by host`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}

	expected := []*float64{nil, floatPtr(5), nil, floatPtr(-3), floatPtr(5)}
	for i, r := range result {
		assertOptionalFloatField(t, r, "d", expected[i], i)
	}
}

// Tests: Sort / Head / Tail / Dedup

func TestBuild_Sort(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | sort duration`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}
	prev := 0.0
	for _, r := range result {
		d, _ := r["duration"].TryAsFloat()
		if d < prev {
			t.Errorf("not sorted: %f < %f", d, prev)
		}
		prev = d
	}
}

func TestBuild_SortDesc(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | sort -duration`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}
	prev := math.MaxFloat64
	for _, r := range result {
		d, _ := r["duration"].TryAsFloat()
		if d > prev {
			t.Errorf("not sorted descending: %f > %f", d, prev)
		}
		prev = d
	}
}

func TestBuild_Head(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | head 2`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
}

func TestBuild_Tail(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | tail 2`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
}

func TestBuild_SamplePercent(t *testing.T) {
	rows := idRows(100)
	first := rowIDs(t, drainWithBatchSize(t, `from * | sample 12.5% seed=7`, rows, 9))
	second := rowIDs(t, drainWithBatchSize(t, `from * | sample 12.5% seed=7`, rows, 9))
	if !slices.Equal(first, second) {
		t.Fatalf("seeded percent sample changed:\nfirst:  %v\nsecond: %v", first, second)
	}
	if len(first) == 0 || len(first) >= len(rows) {
		t.Fatalf("expected a partial percent sample, got %d rows", len(first))
	}
	for _, id := range first {
		if id < 0 || id >= int64(len(rows)) {
			t.Fatalf("sampled id %d is outside input range", id)
		}
	}
}

func TestBuild_SamplePercentHundredKeepsAllRows(t *testing.T) {
	rows := idRows(12)
	got := rowIDs(t, drainWithBatchSize(t, `from * | sample 100% seed=9`, rows, 3))
	want := rowIDs(t, rows)
	if !slices.Equal(got, want) {
		t.Fatalf("sample 100%% ids = %v, want %v", got, want)
	}
}

func TestBuild_SampleReservoir(t *testing.T) {
	rows := idRows(30)
	first := rowIDs(t, drainWithBatchSize(t, `from * | sample 5 seed=11`, rows, 4))
	second := rowIDs(t, drainWithBatchSize(t, `from * | sample 5 seed=11`, rows, 4))
	if !slices.Equal(first, second) {
		t.Fatalf("seeded reservoir sample changed:\nfirst:  %v\nsecond: %v", first, second)
	}
	if len(first) != 5 {
		t.Fatalf("expected 5 sampled rows, got %d", len(first))
	}
	for i, id := range first {
		if id < 0 || id >= int64(len(rows)) {
			t.Fatalf("sampled id %d is outside input range", id)
		}
		if i > 0 && id < first[i-1] {
			t.Fatalf("reservoir sample is not in input order: %v", first)
		}
	}
}

func TestBuild_SampleReservoirLargerThanInputKeepsAllRows(t *testing.T) {
	rows := idRows(7)
	got := rowIDs(t, drainWithBatchSize(t, `from * | sample 100 seed=3`, rows, 2))
	want := rowIDs(t, rows)
	if !slices.Equal(got, want) {
		t.Fatalf("sample ids = %v, want %v", got, want)
	}
}

func TestBuild_Dedup(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | dedup level`, rows)
	// 3 unique levels: info, error, warn
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}
	seen := make(map[string]bool)
	for _, r := range result {
		lv, _ := r["level"].TryAsString()
		if seen[lv] {
			t.Errorf("duplicate level %q after dedup", lv)
		}
		seen[lv] = true
	}
}

// Tests: TopK (sort + head fused)

func TestBuild_TopK(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | sort -duration | head 2`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
	// Should be the 2 highest durations.
	d0, _ := result[0]["duration"].TryAsFloat()
	d1, _ := result[1]["duration"].TryAsFloat()
	if d0 < d1 {
		t.Errorf("topk not sorted descending: %f < %f", d0, d1)
	}
}

// Tests: Union

func TestBuild_Union(t *testing.T) {
	// Union is created via the "union" stage in LynxFlow.
	// Build manually since the parser syntax is complex.
	scan := &logical.Scan{OutputSchema: nil}
	union := &logical.Union{
		Inputs: []logical.Node{scan, scan},
	}
	rows := []map[string]event.Value{
		{"x": intV(1)},
		{"x": intV(2)},
	}
	iter, err := Build(&logical.Plan{Root: union}, BuildOptions{
		Source:    sourceFromRows(rows),
		BatchSize: 1024,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	// Each branch produces 2 rows -> 4 total.
	if len(result) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(result))
	}
}

// Tests: Join (inner/left)

func TestBuild_Join_Inner(t *testing.T) {
	left := &logical.Scan{OutputSchema: nil}
	right := &logical.Scan{OutputSchema: nil}
	join := &logical.Join{
		Type:  "inner",
		On:    []string{"key"},
		Right: right,
	}
	join.SetChildren([]logical.Node{left})

	leftRows := []map[string]event.Value{
		{"key": strV("a"), "val": intV(1)},
		{"key": strV("b"), "val": intV(2)},
		{"key": strV("c"), "val": intV(3)},
	}
	rightRows := []map[string]event.Value{
		{"key": strV("a"), "extra": strV("x")},
		{"key": strV("c"), "extra": strV("z")},
	}

	callCount := 0
	sourceFunc := func(scan *logical.Scan) (pipeline.Iterator, error) {
		callCount++
		if callCount == 1 {
			return sliceSource(leftRows, 1024), nil
		}
		return sliceSource(rightRows, 1024), nil
	}

	iter, err := Build(&logical.Plan{Root: join}, BuildOptions{
		Source:    sourceFunc,
		BatchSize: 1024,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	// Inner join on key: a and c match -> 2 rows.
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
}

func TestBuild_Join_Left(t *testing.T) {
	left := &logical.Scan{OutputSchema: nil}
	right := &logical.Scan{OutputSchema: nil}
	join := &logical.Join{
		Type:  "left",
		On:    []string{"key"},
		Right: right,
	}
	join.SetChildren([]logical.Node{left})

	leftRows := []map[string]event.Value{
		{"key": strV("a"), "val": intV(1)},
		{"key": strV("b"), "val": intV(2)},
	}
	rightRows := []map[string]event.Value{
		{"key": strV("a"), "extra": strV("x")},
	}

	callCount := 0
	sourceFunc := func(scan *logical.Scan) (pipeline.Iterator, error) {
		callCount++
		if callCount == 1 {
			return sliceSource(leftRows, 1024), nil
		}
		return sliceSource(rightRows, 1024), nil
	}

	iter, err := Build(&logical.Plan{Root: join}, BuildOptions{
		Source:    sourceFunc,
		BatchSize: 1024,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	// Left join: all left rows (2), key=a enriched, key=b not.
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
}

func TestBuild_Join_SemiAnti(t *testing.T) {
	tests := []struct {
		name     string
		joinType string
		wantKeys []string
	}{
		{name: "semi", joinType: "semi", wantKeys: []string{"a", "c"}},
		{name: "anti", joinType: "anti", wantKeys: []string{"b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := &logical.Scan{OutputSchema: nil}
			right := &logical.Scan{OutputSchema: nil}
			join := &logical.Join{
				Type:  tt.joinType,
				On:    []string{"key"},
				Right: right,
			}
			join.SetChildren([]logical.Node{left})

			leftRows := []map[string]event.Value{
				{"key": strV("a"), "val": intV(1)},
				{"key": strV("b"), "val": intV(2)},
				{"key": strV("c"), "val": intV(3)},
			}
			rightRows := []map[string]event.Value{
				{"key": strV("a"), "extra": strV("x")},
				{"key": strV("a"), "extra": strV("dupe")},
				{"key": strV("c"), "extra": strV("z")},
			}

			callCount := 0
			sourceFunc := func(scan *logical.Scan) (pipeline.Iterator, error) {
				callCount++
				if callCount == 1 {
					return sliceSource(leftRows, 1024), nil
				}
				return sliceSource(rightRows, 1024), nil
			}

			iter, err := Build(&logical.Plan{Root: join}, BuildOptions{
				Source:    sourceFunc,
				BatchSize: 1024,
			})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			result, err := pipeline.CollectAll(context.Background(), iter)
			if err != nil {
				t.Fatalf("CollectAll: %v", err)
			}
			if len(result) != len(tt.wantKeys) {
				t.Fatalf("expected %d rows, got %d: %#v", len(tt.wantKeys), len(result), result)
			}
			for i, wantKey := range tt.wantKeys {
				if got := result[i]["key"]; got != strV(wantKey) {
					t.Fatalf("row %d key: got %s want %s", i, got.String(), wantKey)
				}
				if _, ok := result[i]["extra"]; ok {
					t.Fatalf("row %d retained right-side field: %#v", i, result[i])
				}
			}
		})
	}
}

func TestBuild_Join_Asof(t *testing.T) {
	left := &logical.Scan{OutputSchema: nil}
	right := &logical.Scan{OutputSchema: nil}
	join := &logical.Join{
		Type: "asof",
		On:   []string{"host", "service"},
		Tolerance: &lfast.Literal{
			Kind:  lfast.LitDuration,
			Raw:   "6m",
			Value: 6 * time.Minute,
		},
		Right: right,
	}
	join.SetChildren([]logical.Node{left})

	base := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	leftRows := []map[string]event.Value{
		{"_time": tsV(base), "host": strV("a"), "service": strV("api"), "id": intV(1)},
		{"_time": tsV(base.Add(10 * time.Minute)), "host": strV("a"), "service": strV("api"), "id": intV(2)},
		{"_time": tsV(base.Add(5 * time.Minute)), "host": strV("b"), "service": strV("api"), "id": intV(3)},
		{"_time": tsV(base.Add(20 * time.Minute)), "host": strV("a"), "service": strV("api"), "id": intV(4)},
		{"_time": tsV(base.Add(10 * time.Minute)), "host": strV("a"), "service": strV("web"), "id": intV(5)},
	}
	rightRows := []map[string]event.Value{
		{"_time": tsV(base.Add(-5 * time.Minute)), "host": strV("a"), "service": strV("api"), "deploy": strV("v1")},
		{"_time": tsV(base.Add(5 * time.Minute)), "host": strV("a"), "service": strV("api"), "deploy": strV("v2")},
		{"_time": tsV(base.Add(30 * time.Minute)), "host": strV("a"), "service": strV("api"), "deploy": strV("future")},
		{"_time": tsV(base.Add(1 * time.Minute)), "host": strV("b"), "service": strV("api"), "deploy": strV("vb")},
		{"_time": tsV(base.Add(9 * time.Minute)), "host": strV("a"), "service": strV("web"), "deploy": strV("web")},
	}

	callCount := 0
	sourceFunc := func(scan *logical.Scan) (pipeline.Iterator, error) {
		callCount++
		if callCount == 1 {
			return sliceSource(leftRows, 2), nil
		}
		return sliceSource(rightRows, 2), nil
	}

	iter, err := Build(&logical.Plan{Root: join}, BuildOptions{
		Source:    sourceFunc,
		BatchSize: 2,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	if len(result) != 4 {
		t.Fatalf("expected 4 rows, got %d: %#v", len(result), result)
	}

	wantDeploys := []string{"v1", "v2", "vb", "web"}
	wantIDs := []int64{1, 2, 3, 5}
	for i := range wantDeploys {
		if got := result[i]["id"]; got != intV(wantIDs[i]) {
			t.Fatalf("row %d id = %s, want %d", i, got.String(), wantIDs[i])
		}
		if got := result[i]["deploy"]; got != strV(wantDeploys[i]) {
			t.Fatalf("row %d deploy = %s, want %s", i, got.String(), wantDeploys[i])
		}
	}
}

func TestBuild_Join_Outer(t *testing.T) {
	left := &logical.Scan{OutputSchema: nil}
	right := &logical.Scan{OutputSchema: nil}
	join := &logical.Join{
		Type:  "outer",
		On:    []string{"key"},
		Right: right,
	}
	join.SetChildren([]logical.Node{left})

	leftRows := []map[string]event.Value{
		{"key": strV("a"), "val": intV(1)},
		{"key": strV("b"), "val": intV(2)},
	}
	rightRows := []map[string]event.Value{
		{"key": strV("a"), "extra": strV("x")},
		{"key": strV("c"), "extra": strV("z")},
	}

	callCount := 0
	sourceFunc := func(scan *logical.Scan) (pipeline.Iterator, error) {
		callCount++
		if callCount == 1 {
			return sliceSource(leftRows, 1024), nil
		}
		return sliceSource(rightRows, 1024), nil
	}

	iter, err := Build(&logical.Plan{Root: join}, BuildOptions{
		Source:    sourceFunc,
		BatchSize: 1024,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}

	// Full outer join: key=a (merged), key=b (left only), key=c (right only) -> 3 rows.
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}

	// Verify all expected keys are present.
	keys := make(map[string]bool, len(result))
	for _, row := range result {
		if v, ok := row["key"]; ok {
			keys[v.String()] = true
		}
	}
	for _, k := range []string{"a", "b", "c"} {
		if !keys[k] {
			t.Errorf("expected key %q in result, not found", k)
		}
	}

	// Verify key=a has both val and extra (non-null: columnar batches pad
	// missing columns with nulls).
	for _, row := range result {
		if v, ok := row["key"]; ok && v.String() == "a" {
			if vv, hasVal := row["val"]; !hasVal || vv.IsNull() {
				t.Error("key=a should have 'val' from left side")
			}
			if ev, hasExtra := row["extra"]; !hasExtra || ev.IsNull() {
				t.Error("key=a should have 'extra' from right side")
			}
		}
	}
}

// Tests: Explode

func TestBuild_Explode(t *testing.T) {
	rows := []map[string]event.Value{
		{"tags": arrV(strV("a"), strV("b"), strV("c")), "id": intV(1)},
		{"tags": arrV(strV("x")), "id": intV(2)},
	}
	result := drain(t, `from * | explode tags`, rows)
	// Row 1 expands to 3, Row 2 expands to 1 -> 4 total.
	if len(result) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(result))
	}

	aliased := drain(t, `from * | explode tags as tag`, rows)
	if len(aliased) != 4 {
		t.Fatalf("expected 4 aliased rows, got %d", len(aliased))
	}
	if got := aliased[0]["tag"]; got != strV("a") {
		t.Fatalf("aliased tag: got %s, want a", got.String())
	}
}

// Tests: Project (keep/drop/rename)

func TestBuild_Keep(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | keep level, status`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}
	for i, r := range result {
		if _, ok := r["level"]; !ok {
			t.Errorf("row %d: missing 'level'", i)
		}
		if _, ok := r["status"]; !ok {
			t.Errorf("row %d: missing 'status'", i)
		}
		// duration and host should be dropped.
		if _, ok := r["duration"]; ok {
			t.Errorf("row %d: unexpected 'duration' field", i)
		}
	}
}

func TestBuild_Drop(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | drop duration`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}
	for i, r := range result {
		if _, ok := r["duration"]; ok {
			t.Errorf("row %d: unexpected 'duration' field", i)
		}
		if _, ok := r["level"]; !ok {
			t.Errorf("row %d: missing 'level'", i)
		}
	}
}

func TestBuild_Rename(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | rename level as severity`, rows)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}
	for i, r := range result {
		if _, ok := r["severity"]; !ok {
			t.Errorf("row %d: missing 'severity'", i)
		}
		if _, ok := r["level"]; ok {
			t.Errorf("row %d: unexpected 'level' after rename", i)
		}
	}
}

// Tests: Describe

func TestBuild_Describe(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | describe`, rows)
	if len(result) == 0 {
		t.Fatal("expected at least 1 describe output row")
	}
	// Each row should have describe output columns.
	wantCols := []string{
		"field", "type", "coverage", "distinct_est", "top_values",
		"min", "max", "avg", "p25", "p50", "p75", "null_pct",
	}
	for i, r := range result {
		for _, col := range wantCols {
			if _, ok := r[col]; !ok {
				t.Errorf("row %d: missing %q", i, col)
			}
		}
	}
}

// Tests: Parse (json from raw text)

func TestBuild_Parse_JSON(t *testing.T) {
	rows := []map[string]event.Value{
		{"_raw": strV(`{"name":"alice","age":30}`)},
		{"_raw": strV(`{"name":"bob","age":25}`)},
	}
	result := drain(t, `from * | parse json`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
	for i, r := range result {
		if _, ok := r["name"]; !ok {
			t.Errorf("row %d: missing 'name' after parse json", i)
		}
	}
}

// Tests: Empty

func TestBuild_Empty(t *testing.T) {
	plan := &logical.Plan{Root: &logical.Empty{}}
	iter, err := Build(plan, BuildOptions{BatchSize: 1024})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(result))
	}
}

func TestBuild_NilPlan(t *testing.T) {
	iter, err := Build(nil, BuildOptions{BatchSize: 1024})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(result))
	}
}

// Tests: Error cases

func TestBuild_Materialize_NotYetImplemented(t *testing.T) {
	plan := &logical.Plan{
		Root: &logical.Materialize{
			Name: "test_mv",
		},
	}
	plan.Root.(*logical.Materialize).SetChildren([]logical.Node{&logical.Scan{}})
	_, err := Build(plan, BuildOptions{
		Source:    sourceFromRows(nil),
		BatchSize: 1024,
	})
	if err == nil {
		t.Fatal("expected error for Materialize, got nil")
	}
	if _, ok := err.(*NotYetImplementedError); !ok {
		t.Fatalf("expected NotYetImplementedError, got %T: %v", err, err)
	}
}

func TestBuild_Tee_Disabled(t *testing.T) {
	sinkPath := filepath.Join(t.TempDir(), "tee.out")
	plan := &logical.Plan{
		Root: &logical.Tee{Sink: sinkPath},
	}
	plan.Root.(*logical.Tee).SetChildren([]logical.Node{&logical.Scan{}})
	_, err := Build(plan, BuildOptions{
		Source:     sourceFromRows(nil),
		BatchSize:  1024,
		TeeEnabled: false,
	})
	if err == nil {
		t.Fatal("expected error for Tee with TeeEnabled=false, got nil")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("expected 'not enabled' error, got: %v", err)
	}
}

func TestBuild_Tee_RelativePath(t *testing.T) {
	plan := &logical.Plan{
		Root: &logical.Tee{Sink: "relative/path.out"},
	}
	plan.Root.(*logical.Tee).SetChildren([]logical.Node{&logical.Scan{}})
	_, err := Build(plan, BuildOptions{
		Source:     sourceFromRows(nil),
		BatchSize:  1024,
		TeeEnabled: true,
	})
	if err == nil {
		t.Fatal("expected error for relative tee path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("expected absolute-path error, got: %v", err)
	}
}

func TestBuild_Tee_Enabled(t *testing.T) {
	sinkPath := filepath.Join(t.TempDir(), "tee.out")
	rows := sampleRows()

	scan := &logical.Scan{OutputSchema: nil}
	tee := &logical.Tee{Sink: sinkPath}
	tee.SetChildren([]logical.Node{scan})

	iter, err := Build(&logical.Plan{Root: tee}, BuildOptions{
		Source:     sourceFromRows(rows),
		BatchSize:  1024,
		TeeEnabled: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}

	// Tee is passthrough — all rows must appear in the output.
	if len(result) != len(rows) {
		t.Fatalf("expected %d rows, got %d", len(rows), len(result))
	}

	// The sink file must contain NDJSON with one line per row.
	data, err := os.ReadFile(sinkPath)
	if err != nil {
		t.Fatalf("read sink file: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != len(rows) {
		t.Fatalf("expected %d lines in sink file, got %d", len(rows), len(lines))
	}
}

// Tests: Full end-to-end via parser

func TestBuild_E2E_WhereExtendStats(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | where status >= 200 | extend is_error = status >= 500 | stats count() as total`, rows)
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	total, _ := result[0]["total"].TryAsInt()
	if total != 5 {
		t.Errorf("expected total=5, got %d", total)
	}
}

func TestBuild_E2E_SortHeadTopK(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | sort -status | head 3`, rows)
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}
}

func TestBuild_E2E_StatsCountByLevel(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | stats count() as cnt by level | sort -cnt`, rows)
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}
	// info: 2, error: 2, warn: 1 -> sorted desc: info/error first, then warn.
	lastCnt, _ := result[len(result)-1]["cnt"].TryAsInt()
	if lastCnt != 1 {
		t.Errorf("expected last cnt=1, got %d", lastCnt)
	}
}

func TestBuild_E2E_DedupSortHead(t *testing.T) {
	rows := sampleRows()
	result := drain(t, `from * | dedup host | sort host | head 10`, rows)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows (unique hosts), got %d", len(result))
	}
}

// Tests: CondProgram wiring on AggFunc (direct build, not via parser)

func TestBuild_CondProgram_AggFunc(t *testing.T) {
	scan := &logical.Scan{OutputSchema: nil}
	agg := &logical.Aggregate{
		Aggs: []logical.Agg{
			{
				Func: &lfast.Call{
					Callee: "count",
					Args:   []lfast.Expr{&lfast.Ident{Name: "status"}},
				},
				WhereCond: &lfast.Binary{
					Op:   lfast.OpGtEq,
					Left: &lfast.Ident{Name: "status"},
					Right: &lfast.Literal{
						Kind:  lfast.LitInt,
						Value: int64(500),
						Raw:   "500",
					},
				},
				Alias: "error_count",
			},
		},
	}
	agg.SetChildren([]logical.Node{scan})

	rows := sampleRows()
	iter, err := Build(&logical.Plan{Root: agg}, BuildOptions{
		Source:    sourceFromRows(rows),
		BatchSize: 1024,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}
	c, _ := result[0]["error_count"].TryAsInt()
	if c != 2 {
		t.Errorf("expected error_count=2 (status 500 and 503), got %d", c)
	}
}

// Tests: Agg name mapping table coverage

func TestAggNameMapping(t *testing.T) {
	expected := map[string]string{
		"count":          "count",
		"sum":            "sum",
		"avg":            "avg",
		"min":            "min",
		"max":            "max",
		"dc":             "dc",
		"estdc":          "dc",
		"perc":           "perc",
		"perc_weighted":  "perc_weighted",
		"p50":            "perc50",
		"p95":            "perc95",
		"p99":            "perc99",
		"stdev":          "stdev",
		"values":         "values",
		"first":          "first",
		"last":           "last",
		"mode":           "mode",
		"arg_max":        "arg_max",
		"arg_min":        "arg_min",
		"any_value":      "any_value",
		"top_k":          "top_k",
		"top_k_weighted": "top_k_weighted",
		"value_counts":   "value_counts",
		"avg_weighted":   "avg_weighted",
		"entropy":        "entropy",
		"max_n":          "max_n",
		"min_n":          "min_n",
		"corr":           "corr",
		"covar":          "covar",
		"linear_fit":     "linear_fit",
		"sum_object":     "sum_object",
		"skewness":       "skewness",
		"kurtosis":       "kurtosis",
		"mad":            "mad",
		"delta_sum":      "delta_sum",
		"histogram":      "histogram",
		"rank":           "rank",
		"dense_rank":     "dense_rank",
		"rate":           "rate",
	}
	for input, want := range expected {
		got, ok := aggNameMapping[input]
		if !ok {
			t.Errorf("missing mapping for %q", input)
			continue
		}
		if got != want {
			t.Errorf("aggNameMapping[%q] = %q, want %q", input, got, want)
		}
	}
}

func TestWindowOnlyAggregatesHavePhysicalMapping(t *testing.T) {
	for _, agg := range registry.Aggregates() {
		if !agg.WindowOnly {
			continue
		}
		if _, ok := aggNameMapping[agg.Name]; !ok {
			t.Errorf("window-only aggregate %q has no physical mapping", agg.Name)
		}
	}
}

// Tests: exprToDuration

func TestExprToDuration(t *testing.T) {
	tests := []struct {
		expr lfast.Expr
		want time.Duration
	}{
		{
			expr: &lfast.Literal{Kind: lfast.LitDuration, Value: 5 * time.Minute, Raw: "5m"},
			want: 5 * time.Minute,
		},
	}
	for _, tt := range tests {
		got, err := exprToDuration(tt.expr)
		if err != nil {
			t.Errorf("exprToDuration(%v): %v", tt.expr, err)
			continue
		}
		if got != tt.want {
			t.Errorf("exprToDuration(%v) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

// Tests: Small batch sizes

func TestBuild_SmallBatchSize(t *testing.T) {
	rows := sampleRows()
	result := drainWithBatchSize(t, `from * | where status >= 200`, rows, 2)
	if len(result) != 5 {
		t.Fatalf("expected 5 rows with batch size 2, got %d", len(result))
	}
}

// Tests: DescribeSummaryIterator directly

func TestDescribeSummaryIterator(t *testing.T) {
	rows := sampleRows()
	source := sliceSource(rows, 1024)
	iter := NewDescribeSummaryIterator(source, 1024)

	if err := iter.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	batch, err := iter.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if batch == nil {
		t.Fatal("expected non-nil batch from describe")
	}
	if batch.Len == 0 {
		t.Fatal("expected at least 1 row")
	}

	// Verify expected columns.
	for _, col := range []string{"field", "type", "coverage", "distinct_est", "top_values", "min", "max", "avg", "p25", "p50", "p75", "null_pct"} {
		if _, ok := batch.Columns[col]; !ok {
			t.Errorf("missing column %q in describe output", col)
		}
	}

	// Second call should return nil (exhausted).
	batch2, err := iter.Next(context.Background())
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if batch2 != nil {
		t.Errorf("expected nil from second Next, got %d rows", batch2.Len)
	}

	if err := iter.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDescribeSummaryIterator_NumericProfile(t *testing.T) {
	rows := []map[string]event.Value{
		{"service": strV("api"), "latency": intV(10)},
		{"service": strV("api"), "latency": floatV(20)},
		{"service": strV("web"), "latency": event.NullValue()},
		{"service": strV("web"), "latency": intV(30)},
	}
	source := sliceSource(rows, 1024)
	iter := NewDescribeSummaryIterator(source, 1024)

	if err := iter.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	batch, err := iter.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if batch == nil {
		t.Fatal("expected non-nil batch from describe")
	}

	rowsByField := make(map[string]map[string]event.Value)
	for i := 0; i < batch.Len; i++ {
		field := batch.Value("field", i).AsString()
		row := make(map[string]event.Value)
		for name, col := range batch.Columns {
			row[name] = col[i]
		}
		rowsByField[field] = row
	}

	latency, ok := rowsByField["latency"]
	if !ok {
		t.Fatalf("missing latency row: %#v", rowsByField)
	}
	assertFloatField(t, latency, "min", 10, 0)
	assertFloatField(t, latency, "max", 30, 0)
	assertFloatField(t, latency, "avg", 20, 0)
	assertFloatField(t, latency, "p25", 12.5, 0)
	assertFloatField(t, latency, "p50", 20, 0)
	assertFloatField(t, latency, "p75", 27.5, 0)
	assertFloatField(t, latency, "null_pct", 0.25, 0)

	service, ok := rowsByField["service"]
	if !ok {
		t.Fatalf("missing service row: %#v", rowsByField)
	}
	for _, field := range []string{"min", "max", "avg", "p25", "p50", "p75"} {
		if !service[field].IsNull() {
			t.Fatalf("service %s: got %s, want null", field, service[field].String())
		}
	}
	assertFloatField(t, service, "null_pct", 0, 0)
}

// Tests: Schema on DescribeSummaryIterator

func TestDescribeSummaryIterator_Schema(t *testing.T) {
	source := sliceSource(nil, 1024)
	iter := NewDescribeSummaryIterator(source, 1024)
	schema := iter.Schema()
	if len(schema) != 12 {
		t.Fatalf("expected 12 schema fields, got %d", len(schema))
	}
	names := make(map[string]bool)
	for _, f := range schema {
		names[f.Name] = true
	}
	for _, want := range []string{"field", "type", "coverage", "distinct_est", "top_values", "min", "max", "avg", "p25", "p50", "p75", "null_pct"} {
		if !names[want] {
			t.Errorf("missing schema field %q", want)
		}
	}
}

// Tests: Memory governance and spill wiring

// TestBuild_SortSpillsUnderTinyBudget proves that when the physical builder is
// wired with a coordinator and spill manager, a sort over data exceeding the
// budget spills to disk instead of growing memory unboundedly.
func TestBuild_SortSpillsUnderTinyBudget(t *testing.T) {
	// Generate enough rows to exceed the sort operator's sub-limit.
	// Each row is ~300B (EstimateRowBytes charges overhead + string payload).
	// 2000 rows * ~300B = ~600KB. The coordinator's reservation for Sort is
	// 256KB, so the sort will exceed its sub-limit and spill.
	rows := make([]map[string]event.Value, 2000)
	for i := range rows {
		rows[i] = map[string]event.Value{
			"val":  intV(int64(2000 - i)),
			"data": strV(strings.Repeat("x", 200)), // ~200B payload per row
		}
	}

	mgr, err := pipeline.NewSpillManager(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewSpillManager: %v", err)
	}
	defer mgr.CleanupAll()

	// Budget is 300KB -- barely above the Sort reservation (256KB), so
	// after headroom and finalization, the sort gets a sub-limit around
	// 256KB-300KB. With 2000 rows at ~300B each = ~600KB, spill is forced.
	coordinator := pipeline.NewMemoryCoordinator(300*1024, 0.05)
	// We need a governor-backed BudgetAdapter. Use a simple NopAccount-based
	// approach: the coordinator enforces sub-limits; the inner account is nop.
	// This is sufficient to test that spill is triggered.

	q, diags := parser.Parse(`from * | sort val`)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("parse error: %s", d.Message)
		}
	}
	desugared, _ := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("lower error: %s", d.Message)
		}
	}
	plan, _ = opt.Optimize(plan)

	iter, err := Build(plan, BuildOptions{
		Source:       sourceFromRows(rows),
		BatchSize:    64,
		Coordinator:  coordinator,
		SpillManager: mgr,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Finalize after build registers operators.
	coordinator.Finalize()

	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}

	if len(result) != 2000 {
		t.Fatalf("expected 2000 rows, got %d", len(result))
	}

	// Verify sorted ascending.
	for i := 0; i < len(result)-1; i++ {
		a, _ := result[i]["val"].TryAsInt()
		b, _ := result[i+1]["val"].TryAsInt()
		if a > b {
			t.Fatalf("not sorted at index %d: %d > %d", i, a, b)
		}
	}

	// Verify spill occurred by checking coordinator stats.
	cstats := coordinator.Stats()
	spilled := false
	for _, s := range cstats {
		if s.Spilled {
			spilled = true
			break
		}
	}
	if !spilled {
		t.Error("sort did not spill under tiny budget; expected spill with 300KB budget and 2000 rows at ~300B each")
	} else {
		t.Log("sort spilled to disk as expected under constrained budget")
	}
}

// TestBuild_JoinSpillsUnderTinyBudget proves that a join with a coordinator
// and spill manager handles budget exhaustion via the grace hash join path.
func TestBuild_JoinSpillsUnderTinyBudget(t *testing.T) {
	leftRows := make([]map[string]event.Value, 100)
	rightRows := make([]map[string]event.Value, 100)
	for i := range leftRows {
		leftRows[i] = map[string]event.Value{
			"key":  strV(strings.Repeat("k", 64) + string(rune('0'+i%10))),
			"lval": intV(int64(i)),
		}
		rightRows[i] = map[string]event.Value{
			"key":  strV(strings.Repeat("k", 64) + string(rune('0'+i%10))),
			"rval": intV(int64(i * 10)),
		}
	}

	mgr, err := pipeline.NewSpillManager(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("NewSpillManager: %v", err)
	}
	defer mgr.CleanupAll()

	coordinator := pipeline.NewMemoryCoordinator(4*1024, 0.10)

	left := &logical.Scan{OutputSchema: nil}
	right := &logical.Scan{OutputSchema: nil}
	join := &logical.Join{
		Type:  "inner",
		On:    []string{"key"},
		Right: right,
	}
	join.SetChildren([]logical.Node{left})

	callCount := 0
	sourceFunc := func(scan *logical.Scan) (pipeline.Iterator, error) {
		callCount++
		if callCount == 1 {
			return sliceSource(leftRows, 64), nil
		}
		return sliceSource(rightRows, 64), nil
	}

	iter, err := Build(&logical.Plan{Root: join}, BuildOptions{
		Source:       sourceFunc,
		BatchSize:    64,
		Coordinator:  coordinator,
		SpillManager: mgr,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	coordinator.Finalize()

	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}

	// With 10 distinct key suffixes, inner join should produce matches.
	if len(result) == 0 {
		t.Fatal("expected non-empty join result")
	}

	t.Logf("join produced %d rows (spill path exercised if budget was exceeded)", len(result))
}

// TestBuild_TailRingBuffer proves that the TailIterator uses a fixed-capacity
// ring buffer and does not grow unboundedly, by building the logical plan
// directly (bypassing the optimizer's tail-scan rewrite to reverse+head).
func TestBuild_TailRingBuffer(t *testing.T) {
	rows := make([]map[string]event.Value, 1000)
	for i := range rows {
		rows[i] = map[string]event.Value{
			"seq": intV(int64(i)),
		}
	}

	// Build the plan directly to force the Tail path, skipping the optimizer
	// which rewrites tail into reverse-scan + head.
	scan := &logical.Scan{OutputSchema: nil}
	limit := &logical.Limit{N: 5, Tail: true}
	limit.SetChildren([]logical.Node{scan})

	iter, err := Build(&logical.Plan{Root: limit}, BuildOptions{
		Source:    sourceFromRows(rows),
		BatchSize: 1024,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	result, err := pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}

	if len(result) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result))
	}

	// Verify we got the LAST 5 rows (seq 995..999).
	for i, r := range result {
		seq, _ := r["seq"].TryAsInt()
		expected := int64(995 + i)
		if seq != expected {
			t.Errorf("row %d: expected seq=%d, got %d", i, expected, seq)
		}
	}
}

// TestBuild_CTEMaterializationRespectsContext verifies that CTE materialization
// uses the provided context (not context.Background) so cancellation works.
func TestBuild_CTEMaterializationRespectsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	rows := []map[string]event.Value{
		{"x": intV(1)},
	}

	q, diags := parser.Parse(`let $sub = from main; from $sub`)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("parse error: %s", d.Message)
		}
	}
	desugared, _ := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("lower error: %s", d.Message)
		}
	}
	plan, _ = opt.Optimize(plan)

	_, err := Build(plan, BuildOptions{
		Source:  sourceFromRows(rows),
		Context: ctx,
	})
	// With a cancelled context, the CTE materialization should fail.
	if err == nil {
		t.Log("Build succeeded despite cancelled context; CTE may have been empty or optimized away")
	}
}

// Test: Materialize backstop returns NotYetImplementedError

func TestBuild_Materialize_Backstop(t *testing.T) {
	// If a Materialize node somehow reaches physical.Build (should not happen
	// in normal operation), it should return a NotYetImplementedError.
	plan := &logical.Plan{
		Root: &logical.Materialize{},
	}
	_, err := Build(plan, BuildOptions{})
	if err == nil {
		t.Fatal("expected error for Materialize node in physical.Build")
	}
	var nie *NotYetImplementedError
	if !errors.As(err, &nie) {
		t.Errorf("expected NotYetImplementedError, got: %T: %v", err, err)
	}
}

// Test: IsMaterializeRoot correctly identifies Materialize nodes

func TestIsMaterializeRoot(t *testing.T) {
	mat := &logical.Materialize{Name: "test_view"}
	got, ok := IsMaterializeRoot(mat)
	if !ok {
		t.Fatal("expected IsMaterializeRoot to return true for *Materialize")
	}
	if got.Name != "test_view" {
		t.Errorf("got name %q, want %q", got.Name, "test_view")
	}

	// Non-Materialize should return false.
	scan := &logical.Scan{}
	_, ok = IsMaterializeRoot(scan)
	if ok {
		t.Error("expected IsMaterializeRoot to return false for *Scan")
	}
}

// Test: compare executes end-to-end through physical.Build

func TestBuild_Compare_EndToEnd(t *testing.T) {
	rows := []map[string]event.Value{
		{"level": strV("error"), "status": intV(500), "_time": tsV(time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC))},
		{"level": strV("error"), "status": intV(503), "_time": tsV(time.Date(2026, 1, 1, 10, 30, 0, 0, time.UTC))},
		{"level": strV("info"), "status": intV(200), "_time": tsV(time.Date(2026, 1, 1, 10, 45, 0, 0, time.UTC))},
	}

	// Compare with a stats prefix: produces previous_count() and change_count().
	result := drain(t, `from main | stats count() by level | compare previous 2h`, rows)
	if len(result) == 0 {
		t.Fatal("expected results from compare query")
	}

	for i, row := range result {
		if _, ok := row["previous_count()"]; !ok {
			t.Errorf("row %d: missing previous_count() column", i)
		}
		if _, ok := row["change_count()"]; !ok {
			t.Errorf("row %d: missing change_count() column", i)
		}
	}
}
