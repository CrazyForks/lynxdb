package logical_test

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/logical"
	"github.com/lynxbase/lynxdb/pkg/logical/opt"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/desugar"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/parser"
)

// parseLowerOpt is the standard pipeline: parse -> desugar -> lower -> optimize.
func parseLowerOpt(t *testing.T, query string) *logical.Plan {
	t.Helper()
	q, diags := parser.Parse(query)
	for _, d := range diags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("parse error: %s", d.Message)
		}
	}
	desugared, _ := desugar.Desugar(q, desugar.Options{DefaultSource: "main"})
	plan, lowerDiags := logical.Lower(desugared, logical.Options{DefaultSource: "main"})
	for _, d := range lowerDiags {
		if d.Severity == parser.SeverityError {
			t.Fatalf("lower error: %s", d.Message)
		}
	}
	plan, _ = opt.Optimize(plan)
	return plan
}

// TestRenderPipeline_RoundTrip verifies the round-trip contract:
// parse -> desugar -> lower -> optimize -> render -> re-parse -> lower -> Dump
// produces the same plan (modulo pushdown re-derivation).
func TestRenderPipeline_RoundTrip(t *testing.T) {
	queries := []string{
		`from main | where status >= 500`,
		`from main | stats count() by level`,
		`from main | where status >= 500 | stats count() as errors by source`,
		`from main | sort -_time`,
		`from main | head 10`,
		`from main | extend svc = source | stats count() by svc`,
		`from main | dedup 1 host`,
		`from main | stats count() by source | sort -count() | head 5`,
		`from main | where level == "error" | stats avg(duration) by endpoint`,
	}

	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			plan1 := parseLowerOpt(t, q)
			rendered := logical.RenderPlan(plan1)
			if rendered == "" {
				t.Fatalf("RenderPlan returned empty string for %q", q)
			}
			t.Logf("rendered: %s", rendered)

			// Re-parse the rendered text.
			plan2 := parseLowerOpt(t, rendered)

			// Compare Dump output. We strip pushdown annotations because
			// they are re-derived by the optimizer and may differ.
			dump1 := stripPushdown(plan1.Dump())
			dump2 := stripPushdown(plan2.Dump())

			if dump1 != dump2 {
				t.Errorf("round-trip mismatch:\noriginal dump:\n%s\nre-parsed dump:\n%s\nrendered text: %s",
					dump1, dump2, rendered)
			}
		})
	}
}

// stripPushdown removes pushdown annotation lines from a Dump string.
func stripPushdown(dump string) string {
	var result []byte
	lines := splitLines(dump)
	for _, line := range lines {
		// Skip pushdown annotation lines.
		trimmed := trimLeadingSpaces(line)
		if len(trimmed) > 8 && string(trimmed[:9]) == "pushdown." {
			continue
		}
		result = append(result, line...)
		result = append(result, '\n')
	}
	return string(result)
}

func splitLines(s string) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, []byte(s[start:i]))
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, []byte(s[start:]))
	}
	return lines
}

func trimLeadingSpaces(b []byte) []byte {
	i := 0
	for i < len(b) && b[i] == ' ' {
		i++
	}
	return b[i:]
}

func TestRenderPipeline_EmptyPlan(t *testing.T) {
	result := logical.RenderPipeline()
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestRenderPlan_Nil(t *testing.T) {
	result := logical.RenderPlan(nil)
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}
