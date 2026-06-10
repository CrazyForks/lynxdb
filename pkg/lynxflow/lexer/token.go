// Package lexer implements a span-carrying lexer for the LynxFlow v2 query
// language (RFC-002). It produces tokens with byte-offset spans into the
// original input and never panics on invalid input; errors are returned as
// Error tokens with diagnostic messages.
package lexer

import "fmt"

// Kind classifies a lexical token.
type Kind uint8

const (
	// Literals and identifiers.

	Ident         Kind = iota // bare identifier [A-Za-z_][A-Za-z0-9_]*
	BacktickIdent             // `backtick-quoted name`
	String                    // "double-quoted string" with escapes
	RawString                 // r"raw string" — no escape processing
	Int                       // decimal 42 or hex 0x2A
	Float                     // 3.14, 1e-6, 2.5e3
	Duration                  // 30s, 1.5h, 100ms — number+unit with no intervening space
	True                      // true (case-insensitive)
	False                     // false
	Null                      // null

	// Punctuation and operators.

	Pipe      // |
	Comma     // ,
	LParen    // (
	RParen    // )
	LBracket  // [
	RBracket  // ]
	LBrace    // {
	RBrace    // }
	Eq        // =   (assignment/search-sugar bind)
	EqEq      // ==
	BangEq    // !=
	Lt        // <
	LtEq      // <=
	Gt        // >
	GtEq      // >=
	Plus      // +
	Minus     // -
	Star      // *
	Slash     // /
	Percent   // %
	Coalesce  // ??
	Question  // ?
	Dot       // .
	SafeNav   // ?.
	Colon     // :
	Semicolon // ;
	Arrow     // ->
	DotDot    // ..
	At        // @
	Dollar    // $
	Bang      // !   (strict-cast suffix like int!(x))

	// Keywords.
	//
	// KEYWORD vs CONTEXTUAL-IDENT SPLIT (RFC-002):
	//
	// RESERVED KEYWORDS (lexed as their own Kind):
	//   Stage-starting words:   from, let, where, parse, extend, keep, drop,
	//                           rename, stats, eventstats, streamstats, sort,
	//                           head, tail, dedup, join, union, explode,
	//                           describe, top, rare, every, rate, latency,
	//                           percentiles, proportion, facets, impact,
	//                           baseline, changes, exemplars, patterns,
	//                           compare, outliers, sessionize, transaction,
	//                           trace, topology, correlate, rollup, xyseries,
	//                           materialize, tee, use
	//   Expression operators:   and, or, not, in, between
	//   Binding/structure:      as, by, with, on, except
	//   Literals:               true, false, null
	//
	// CONTEXTUAL IDENTS (lexed as plain Ident, context-sensitive in parser):
	//   previous, first_of, into, prefix, on_error, window, current, type,
	//   field, method, per, limit, maxpause, startswith, endswith, maxspan,
	//   trace_id, span_id, parent_id, source_field, dest_field, weight_field,
	//   max_nodes, max_templates, similarity, retention, partition_by, sink,
	//   fragment, inner, left, outer, propagate, strict, asc, desc
	//
	// Rationale: only words that START a stage or that are EXPRESSION OPERATORS
	// need reserving; option names like "window", "type", "per" etc. are
	// perfectly legal field names and only acquire meaning when the parser is
	// in an option-value position for a specific stage. This minimises user
	// surprise when field names collide with the language. Keywords that are
	// also function names (e.g. "join", "filter", "map") can be used as
	// function calls because the parser will see ident followed by "(" and
	// resolve accordingly.

	KwFrom
	KwLet
	KwWhere
	KwParse
	KwExtend
	KwKeep
	KwDrop
	KwRename
	KwStats
	KwEventstats
	KwStreamstats
	KwSort
	KwHead
	KwTail
	KwDedup
	KwJoin
	KwUnion
	KwExplode
	KwDescribe
	KwTop
	KwRare
	KwEvery
	KwRate
	KwLatency
	KwPercentiles
	KwProportion
	KwFacets
	KwImpact
	KwBaseline
	KwChanges
	KwExemplars
	KwPatterns
	KwCompare
	KwOutliers
	KwSessionize
	KwTransaction
	KwTrace
	KwTopology
	KwCorrelate
	KwRollup
	KwXyseries
	KwMaterialize
	KwTee
	KwUse
	KwAnd
	KwOr
	KwNot
	KwIn
	KwBetween
	KwAs
	KwBy
	KwWith
	KwOn
	KwExcept

	// Sentinel kinds.

	Error // lexer error — Token.Text holds the diagnostic message
	EOF
)

// Token is a single lexical element of a LynxFlow v2 query. Tokens are
// passed by value (no per-token heap allocation).
type Token struct {
	Kind  Kind
	Start int    // byte offset of first character in source
	End   int    // byte offset one past last character (half-open)
	Text  string // slice of source input for this token (or error message for Error kind)
}

// Span is a half-open byte range [Start, End) into the source text.
type Span struct {
	Start int
	End   int
}

// Diag is a diagnostic message produced during lexing.
type Diag struct {
	Span    Span
	Message string
}

// String returns a human-readable representation.
func (t Token) String() string {
	if t.Kind == Error {
		return fmt.Sprintf("Error(%d:%d %q)", t.Start, t.End, t.Text)
	}
	if t.Kind == EOF {
		return fmt.Sprintf("EOF(%d)", t.Start)
	}
	return fmt.Sprintf("%s(%d:%d %q)", t.Kind.String(), t.Start, t.End, t.Text)
}

var kindNames = [...]string{
	Ident:         "Ident",
	BacktickIdent: "BacktickIdent",
	String:        "String",
	RawString:     "RawString",
	Int:           "Int",
	Float:         "Float",
	Duration:      "Duration",
	True:          "True",
	False:         "False",
	Null:          "Null",
	Pipe:          "Pipe",
	Comma:         "Comma",
	LParen:        "LParen",
	RParen:        "RParen",
	LBracket:      "LBracket",
	RBracket:      "RBracket",
	LBrace:        "LBrace",
	RBrace:        "RBrace",
	Eq:            "Eq",
	EqEq:          "EqEq",
	BangEq:        "BangEq",
	Lt:            "Lt",
	LtEq:          "LtEq",
	Gt:            "Gt",
	GtEq:          "GtEq",
	Plus:          "Plus",
	Minus:         "Minus",
	Star:          "Star",
	Slash:         "Slash",
	Percent:       "Percent",
	Coalesce:      "Coalesce",
	Question:      "Question",
	Dot:           "Dot",
	SafeNav:       "SafeNav",
	Colon:         "Colon",
	Semicolon:     "Semicolon",
	Arrow:         "Arrow",
	DotDot:        "DotDot",
	At:            "At",
	Dollar:        "Dollar",
	Bang:          "Bang",
	KwFrom:        "KwFrom",
	KwLet:         "KwLet",
	KwWhere:       "KwWhere",
	KwParse:       "KwParse",
	KwExtend:      "KwExtend",
	KwKeep:        "KwKeep",
	KwDrop:        "KwDrop",
	KwRename:      "KwRename",
	KwStats:       "KwStats",
	KwEventstats:  "KwEventstats",
	KwStreamstats: "KwStreamstats",
	KwSort:        "KwSort",
	KwHead:        "KwHead",
	KwTail:        "KwTail",
	KwDedup:       "KwDedup",
	KwJoin:        "KwJoin",
	KwUnion:       "KwUnion",
	KwExplode:     "KwExplode",
	KwDescribe:    "KwDescribe",
	KwTop:         "KwTop",
	KwRare:        "KwRare",
	KwEvery:       "KwEvery",
	KwRate:        "KwRate",
	KwLatency:     "KwLatency",
	KwPercentiles: "KwPercentiles",
	KwProportion:  "KwProportion",
	KwFacets:      "KwFacets",
	KwImpact:      "KwImpact",
	KwBaseline:    "KwBaseline",
	KwChanges:     "KwChanges",
	KwExemplars:   "KwExemplars",
	KwPatterns:    "KwPatterns",
	KwCompare:     "KwCompare",
	KwOutliers:    "KwOutliers",
	KwSessionize:  "KwSessionize",
	KwTransaction: "KwTransaction",
	KwTrace:       "KwTrace",
	KwTopology:    "KwTopology",
	KwCorrelate:   "KwCorrelate",
	KwRollup:      "KwRollup",
	KwXyseries:    "KwXyseries",
	KwMaterialize: "KwMaterialize",
	KwTee:         "KwTee",
	KwUse:         "KwUse",
	KwAnd:         "KwAnd",
	KwOr:          "KwOr",
	KwNot:         "KwNot",
	KwIn:          "KwIn",
	KwBetween:     "KwBetween",
	KwAs:          "KwAs",
	KwBy:          "KwBy",
	KwWith:        "KwWith",
	KwOn:          "KwOn",
	KwExcept:      "KwExcept",
	Error:         "Error",
	EOF:           "EOF",
}

// String returns the name of the token kind.
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", k)
}

// IsKeyword reports whether this kind is a reserved keyword.
func (k Kind) IsKeyword() bool {
	return k >= KwFrom && k <= KwExcept
}

// keywords maps lowercase keyword text to the corresponding Kind.
// Built once at init-free package load time via a variable initializer.
var keywords = func() map[string]Kind {
	m := map[string]Kind{
		"from":        KwFrom,
		"let":         KwLet,
		"where":       KwWhere,
		"parse":       KwParse,
		"extend":      KwExtend,
		"keep":        KwKeep,
		"drop":        KwDrop,
		"rename":      KwRename,
		"stats":       KwStats,
		"eventstats":  KwEventstats,
		"streamstats": KwStreamstats,
		"sort":        KwSort,
		"head":        KwHead,
		"tail":        KwTail,
		"dedup":       KwDedup,
		"join":        KwJoin,
		"union":       KwUnion,
		"explode":     KwExplode,
		"describe":    KwDescribe,
		"top":         KwTop,
		"rare":        KwRare,
		"every":       KwEvery,
		"rate":        KwRate,
		"latency":     KwLatency,
		"percentiles": KwPercentiles,
		"proportion":  KwProportion,
		"facets":      KwFacets,
		"impact":      KwImpact,
		"baseline":    KwBaseline,
		"changes":     KwChanges,
		"exemplars":   KwExemplars,
		"patterns":    KwPatterns,
		"compare":     KwCompare,
		"outliers":    KwOutliers,
		"sessionize":  KwSessionize,
		"transaction": KwTransaction,
		"trace":       KwTrace,
		"topology":    KwTopology,
		"correlate":   KwCorrelate,
		"rollup":      KwRollup,
		"xyseries":    KwXyseries,
		"materialize": KwMaterialize,
		"tee":         KwTee,
		"use":         KwUse,
		"and":         KwAnd,
		"or":          KwOr,
		"not":         KwNot,
		"in":          KwIn,
		"between":     KwBetween,
		"as":          KwAs,
		"by":          KwBy,
		"with":        KwWith,
		"on":          KwOn,
		"except":      KwExcept,
		"true":        True,
		"false":       False,
		"null":        Null,
	}
	return m
}()

// LineCol computes 1-based line and column numbers for the given byte offset
// within source. This is intended for caret rendering in diagnostics.
func LineCol(source string, offset int) (line, col int) {
	if offset > len(source) {
		offset = len(source)
	}
	line = 1
	lineStart := 0
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	col = offset - lineStart + 1
	return line, col
}
