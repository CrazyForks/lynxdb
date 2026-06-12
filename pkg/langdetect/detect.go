// Package langdetect provides language detection for query strings. After the
// removal of pkg/spl2 (RFC-002 Phase 10), the only supported language is
// LynxFlow. Requesting "spl2" returns an explicit error via ValidateExplicitLanguage.
//
// The primary entry point is Detect, which always returns LangLynxFlow.
// ValidateExplicitLanguage gates the API boundary: "" and "lynxflow" pass;
// "spl2" returns a migration error.
package langdetect

import (
	"strings"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
)

// Language identifies which parser/execution path to use.
type Language string

const (
	// LangLynxFlow selects the LynxFlow v2 parser and execution path.
	// This is the only supported language post-RFC-002.
	LangLynxFlow Language = "lynxflow"
)

// Result holds the outcome of language detection.
type Result struct {
	// Language is the resolved language (always "lynxflow" post-RFC-002).
	Language Language
	// Explicit is true when the caller specified the language explicitly.
	Explicit bool
}

// Detect resolves the language for a query. Post-RFC-002 Phase 10, this always
// returns LangLynxFlow. Explicit language="spl2" is rejected at the API layer
// via ValidateExplicitLanguage, not here.
func Detect(query string, explicitLang string) Result {
	switch Language(strings.ToLower(strings.TrimSpace(explicitLang))) {
	case LangLynxFlow:
		return Result{Language: LangLynxFlow, Explicit: true}
	}
	// Default: lynxflow (the only supported language).
	return Result{Language: LangLynxFlow, Explicit: explicitLang != ""}
}

// ValidateExplicitLanguage returns an error message if the language value is
// invalid. Returns "" for valid or absent values. Post-RFC-002, only "lynxflow"
// is valid; "spl2" returns a migration error.
func ValidateExplicitLanguage(lang string) string {
	if lang == "" {
		return ""
	}
	switch Language(strings.ToLower(strings.TrimSpace(lang))) {
	case LangLynxFlow:
		return ""
	case "spl2":
		return `language "spl2" is no longer supported; migrate queries to LynxFlow — see CHANGELOG for migration guide`
	}
	return `invalid language: must be "lynxflow"`
}

// HasErrorDiag reports whether any diagnostic has error severity.
func HasErrorDiag(diags []parser.Diag) bool {
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			return true
		}
	}
	return false
}
