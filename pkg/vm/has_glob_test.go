package vm

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/event"
)

// has_glob(field, pattern): case-insensitive whole-token glob match.
func TestHasGlobTokenMatch(t *testing.T) {
	raw := event.StringValue("Accepted password for User from 10.0.0.1 port 22 ssh2")
	fields := map[string]event.Value{"_raw": raw}

	for pattern, want := range map[string]bool{
		"us*r":      true,  // middle star matches token "user"
		"user":      true,  // no metachars still works (exact token)
		"*ser":      true,  // leading star
		"use*":      true,  // trailing star
		"u?er":      true,  // single-char wildcard
		"US*R":      true,  // case-insensitive
		"pass*":     true,  // matches "password"
		"*word":     true,  // matches "password"
		"ss?2":      true,  // matches "ssh2"
		"us*z":      false, // no token ends in z after us
		"?ser":      true,  // matches "user"
		"acc*ed":    true,  // matches "accepted"
		"password*": true,  // exact token with trailing star
		"*0*":       true,  // matches "10" / "0" / "1" tokens
		"xyz*":      false,
	} {
		got, _ := runLF(t, call("has_glob", ident("_raw"), litStr(pattern)), fields)
		assertBool(t, got, want, "has_glob "+pattern)
	}
}

// Whole-token contract: the glob must cover the entire token, not a fragment.
func TestHasGlobWholeTokenOnly(t *testing.T) {
	fields := map[string]event.Value{"_raw": event.StringValue("username logged in")}
	// "us*r" must NOT match "username" (token doesn't end in r at a token
	// boundary) — but "us*r*" and "us*e" variants behave per glob anchoring.
	got, _ := runLF(t, call("has_glob", ident("_raw"), litStr("us*r")), fields)
	assertBool(t, got, false, "us*r vs username")

	got, _ = runLF(t, call("has_glob", ident("_raw"), litStr("us*me")), fields)
	assertBool(t, got, true, "us*me vs username")
}

// Escaped metacharacters in the pattern match literal characters — and since
// the tokenizer strips punctuation, a literal '*' can never appear in a
// token, so such patterns match nothing.
func TestHasGlobEscapedStarNeverMatchesToken(t *testing.T) {
	fields := map[string]event.Value{"_raw": event.StringValue("weird us*r literal text")}
	got, _ := runLF(t, call("has_glob", ident("_raw"), litStr(`us\*r`)), fields)
	assertBool(t, got, false, `has_glob us\*r`)
}

// Null field propagates null (NullOnFailure, mirrors has()).
func TestHasGlobNullField(t *testing.T) {
	got, _ := runLF(t, call("has_glob", ident("missing"), litStr("us*r")), nil)
	assertNull(t, got, "has_glob on missing field")
}

// Invalid pattern is a compile error, not a per-row failure.
func TestHasGlobInvalidPatternIsCompileError(t *testing.T) {
	if _, err := CompileLynxFlow(call("has_glob", ident("_raw"), litStr("[ab"))); err == nil {
		t.Fatal("expected compile error for unterminated character class")
	}
}

// glob() still matches whole values case-sensitively, including brace
// alternatives from the RFC glob syntax.
func TestGlobWholeValueSemantics(t *testing.T) {
	fields := map[string]event.Value{"host": event.StringValue("web-01")}
	for pattern, want := range map[string]bool{
		"web-*":        true,
		"WEB-*":        false, // case-sensitive
		"web-0?":       true,
		"web":          false, // whole-value anchored
		"{web,db}-01":  true,
		"{db,cache}-*": false,
		"web-[0-9]1":   true,
	} {
		got, _ := runLF(t, call("glob", ident("host"), litStr(pattern)), fields)
		assertBool(t, got, want, "glob "+pattern)
	}
}
