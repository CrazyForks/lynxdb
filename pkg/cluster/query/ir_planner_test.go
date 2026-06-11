package query

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/opt"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
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
}

func TestPlanDistributedQueryIR_SortOnly(t *testing.T) {
	plan := parseLowerOptIR(t, `from main | sort -_time`)
	result, err := PlanDistributedQueryIR(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Sort is not pushable -> coord-only
	if result.Strategy != MergeConcat {
		t.Errorf("expected MergeConcat, got %v", result.Strategy)
	}
}

func TestPlanDistributedQueryIR_WhereStatsAvg(t *testing.T) {
	plan := parseLowerOptIR(t, `from main | where status >= 500 | stats avg(duration) by endpoint`)
	result, err := PlanDistributedQueryIR(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Strategy != MergePartialAgg {
		t.Errorf("expected MergePartialAgg, got %v", result.Strategy)
	}
	if result.PartialAggSpec == nil {
		t.Fatal("expected PartialAggSpec to be non-nil")
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
		{"aggregate", &logical.Aggregate{}, true},
		{"limit", &logical.Limit{}, false},
		{"sort", &logical.Sort{}, false},
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

// TestSplitLynxFlow tests the IR distributed planning for various query patterns.
// RFC-002: SPL2 parity comparison removed; only LynxFlow IR path tested.
func TestSplitLynxFlow(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		strategy MergeStrategy
	}{
		{"search_only", `from main | where _raw == "error"`, MergeConcat},
		{"stats_count", `from main | stats count()`, MergePartialAgg},
		{"stats_count_by", `from main | stats count() by source`, MergePartialAgg},
		{"where_stats", `from main | where status >= 500 | stats count() by source`, MergePartialAgg},
		{"sort_only", `from main | sort -_time`, MergeConcat},
		{"where_sort", `from main | where level == "error" | sort -_time`, MergeConcat},
		{"stats_avg", `from main | stats avg(duration) by endpoint`, MergePartialAgg},
		{"eval_stats", `from main | extend svc = source | stats count() by svc`, MergePartialAgg},
		{"dedup", `from main | dedup 1 host`, MergeConcat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			irPlan := parseLowerOptIR(t, tt.query)
			result, err := PlanDistributedQueryIR(irPlan)
			if err != nil {
				t.Fatalf("PlanDistributedQueryIR error: %v", err)
			}
			if result.Strategy != tt.strategy {
				t.Errorf("strategy = %v, want %v", result.Strategy, tt.strategy)
			}
		})
	}
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
