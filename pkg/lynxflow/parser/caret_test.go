package parser

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
)

func TestRenderDiag_SimpleError(t *testing.T) {
	source := `from main | stats cunt()`
	d := Diag{
		Code:     CodeUnexpectedToken,
		Severity: SeverityError,
		Span:     ast.Span{Start: 18, End: 22},
		Message:  "unknown function \"cunt\"",
	}

	got := RenderDiag(source, d)
	want := `error[E001] at 1:19: unknown function "cunt"
  from main | stats cunt()
                    ^~~~
`
	if got != want {
		t.Errorf("RenderDiag:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDiag_WithSuggestion(t *testing.T) {
	source := `from main | where status = 500`
	d := Diag{
		Code:       CodeSingleEquals,
		Severity:   SeverityError,
		Span:       ast.Span{Start: 25, End: 26},
		Message:    "single = is assignment; use == for comparison",
		Suggestion: "change = to ==",
	}

	got := RenderDiag(source, d)
	want := `error[E003] at 1:26: single = is assignment; use == for comparison
  from main | where status = 500
                           ^
  suggestion: change = to ==
`
	if got != want {
		t.Errorf("RenderDiag:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDiag_ZeroWidthSpan(t *testing.T) {
	source := `from main |`
	d := Diag{
		Code:     CodeUnexpectedToken,
		Severity: SeverityError,
		Span:     ast.Span{Start: 11, End: 11},
		Message:  "unexpected end of input",
	}

	got := RenderDiag(source, d)
	want := `error[E001] at 1:12: unexpected end of input
  from main |
             ^
`
	if got != want {
		t.Errorf("RenderDiag:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDiag_MultiLineQuery(t *testing.T) {
	source := "from main\n| stats cunt()"
	d := Diag{
		Code:     CodeUnexpectedToken,
		Severity: SeverityError,
		Span:     ast.Span{Start: 18, End: 22},
		Message:  "unknown function",
	}

	got := RenderDiag(source, d)
	want := `error[E001] at 2:9: unknown function
  | stats cunt()
          ^~~~
`
	if got != want {
		t.Errorf("RenderDiag:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderDiag_Warning(t *testing.T) {
	source := `from main | stats count()`
	d := Diag{
		Code:     "W001",
		Severity: SeverityWarning,
		Span:     ast.Span{Start: 0, End: 9},
		Message:  "explicit source recommended",
	}

	got := RenderDiag(source, d)
	want := `warning[W001] at 1:1: explicit source recommended
  from main | stats count()
  ^~~~~~~~~
`
	if got != want {
		t.Errorf("RenderDiag:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
