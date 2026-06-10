package query

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/opt"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/spl2"
)

// parseLowerOptIR is the standard LynxFlow pipeline: parse -> desugar -> lower -> optimize.
func parseLowerOptIR(t *testing.T, query string) *logical.Plan {
	t.Helper()
	q, diags := parser.Parse(query)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("parse error for %q: %s", query, d.Message)
		}
	}
	desugared, _ := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("lower error for %q: %s", query, d.Message)
		}
	}
	plan, _ = opt.Optimize(plan)
	return plan
}

func TestPlanDistributedQueryIR_NilPlan(t *testing.T) {
	plan, err := PlanDistributedQueryIR(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Strategy != MergeConcat {
		t.Errorf("expected MergeConcat, got %v", plan.Strategy)
	}
}

func TestPlanDistributedQueryIR_StatsCount(t *testing.T) {
	plan := parseLowerOptIR(t, `from main | stats count()`)
	result, err := PlanDistributedQueryIR(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Strategy != MergePartialAgg {
		t.Errorf("expected MergePartialAgg, got %v", result.Strategy)
	}
	if result.PartialAggSpec == nil {
		t.Fatal("expected non-nil PartialAggSpec")
	}
	if len(result.PartialAggSpec.Funcs) != 1 {
		t.Fatalf("expected 1 agg func, got %d", len(result.PartialAggSpec.Funcs))
	}
	if result.PartialAggSpec.Funcs[0].Name != "count" {
		t.Errorf("expected count, got %s", result.PartialAggSpec.Funcs[0].Name)
	}
}

func TestPlanDistributedQueryIR_StatsCountByField(t *testing.T) {
	plan := parseLowerOptIR(t, `from main | stats count() by source`)
	result, err := PlanDistributedQueryIR(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Strategy != MergePartialAgg {
		t.Errorf("expected MergePartialAgg, got %v", result.Strategy)
	}
	if len(result.PartialAggSpec.GroupBy) == 0 {
		t.Error("expected non-empty group by")
	}
	if result.PartialAggSpec.GroupBy[0] != "source" {
		t.Errorf("expected group by source, got %v", result.PartialAggSpec.GroupBy)
	}
}

func TestPlanDistributedQueryIR_SortIsCoordOnly(t *testing.T) {
	plan := parseLowerOptIR(t, `from main | sort -_time`)
	result, err := PlanDistributedQueryIR(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Strategy != MergeConcat {
		t.Errorf("expected MergeConcat, got %v", result.Strategy)
	}
	// Scan is pushable but sort is not.
	if len(result.ShardNodes) != 1 {
		t.Errorf("expected 1 shard node (scan), got %d", len(result.ShardNodes))
	}
	if len(result.CoordNodes) != 1 {
		t.Errorf("expected 1 coord node (sort), got %d", len(result.CoordNodes))
	}
}

func TestPlanDistributedQueryIR_DedupIsCoordOnly(t *testing.T) {
	plan := parseLowerOptIR(t, `from main | dedup 1 host`)
	result, err := PlanDistributedQueryIR(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Scan pushable, dedup not.
	if len(result.ShardNodes) != 1 {
		t.Errorf("expected 1 shard node, got %d", len(result.ShardNodes))
	}
	if len(result.CoordNodes) != 1 {
		t.Errorf("expected 1 coord node, got %d", len(result.CoordNodes))
	}
}

func TestIsPushableIR(t *testing.T) {
	tests := []struct {
		name     string
		node     logical.Node
		pushable bool
	}{
		{"scan", &logical.Scan{}, true},
		{"filter", &logical.Filter{}, true},
		{"extend", &logical.Extend{}, true},
		{"parse", &logical.Parse{}, true},
		{"aggregate_plain", &logical.Aggregate{}, true},
		{"aggregate_eventstats", &logical.Aggregate{Window: &logical.WindowSpec{Variant: logical.WindowEventstats}}, false},
		{"aggregate_streamstats", &logical.Aggregate{Window: &logical.WindowSpec{Variant: logical.WindowStreamstats}}, false},
		{"project_keep", &logical.Project{Cols: []logical.ProjectCol{{Action: logical.ProjectKeep, Name: "x"}}}, true},
		{"project_drop", &logical.Project{Cols: []logical.ProjectCol{{Action: logical.ProjectDrop, Name: "x"}}}, false},
		{"sort", &logical.Sort{}, false},
		{"limit", &logical.Limit{}, false},
		{"topk", &logical.TopK{}, false},
		{"dedup", &logical.Dedup{}, false},
		{"join", &logical.Join{}, false},
		{"union", &logical.Union{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPushableIR(tt.node); got != tt.pushable {
				t.Errorf("isPushableIR(%s) = %v, want %v", tt.name, got, tt.pushable)
			}
		})
	}
}

// TestSplitParity_OldVsNew compares old SPL2 split decisions with new IR split
// decisions for equivalent queries. Each entry has an SPL2 query and its
// LynxFlow equivalent. We verify: same strategy, same split boundary (number
// of shard vs coord stages), same partial agg function names.
func TestSplitParity_OldVsNew(t *testing.T) {
	type parityTest struct {
		name      string
		spl2Q     string
		lynxflowQ string
	}

	tests := []parityTest{
		// NOTE: "search_only" is intentionally excluded from the non-scan count
		// comparison. SPL2's SearchCommand is a fused scan+filter (counted as
		// "search" type by countNonScanSPL2 which skips it), while LynxFlow
		// separates into Scan + Filter (Filter is non-scan). The STRATEGY and
		// coord count still match, which is what matters for the split boundary.
		{
			name:      "search_only",
			spl2Q:     `search "error"`,
			lynxflowQ: `from main | where _raw == "error"`,
		},
		{
			name:      "stats_count",
			spl2Q:     `| stats count`,
			lynxflowQ: `from main | stats count()`,
		},
		{
			name:      "stats_count_by",
			spl2Q:     `| stats count by source`,
			lynxflowQ: `from main | stats count() by source`,
		},
		{
			name:      "where_stats",
			spl2Q:     `| where status >= 500 | stats count by source`,
			lynxflowQ: `from main | where status >= 500 | stats count() by source`,
		},
		{
			name:      "sort_only",
			spl2Q:     `| sort -_time`,
			lynxflowQ: `from main | sort -_time`,
		},
		{
			name:      "where_sort",
			spl2Q:     `| where level="error" | sort -_time`,
			lynxflowQ: `from main | where level == "error" | sort -_time`,
		},
		{
			name:      "stats_avg",
			spl2Q:     `| stats avg(duration) by endpoint`,
			lynxflowQ: `from main | stats avg(duration) by endpoint`,
		},
		{
			name:      "eval_stats",
			spl2Q:     `| eval svc=source | stats count by svc`,
			lynxflowQ: `from main | extend svc = source | stats count() by svc`,
		},
		{
			name:      "dedup",
			spl2Q:     `| dedup host`,
			lynxflowQ: `from main | dedup 1 host`,
		},
		{
			name:      "where_eval_dedup",
			spl2Q:     `| where status >= 500 | eval svc=source | dedup svc`,
			lynxflowQ: `from main | where status >= 500 | extend svc = source | dedup 1 svc`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Old path: SPL2
			spl2Prog, err := spl2.ParseProgram(tt.spl2Q)
			if err != nil {
				t.Fatalf("spl2 parse error: %v", err)
			}
			oldPlan, err := PlanDistributedQuery(spl2Prog)
			if err != nil {
				t.Fatalf("old plan error: %v", err)
			}

			// New path: LynxFlow IR
			irPlan := parseLowerOptIR(t, tt.lynxflowQ)
			newPlan, err := PlanDistributedQueryIR(irPlan)
			if err != nil {
				t.Fatalf("new plan error: %v", err)
			}

			// Compare strategy.
			if oldPlan.Strategy != newPlan.Strategy {
				t.Errorf("strategy mismatch: old=%v new=%v", oldPlan.Strategy, newPlan.Strategy)
			}

			// Compare shard/coord split counts.
			// The IR always has an explicit Scan node at position 0, while
			// SPL2 may or may not have a SearchCommand. SPL2's SearchCommand
			// is a fused scan+filter; in LynxFlow that becomes Scan + Filter
			// (one extra non-scan node). We allow a difference of 1 when the
			// SPL2 side has a SearchCommand in the shard list.
			oldShardNonScan := countNonScanSPL2(oldPlan.ShardCommands)
			newShardNonScan := countNonScanIR(newPlan.ShardNodes)
			hasSearchCmd := hasSearchCommand(oldPlan.ShardCommands)
			diff := newShardNonScan - oldShardNonScan
			if diff != 0 && !(hasSearchCmd && diff == 1) {
				t.Errorf("shard non-scan count mismatch: old=%d new=%d (hasSearch=%v)",
					oldShardNonScan, newShardNonScan, hasSearchCmd)
			}

			oldCoordCount := len(oldPlan.CoordCommands)
			newCoordCount := len(newPlan.CoordNodes)
			if oldCoordCount != newCoordCount {
				t.Errorf("coord count mismatch: old=%d new=%d",
					oldCoordCount, newCoordCount)
			}

			// Compare partial agg function names if applicable.
			if oldPlan.PartialAggSpec != nil && newPlan.PartialAggSpec != nil {
				if len(oldPlan.PartialAggSpec.Funcs) != len(newPlan.PartialAggSpec.Funcs) {
					t.Errorf("partial agg func count mismatch: old=%d new=%d",
						len(oldPlan.PartialAggSpec.Funcs), len(newPlan.PartialAggSpec.Funcs))
				} else {
					for i := range oldPlan.PartialAggSpec.Funcs {
						if oldPlan.PartialAggSpec.Funcs[i].Name != newPlan.PartialAggSpec.Funcs[i].Name {
							t.Errorf("partial agg func[%d] name mismatch: old=%s new=%s",
								i, oldPlan.PartialAggSpec.Funcs[i].Name, newPlan.PartialAggSpec.Funcs[i].Name)
						}
					}
				}
			}

			t.Logf("PARITY OK: strategy=%v oldShard=%d newShard=%d oldCoord=%d newCoord=%d shardQuery=%q",
				newPlan.Strategy,
				len(oldPlan.ShardCommands), len(newPlan.ShardNodes),
				oldCoordCount, newCoordCount,
				newPlan.ShardQuery)
		})
	}
}

// countNonScanSPL2 counts commands that are not SearchCommand (the SPL2
// equivalent of Scan).
func countNonScanSPL2(cmds []spl2.Command) int {
	count := 0
	for _, cmd := range cmds {
		if _, ok := cmd.(*spl2.SearchCommand); !ok {
			count++
		}
	}
	return count
}

// countNonScanIR counts nodes that are not Scan.
func countNonScanIR(nodes []logical.Node) int {
	count := 0
	for _, n := range nodes {
		if _, ok := n.(*logical.Scan); !ok {
			count++
		}
	}
	return count
}

// hasSearchCommand returns true if the SPL2 command list contains a SearchCommand.
func hasSearchCommand(cmds []spl2.Command) bool {
	for _, cmd := range cmds {
		if _, ok := cmd.(*spl2.SearchCommand); ok {
			return true
		}
	}
	return false
}
