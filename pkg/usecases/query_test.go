package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/config"
	"github.com/lynxbase/lynxdb/pkg/planner"
)

func TestExplain_ValidQuery(t *testing.T) {
	svc := NewQueryService(planner.New(), nil, config.QueryConfig{})

	result, err := svc.Explain(context.Background(), ExplainRequest{
		Query: "from main | head 100",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Fatal("expected valid query")
	}
	if result.Parsed == nil {
		t.Fatal("expected Parsed to be non-nil")
	}
	if result.Parsed.ResultType != "events" {
		t.Errorf("expected events, got %s", result.Parsed.ResultType)
	}
	// Pipeline field tracking not yet implemented for logical plan (RFC-002 stub).
}

func TestExplain_AggregateQuery(t *testing.T) {
	svc := NewQueryService(planner.New(), nil, config.QueryConfig{})

	result, err := svc.Explain(context.Background(), ExplainRequest{
		Query: "from main | stats count() as count by host",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Fatal("expected valid query")
	}
	if result.Parsed.ResultType != "aggregate" {
		t.Errorf("expected aggregate, got %s", result.Parsed.ResultType)
	}
}

func TestExplain_InvalidQuery(t *testing.T) {
	svc := NewQueryService(planner.New(), nil, config.QueryConfig{})

	result, err := svc.Explain(context.Background(), ExplainRequest{
		Query: "|||invalid",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsValid {
		t.Fatal("expected invalid query")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}
}

func TestExplain_CostEstimation(t *testing.T) {
	svc := NewQueryService(planner.New(), nil, config.QueryConfig{})

	tests := []struct {
		name  string
		query string
		cost  string
	}{
		{"high cost (full scan)", "from * | head 1000", "high"},
		{"medium cost (search terms)", `from main | where contains(_raw, "error") | head 1000`, "medium"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := svc.Explain(context.Background(), ExplainRequest{Query: tt.query})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsValid {
				t.Fatal("expected query to parse successfully, but IsValid=false")
			}
			if result.Parsed.EstimatedCost != tt.cost {
				t.Errorf("expected cost %q, got %q", tt.cost, result.Parsed.EstimatedCost)
			}
		})
	}
}

// E2: Physical plan tests
// TODO(RFC-002): extractPhysicalPlan is a stub returning nil.
// These tests are skipped until the physical plan extraction is implemented
// against the logical plan.

func TestExplain_PhysicalPlan_CountStar(t *testing.T) {
	t.Skip("extractPhysicalPlan not yet implemented for logical plan (RFC-002)")
}

func TestExplain_PhysicalPlan_PartialAgg(t *testing.T) {
	t.Skip("extractPhysicalPlan not yet implemented for logical plan (RFC-002)")
}

func TestExplain_PhysicalPlan_RexLiteralPreFilter(t *testing.T) {
	t.Skip("extractPhysicalPlan not yet implemented for logical plan (RFC-002)")
}

func TestExplain_PhysicalPlan_TopKAgg(t *testing.T) {
	t.Skip("extractPhysicalPlan not yet implemented for logical plan (RFC-002)")
}

func TestExplain_PhysicalPlan_NilForSimpleQuery(t *testing.T) {
	svc := NewQueryService(planner.New(), nil, config.QueryConfig{})

	result, err := svc.Explain(context.Background(), ExplainRequest{
		Query: "from main | head 100",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsValid {
		t.Fatal("expected valid query")
	}
	// extractPhysicalPlan is a stub returning nil -- this should still be nil.
	if result.Parsed.PhysicalPlan != nil {
		t.Errorf("expected nil PhysicalPlan for simple query, got %+v", result.Parsed.PhysicalPlan)
	}
}

// U3: Sentinel error tests

func TestHistogram_ValidationErrors(t *testing.T) {
	svc := NewQueryService(planner.New(), nil, config.QueryConfig{})

	_, err := svc.Histogram(context.Background(), HistogramRequest{
		From: "not-a-date",
		To:   "now",
	})
	if err == nil {
		t.Fatal("expected error for invalid from")
	}
	if !errors.Is(err, ErrInvalidFrom) {
		t.Errorf("expected ErrInvalidFrom, got: %v", err)
	}

	_, err = svc.Histogram(context.Background(), HistogramRequest{
		From: "-1h",
		To:   "not-a-date",
	})
	if err == nil {
		t.Fatal("expected error for invalid to")
	}
	if !errors.Is(err, ErrInvalidTo) {
		t.Errorf("expected ErrInvalidTo, got: %v", err)
	}

	_, err = svc.Histogram(context.Background(), HistogramRequest{
		From: "2025-01-02T00:00:00Z",
		To:   "2025-01-01T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected error for from > to")
	}
	if !errors.Is(err, ErrFromBeforeTo) {
		t.Errorf("expected ErrFromBeforeTo, got: %v", err)
	}
}

// --- Pipeline field tracking tests ---
// TODO(RFC-002): annotatePipelineFields is a stub returning nil.
// These tests are skipped until pipeline stage annotation is implemented
// against the logical plan.

func TestExplain_FieldTracking_SourceStage(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_SourceStageDoesNotExpandCatalog(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_StatsReplacesFields(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_EvalAddsFields(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_FieldsRemove(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_TableKeepsOnly(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_MultiStage(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_RexAddsNamedGroups(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_RenameSwapsFields(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}

func TestExplain_FieldTracking_TopReplaces(t *testing.T) {
	t.Skip("annotatePipelineFields not yet implemented for logical plan (RFC-002)")
}
