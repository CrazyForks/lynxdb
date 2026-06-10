package parser

import (
	"fmt"
	"strings"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
	"github.com/lynxbase/lynxdb/pkg/lynxflow/lexer"
)

// RenderDiag renders a human-readable diagnostic with caret underlining.
// The output has the form:
//
//	error[E001] at 1:15: unexpected token
//	  from main | stats cunt()
//	                    ^~~~
//
// Multi-line queries show only the line containing the span start.
// If span is zero-width, a single ^ is shown.
func RenderDiag(source string, d Diag) string {
	return renderDiagSpan(source, d.Severity, d.Code, d.Span, d.Message, d.Suggestion)
}

// renderDiagSpan is the implementation shared by RenderDiag.
func renderDiagSpan(source string, sev Severity, code DiagCode, span ast.Span, message, suggestion string) string {
	var b strings.Builder

	line, col := lexer.LineCol(source, span.Start)

	// Header: severity[code] at line:col: message
	fmt.Fprintf(&b, "%s[%s] at %d:%d: %s\n", sev, code, line, col, message)

	// Extract the source line containing the span start.
	srcLine := extractLine(source, span.Start)
	if srcLine != "" {
		b.WriteString("  ")
		b.WriteString(srcLine)
		b.WriteByte('\n')

		// Caret underline: col-1 spaces + ^~~~
		width := span.End - span.Start
		if width <= 0 {
			width = 1
		}
		// Clamp width to line length to avoid overrun.
		lineRemaining := len(srcLine) - (col - 1)
		if lineRemaining < 0 {
			lineRemaining = 0
		}
		if width > lineRemaining {
			width = lineRemaining
		}
		if width < 1 {
			width = 1
		}

		b.WriteString("  ")
		b.WriteString(strings.Repeat(" ", col-1))
		b.WriteByte('^')
		if width > 1 {
			b.WriteString(strings.Repeat("~", width-1))
		}
		b.WriteByte('\n')
	}

	if suggestion != "" {
		fmt.Fprintf(&b, "  suggestion: %s\n", suggestion)
	}

	return b.String()
}

// extractLine returns the text of the line containing the byte offset.
func extractLine(source string, offset int) string {
	if offset > len(source) {
		offset = len(source)
	}
	// Find line start.
	lineStart := 0
	for i := offset - 1; i >= 0; i-- {
		if source[i] == '\n' {
			lineStart = i + 1
			break
		}
	}
	// Find line end.
	lineEnd := len(source)
	for i := offset; i < len(source); i++ {
		if source[i] == '\n' {
			lineEnd = i
			break
		}
	}
	return source[lineStart:lineEnd]
}
