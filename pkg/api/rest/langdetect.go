package rest

import (
	"strings"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
	"github.com/lynxbase/lynxdb/pkg/spl2"
)

// langDetectResult holds the outcome of language detection.
type langDetectResult struct {
	// Language is the resolved language ("lynxflow" or "spl2").
	Language QueryLanguage
	// Explicit is true when the caller specified the language explicitly.
	Explicit bool
	// DetectNotice is non-empty when detection was used (not explicit) and
	// provides a human-readable notice about the detection result.
	DetectNotice string
}

// detectQueryLanguage resolves the language for a query.
//
// Detection heuristic (applied when language is empty/absent):
//  1. If the trimmed, case-folded query starts with "from " or "let $",
//     try lynxflow first (fast heuristic).
//  2. Try lynxflow parse; if it produces zero error-severity diagnostics,
//     choose lynxflow.
//  3. Try spl2 parse; if it succeeds, choose spl2 with an advisory lint
//     suggesting explicit language.
//  4. If both fail, return lynxflow (the default) with the lynxflow
//     diagnostics surfaced as the error.
//
// When language is explicit ("lynxflow" or "spl2"), no detection runs.
func detectQueryLanguage(query string, explicitLang string) langDetectResult {
	// Explicit language — no detection.
	switch QueryLanguage(strings.ToLower(strings.TrimSpace(explicitLang))) {
	case LangLynxFlow:
		return langDetectResult{Language: LangLynxFlow, Explicit: true}
	case LangSPL2:
		return langDetectResult{Language: LangSPL2, Explicit: true}
	}

	// Detection heuristic (PLAN §18.2, conservative v1 variant).
	//
	// Order:
	// 1. Try lynxflow parse. Record clean/fail.
	// 2. Try spl2 parse. Record clean/fail.
	// 3. Decision matrix:
	//    - LF clean, SPL2 fails  -> lynxflow (only LF understands it)
	//    - LF fails,  SPL2 clean -> spl2
	//    - Both clean             -> spl2 with a notice (deviation from
	//                                PLAN.md §18.2 until the lynxflow REST
	//                                path reaches parity; flips before P10)
	//    - Both fail              -> lynxflow (default language; callers
	//                                surface the lynxflow diagnostics)
	//
	// Old-syntax-only spellings (index=, count without parens, = in where)
	// fail the lynxflow parse, so existing SPL2 queries keep routing to spl2
	// during the dual-runtime window; explicit language always wins.

	// Try lynxflow parse.
	lfAST, diags := parser.Parse(query)
	lfClean := !hasErrorDiag(diags)

	// Semantic validation: even if the parse is clean, check that every
	// aggregate and function call used by the query is registered in the
	// LynxFlow registry. SPL2-only aliases (mean, median, distinct_count,
	// percentile95, exactperc95, ...) parse cleanly in LynxFlow but have no
	// runtime implementation, so they must fall through to the SPL2 path.
	if lfClean && lfAST != nil {
		desugared, _ := desugar.Desugar(lfAST, desugar.Options{DefaultSource: "main"})
		if !lfSemanticClean(desugared) {
			lfClean = false
		}
	}

	// Try spl2 parse.
	_, spl2Err := spl2.ParseProgram(spl2.NormalizeQuery(query))
	spl2Clean := spl2Err == nil

	switch {
	case lfClean && !spl2Clean:
		// Only lynxflow understands this query.
		return langDetectResult{
			Language: LangLynxFlow,
			Explicit: false,
			DetectNotice: "language detected as lynxflow (spl2 parse failed); " +
				"set language=lynxflow to suppress this notice",
		}

	case !lfClean && spl2Clean:
		// Only spl2 understands this query.
		return langDetectResult{
			Language: LangSPL2,
			Explicit: false,
			DetectNotice: "language detected as spl2; " +
				"set language=spl2 or language=lynxflow to suppress this notice",
		}

	case lfClean && spl2Clean:
		// Both parse cleanly and the LynxFlow registry validates all callees.
		// Route to LynxFlow per PLAN.md §18.2 (parity reached in Phase 8b).
		return langDetectResult{
			Language: LangLynxFlow,
			Explicit: false,
			DetectNotice: "query parses as both lynxflow and spl2; " +
				"using lynxflow; " +
				"set language=spl2 to force the legacy path",
		}

	default:
		// Both failed — default to lynxflow (the future default).
		return langDetectResult{
			Language: LangLynxFlow,
			Explicit: false,
			DetectNotice: "language defaulted to lynxflow (neither parser succeeded); " +
				"set language explicitly to control behavior",
		}
	}
}

// hasErrorDiag reports whether any diagnostic has error severity.
func hasErrorDiag(diags []parser.Diag) bool {
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			return true
		}
	}
	return false
}

// lfSemanticClean checks that every aggregate-position callee and every
// expression-position function call in the desugared AST is a registered
// LynxFlow aggregate or function. This catches SPL2-only aliases (mean,
// median, distinct_count, percentile95, etc.) that parse cleanly but have
// no LynxFlow runtime implementation.
//
// The walk is allocation-light: it visits the AST without building any
// intermediate data structures.
func lfSemanticClean(q *ast.Query) bool {
	if q == nil {
		return true
	}
	// Walk the main pipeline and all CTEs.
	for _, let := range q.Lets {
		if !lfPipelineClean(let.Pipeline) {
			return false
		}
	}
	return lfPipelineClean(q.Pipeline)
}

// lfPipelineClean checks a single pipeline for unknown aggregates/functions.
func lfPipelineClean(p ast.Pipeline) bool {
	for _, s := range p.Stages {
		if !lfStageClean(s) {
			return false
		}
	}
	return true
}

// lfStageClean checks a single stage for unknown aggregates/functions.
func lfStageClean(s ast.Stage) bool {
	// Stats aggregates: every callee must be a registered aggregate.
	if sp := s.Stats; sp != nil {
		if !lfAggListClean(sp) {
			return false
		}
	}
	if sp := s.Eventstats; sp != nil {
		if !lfAggListClean(sp) {
			return false
		}
	}
	if sp := s.Streamstats; sp != nil {
		if !lfAggListClean(&sp.StatsPayload) {
			return false
		}
	}

	// Check expressions in where, extend, sort for unknown function calls.
	if s.Where != nil && !lfExprClean(s.Where.Expr) {
		return false
	}
	if s.Extend != nil {
		for _, a := range s.Extend.Assignments {
			if !lfExprClean(a.Value) {
				return false
			}
		}
	}

	// Recurse into sub-pipelines (join, union).
	if s.Join != nil && s.Join.Right != nil {
		if s.Join.Right.Pipeline != nil {
			if !lfPipelineClean(*s.Join.Right.Pipeline) {
				return false
			}
		}
	}
	if s.Union != nil {
		for _, src := range s.Union.Sources {
			if src.Pipeline != nil {
				if !lfPipelineClean(*src.Pipeline) {
					return false
				}
			}
		}
	}

	return true
}

// lfAggListClean checks that every aggregate callee is registered.
func lfAggListClean(sp *ast.StatsPayload) bool {
	for _, agg := range sp.Aggs {
		call, ok := agg.Func.(*ast.Call)
		if !ok {
			continue
		}
		name := strings.ToLower(call.Callee)
		if _, found := registry.LookupAggregate(name); !found {
			return false
		}
		// Check arguments for unknown function calls.
		for _, arg := range call.Args {
			if !lfExprClean(arg) {
				return false
			}
		}
		// Check where condition.
		if agg.WhereCond != nil && !lfExprClean(agg.WhereCond) {
			return false
		}
	}
	// Check by-clause expressions.
	for _, expr := range sp.By {
		if !lfExprClean(expr) {
			return false
		}
	}
	return true
}

// lfExprClean recursively checks that every function call in an expression
// tree is a registered LynxFlow function or aggregate.
func lfExprClean(e ast.Expr) bool {
	if e == nil {
		return true
	}
	switch x := e.(type) {
	case *ast.Call:
		name := strings.ToLower(x.Callee)
		_, isFunc := registry.LookupFunction(name)
		_, isAgg := registry.LookupAggregate(name)
		if !isFunc && !isAgg {
			return false
		}
		for _, arg := range x.Args {
			if !lfExprClean(arg) {
				return false
			}
		}
		if x.Receiver != nil && !lfExprClean(x.Receiver) {
			return false
		}
	case *ast.Binary:
		if !lfExprClean(x.Left) || !lfExprClean(x.Right) {
			return false
		}
	case *ast.Unary:
		if !lfExprClean(x.Operand) {
			return false
		}
	case *ast.In:
		if !lfExprClean(x.LHS) || !lfExprClean(x.RHS) {
			return false
		}
	case *ast.Between:
		if !lfExprClean(x.X) || !lfExprClean(x.Lo) || !lfExprClean(x.Hi) {
			return false
		}
	case *ast.Member:
		if !lfExprClean(x.Object) {
			return false
		}
	case *ast.SafeMember:
		if !lfExprClean(x.Object) {
			return false
		}
	case *ast.Index:
		if !lfExprClean(x.Object) || !lfExprClean(x.Idx) {
			return false
		}
	case *ast.Array:
		for _, elem := range x.Elems {
			if !lfExprClean(elem) {
				return false
			}
		}
	case *ast.Object:
		for _, p := range x.Entries {
			if !lfExprClean(p.Value) {
				return false
			}
		}
	case *ast.Lambda:
		if !lfExprClean(x.Body) {
			return false
		}
	case *ast.Paren:
		if !lfExprClean(x.Inner) {
			return false
		}
		// Ident, Literal, Wildcard, ErrorExpr — always clean.
	}
	return true
}

// validateExplicitLanguage returns an error message if the language value is
// invalid. Returns "" for valid or absent values.
func validateExplicitLanguage(lang string) string {
	if lang == "" {
		return ""
	}
	switch QueryLanguage(strings.ToLower(strings.TrimSpace(lang))) {
	case LangLynxFlow, LangSPL2:
		return ""
	}
	return "invalid language: must be \"lynxflow\" or \"spl2\""
}
