package desugar

import (
	"strings"
	"testing"
)

// Bare-word search sugar routes by glob-ness (§3.1):
//   - unescaped metacharacters -> has_glob(_raw, pattern), CI token glob
//   - all-stars pattern        -> exists(_raw) (mirrors D35 field=*)
//   - escaped metacharacters   -> contains(_raw, literal) — the tokenizer
//     strips * ? \, so has() could never match the literal text
//   - plain words              -> has(_raw, word), unchanged
func TestDesugarBareGlobWords(t *testing.T) {
	for src, want := range map[string]string{
		`from main us*r`:       `has_glob(_raw, "us*r")`,
		`from main user*`:      `has_glob(_raw, "user*")`,
		`from main *user`:      `has_glob(_raw, "*user")`,
		`from main ?ser`:       `has_glob(_raw, "?ser")`,
		`from main err*r*`:     `has_glob(_raw, "err*r*")`,
		`from main *`:          `exists(_raw)`,
		`from main us\*r`:      `contains(_raw, "us*r")`,
		`from main \*user`:     `contains(_raw, "*user")`,
		`from main timeout`:    `has(_raw, "timeout")`,
		`from main error-rate`: `has(_raw, "error-rate")`,
	} {
		q := mustParse(t, src)
		dq, _ := Desugar(q, Options{})
		got := dq.String()
		if !strings.Contains(got, want) {
			t.Errorf("%s: desugared to %q, want it to contain %q", src, got, want)
		}
	}
}

// Glob bare words compose with other sugar terms under juxtaposition-and.
func TestDesugarBareGlobComposition(t *testing.T) {
	q := mustParse(t, `from main us*r status>=500 not time?ut`)
	dq, _ := Desugar(q, Options{})
	got := dq.String()
	for _, want := range []string{
		`has_glob(_raw, "us*r")`,
		`status >= 500`,
		`has_glob(_raw, "time?ut")`,
		`not`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("desugared to %q, want it to contain %q", got, want)
		}
	}
}
