package parser

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/format"
)

// firstSugarBareWord parses src and digs out the first SearchBareWord from
// the from-stage sugar terms (walking juxtaposition 'and' chains leftward).
func firstSugarBareWord(t *testing.T, src string) *ast.SearchBareWord {
	t.Helper()
	q, diags := Parse(src)
	if len(diags) != 0 {
		t.Fatalf("%s: unexpected diags: %v", src, diagMsgs(diags))
	}
	se := q.Pipeline.Source.SugarTerms
	for {
		switch s := se.(type) {
		case *ast.SearchBareWord:
			return s
		case *ast.SearchBinary:
			se = s.Left
		default:
			t.Fatalf("%s: no SearchBareWord in sugar terms (got %T)", src, se)
			return nil
		}
	}
}

// Bare search terms with unescaped glob metacharacters parse as glob words in
// every position: middle (us*r), trailing (user*), leading (*user, ?ser),
// multiple stars, and single '?'.
func TestParseBareGlobVariations(t *testing.T) {
	for src, wantPattern := range map[string]string{
		`from main us*r`:        "us*r",
		`from main user*`:       "user*",
		`from main *user`:       "*user",
		`from main *user*`:      "*user*",
		`from main ?ser`:        "?ser",
		`from main u?er`:        "u?er",
		`from main err*r*`:      "err*r*",
		`from main web-*-prod`:  "web-*-prod",
		`from main *`:           "*",
		`from main us*r status`: "us*r",
	} {
		w := firstSugarBareWord(t, src)
		if !w.Glob {
			t.Errorf("%s: expected Glob=true", src)
		}
		if w.Word != wantPattern {
			t.Errorf("%s: Word = %q, want %q", src, w.Word, wantPattern)
		}
	}
}

// Escaped metacharacters make the word literal: Glob=false and Word holds the
// unescaped text.
func TestParseBareGlobEscapes(t *testing.T) {
	for src, wantWord := range map[string]string{
		`from main us\*r`:   "us*r",
		`from main \*user`:  "*user",
		`from main user\?`:  "user?",
		`from main a\\b`:    `a\b`,
		`from main user\**`: "", // mixed: escaped star + real star stays a glob
	} {
		w := firstSugarBareWord(t, src)
		if wantWord == "" {
			if !w.Glob {
				t.Errorf("%s: expected Glob=true for mixed escape+glob", src)
			}
			if w.Word != `user\**` {
				t.Errorf("%s: Word = %q, want escaped pattern preserved", src, w.Word)
			}
			continue
		}
		if w.Glob {
			t.Errorf("%s: expected Glob=false", src)
		}
		if w.Word != wantWord {
			t.Errorf("%s: Word = %q, want %q", src, w.Word, wantWord)
		}
	}
}

// Escaped metacharacters in key=value sugar values produce literal (non-glob)
// values; unescaped ones stay glob values.
func TestParseSearchValueEscapes(t *testing.T) {
	for _, tc := range []struct {
		src      string
		wantGlob bool
	}{
		{`from main host=web-\*`, false},
		{`from main host=\*`, false},
		{`from main host=web-*`, true},
		{`from main msg=*user*`, true},
		{`from main msg=\*user*`, true}, // trailing unescaped star
	} {
		q, diags := Parse(tc.src)
		if len(diags) != 0 {
			t.Errorf("%s: unexpected diags: %v", tc.src, diagMsgs(diags))
			continue
		}
		kv, ok := q.Pipeline.Source.SugarTerms.(*ast.SearchKeyValue)
		if !ok {
			t.Errorf("%s: expected SearchKeyValue, got %T", tc.src, q.Pipeline.Source.SugarTerms)
			continue
		}
		_, isGlob := kv.Value.(*ast.SearchGlobValue)
		if isGlob != tc.wantGlob {
			t.Errorf("%s: glob value = %v, want %v", tc.src, isGlob, tc.wantGlob)
		}
	}
}

// Formatter fixpoint: bare glob words and escaped literals must survive a
// format→parse round trip without changing semantics.
func TestBareGlobFormatterFixpoint(t *testing.T) {
	for _, src := range []string{
		`from main us*r`,
		`from main *user`,
		`from main ?ser err?r*`,
		`from main us\*r`,
		`from main \*user`,
		`from main us*r status>=500`,
		`from main not us*r or timeout`,
	} {
		q, diags := Parse(src)
		if len(diags) != 0 {
			t.Errorf("%s: unexpected diags: %v", src, diagMsgs(diags))
			continue
		}
		formatted := format.Query(q)
		q2, diags2 := Parse(formatted)
		if len(diags2) != 0 {
			t.Errorf("%s: formatted %q does not reparse: %v", src, formatted, diagMsgs(diags2))
			continue
		}
		if again := format.Query(q2); again != formatted {
			t.Errorf("%s: formatter not a fixpoint: %q -> %q", src, formatted, again)
		}
	}
}

// Escaping must round-trip with identical glob-ness, not just reparse.
func TestBareGlobEscapeRoundTripSemantics(t *testing.T) {
	src := `from main us\*r`
	q, diags := Parse(src)
	if len(diags) != 0 {
		t.Fatalf("diags: %v", diagMsgs(diags))
	}
	formatted := format.Query(q)
	q2, diags2 := Parse(formatted)
	if len(diags2) != 0 {
		t.Fatalf("reparse %q: %v", formatted, diagMsgs(diags2))
	}
	w1 := q.Pipeline.Source.SugarTerms.(*ast.SearchBareWord)
	w2 := q2.Pipeline.Source.SugarTerms.(*ast.SearchBareWord)
	if w1.Glob != w2.Glob || w1.Word != w2.Word {
		t.Fatalf("round trip changed semantics: %+v -> %+v (formatted %q)", w1, w2, formatted)
	}
}
