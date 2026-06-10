package rest

import (
	"strings"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
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
	_, diags := parser.Parse(query)
	lfClean := !hasErrorDiag(diags)

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
		// Both parse — conservatively choose spl2 for now. PLAN.md §18.2
		// wants ambiguous -> lynxflow, but the lynxflow REST path has not
		// reached envelope/alias parity yet (aggregate response type,
		// buffered-event visibility); the flip happens before Phase 10.
		return langDetectResult{
			Language: LangSPL2,
			Explicit: false,
			DetectNotice: "query parses as both lynxflow and spl2; " +
				"using spl2 for backward compatibility; " +
				"set language=lynxflow to use the new language",
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
