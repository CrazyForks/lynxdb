package explain_test

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lynxbase/lynxdb/pkg/engine/pipeline"
	"github.com/lynxbase/lynxdb/pkg/event"
	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/explain"
	"github.com/lynxbase/lynxdb/pkg/logical/opt"
	"github.com/lynxbase/lynxdb/pkg/logical/physical"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
)

var update = flag.Bool("update", false, "update golden files")

const goldenDir = "../testdata/golden/explain"

// Golden EXPLAIN tests — 8 representative queries

var explainCases = []struct {
	Name  string
	Query string
}{
	{
		Name:  "simple_search",
		Query: `from nginx[-1h] timeout`,
	},
	{
		Name:  "stats_by_host",
		Query: `from nginx[-1h] timeout | stats count() by host`,
	},
	{
		Name:  "filter_aggregate_topk",
		Query: `from app | where status >= 500 | stats count() by uri | sort -count() | head 10`,
	},
	{
		Name:  "parse_json_filter",
		Query: `from app | parse json | where level == "error"`,
	},
	{
		Name:  "dedup_limit",
		Query: `from app | dedup host | head 5`,
	},
	{
		Name:  "join_inner",
		Query: `let $right = from users | keep user_id, name; from events | join on user_id with $right`,
	},
	{
		Name:  "union_two",
		Query: `from app | union [from nginx]`,
	},
	{
		Name:  "sort_tail",
		Query: `from app | sort -_time | tail 20`,
	},
}

func TestExplainGolden(t *testing.T) {
	for _, tc := range explainCases {
		t.Run(tc.Name, func(t *testing.T) {
			plan, info := preparePlan(t, tc.Query)
			got := explain.Render(plan, info, nil)

			golden := filepath.Join(goldenDir, tc.Name+".txt")
			if *update {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("golden file not found (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("EXPLAIN mismatch for %q\n--- want ---\n%s--- got ---\n%s",
					tc.Query, string(want), got)
			}
		})
	}
}

// ANALYZE test — verify per-node row counts

func TestAnalyze_RowCounts(t *testing.T) {
	// Build a 3-stage query: scan 10 rows -> filter ~5 rows -> limit 3
	query := `from main | where status >= 500 | head 3`

	rows := makeStatusRows(10)

	plan, info := preparePlanFromQuery(t, query)

	source := physical.NewStorageSourceFromMap(
		map[string][]*event.Event{"main": rows}, "main",
	)
	collect := make(map[logical.Node]*explain.NodeStats)

	iter, err := physical.Build(plan, physical.BuildOptions{
		Source:    source,
		BatchSize: 1024,
		Now:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Collect:   collect,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	_, err = pipeline.CollectAll(context.Background(), iter)
	if err != nil {
		t.Fatalf("CollectAll: %v", err)
	}

	// Verify stats were collected.
	if len(collect) == 0 {
		t.Fatal("no stats collected")
	}

	// Find the Scan node stats — should have processed all 10 rows.
	var scanStats *explain.NodeStats
	var filterStats *explain.NodeStats
	var limitStats *explain.NodeStats
	for node, ns := range collect {
		switch node.(type) {
		case *logical.Scan:
			scanStats = ns
		case *logical.Filter:
			filterStats = ns
		case *logical.Limit:
			limitStats = ns
		}
	}

	if scanStats == nil {
		t.Fatal("no Scan stats found")
	}
	if scanStats.Rows != 10 {
		t.Errorf("Scan rows = %d, want 10", scanStats.Rows)
	}

	if filterStats == nil {
		t.Fatal("no Filter stats found")
	}
	// 5 out of 10 rows have status >= 500
	if filterStats.Rows != 5 {
		t.Errorf("Filter rows = %d, want 5", filterStats.Rows)
	}

	if limitStats == nil {
		t.Fatal("no Limit stats found")
	}
	if limitStats.Rows != 3 {
		t.Errorf("Limit rows = %d, want 3", limitStats.Rows)
	}

	// Verify the rendered EXPLAIN ANALYZE contains stats.
	rendered := explain.Render(plan, info, collect)
	if !strings.Contains(rendered, "rows=") {
		t.Errorf("EXPLAIN ANALYZE output missing rows= annotations:\n%s", rendered)
	}
}

// Render determinism — two renders must be identical

func TestRenderDeterminism(t *testing.T) {
	query := `from nginx[-1h] timeout | stats count() by host`
	plan, info := preparePlan(t, query)

	r1 := explain.Render(plan, info, nil)
	r2 := explain.Render(plan, info, nil)
	if r1 != r2 {
		t.Errorf("non-deterministic render:\n--- first ---\n%s--- second ---\n%s", r1, r2)
	}
}

// Nil/empty plan safety

func TestRenderNilPlan(t *testing.T) {
	result := explain.Render(nil, explain.Info{}, nil)
	if result != "(empty plan)\n" {
		t.Errorf("nil plan render = %q, want %q", result, "(empty plan)\n")
	}
}

func TestRenderEmptyRoot(t *testing.T) {
	plan := &logical.Plan{}
	result := explain.Render(plan, explain.Info{}, nil)
	// Should handle nil root gracefully.
	if !strings.Contains(result, "nil") {
		t.Errorf("empty root render should contain nil: %q", result)
	}
}

// Helpers

func preparePlan(t *testing.T, query string) (*logical.Plan, explain.Info) {
	t.Helper()
	return preparePlanFromQuery(t, query)
}

func preparePlanFromQuery(t *testing.T, query string) (*logical.Plan, explain.Info) {
	t.Helper()
	q, diags := parser.Parse(query)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("parse error: %s", d.Message)
		}
	}

	desugared, rewrites := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})

	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("lower error: %s", d.Message)
		}
	}

	plan, applied := opt.Optimize(plan)

	return plan, explain.Info{
		Rewrites: rewrites,
		Applied:  applied,
	}
}

// makeStatusRows generates N rows with alternating status 200/500.
func makeStatusRows(n int) []*event.Event {
	events := make([]*event.Event, n)
	for i := 0; i < n; i++ {
		status := int64(200)
		if i%2 == 0 {
			status = 500
		}
		ev := &event.Event{
			Time:   time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
			Raw:    "test log line",
			Source: "main",
			Fields: map[string]event.Value{
				"status": event.IntValue(status),
				"host":   event.StringValue("web-01"),
			},
		}
		events[i] = ev
	}
	return events
}
