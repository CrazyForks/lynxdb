package physical

import (
	"fmt"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/event"
)

// Regression: bare glob terms (us*r) must not falsely skip parts.
//
// Before TokenGlobs, `from main us*r` desugared to has(_raw, "us*r"), whose
// literal was tokenized into ["us", "r"] and pushed as exact RawTerms — the
// inverted index then "proved" no events matched and skipped every part even
// though tokens like "user" matched the glob.

func TestPartSource_TokenGlobNoFalseSkip(t *testing.T) {
	dir := t.TempDir()
	writePartFile(t, dir, "main", makeTestEvents(50, "ssh", func(i int) string {
		if i%2 == 0 {
			return fmt.Sprintf("Accepted password for user from 10.0.0.%d", i)
		}
		return fmt.Sprintf("session opened for root id=%d", i)
	}))

	var parts []PartHandle
	for _, f := range findPartFiles(t, dir) {
		parts = append(parts, openPartHandle(t, f, "main"))
	}
	if len(parts) == 0 {
		t.Fatal("no parts written")
	}

	results, ss := drainPartSource(t, `from main us*r`, parts)

	if ss.PartsSkipped.Load() != 0 {
		t.Errorf("PartsSkipped: got %d, want 0 (glob must not falsely skip)", ss.PartsSkipped.Load())
	}
	if len(results) != 25 {
		t.Errorf("results: got %d, want 25 (events with token 'user')", len(results))
	}
}

func TestPartSource_TokenGlobSkipsNonMatching(t *testing.T) {
	dir1, dir2 := t.TempDir(), t.TempDir()
	// Part 1 has "user" tokens; part 2 has none matching us*r.
	writePartFile(t, dir1, "main", makeTestEvents(20, "ssh", func(i int) string {
		return fmt.Sprintf("Accepted password for user from 10.0.0.%d", i)
	}))
	writePartFile(t, dir2, "main", makeTestEvents(20, "web", func(i int) string {
		return fmt.Sprintf("GET /api/health %d", i)
	}))

	var parts []PartHandle
	for _, f := range append(findPartFiles(t, dir1), findPartFiles(t, dir2)...) {
		parts = append(parts, openPartHandle(t, f, "main"))
	}
	if len(parts) < 2 {
		t.Fatalf("expected at least 2 parts, got %d", len(parts))
	}

	results, ss := drainPartSource(t, `from main us*r`, parts)

	if ss.PartsSkipped.Load() == 0 {
		t.Error("PartsSkipped: got 0, want >0 (FST expansion should prove part 2 empty)")
	}
	if len(results) != 20 {
		t.Errorf("results: got %d, want 20", len(results))
	}
}

// Glob variations against the part-backed path: leading, trailing, ?, and a
// query that matches nothing.
func TestPartSource_TokenGlobVariations(t *testing.T) {
	dir := t.TempDir()
	writePartFile(t, dir, "main", makeTestEvents(10, "ssh", func(i int) string {
		return fmt.Sprintf("Failed password for invalid user admin%d port 22 ssh2", i)
	}))

	var parts []PartHandle
	for _, f := range findPartFiles(t, dir) {
		parts = append(parts, openPartHandle(t, f, "main"))
	}

	for query, want := range map[string]int{
		`from main *ssword`:   10, // leading star → "password"
		`from main pass*`:     10, // trailing star
		`from main adm?n*`:    10, // ? + trailing star → "admin0".."admin9"
		`from main inval?d`:   10, // ? in the middle
		`from main zz*`:       0,
		`from main us*r ssh2`: 10, // glob + exact term conjunction
	} {
		results, _ := drainPartSource(t, query, parts)
		if len(results) != want {
			t.Errorf("%s: got %d results, want %d", query, len(results), want)
		}
	}
}

// Ephemeral (pipe mode) pushdown for token globs.

func TestEphemeralPushdown_TokenGlobs(t *testing.T) {
	events := map[string][]*event.Event{
		"main": {
			mkEvent("Accepted password for user from host-a"),
			mkEvent("GET /api/health 200"),
			mkEvent("Invalid USER admin"), // case-insensitive
		},
	}

	results, _ := drainWithStats(t, `from main us*r`, events)
	if len(results) != 2 {
		t.Errorf("us*r: got %d results, want 2", len(results))
	}

	results, _ = drainWithStats(t, `from main *ealth`, events)
	if len(results) != 1 {
		t.Errorf("*ealth: got %d results, want 1", len(results))
	}
}

// Escaped metacharacters search for the literal text via contains().
func TestEphemeralPushdown_EscapedGlob(t *testing.T) {
	events := map[string][]*event.Event{
		"main": {
			mkEvent("weird literal us*r appeared"),
			mkEvent("plain user line"),
		},
	}

	results, _ := drainWithStats(t, `from main us\*r`, events)
	if len(results) != 1 {
		t.Errorf(`us\*r: got %d results, want 1 (literal match only)`, len(results))
	}
}
