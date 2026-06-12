//go:build e2e

package e2e

import (
	"testing"
)

// TestE2E_SourceDebug_DirectNormalizedQuery tests various query forms for
// multi-source filtering. All queries use the LynxFlow syntax.
func TestE2E_SourceDebug_DirectNormalizedQuery(t *testing.T) {
	h := setupMultiSource(t)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		// Baseline: from * without filter returns all events
		{"from * all", `from * | stats count() as count`, 18},
		// from specific index
		{"from nginx", `from nginx | stats count() as count`, 10},
		// from * with index!= filter
		{"from * where index!=redis", `from * | where index != "redis" | stats count() as count`, 15},
		// from * with _source= filter
		{"from * where _source=nginx", `from * | where _source == "nginx" | stats count() as count`, 10},
		// Try using _source field name (the canonical field name for source)
		{"from * where _source=nginx", `from * | where _source == "nginx" | stats count() as count`, 10},
		// Check: does WHERE index=="nginx" also work?
		{"from * where index=nginx", `from * | where index == "nginx" | stats count() as count`, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := h.MustQuery(tt.query)
			got := GetInt(r, "count")
			if got != tt.want {
				t.Errorf("query %q: got count=%d, want %d", tt.query, got, tt.want)
			}
		})
	}
}
