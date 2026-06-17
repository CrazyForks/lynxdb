package views

import (
	"testing"
)

func TestAnalyzeLynxFlow_CountByHost(t *testing.T) {
	an, err := AnalyzeLynxFlow(`from main | stats count() by host`)
	if err != nil {
		t.Fatalf("AnalyzeLynxFlow: %v", err)
	}
	if an.SourceIndex != "main" {
		t.Errorf("SourceIndex: got %q, want %q", an.SourceIndex, "main")
	}
	if !an.IsAggregation {
		t.Fatal("expected IsAggregation=true")
	}
	if an.AggSpec == nil {
		t.Fatal("AggSpec is nil")
	}
	if len(an.AggSpec.Funcs) != 1 {
		t.Fatalf("AggSpec.Funcs: got %d, want 1", len(an.AggSpec.Funcs))
	}
	if an.AggSpec.Funcs[0].Name != "count" {
		t.Errorf("func name: got %q, want %q", an.AggSpec.Funcs[0].Name, "count")
	}
	if len(an.GroupBy) != 1 || an.GroupBy[0] != "host" {
		t.Errorf("GroupBy: got %v, want [host]", an.GroupBy)
	}
	if an.Plan == nil {
		t.Fatal("Plan is nil")
	}
}

func TestAnalyzeLynxFlow_FilteredAgg(t *testing.T) {
	an, err := AnalyzeLynxFlow(`from main | where status >= 500 | stats count(), avg(duration) by host`)
	if err != nil {
		t.Fatalf("AnalyzeLynxFlow: %v", err)
	}
	if !an.IsAggregation {
		t.Fatal("expected IsAggregation=true")
	}
	if an.StreamingPlan == nil {
		t.Fatal("StreamingPlan is nil (expected pre-agg filter)")
	}
	// count + avg + hidden _mv_auto_count = 3 funcs.
	if len(an.AggSpec.Funcs) < 2 {
		t.Fatalf("AggSpec.Funcs: got %d, want at least 2 (count + avg)", len(an.AggSpec.Funcs))
	}
}

func TestAnalyzeLynxFlow_ProjectionView(t *testing.T) {
	an, err := AnalyzeLynxFlow(`from main | where level == "error"`)
	if err != nil {
		t.Fatalf("AnalyzeLynxFlow: %v", err)
	}
	if an.IsAggregation {
		t.Fatal("expected IsAggregation=false for projection view")
	}
	if an.AggSpec != nil {
		t.Fatal("AggSpec should be nil for projection view")
	}
}

func TestAnalyzeLynxFlow_RejectsUnsupportedAgg(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"values", `from main | stats values(host)`},
		{"earliest", `from main | stats earliest(msg)`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AnalyzeLynxFlow(tt.query)
			if err == nil {
				t.Fatalf("expected error for unsupported agg %q", tt.name)
			}
		})
	}
}

func TestAnalyzeLynxFlow_MergeableStateAggs(t *testing.T) {
	an, err := AnalyzeLynxFlow(`from main | stats stdev(duration) as sd, p99(duration) as p99, perc_weighted(duration, weight, 95) as p95w by host`)
	if err != nil {
		t.Fatalf("AnalyzeLynxFlow: %v", err)
	}
	if an.AggSpec == nil {
		t.Fatal("AggSpec is nil")
	}
	if len(an.AggSpec.Funcs) != 3 {
		t.Fatalf("AggSpec.Funcs: got %d, want 3", len(an.AggSpec.Funcs))
	}

	if fn := an.AggSpec.Funcs[0]; fn.Name != "stdev" || fn.Field != "duration" || fn.Alias != "sd" {
		t.Fatalf("stdev func: got %+v", fn)
	}
	if fn := an.AggSpec.Funcs[1]; fn.Name != "perc99" || fn.Field != "duration" || fn.Alias != "p99" {
		t.Fatalf("p99 func: got %+v", fn)
	}
	if fn := an.AggSpec.Funcs[2]; fn.Name != "perc_weighted" || fn.Field != "duration" || fn.WeightField != "weight" || fn.Quantile != 0.95 {
		t.Fatalf("perc_weighted func: got %+v", fn)
	}
}

func TestAnalyzeLynxFlow_RejectsJoin(t *testing.T) {
	// Join requires two sources — should be rejected.
	_, err := AnalyzeLynxFlow(`from main | join type=inner host [from other]`)
	if err == nil {
		t.Fatal("expected error for join in MV")
	}
}

// TestAnalyzeLynxFlow_AggSpecCompatibleWithSPL2 was deleted in RFC-002 P10:
// AnalyzeQuery (spl2 path) is a stub returning nil. Cross-language
// compatibility is no longer relevant — LynxFlow is the only language.
