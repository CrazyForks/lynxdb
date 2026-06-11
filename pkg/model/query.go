// Package model provides shared data types used across layers.
package model

import (
	"sort"
	"time"
)

// TimeBounds represents a concrete time range for storage queries.
type TimeBounds struct {
	Earliest time.Time
	Latest   time.Time
}

// PrepareQueryLints fills in default reasons and severity, then sorts by
// severity (errors first) then position.
func PrepareQueryLints(lints []QueryLint) []QueryLint {
	if len(lints) == 0 {
		return lints
	}
	out := append([]QueryLint(nil), lints...)
	for i := range out {
		if out[i].Severity == "" {
			out[i].Severity = "info"
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		li := lintSeverityRank(out[i].Severity)
		lj := lintSeverityRank(out[j].Severity)
		if li != lj {
			return li < lj
		}
		if out[i].Position != out[j].Position {
			return lintPositionRank(out[i].Position) < lintPositionRank(out[j].Position)
		}
		if out[i].Code != out[j].Code {
			return out[i].Code < out[j].Code
		}
		return out[i].Message < out[j].Message
	})
	return out
}

func lintSeverityRank(s string) int {
	switch s {
	case "error":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func lintPositionRank(p int) int {
	if p == 0 {
		return 1<<31 - 1
	}
	return p
}

// QueryRewrite describes a normalization or desugaring rewrite applied to a
// query before execution.
type QueryRewrite struct {
	Before string `json:"before"`
	After  string `json:"after"`
	Reason string `json:"reason"`
}

// QueryLint describes an advisory diagnostic (lint) about a query.
type QueryLint struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Reason   string `json:"reason,omitempty"`
	Severity string `json:"severity,omitempty"`
	Position int    `json:"position"`
}

// ResultRow represents a single row in query results.
type ResultRow struct {
	Fields map[string]interface{}
}

// IndexStore holds events per index for multi-index queries.
type IndexStore struct {
	Indexes map[string][]ResultRow
}

// QuerySuggestion describes an actionable suggestion for improving a query.
type QuerySuggestion struct {
	Text       string `json:"text"`
	Reason     string `json:"reason"`
	SourceCode string `json:"source_code,omitempty"`
	Message    string `json:"message,omitempty"`
}

// SuggestionsFromLints extracts advisory suggestions from lints.
// Lints carrying a "shortcut available" pattern (code LintShortcutAvailable)
// are converted to QuerySuggestion entries with reason "shortcut".
// The message format expected: "...; Equivalent: `<text>`" — the suggestion
// text is the content between backticks.
func SuggestionsFromLints(lints []QueryLint) []QuerySuggestion {
	var out []QuerySuggestion
	for _, l := range lints {
		if l.Code != LintShortcutAvailable {
			continue
		}
		// Extract text between "Equivalent: `" and "`".
		const prefix = "Equivalent: `"
		idx := 0
		msg := l.Message
		for {
			i := indexOf(msg[idx:], prefix)
			if i < 0 {
				break
			}
			start := idx + i + len(prefix)
			end := indexOf(msg[start:], "`")
			if end < 0 {
				break
			}
			text := msg[start : start+end]
			out = append(out, QuerySuggestion{
				Text:       text,
				Reason:     "shortcut",
				SourceCode: l.Code,
				Message:    l.Message,
			})
			idx = start + end + 1
		}
	}
	return out
}

// indexOf returns the index of sub in s, or -1.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Lint code constants used by multiple packages.
const (
	LintBroadSearch       = "BROAD_SEARCH"
	LintAllSourcesHigh    = "ALL_SOURCES_HIGH_VOLUME"
	LintShortcutAvailable = "LF09"
)

// SourceScope constants describe how a query addresses sources.
const (
	SourceScopeAll    = "all"
	SourceScopeSingle = "single"
	SourceScopeList   = "list"
	SourceScopeGlob   = "glob"
)
