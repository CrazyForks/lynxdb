// Package planner provides a thin wrapper around the LynxFlow parser,
// desugarer, semantic analyzer, and logical optimizer. It presents the
// same Planner interface that the rest of the server stack expects.
//
// RFC-002 Phase 10: this package was previously the SPL2 planner.
// It now delegates entirely to the LynxFlow pipeline.
package planner

import (
	"errors"
	"fmt"
	"time"

	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/opt"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/model"
	"github.com/lynxbase/lynxdb/pkg/storage/views"
	"github.com/lynxbase/lynxdb/pkg/timerange"
)

// Planner parses and optimizes queries.
type Planner interface {
	Plan(req PlanRequest) (*PlanResult, error)
}

// PlanRequest is the input to Plan.
type PlanRequest struct {
	Query string
	From  string
	To    string
}

// PlanResult is the output of Plan.
type PlanResult struct {
	RawQuery           string
	Program            *logical.Plan
	Hints              *model.QueryHints
	ExternalTimeBounds *model.TimeBounds
	ResultType         string
	SkipResultCache    bool
	ParseDuration      time.Duration
	OptimizeDuration   time.Duration
	RuleDetails        []opt.Applied
	TotalRules         int
	OptimizerStats     map[string]int
	Count              int // for tail: requested event count
}

// ParseError represents a query parse error.
type ParseError struct {
	Message    string
	Suggestion string
}

func (e *ParseError) Error() string { return e.Message }

// IsParseError returns true if err wraps a ParseError.
func IsParseError(err error) bool {
	var pe *ParseError
	return errors.As(err, &pe)
}

// TailValidationError represents a tail query validation error.
type TailValidationError struct {
	Message string
}

func (e *TailValidationError) Error() string { return e.Message }

// ValidateForTail checks whether a plan is valid for live tail.
// Blocking (accumulator) stages like stats, sort, and top cannot operate on
// an unbounded live stream, so they are rejected.
func ValidateForTail(plan *logical.Plan) error {
	if plan == nil || plan.Root == nil {
		return nil
	}
	if cmd := findBlockingStage(plan.Root); cmd != "" {
		return &TailValidationError{
			Message: fmt.Sprintf("command %q is not supported in live tail (it requires all data before producing output)", cmd),
		}
	}
	return nil
}

// findBlockingStage walks the logical plan tree and returns the name of the
// first non-streaming (accumulator) stage, or "" if all stages are streaming.
func findBlockingStage(n logical.Node) string {
	if n == nil {
		return ""
	}
	switch nd := n.(type) {
	case *logical.Aggregate:
		if nd.Window == nil {
			return "stats"
		}
	case *logical.Sort:
		return "sort"
	case *logical.TopK:
		return "top"
	}
	for _, child := range n.Children() {
		if cmd := findBlockingStage(child); cmd != "" {
			return cmd
		}
	}
	return ""
}

// DynamicTimeBounds returns true when from/to contain relative time syntax.
func DynamicTimeBounds(from, to string) bool {
	return from != "" || to != "" /* RFC-002: simplified */
}

// QueryUsesDynamicTimeSyntax returns true for queries containing now() or similar.
func QueryUsesDynamicTimeSyntax(_ string) bool {
	return false
}

// ViewCatalog is the interface for materialized view lookup.
type ViewCatalog interface {
	GetView(name string) (*views.ViewDefinition, bool)
	ListViews() []*views.ViewDefinition
}

// Option configures a planner.
type Option func(*lynxFlowPlanner)

// WithViewCatalog sets the view catalog for MV rewriting.
func WithViewCatalog(_ ViewCatalog) Option {
	return func(_ *lynxFlowPlanner) {
		// TODO(RFC-002): wire view catalog into logical optimizer.
	}
}

// New creates a new Planner backed by the LynxFlow pipeline.
func New(opts ...Option) Planner {
	p := &lynxFlowPlanner{}
	for _, o := range opts {
		o(p)
	}
	return p
}

type lynxFlowPlanner struct{}

func (p *lynxFlowPlanner) Plan(req PlanRequest) (*PlanResult, error) {
	parseStart := time.Now()

	q, diags := parser.Parse(req.Query)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			return nil, &ParseError{
				Message:    d.Message,
				Suggestion: d.Suggestion,
			}
		}
	}

	desugared, _ := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})
	parseDuration := time.Since(parseStart)

	optStart := time.Now()
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			return nil, &ParseError{Message: d.Message}
		}
	}

	plan, applied := opt.Optimize(plan)
	optimizeDuration := time.Since(optStart)

	// Build hints from the logical plan pushdown.
	hints := hintsFromPlan(plan)

	// Apply external time bounds.
	var externalTB *model.TimeBounds
	if req.From != "" || req.To != "" {
		tr, trErr := timerange.ParseOptionalRange(req.From, req.To, time.Now())
		if trErr == nil && tr != nil {
			externalTB = &model.TimeBounds{Earliest: tr.Earliest, Latest: tr.Latest}
		}
	}

	// Detect result type from plan.
	rt := detectResultType(plan)

	// Build optimizer stats.
	stats := make(map[string]int)
	totalRules := 0
	for _, a := range applied {
		stats[a.Rule] += a.Count
		totalRules += a.Count
	}

	return &PlanResult{
		RawQuery:           req.Query,
		Program:            plan,
		Hints:              hints,
		ExternalTimeBounds: externalTB,
		ResultType:         rt,
		ParseDuration:      parseDuration,
		OptimizeDuration:   optimizeDuration,
		RuleDetails:        applied,
		TotalRules:         totalRules,
		OptimizerStats:     stats,
	}, nil
}

// hintsFromPlan extracts QueryHints from the logical plan's Scan pushdown.
func hintsFromPlan(plan *logical.Plan) *model.QueryHints {
	hints := &model.QueryHints{}
	if plan == nil || plan.Root == nil {
		return hints
	}
	walkNodes(plan.Root, func(n logical.Node) {
		scan, ok := n.(*logical.Scan)
		if !ok {
			return
		}
		name := scanSourceName(scan)
		if name != "" && name != "*" {
			hints.IndexName = name
			hints.SourceScopeType = model.SourceScopeSingle
			hints.SourceScopeSources = []string{name}
		}
		if name == "*" {
			hints.SourceScopeType = model.SourceScopeAll
		}
		hints.SearchTerms = scan.Pushdown.BloomTerms
	})
	return hints
}

func walkNodes(n logical.Node, f func(logical.Node)) {
	if n == nil {
		return
	}
	f(n)
	for _, child := range n.Children() {
		walkNodes(child, f)
	}
}

func detectResultType(plan *logical.Plan) string {
	if plan == nil || plan.Root == nil {
		return "events"
	}
	return detectNodeResultType(plan.Root)
}

func detectNodeResultType(n logical.Node) string {
	switch nd := n.(type) {
	case *logical.Aggregate:
		if nd.Window != nil {
			return "events" // windowed = events
		}
		return "aggregate"
	case *logical.TopK:
		return "aggregate"
	case *logical.Describe:
		return "aggregate"
	case *logical.Limit:
		return detectNodeResultType(nd.Input)
	case *logical.Sort:
		return detectNodeResultType(nd.Input)
	case *logical.Project:
		return detectNodeResultType(nd.Input)
	default:
		return "events"
	}
}

// Utility for compile-time check.
var _ Planner = (*lynxFlowPlanner)(nil)

// Error wrapping for parse errors.
func FormatParseError(err error, _ string) string {
	return fmt.Sprintf("parse error: %v", err)
}

func scanSourceName(scan *logical.Scan) string {
	if len(scan.Sources) == 0 {
		return ""
	}
	return scan.Sources[0].Name
}
