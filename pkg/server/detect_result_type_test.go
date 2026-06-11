package server

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/logical"
)

func TestDetectResultType(t *testing.T) {
	scan := &logical.Scan{}

	tests := []struct {
		name string
		root logical.Node
		want ResultType
	}{
		{
			name: "nil_plan_returns_events",
			root: nil,
			want: ResultTypeEvents,
		},
		{
			name: "aggregate_returns_aggregate",
			root: &logical.Aggregate{},
			want: ResultTypeAggregate,
		},
		{
			name: "topk_returns_aggregate",
			root: &logical.TopK{},
			want: ResultTypeAggregate,
		},
		{
			name: "describe_returns_aggregate",
			root: &logical.Describe{},
			want: ResultTypeAggregate,
		},
		{
			name: "window_aggregate_returns_events",
			root: &logical.Aggregate{
				Window: &logical.WindowSpec{Variant: logical.WindowEventstats},
			},
			want: ResultTypeEvents,
		},
		{
			name: "limit_over_aggregate_returns_aggregate",
			root: &logical.Limit{N: 10},
			want: ResultTypeAggregate,
		},
		{
			name: "sort_over_aggregate_returns_aggregate",
			root: &logical.Sort{},
			want: ResultTypeAggregate,
		},
		{
			name: "project_over_aggregate_returns_aggregate",
			root: &logical.Project{},
			want: ResultTypeAggregate,
		},
		{
			name: "scan_returns_events",
			root: scan,
			want: ResultTypeEvents,
		},
		{
			name: "limit_over_scan_returns_events",
			root: &logical.Limit{N: 5},
			want: ResultTypeEvents,
		},
		{
			name: "sort_over_topk_returns_aggregate",
			root: &logical.Sort{},
			want: ResultTypeAggregate,
		},
	}

	// Wire up inputs for composite cases.
	tests[5].root.(*logical.Limit).Input = &logical.Aggregate{}   // limit_over_aggregate
	tests[6].root.(*logical.Sort).Input = &logical.Aggregate{}    // sort_over_aggregate
	tests[7].root.(*logical.Project).Input = &logical.Aggregate{} // project_over_aggregate
	tests[9].root.(*logical.Limit).Input = scan                   // limit_over_scan
	tests[10].root.(*logical.Sort).Input = &logical.TopK{}        // sort_over_topk

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var prog *logical.Plan
			if tt.root != nil {
				prog = &logical.Plan{Root: tt.root}
			}
			got := DetectResultType(prog)
			if got != tt.want {
				t.Errorf("DetectResultType() = %q, want %q", got, tt.want)
			}
		})
	}
}
