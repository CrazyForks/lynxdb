package opt

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test: predicate-pushdown — has_glob() -> TokenGlobs
// ---------------------------------------------------------------------------

func TestPredicatePushdown_TokenGlobs(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		contains    []string
		notContains []string
	}{
		{
			name:  "bare_glob_sugar",
			query: `from main us*r`,
			contains: []string{
				`pushdown.token_glob: "us*r"`,
				`Filter(has_glob(_raw, "us*r"))`,
			},
			// The glob's literal fragments must NOT leak into exact-term or
			// bloom pushdown — that is the original false-prune bug.
			notContains: []string{
				`pushdown.raw_term`,
				`pushdown.bloom_term`,
			},
		},
		{
			name:  "explicit_has_glob_lowercased",
			query: `from main | where has_glob(_raw, "US*R")`,
			contains: []string{
				`pushdown.token_glob: "us*r"`,
			},
		},
		{
			name:  "glob_composes_with_terms",
			query: `from main user err*r`,
			contains: []string{
				`pushdown.raw_term: "user"`,
				`pushdown.token_glob: "err*r"`,
			},
		},
		{
			name:  "leading_meta_glob",
			query: `from main *user`,
			contains: []string{
				`pushdown.token_glob: "*user"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dump := optimizedDump(t, tt.query)
			for _, c := range tt.contains {
				if !strings.Contains(dump, c) {
					t.Errorf("dump does not contain %q\nGot:\n%s", c, dump)
				}
			}
			for _, c := range tt.notContains {
				if strings.Contains(dump, c) {
					t.Errorf("dump must NOT contain %q\nGot:\n%s", c, dump)
				}
			}
		})
	}
}
