package format

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
)

var update = flag.Bool("update", false, "update golden files")

// ---------------------------------------------------------------------------
// Corpus + examples loading
// ---------------------------------------------------------------------------

type corpusEntry struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Source   string   `json:"source"`
	SPL2     string   `json:"spl2"`
	LynxFlow string   `json:"lynxflow"`
	Features []string `json:"features"`
	Notes    string   `json:"notes"`
}

func loadCorpus(t *testing.T) []corpusEntry {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "testdata", "corpus", "corpus.jsonl"))
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer f.Close()

	var entries []corpusEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		var e corpusEntry
		if err := json.Unmarshal([]byte(text), &e); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		entries = append(entries, e)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan corpus: %v", err)
	}
	return entries
}

// rfc002Examples returns the 18 §13 examples (the "after" LynxFlow form).
func rfc002Examples() []struct {
	id    string
	query string
} {
	return []struct {
		id    string
		query string
	}{
		{"ex01", `from app[-1h] error timeout`},
		{"ex02", `from nginx[-1h] timeout status>=500`},
		{"ex03", `| parse json | where status >= 500`},
		{"ex04", `| parse logfmt prefix log.`},
		{"ex05", `| parse regex r"user=(?<user>\w+) ip=(?<ip>[\d.]+)" into (ip as string)`},
		{"ex06", `| keep host, status`},
		{"ex07", `| drop _raw`},
		{"ex08", `| keep * except _raw`},
		{"ex09", `| rename duration_ms as latency`},
		{"ex10", `| extend is_err = status >= 500`},
		{"ex11", `| stats count(), avg(dur) by service`},
		{"ex12", `| every 5m by service stats count()`},
		{"ex13", `| latency dur every 5m by endpoint`},
		{"ex14", `| parse json | where user.role == "admin" and any(tags, t -> t.name == "vip")`},
		{"ex15", `| where exists(amount)`},
		{"ex16", `| parse json | where exists(_error) | keep _error, _error_detail, _raw | head 20`},
		{"ex17", `from nginx | parse combined | where status == 500 and has(_raw, "upstream")`},
		{"ex18", `from app[-1h] | parse first_of(json, logfmt) | keep _time, service, status, dur | sort -dur | head 50`},
	}
}

// ---------------------------------------------------------------------------
// 1. Fixpoint property: format(parse(format(parse(q)))) == format(parse(q))
// ---------------------------------------------------------------------------

func TestFixpoint_Corpus(t *testing.T) {
	entries := loadCorpus(t)
	for _, e := range entries {
		t.Run(e.ID, func(t *testing.T) {
			assertFixpoint(t, e.LynxFlow)
		})
	}
}

func TestFixpoint_RFC002Examples(t *testing.T) {
	for _, ex := range rfc002Examples() {
		t.Run(ex.id, func(t *testing.T) {
			assertFixpoint(t, ex.query)
		})
	}
}

func assertFixpoint(t *testing.T, input string) {
	t.Helper()

	// Step 1: parse the original
	q1, diags1 := parser.Parse(input)
	if len(diags1) > 0 {
		t.Skipf("input has parse diags (not a fixpoint candidate): %v", diagStrings(diags1))
		return
	}

	// Step 2: format
	formatted1 := Query(q1)

	// Step 3: re-parse the formatted output
	q2, diags2 := parser.Parse(formatted1)
	if len(diags2) > 0 {
		t.Fatalf("format output re-parses with diags:\n  input:     %q\n  formatted: %q\n  diags: %v",
			input, formatted1, diagStrings(diags2))
	}

	// Step 4: format again
	formatted2 := Query(q2)

	// Step 5: the two formatted outputs must be identical (fixpoint)
	if formatted1 != formatted2 {
		t.Errorf("fixpoint violated:\n  input:      %q\n  format(1):  %q\n  format(2):  %q",
			input, formatted1, formatted2)
	}
}

// ---------------------------------------------------------------------------
// 2. Expression fixpoint
// ---------------------------------------------------------------------------

func TestFixpoint_Expr(t *testing.T) {
	exprs := []string{
		"a or b and c",
		"not a and b",
		"a + b * c",
		"-a * b",
		"a ?? b ?? c",
		"x in [1, 2, 3]",
		"x between 1 and 10",
		`user.role == "admin"`,
		"a.b[0]?.c(1).d",
		`any(tags, t -> t.name == "vip")`,
		"(a + b) * c",
		"a + (b + c)",
		`if(stdev_f > 0, delta_f / stdev_f, null)`,
		"int!(x)",
		"status >= 500",
		`error ?? "none"`,
	}
	for _, input := range exprs {
		t.Run(input, func(t *testing.T) {
			e1, d1 := parser.ParseExpr(input)
			if len(d1) > 0 {
				t.Skipf("parse diags: %v", diagStrings(d1))
				return
			}
			f1 := Expr(e1)
			e2, d2 := parser.ParseExpr(f1)
			if len(d2) > 0 {
				t.Fatalf("re-parse diags for %q -> %q: %v", input, f1, diagStrings(d2))
			}
			f2 := Expr(e2)
			if f1 != f2 {
				t.Errorf("fixpoint violated:\n  input: %q\n  f1: %q\n  f2: %q", input, f1, f2)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. Golden AST dumps
// ---------------------------------------------------------------------------

func TestGoldenAST_Corpus(t *testing.T) {
	entries := loadCorpus(t)
	goldenDir := filepath.Join("..", "testdata", "golden", "ast")

	for _, e := range entries {
		t.Run(e.ID, func(t *testing.T) {
			q, diags := parser.Parse(e.LynxFlow)
			if len(diags) > 0 {
				t.Skipf("parse diags: %v", diagStrings(diags))
				return
			}
			got := ast.Dump(q)
			goldenFile := filepath.Join(goldenDir, e.ID+".txt")
			compareOrUpdate(t, goldenFile, got)
		})
	}
}

func TestGoldenAST_RFC002Examples(t *testing.T) {
	goldenDir := filepath.Join("..", "testdata", "golden", "ast")

	for _, ex := range rfc002Examples() {
		t.Run(ex.id, func(t *testing.T) {
			q, diags := parser.Parse(ex.query)
			if len(diags) > 0 {
				t.Skipf("parse diags: %v", diagStrings(diags))
				return
			}
			got := ast.Dump(q)
			goldenFile := filepath.Join(goldenDir, ex.id+".txt")
			compareOrUpdate(t, goldenFile, got)
		})
	}
}

// ---------------------------------------------------------------------------
// 4. Golden rewrite dumps
// ---------------------------------------------------------------------------

func TestGoldenRewrites_Corpus(t *testing.T) {
	entries := loadCorpus(t)
	goldenDir := filepath.Join("..", "testdata", "golden", "rewrites")

	for _, e := range entries {
		t.Run(e.ID, func(t *testing.T) {
			q, diags := parser.Parse(e.LynxFlow)
			if len(diags) > 0 {
				t.Skipf("parse diags: %v", diagStrings(diags))
				return
			}
			_, rewrites := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})
			if len(rewrites) == 0 {
				goldenFile := filepath.Join(goldenDir, e.ID+".txt")
				// If a golden file exists but there are no rewrites, update or fail.
				if *update {
					os.Remove(goldenFile)
				} else if _, err := os.Stat(goldenFile); err == nil {
					t.Errorf("golden file %s exists but no rewrites produced", goldenFile)
				}
				return
			}

			var b strings.Builder
			for _, rw := range rewrites {
				fmt.Fprintf(&b, "%s: %s => %s\n", rw.Reason, rw.Before, rw.After)
			}
			got := b.String()
			goldenFile := filepath.Join(goldenDir, e.ID+".txt")
			compareOrUpdate(t, goldenFile, got)
		})
	}
}

// ---------------------------------------------------------------------------
// 5. Specific formatting tests
// ---------------------------------------------------------------------------

func TestFormat_MinimalParens(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// or vs and: no parens needed
		{"a or b and c", "a or b and c"},
		// Explicit paren changes meaning: (a or b) and c
		{"(a or b) and c", "(a or b) and c"},
		// Double nesting
		{"((a or b))", "a or b"},
		// not precedence
		{"not a and b", "not a and b"},
		{"not (a and b)", "not (a and b)"},
		// Arithmetic precedence
		{"a + b * c", "a + b * c"},
		{"(a + b) * c", "(a + b) * c"},
		// Left-associative: no parens on left
		{"(a + b) + c", "a + b + c"},
		// Right-nesting in left-assoc: needs parens
		{"a + (b + c)", "a + (b + c)"},
		// Comparison with arithmetic
		{"a + 1 > b * 2", "a + 1 > b * 2"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			e, d := parser.ParseExpr(tt.input)
			if len(d) > 0 {
				t.Fatalf("parse diags: %v", diagStrings(d))
			}
			got := Expr(e)
			if got != tt.want {
				t.Errorf("Expr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormat_MultiStagePipeline(t *testing.T) {
	input := `| where status >= 500 | stats count() by service | sort -count | head 10`
	q, d := parser.Parse(input)
	if len(d) > 0 {
		t.Fatalf("parse diags: %v", diagStrings(d))
	}
	got := Query(q)
	want := "| where status >= 500\n| stats count() by service\n| sort -count\n| head 10"
	if got != want {
		t.Errorf("Query output:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestFormat_SingleStagePipeline(t *testing.T) {
	input := `| head 10`
	q, d := parser.Parse(input)
	if len(d) > 0 {
		t.Fatalf("parse diags: %v", diagStrings(d))
	}
	got := Query(q)
	want := "head 10"
	if got != want {
		t.Errorf("Query output:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestFormat_LetCTE(t *testing.T) {
	input := `let $errs = from main | where level == "ERROR"; from $errs | stats count()`
	q, d := parser.Parse(input)
	if len(d) > 0 {
		t.Fatalf("parse diags: %v", diagStrings(d))
	}
	got := Query(q)
	want := "let $errs = from main | where level == \"ERROR\";\nfrom $errs\n| stats count()"
	if got != want {
		t.Errorf("Query output:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestFormat_StringEscaping(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", `"hello"`, `"hello"`},
		{"with_escape", `"line\n"`, `"line\n"`},
		{"with_backslash", `"C:\\Windows"`, `"C:\\Windows"`},
		{"with_tab", `"a\tb"`, `"a\tb"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, d := parser.ParseExpr(tt.input)
			if len(d) > 0 {
				t.Fatalf("parse diags: %v", diagStrings(d))
			}
			got := Expr(e)
			if got != tt.want {
				t.Errorf("Expr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormat_Literals(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"42", "42"},
		{"0x2A", "0x2A"},
		{"3.14", "3.14"},
		{"true", "true"},
		{"false", "false"},
		{"null", "null"},
		{"30s", "30s"},
		{"1h", "1h"},
		{`r"\d+"`, `r"\d+"`},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			e, d := parser.ParseExpr(tt.input)
			if len(d) > 0 {
				t.Fatalf("parse diags: %v", diagStrings(d))
			}
			got := Expr(e)
			if got != tt.want {
				t.Errorf("Expr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormat_BacktickIdent(t *testing.T) {
	e, d := parser.ParseExpr("`field-with-dash`")
	if len(d) > 0 {
		t.Fatalf("parse diags: %v", diagStrings(d))
	}
	got := Expr(e)
	if got != "`field-with-dash`" {
		t.Errorf("got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Golden test helpers
// ---------------------------------------------------------------------------

func compareOrUpdate(t *testing.T, path, got string) {
	t.Helper()
	if *update {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to generate)", path, err)
	}
	if string(want) != got {
		t.Errorf("golden mismatch %s:\n--- want ---\n%s\n--- got ---\n%s",
			path, string(want), got)
	}
}

func diagStrings(diags []parser.Diag) []string {
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = fmt.Sprintf("[%s] %s", d.Code, d.Message)
	}
	return msgs
}
