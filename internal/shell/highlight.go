package shell

// HighlightSPL2 applies syntax highlighting to a query string.
// RFC-002 Phase 10: spl2 lexer removed; returns input unstyled.
// TODO(RFC-002): reimplement with lynxflow lexer.
func HighlightSPL2(input string, _ *ShellTheme) string {
	return input
}
