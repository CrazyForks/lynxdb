package views

import enginepipeline "github.com/lynxbase/lynxdb/pkg/engine/pipeline"

// AnalyzeQuery determines if a query can be answered by a materialized view.
// RFC-002: spl2 AST analysis removed. Returns an empty analysis until logical
// plan analysis is implemented. SPL2 views should be created via AnalyzeLynxFlow.
func AnalyzeQuery(_ string) (*QueryAnalysis, error) {
	return &QueryAnalysis{}, nil
}

// QueryAnalysis holds the result of analyzing a query for MV matching.
type QueryAnalysis struct {
	MatchedView   string
	Speedup       string
	IsAggregation bool
	AggSpec       *enginepipeline.PartialAggSpec
	StreamingCmds interface{} // stub
}

// MVAutoCountAlias is the alias used for auto-injected count aggregations.
const MVAutoCountAlias = "__mv_auto_count"
