package lexer

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Table-driven tests: every token kind
// ---------------------------------------------------------------------------

func TestTokenKinds(t *testing.T) {
	tests := []struct {
		input string
		want  []Kind
	}{
		// Identifiers.
		{"foo", []Kind{Ident, EOF}},
		{"_bar", []Kind{Ident, EOF}},
		{"abc_123", []Kind{Ident, EOF}},

		// Backtick ident.
		{"`field-with-dash`", []Kind{BacktickIdent, EOF}},
		{"`a.b`", []Kind{BacktickIdent, EOF}},

		// Strings.
		{`"hello"`, []Kind{String, EOF}},
		{`"with \"escape\""`, []Kind{String, EOF}},
		{`"newline\n"`, []Kind{String, EOF}},
		{`"unicode \u{1F600}"`, []Kind{String, EOF}},

		// Raw strings.
		{`r"no escapes\n"`, []Kind{RawString, EOF}},

		// Integers.
		{"42", []Kind{Int, EOF}},
		{"0", []Kind{Int, EOF}},
		{"0x2A", []Kind{Int, EOF}},
		{"0xff", []Kind{Int, EOF}},

		// Floats.
		{"3.14", []Kind{Float, EOF}},
		{"1e-6", []Kind{Float, EOF}},
		{"2.5e3", []Kind{Float, EOF}},
		{"1E10", []Kind{Float, EOF}},

		// Durations.
		{"30s", []Kind{Duration, EOF}},
		{"5m", []Kind{Duration, EOF}},
		{"1h", []Kind{Duration, EOF}},
		{"7d", []Kind{Duration, EOF}},
		{"1w", []Kind{Duration, EOF}},
		{"100ms", []Kind{Duration, EOF}},
		{"50us", []Kind{Duration, EOF}},
		{"200ns", []Kind{Duration, EOF}},
		{"1.5h", []Kind{Duration, EOF}},

		// Keywords (case-insensitive).
		{"from", []Kind{KwFrom, EOF}},
		{"FROM", []Kind{KwFrom, EOF}},
		{"From", []Kind{KwFrom, EOF}},
		{"where", []Kind{KwWhere, EOF}},
		{"WHERE", []Kind{KwWhere, EOF}},
		{"and", []Kind{KwAnd, EOF}},
		{"AND", []Kind{KwAnd, EOF}},
		{"or", []Kind{KwOr, EOF}},
		{"not", []Kind{KwNot, EOF}},
		{"in", []Kind{KwIn, EOF}},
		{"between", []Kind{KwBetween, EOF}},
		{"as", []Kind{KwAs, EOF}},
		{"by", []Kind{KwBy, EOF}},
		{"with", []Kind{KwWith, EOF}},
		{"on", []Kind{KwOn, EOF}},
		{"except", []Kind{KwExcept, EOF}},
		{"true", []Kind{True, EOF}},
		{"false", []Kind{False, EOF}},
		{"null", []Kind{Null, EOF}},
		{"TRUE", []Kind{True, EOF}},
		{"NULL", []Kind{Null, EOF}},

		// All stage keywords.
		{"let", []Kind{KwLet, EOF}},
		{"parse", []Kind{KwParse, EOF}},
		{"extend", []Kind{KwExtend, EOF}},
		{"keep", []Kind{KwKeep, EOF}},
		{"drop", []Kind{KwDrop, EOF}},
		{"rename", []Kind{KwRename, EOF}},
		{"stats", []Kind{KwStats, EOF}},
		{"eventstats", []Kind{KwEventstats, EOF}},
		{"streamstats", []Kind{KwStreamstats, EOF}},
		{"sort", []Kind{KwSort, EOF}},
		{"head", []Kind{KwHead, EOF}},
		{"tail", []Kind{KwTail, EOF}},
		{"dedup", []Kind{KwDedup, EOF}},
		{"join", []Kind{KwJoin, EOF}},
		{"union", []Kind{KwUnion, EOF}},
		{"explode", []Kind{KwExplode, EOF}},
		{"describe", []Kind{KwDescribe, EOF}},
		{"top", []Kind{KwTop, EOF}},
		{"rare", []Kind{KwRare, EOF}},
		{"every", []Kind{KwEvery, EOF}},
		{"rate", []Kind{KwRate, EOF}},
		{"latency", []Kind{KwLatency, EOF}},
		{"percentiles", []Kind{KwPercentiles, EOF}},
		{"proportion", []Kind{KwProportion, EOF}},
		{"facets", []Kind{KwFacets, EOF}},
		{"impact", []Kind{KwImpact, EOF}},
		{"baseline", []Kind{KwBaseline, EOF}},
		{"changes", []Kind{KwChanges, EOF}},
		{"exemplars", []Kind{KwExemplars, EOF}},
		{"patterns", []Kind{KwPatterns, EOF}},
		{"compare", []Kind{KwCompare, EOF}},
		{"outliers", []Kind{KwOutliers, EOF}},
		{"sessionize", []Kind{KwSessionize, EOF}},
		{"transaction", []Kind{KwTransaction, EOF}},
		{"trace", []Kind{KwTrace, EOF}},
		{"topology", []Kind{KwTopology, EOF}},
		{"correlate", []Kind{KwCorrelate, EOF}},
		{"rollup", []Kind{KwRollup, EOF}},
		{"xyseries", []Kind{KwXyseries, EOF}},
		{"materialize", []Kind{KwMaterialize, EOF}},
		{"tee", []Kind{KwTee, EOF}},
		{"use", []Kind{KwUse, EOF}},

		// Punctuation.
		{"|", []Kind{Pipe, EOF}},
		{",", []Kind{Comma, EOF}},
		{"(", []Kind{LParen, EOF}},
		{")", []Kind{RParen, EOF}},
		{"[", []Kind{LBracket, EOF}},
		{"]", []Kind{RBracket, EOF}},
		{"{", []Kind{LBrace, EOF}},
		{"}", []Kind{RBrace, EOF}},
		{"=", []Kind{Eq, EOF}},
		{"==", []Kind{EqEq, EOF}},
		{"!=", []Kind{BangEq, EOF}},
		{"<", []Kind{Lt, EOF}},
		{"<=", []Kind{LtEq, EOF}},
		{">", []Kind{Gt, EOF}},
		{">=", []Kind{GtEq, EOF}},
		{"+", []Kind{Plus, EOF}},
		{"-", []Kind{Minus, EOF}},
		{"*", []Kind{Star, EOF}},
		{"/", []Kind{Slash, EOF}},
		{"%", []Kind{Percent, EOF}},
		{"??", []Kind{Coalesce, EOF}},
		{"?", []Kind{Question, EOF}},
		{".", []Kind{Dot, EOF}},
		{"?.", []Kind{SafeNav, EOF}},
		{":", []Kind{Colon, EOF}},
		{";", []Kind{Semicolon, EOF}},
		{"->", []Kind{Arrow, EOF}},
		{"..", []Kind{DotDot, EOF}},
		{"@", []Kind{At, EOF}},
		{"$", []Kind{Dollar, EOF}},
		{"!", []Kind{Bang, EOF}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens, _ := Lex(tt.input)
			if len(tokens) != len(tt.want) {
				t.Fatalf("Lex(%q): got %d tokens, want %d\ntokens: %v", tt.input, len(tokens), len(tt.want), tokens)
			}
			for i, tok := range tokens {
				if tok.Kind != tt.want[i] {
					t.Errorf("Lex(%q)[%d]: got kind %v, want %v (token: %v)", tt.input, i, tok.Kind, tt.want[i], tok)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Ambiguity tests
// ---------------------------------------------------------------------------

func TestAmbiguity_DurationVsIntIdent(t *testing.T) {
	// 1h = duration; "1 h" = int then ident; 5m = duration; "5 m" = int ident
	tests := []struct {
		input string
		want  []Kind
	}{
		{"1h", []Kind{Duration, EOF}},
		{"1 h", []Kind{Int, Ident, EOF}},
		{"5m", []Kind{Duration, EOF}},
		{"5 m", []Kind{Int, Ident, EOF}},
		{"1.5h", []Kind{Duration, EOF}},
		{"100ms", []Kind{Duration, EOF}},
		{"0x2a", []Kind{Int, EOF}},
		{"1e-6", []Kind{Float, EOF}},
		// "5message" should be int + ident (not duration 5m + ident essage).
		{"5message", []Kind{Int, Ident, EOF}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			tokens, _ := Lex(tt.input)
			got := kinds(tokens)
			assertKinds(t, tt.input, got, tt.want)
		})
	}
}

func TestAmbiguity_DotDotVsDot(t *testing.T) {
	// [-7d..-1d] must lex as [ - 7d .. - 1d ]
	tokens, diags := Lex("[-7d..-1d]")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{LBracket, Minus, Duration, DotDot, Minus, Duration, RBracket, EOF}
	assertKinds(t, "[-7d..-1d]", kinds(tokens), want)

	// 1..2 = int 1, .., int 2
	tokens2, diags2 := Lex("1..2")
	if len(diags2) > 0 {
		t.Fatalf("unexpected diags: %v", diags2)
	}
	want2 := []Kind{Int, DotDot, Int, EOF}
	assertKinds(t, "1..2", kinds(tokens2), want2)
}

func TestAmbiguity_SafeNavVsQuestionCoalesce(t *testing.T) {
	// ?. = safe nav, ?? = coalesce
	tokens, _ := Lex("a?.b ?? c")
	want := []Kind{Ident, SafeNav, Ident, Coalesce, Ident, EOF}
	assertKinds(t, "a?.b ?? c", kinds(tokens), want)
}

func TestAmbiguity_EqVsEqEq(t *testing.T) {
	tokens, _ := Lex("a == b = c != d")
	want := []Kind{Ident, EqEq, Ident, Eq, Ident, BangEq, Ident, EOF}
	assertKinds(t, "a == b = c != d", kinds(tokens), want)
}

func TestAmbiguity_BangVsBangEq(t *testing.T) {
	// int!(x) should be: ident "int", bang "!", lparen, ident "x", rparen
	tokens, _ := Lex("int!(x)")
	want := []Kind{Ident, Bang, LParen, Ident, RParen, EOF}
	assertKinds(t, "int!(x)", kinds(tokens), want)

	// Note: "int" is a function name in the registry, but it is NOT a reserved
	// keyword -- it stays as Ident. The parser resolves it as a function call.
}

func TestAmbiguity_ArrowVsMinus(t *testing.T) {
	// -> = arrow, - alone = minus
	tokens, _ := Lex("x -> y - z")
	want := []Kind{Ident, Arrow, Ident, Minus, Ident, EOF}
	assertKinds(t, "x -> y - z", kinds(tokens), want)
}

func TestAmbiguity_RawStringVsIdent(t *testing.T) {
	// r" immediately = raw string; r alone (or r + space) = ident
	tokens, _ := Lex(`r"pattern"`)
	want := []Kind{RawString, EOF}
	assertKinds(t, `r"pattern"`, kinds(tokens), want)

	tokens2, _ := Lex(`r "pattern"`)
	// r is not immediately followed by " so it's a plain ident named r.
	want2 := []Kind{Ident, String, EOF}
	assertKinds(t, `r "pattern"`, kinds(tokens2), want2)

	tokens3, _ := Lex(`rx`)
	want3 := []Kind{Ident, EOF}
	assertKinds(t, "rx", kinds(tokens3), want3)
}

func TestAmbiguity_FloatNoTrailingDot(t *testing.T) {
	// "1." is int 1 then dot (floats require digits after the dot).
	tokens, _ := Lex("1.")
	want := []Kind{Int, Dot, EOF}
	assertKinds(t, "1.", kinds(tokens), want)

	// "1.0" is a float.
	tokens2, _ := Lex("1.0")
	want2 := []Kind{Float, EOF}
	assertKinds(t, "1.0", kinds(tokens2), want2)
}

// ---------------------------------------------------------------------------
// Span correctness
// ---------------------------------------------------------------------------

func TestSpanCorrectness(t *testing.T) {
	input := `from app[-1h] | where status >= 500`
	tokens, diags := Lex(input)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}

	// Spot-check specific tokens.
	expected := []struct {
		kind       Kind
		start, end int
		text       string
	}{
		{KwFrom, 0, 4, "from"},
		{Ident, 5, 8, "app"},
		{LBracket, 8, 9, "["},
		{Minus, 9, 10, "-"},
		{Duration, 10, 12, "1h"},
		{RBracket, 12, 13, "]"},
		{Pipe, 14, 15, "|"},
		{KwWhere, 16, 21, "where"},
		{Ident, 22, 28, "status"},
		{GtEq, 29, 31, ">="},
		{Int, 32, 35, "500"},
		{EOF, 35, 35, ""},
	}

	if len(tokens) != len(expected) {
		t.Fatalf("got %d tokens, want %d\ntokens: %v", len(tokens), len(expected), tokens)
	}

	for i, exp := range expected {
		tok := tokens[i]
		if tok.Kind != exp.kind || tok.Start != exp.start || tok.End != exp.end || tok.Text != exp.text {
			t.Errorf("token[%d]: got {%v, %d, %d, %q}, want {%v, %d, %d, %q}",
				i, tok.Kind, tok.Start, tok.End, tok.Text,
				exp.kind, exp.start, exp.end, exp.text)
		}
	}
}

func TestSpan_AllTokensCovered(t *testing.T) {
	input := `let $x = from main[-1h] | stats count() by service; from $x | sort -count | head 10`
	tokens, diags := Lex(input)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}

	prev := 0
	for i, tok := range tokens {
		if tok.Kind == EOF {
			break
		}
		if tok.Start < prev {
			t.Errorf("token[%d] (%v): Start %d < prev End %d -- tokens overlap", i, tok, tok.Start, prev)
		}
		if tok.End < tok.Start {
			t.Errorf("token[%d] (%v): End %d < Start %d", i, tok, tok.End, tok.Start)
		}
		prev = tok.End
	}
}

// ---------------------------------------------------------------------------
// Comments
// ---------------------------------------------------------------------------

func TestLineComment(t *testing.T) {
	tokens, diags := Lex("foo // this is a comment\nbar")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{Ident, Ident, EOF}
	assertKinds(t, "line comment", kinds(tokens), want)
	if tokens[0].Text != "foo" || tokens[1].Text != "bar" {
		t.Errorf("expected foo, bar; got %q, %q", tokens[0].Text, tokens[1].Text)
	}
}

func TestBlockComment(t *testing.T) {
	tokens, diags := Lex("a /* block */ b")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{Ident, Ident, EOF}
	assertKinds(t, "block comment", kinds(tokens), want)
}

func TestNestedBlockComment(t *testing.T) {
	tokens, diags := Lex("a /* outer /* inner */ still comment */ b")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{Ident, Ident, EOF}
	assertKinds(t, "nested block comment", kinds(tokens), want)
}

func TestUnterminatedBlockComment(t *testing.T) {
	tokens, diags := Lex("a /* unterminated")
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, "unterminated block comment") {
		t.Errorf("expected 'unterminated block comment' in message, got %q", diags[0].Message)
	}
	// Should still get the "a" ident and then the error.
	hasIdent := false
	hasError := false
	for _, tok := range tokens {
		if tok.Kind == Ident && tok.Text == "a" {
			hasIdent = true
		}
		if tok.Kind == Error {
			hasError = true
		}
	}
	if !hasIdent || !hasError {
		t.Errorf("expected ident 'a' and error token; tokens: %v", tokens)
	}
}

// ---------------------------------------------------------------------------
// Error recovery
// ---------------------------------------------------------------------------

func TestErrorRecovery_MultipleErrors(t *testing.T) {
	// Two bad characters and a single-quote, then valid tokens.
	input := "~ ' foo"
	tokens, diags := Lex(input)
	if len(diags) < 2 {
		t.Fatalf("expected at least 2 diags, got %d: %v", len(diags), diags)
	}
	// Should get: Error(~), Error('), Ident(foo), EOF
	hasIdent := false
	errCount := 0
	for _, tok := range tokens {
		if tok.Kind == Error {
			errCount++
		}
		if tok.Kind == Ident && tok.Text == "foo" {
			hasIdent = true
		}
	}
	if errCount < 2 {
		t.Errorf("expected at least 2 error tokens, got %d; tokens: %v", errCount, tokens)
	}
	if !hasIdent {
		t.Errorf("expected to recover and find 'foo'; tokens: %v", tokens)
	}
}

func TestErrorRecovery_UnterminatedString(t *testing.T) {
	tokens, diags := Lex(`"unterminated`)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "unterminated string") {
		t.Errorf("expected unterminated string message, got %q", diags[0].Message)
	}
	_ = tokens
}

func TestErrorRecovery_BadEscape(t *testing.T) {
	tokens, diags := Lex(`"bad \x escape"`)
	// The lexer errors on \x inside the string. After the error, the remaining
	// input is rescanned, which may produce additional diagnostics (e.g. the
	// trailing characters are not inside a string anymore). We check that the
	// first error is about the invalid escape.
	if len(diags) < 1 {
		t.Fatalf("expected at least 1 diag, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "invalid escape") {
		t.Errorf("expected 'invalid escape' message, got %q", diags[0].Message)
	}
	_ = tokens
}

func TestErrorRecovery_BadUnicodeEscape(t *testing.T) {
	// Test \u without braces. We need the literal bytes: " \ u 0 0 4 1 "
	// in the LynxFlow source. Use a Go raw string to prevent Go from
	// interpreting the backslash-u.
	input := "\"bad \\u0041\""
	tokens, diags := Lex(input)
	if len(diags) < 1 {
		t.Fatalf("expected at least 1 diag for \\u without braces, got %d: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, `\u{NNNN}`) {
		t.Errorf("expected \\u{NNNN} hint, got %q", diags[0].Message)
	}
	_ = tokens
}

func TestErrorRecovery_BadHex(t *testing.T) {
	tokens, diags := Lex("0x")
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d: %v", len(diags), diags)
	}
	if !strings.Contains(diags[0].Message, "hex") {
		t.Errorf("expected hex error, got %q", diags[0].Message)
	}
	_ = tokens
}

func TestErrorRecovery_SingleQuote(t *testing.T) {
	tokens, diags := Lex("'hello'")
	if len(diags) < 1 {
		t.Fatalf("expected at least 1 diag, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "single quotes") {
		t.Errorf("expected single-quote hint, got %q", diags[0].Message)
	}
	_ = tokens
}

// ---------------------------------------------------------------------------
// Keyword case insensitivity
// ---------------------------------------------------------------------------

func TestKeywordCaseInsensitivity(t *testing.T) {
	variants := []string{"from", "FROM", "From", "fRoM"}
	for _, v := range variants {
		tokens, _ := Lex(v)
		if tokens[0].Kind != KwFrom {
			t.Errorf("Lex(%q)[0].Kind = %v, want KwFrom", v, tokens[0].Kind)
		}
		// Text should preserve original casing.
		if tokens[0].Text != v {
			t.Errorf("Lex(%q)[0].Text = %q, want %q", v, tokens[0].Text, v)
		}
	}
}

// ---------------------------------------------------------------------------
// LineCol helper
// ---------------------------------------------------------------------------

func TestLineCol(t *testing.T) {
	src := "line1\nline2\nline3"
	tests := []struct {
		offset   int
		wantLine int
		wantCol  int
	}{
		{0, 1, 1},
		{4, 1, 5},
		{5, 1, 6}, // the \n itself
		{6, 2, 1},
		{11, 2, 6},
		{12, 3, 1},
		{16, 3, 5},
	}
	for _, tt := range tests {
		line, col := LineCol(src, tt.offset)
		if line != tt.wantLine || col != tt.wantCol {
			t.Errorf("LineCol(%q, %d) = (%d, %d), want (%d, %d)",
				src, tt.offset, line, col, tt.wantLine, tt.wantCol)
		}
	}
}

// ---------------------------------------------------------------------------
// Corpus test: lex all 63 lynxflow values, assert zero error tokens
// ---------------------------------------------------------------------------

type corpusEntry struct {
	ID       string `json:"id"`
	LynxFlow string `json:"lynxflow"`
}

func TestCorpusLexesWithoutErrors(t *testing.T) {
	f, err := os.Open("../testdata/corpus/corpus.jsonl")
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	count := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e corpusEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("bad JSON on line %d: %v", count+1, err)
		}
		count++

		tokens, diags := Lex(e.LynxFlow)
		if len(diags) > 0 {
			t.Errorf("corpus %s (%q): %d error(s): %v", e.ID, e.LynxFlow, len(diags), diags)
			for _, tok := range tokens {
				if tok.Kind == Error {
					t.Logf("  Error token: %v", tok)
				}
			}
		}

		// Verify every token has a valid span within bounds.
		for i, tok := range tokens {
			if tok.Start < 0 || tok.End < tok.Start {
				t.Errorf("corpus %s token[%d]: invalid span [%d, %d)", e.ID, i, tok.Start, tok.End)
			}
			if tok.Kind != EOF && tok.End > len(e.LynxFlow) {
				t.Errorf("corpus %s token[%d]: End %d > len(input) %d", e.ID, i, tok.End, len(e.LynxFlow))
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count < 50 {
		t.Fatalf("expected at least 50 corpus entries, got %d", count)
	}
	t.Logf("successfully lexed %d corpus entries with zero errors", count)
}

// ---------------------------------------------------------------------------
// Composite query tests
// ---------------------------------------------------------------------------

func TestCompositeQuery(t *testing.T) {
	input := `let $errs = from app[-1h] | where level == "ERROR"; from $errs | stats count() by service`
	tokens, diags := Lex(input)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}

	want := []Kind{
		KwLet, Dollar, Ident, Eq, KwFrom, Ident, LBracket, Minus, Duration, RBracket,
		Pipe, KwWhere, Ident, EqEq, String, Semicolon,
		KwFrom, Dollar, Ident, Pipe, KwStats, Ident, LParen, RParen, KwBy, Ident,
		EOF,
	}
	assertKinds(t, "composite query", kinds(tokens), want)
}

func TestSearchSugarQuery(t *testing.T) {
	input := `from nginx[-1h] timeout status>=500`
	tokens, diags := Lex(input)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{
		KwFrom, Ident, LBracket, Minus, Duration, RBracket,
		Ident, Ident, GtEq, Int,
		EOF,
	}
	assertKinds(t, "search sugar", kinds(tokens), want)
}

func TestObjectLiteral(t *testing.T) {
	input := `{service: "api", retry: true}`
	tokens, diags := Lex(input)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{
		LBrace, Ident, Colon, String, Comma, Ident, Colon, True, RBrace,
		EOF,
	}
	assertKinds(t, "object literal", kinds(tokens), want)
}

func TestLambda(t *testing.T) {
	input := `any(tags, t -> t.name == "vip")`
	tokens, diags := Lex(input)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{
		Ident, LParen, Ident, Comma, Ident, Arrow, Ident, Dot, Ident, EqEq, String, RParen,
		EOF,
	}
	assertKinds(t, "lambda", kinds(tokens), want)
}

func TestStrictCast(t *testing.T) {
	input := `int!(x)`
	tokens, diags := Lex(input)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	// "int" is not a reserved keyword -- it lexes as Ident.
	want := []Kind{Ident, Bang, LParen, Ident, RParen, EOF}
	assertKinds(t, "strict cast", kinds(tokens), want)
}

func TestTimeRangeSnap(t *testing.T) {
	input := `from app[-1h][@h]`
	tokens, diags := Lex(input)
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{
		KwFrom, Ident, LBracket, Minus, Duration, RBracket, LBracket, At, Ident, RBracket,
		EOF,
	}
	assertKinds(t, "time range snap", kinds(tokens), want)
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestEmptyInput(t *testing.T) {
	tokens, diags := Lex("")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(tokens) != 1 || tokens[0].Kind != EOF {
		t.Errorf("expected single EOF, got %v", tokens)
	}
}

func TestWhitespaceOnly(t *testing.T) {
	tokens, diags := Lex("  \t\n\r  ")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(tokens) != 1 || tokens[0].Kind != EOF {
		t.Errorf("expected single EOF, got %v", tokens)
	}
}

func TestCommentOnly(t *testing.T) {
	tokens, diags := Lex("// just a comment")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	if len(tokens) != 1 || tokens[0].Kind != EOF {
		t.Errorf("expected single EOF, got %v", tokens)
	}
}

func TestUnterminatedBacktick(t *testing.T) {
	_, diags := Lex("`unterminated")
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "unterminated backtick") {
		t.Errorf("expected backtick error, got %q", diags[0].Message)
	}
}

func TestUnterminatedRawString(t *testing.T) {
	_, diags := Lex(`r"unterminated`)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "unterminated raw string") {
		t.Errorf("expected raw string error, got %q", diags[0].Message)
	}
}

func TestMultiByteUnicode(t *testing.T) {
	// Unicode characters outside tokens should produce error tokens.
	tokens, diags := Lex("foo \xc3\xa9 bar") // "foo <e-acute> bar"
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d: %v", len(diags), diags)
	}
	// Should still recover and lex bar.
	found := false
	for _, tok := range tokens {
		if tok.Kind == Ident && tok.Text == "bar" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find ident 'bar' after recovery; tokens: %v", tokens)
	}
}

func TestSlashNotComment(t *testing.T) {
	// A lone / at end of input should be a Slash token, not the start of a comment.
	tokens, diags := Lex("a / b")
	if len(diags) > 0 {
		t.Fatalf("unexpected diags: %v", diags)
	}
	want := []Kind{Ident, Slash, Ident, EOF}
	assertKinds(t, "slash operator", kinds(tokens), want)
}

func TestExponentEdgeCases(t *testing.T) {
	// "1e" without digits -- "e" is a separate ident.
	tokens, _ := Lex("1e")
	want := []Kind{Int, Ident, EOF}
	assertKinds(t, "1e", kinds(tokens), want)

	// "1e+" without following digit -- int + ident "e" + plus.
	// Actually: "1" int, "e" ident, "+" plus.
	// Wait -- let me check: the lexer reads digits, sees 'e', peeks ahead
	// for sign+digit. "e+" with no digit after "+" means e is not part of
	// the number. So: int "1", ident "e", plus "+".
	tokens2, _ := Lex("1e+")
	want2 := []Kind{Int, Ident, Plus, EOF}
	assertKinds(t, "1e+", kinds(tokens2), want2)
}

func TestPullAPI(t *testing.T) {
	lex := New("a + b")
	tok1 := lex.Next()
	if tok1.Kind != Ident || tok1.Text != "a" {
		t.Errorf("token 1: got %v", tok1)
	}
	tok2 := lex.Next()
	if tok2.Kind != Plus {
		t.Errorf("token 2: got %v", tok2)
	}
	tok3 := lex.Next()
	if tok3.Kind != Ident || tok3.Text != "b" {
		t.Errorf("token 3: got %v", tok3)
	}
	tok4 := lex.Next()
	if tok4.Kind != EOF {
		t.Errorf("token 4: got %v", tok4)
	}
	// After EOF, repeated calls should still return EOF.
	tok5 := lex.Next()
	if tok5.Kind != EOF {
		t.Errorf("token 5 (after EOF): got %v", tok5)
	}
}

func TestTokenString(t *testing.T) {
	tok := Token{Kind: Ident, Start: 0, End: 3, Text: "foo"}
	s := tok.String()
	if s != `Ident(0:3 "foo")` {
		t.Errorf("Token.String() = %q", s)
	}
}

func TestKindString(t *testing.T) {
	if Ident.String() != "Ident" {
		t.Errorf("Ident.String() = %q", Ident.String())
	}
	if KwFrom.String() != "KwFrom" {
		t.Errorf("KwFrom.String() = %q", KwFrom.String())
	}
	if Kind(255).String() != "Kind(255)" {
		t.Errorf("Kind(255).String() = %q", Kind(255).String())
	}
}

func TestIsKeyword(t *testing.T) {
	if !KwFrom.IsKeyword() {
		t.Error("KwFrom should be keyword")
	}
	if !KwExcept.IsKeyword() {
		t.Error("KwExcept should be keyword")
	}
	if Ident.IsKeyword() {
		t.Error("Ident should not be keyword")
	}
	if EOF.IsKeyword() {
		t.Error("EOF should not be keyword")
	}
}

// ---------------------------------------------------------------------------
// Fuzz test
// ---------------------------------------------------------------------------

func FuzzLex(f *testing.F) {
	// Seed with corpus queries.
	corpusFile, err := os.Open("../testdata/corpus/corpus.jsonl")
	if err != nil {
		f.Fatalf("open corpus: %v", err)
	}
	defer corpusFile.Close()
	sc := bufio.NewScanner(corpusFile)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e corpusEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		f.Add(e.LynxFlow)
	}

	// Seed with tricky ambiguity cases.
	f.Add("1h")
	f.Add("1.5h")
	f.Add("5m")
	f.Add("5 m")
	f.Add("[-7d..-1d]")
	f.Add("1..2")
	f.Add("a?.b ?? c")
	f.Add("int!(x)")
	f.Add("x -> y - z")
	f.Add(`r"pattern"`)
	f.Add(`r "pattern"`)
	f.Add("0x2A")
	f.Add("1e-6")
	f.Add(`"hello \u{1F600}"`)
	f.Add("'single'")
	f.Add("/* nested /* comment */ */")
	f.Add("/* unterminated")
	f.Add(`"unterminated`)
	f.Add("`unterminated")
	f.Add(`r"unterminated`)
	f.Add("0x")
	f.Add("~!@#$%")
	f.Add("")
	f.Add("  \t\n  ")

	f.Fuzz(func(t *testing.T, input string) {
		tokens, _ := Lex(input)

		// Property 1: never panics (if we got here, it didn't).

		// Property 2: every token's span is within bounds.
		for i, tok := range tokens {
			if tok.Start < 0 {
				t.Errorf("token[%d] Start < 0: %d", i, tok.Start)
			}
			if tok.End < tok.Start {
				t.Errorf("token[%d] End < Start: %d < %d", i, tok.End, tok.Start)
			}
			if tok.Kind != EOF && tok.End > len(input) {
				t.Errorf("token[%d] End > len(input): %d > %d", i, tok.End, len(input))
			}
		}

		// Property 3: tokens cover monotonically non-decreasing offsets.
		prevEnd := 0
		for i, tok := range tokens {
			if tok.Kind == EOF {
				break
			}
			if tok.Start < prevEnd {
				t.Errorf("token[%d] Start %d < previous End %d -- overlap", i, tok.Start, prevEnd)
			}
			prevEnd = tok.End
		}

		// Property 4: last token is always EOF.
		if len(tokens) == 0 {
			t.Fatal("Lex returned empty slice (should have at least EOF)")
		}
		if tokens[len(tokens)-1].Kind != EOF {
			t.Errorf("last token is %v, want EOF", tokens[len(tokens)-1].Kind)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func kinds(tokens []Token) []Kind {
	out := make([]Kind, len(tokens))
	for i, t := range tokens {
		out[i] = t.Kind
	}
	return out
}

func assertKinds(t *testing.T, label string, got, want []Kind) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d kinds, want %d\ngot:  %v\nwant: %v", label, len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: kind[%d] = %v, want %v", label, i, got[i], want[i])
		}
	}
}
