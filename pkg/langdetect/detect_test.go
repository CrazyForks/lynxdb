package langdetect

import (
	"testing"
)

func TestDetect_ExplicitLynxFlow(t *testing.T) {
	r := Detect("from main | stats count()", "lynxflow")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
	if !r.Explicit {
		t.Fatal("expected explicit=true")
	}
}

func TestDetect_ExplicitSPL2_ReturnsLynxFlow(t *testing.T) {
	// Post-RFC-002: explicit spl2 is rejected at the API layer.
	// Detect itself maps everything to lynxflow.
	r := Detect("index=main | stats count", "spl2")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
}

func TestDetect_LynxFlowOnlyQuery(t *testing.T) {
	// CTEs are lynxflow-only syntax.
	r := Detect("let $x = from main | stats count(); from $x", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
}

func TestDetect_DefaultsToLynxFlow(t *testing.T) {
	// Post-RFC-002: all queries default to lynxflow.
	r := Detect("index=main | stats count", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
}

func TestDetectStrict_DefaultsToLynxFlow(t *testing.T) {
	// Post-RFC-002: DetectStrict also defaults to lynxflow.
	r := DetectStrict("from main | stats count()", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow (strict mode)", r.Language)
	}
}

func TestDetectStrict_LynxFlowOnlyRoutes(t *testing.T) {
	r := DetectStrict("let $x = from main | stats count(); from $x", "")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
}

func TestDetectStrict_ExplicitLynxFlow(t *testing.T) {
	r := DetectStrict("from main | stats count()", "lynxflow")
	if r.Language != LangLynxFlow {
		t.Fatalf("language: got %s, want lynxflow", r.Language)
	}
	if !r.Explicit {
		t.Fatal("expected explicit=true")
	}
}

func TestValidateExplicitLanguage(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"lynxflow", ""},
		{"LynxFlow", ""},
		{"spl2", `language "spl2" is no longer supported; migrate queries to LynxFlow — see CHANGELOG for migration guide`},
		{"SPL2", `language "spl2" is no longer supported; migrate queries to LynxFlow — see CHANGELOG for migration guide`},
		{"invalid", `invalid language: must be "lynxflow"`},
	}
	for _, tt := range tests {
		got := ValidateExplicitLanguage(tt.input)
		if got != tt.want {
			t.Errorf("ValidateExplicitLanguage(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
