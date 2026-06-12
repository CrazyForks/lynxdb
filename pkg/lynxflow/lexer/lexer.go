package lexer

import (
	"strings"
	"unicode/utf8"
)

// Lexer is a pull-based lexer for the LynxFlow v2 query language (RFC-002).
// Create one with New and call Next repeatedly until EOF is returned.
// The lexer never panics; invalid input produces Error tokens.
type Lexer struct {
	src     string // full source text
	pos     int    // current byte offset
	pending *Token // buffered error from comment/whitespace skipping (e.g. unterminated block comment)
}

// New creates a Lexer for the given source text.
func New(src string) *Lexer {
	return &Lexer{src: src}
}

// Next returns the next token. After the end of input, every call returns EOF.
func (l *Lexer) Next() Token {
	// Drain any pending error injected during whitespace/comment skipping.
	if l.pending != nil {
		tok := *l.pending
		l.pending = nil
		return tok
	}

	l.skipWhitespaceAndComments()

	// Check for pending error from comment parsing.
	if l.pending != nil {
		tok := *l.pending
		l.pending = nil
		return tok
	}

	if l.pos >= len(l.src) {
		return Token{Kind: EOF, Start: l.pos, End: l.pos}
	}

	start := l.pos
	ch := l.src[l.pos]

	// --- raw string r"..." ------------------------------------------------
	// r immediately followed by " with no space.
	if ch == 'r' && l.pos+1 < len(l.src) && l.src[l.pos+1] == '"' {
		return l.lexRawString()
	}

	// --- identifiers / keywords -------------------------------------------
	if isIdentStart(ch) {
		return l.lexIdentOrKeyword()
	}

	// --- numbers and duration ---------------------------------------------
	if ch >= '0' && ch <= '9' {
		return l.lexNumber()
	}

	// --- strings ----------------------------------------------------------
	if ch == '"' {
		return l.lexString()
	}

	// --- backtick identifiers ---------------------------------------------
	if ch == '`' {
		return l.lexBacktickIdent()
	}

	// --- single-quote error (D18) -----------------------------------------
	if ch == '\'' {
		l.pos++
		return Token{Kind: Error, Start: start, End: l.pos,
			Text: "single quotes are not allowed; use double quotes for strings or backticks for identifiers"}
	}

	// --- multi-character operators -----------------------------------------
	if l.pos+1 < len(l.src) {
		two := l.src[l.pos : l.pos+2]
		switch two {
		case "==":
			l.pos += 2
			return Token{Kind: EqEq, Start: start, End: l.pos, Text: "=="}
		case "!=":
			l.pos += 2
			return Token{Kind: BangEq, Start: start, End: l.pos, Text: "!="}
		case "<=":
			l.pos += 2
			return Token{Kind: LtEq, Start: start, End: l.pos, Text: "<="}
		case ">=":
			l.pos += 2
			return Token{Kind: GtEq, Start: start, End: l.pos, Text: ">="}
		case "??":
			l.pos += 2
			return Token{Kind: Coalesce, Start: start, End: l.pos, Text: "??"}
		case "?.":
			l.pos += 2
			return Token{Kind: SafeNav, Start: start, End: l.pos, Text: "?."}
		case "->":
			l.pos += 2
			return Token{Kind: Arrow, Start: start, End: l.pos, Text: "->"}
		case "..":
			l.pos += 2
			return Token{Kind: DotDot, Start: start, End: l.pos, Text: ".."}
		}
	}

	// --- single-character tokens -------------------------------------------
	l.pos++
	switch ch {
	case '|':
		return Token{Kind: Pipe, Start: start, End: l.pos, Text: "|"}
	case ',':
		return Token{Kind: Comma, Start: start, End: l.pos, Text: ","}
	case '(':
		return Token{Kind: LParen, Start: start, End: l.pos, Text: "("}
	case ')':
		return Token{Kind: RParen, Start: start, End: l.pos, Text: ")"}
	case '[':
		return Token{Kind: LBracket, Start: start, End: l.pos, Text: "["}
	case ']':
		return Token{Kind: RBracket, Start: start, End: l.pos, Text: "]"}
	case '{':
		return Token{Kind: LBrace, Start: start, End: l.pos, Text: "{"}
	case '}':
		return Token{Kind: RBrace, Start: start, End: l.pos, Text: "}"}
	case '=':
		return Token{Kind: Eq, Start: start, End: l.pos, Text: "="}
	case '<':
		return Token{Kind: Lt, Start: start, End: l.pos, Text: "<"}
	case '>':
		return Token{Kind: Gt, Start: start, End: l.pos, Text: ">"}
	case '+':
		return Token{Kind: Plus, Start: start, End: l.pos, Text: "+"}
	case '-':
		return Token{Kind: Minus, Start: start, End: l.pos, Text: "-"}
	case '*':
		return Token{Kind: Star, Start: start, End: l.pos, Text: "*"}
	case '/':
		return Token{Kind: Slash, Start: start, End: l.pos, Text: "/"}
	case '%':
		return Token{Kind: Percent, Start: start, End: l.pos, Text: "%"}
	case '?':
		return Token{Kind: Question, Start: start, End: l.pos, Text: "?"}
	case '.':
		return Token{Kind: Dot, Start: start, End: l.pos, Text: "."}
	case ':':
		return Token{Kind: Colon, Start: start, End: l.pos, Text: ":"}
	case ';':
		return Token{Kind: Semicolon, Start: start, End: l.pos, Text: ";"}
	case '@':
		return Token{Kind: At, Start: start, End: l.pos, Text: "@"}
	case '$':
		return Token{Kind: Dollar, Start: start, End: l.pos, Text: "$"}
	case '!':
		return Token{Kind: Bang, Start: start, End: l.pos, Text: "!"}
	case '\\':
		return Token{Kind: Backslash, Start: start, End: l.pos, Text: `\`}
	}

	// Unknown character -- emit error and skip one rune.
	_, size := utf8.DecodeRuneInString(l.src[start:])
	// We already advanced by 1 byte; if the rune is multi-byte advance the rest.
	if size > 1 {
		l.pos = start + size
	}
	return Token{Kind: Error, Start: start, End: l.pos,
		Text: "unexpected character: " + l.src[start:l.pos]}
}

// Lex tokenises the entire input and returns all tokens (including EOF) plus
// any diagnostics. This is a convenience wrapper over the pull API.
func Lex(input string) ([]Token, []Diag) {
	lex := New(input)
	// Pre-allocate a reasonable capacity to reduce slice growth for typical queries.
	tokens := make([]Token, 0, 32)
	var diags []Diag
	for {
		tok := lex.Next()
		tokens = append(tokens, tok)
		if tok.Kind == Error {
			diags = append(diags, Diag{
				Span:    Span{Start: tok.Start, End: tok.End},
				Message: tok.Text,
			})
		}
		if tok.Kind == EOF {
			break
		}
	}
	return tokens, diags
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// skipWhitespaceAndComments advances past whitespace and comments.
// Line comments: // to end-of-line.
// Block comments: /* */ nestable. Unterminated block comment produces an Error
// token stored as l.pending.
func (l *Lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.src) {
		ch := l.src[l.pos]

		// Whitespace.
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			l.pos++
			continue
		}

		// Not a slash -- nothing more to skip.
		if ch != '/' || l.pos+1 >= len(l.src) {
			return
		}

		next := l.src[l.pos+1]

		// Line comment.
		if next == '/' {
			l.pos += 2
			for l.pos < len(l.src) && l.src[l.pos] != '\n' {
				l.pos++
			}
			continue
		}

		// Block comment (nestable).
		if next == '*' {
			l.skipBlockComment()
			if l.pending != nil {
				return // unterminated block comment -- surface error
			}
			continue
		}

		// Just a slash -- not a comment.
		return
	}
}

// skipBlockComment consumes a (possibly nested) /* */ comment. If the comment
// is unterminated, stores a pending Error token.
func (l *Lexer) skipBlockComment() {
	start := l.pos
	l.pos += 2 // skip opening /*
	depth := 1
	for l.pos < len(l.src) && depth > 0 {
		if l.pos+1 < len(l.src) {
			two := l.src[l.pos : l.pos+2]
			if two == "/*" {
				depth++
				l.pos += 2
				continue
			}
			if two == "*/" {
				depth--
				l.pos += 2
				continue
			}
		}
		l.pos++
	}
	if depth > 0 {
		l.pending = &Token{Kind: Error, Start: start, End: l.pos,
			Text: "unterminated block comment"}
	}
}

// lexIdentOrKeyword lexes a bare identifier or keyword.
func (l *Lexer) lexIdentOrKeyword() Token {
	start := l.pos
	for l.pos < len(l.src) && isIdentContinue(l.src[l.pos]) {
		l.pos++
	}
	text := l.src[start:l.pos]
	lower := strings.ToLower(text)
	if kind, ok := keywords[lower]; ok {
		return Token{Kind: kind, Start: start, End: l.pos, Text: text}
	}
	return Token{Kind: Ident, Start: start, End: l.pos, Text: text}
}

// lexNumber lexes an integer, float, hex integer, or duration literal.
//
// Ambiguity rules (RFC-002):
//   - 0x2a  -> hex Int
//   - 1e-6  -> Float (exponent)
//   - 1.5h  -> Duration (float + duration unit, no space)
//   - 5m    -> Duration (int + duration unit, no space)
//   - 5 m   -> Int then Ident (space separates)
//   - 1..2  -> Int 1, DotDot, Int 2 (no float "1." -- floats require digit after dot)
//   - 3.14  -> Float
func (l *Lexer) lexNumber() Token {
	start := l.pos

	// Hex literal: 0x...
	if l.src[l.pos] == '0' && l.pos+1 < len(l.src) && (l.src[l.pos+1] == 'x' || l.src[l.pos+1] == 'X') {
		l.pos += 2
		if l.pos >= len(l.src) || !isHexDigit(l.src[l.pos]) {
			return Token{Kind: Error, Start: start, End: l.pos,
				Text: "invalid hex literal: expected hex digits after 0x"}
		}
		for l.pos < len(l.src) && isHexDigit(l.src[l.pos]) {
			l.pos++
		}
		return Token{Kind: Int, Start: start, End: l.pos, Text: l.src[start:l.pos]}
	}

	// Decimal digits.
	for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
		l.pos++
	}

	isFloat := false

	// Fractional part -- but NOT if next two chars are ".." (range operator).
	if l.pos < len(l.src) && l.src[l.pos] == '.' {
		// Check for ".." (range) -- do NOT consume the dot.
		if l.pos+1 < len(l.src) && l.src[l.pos+1] == '.' {
			// 1..2 -- leave as int, DotDot will be next.
		} else if l.pos+1 < len(l.src) && l.src[l.pos+1] >= '0' && l.src[l.pos+1] <= '9' {
			// 3.14 -- consume dot and fractional digits.
			isFloat = true
			l.pos++ // consume '.'
			for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
				l.pos++
			}
		}
		// else: bare "1." without following digit or ".." -- leave as int, dot is separate token.
	}

	// Exponent part.
	if l.pos < len(l.src) && (l.src[l.pos] == 'e' || l.src[l.pos] == 'E') {
		// Peek: must be followed by optional sign then digits.
		ep := l.pos + 1
		if ep < len(l.src) && (l.src[ep] == '+' || l.src[ep] == '-') {
			ep++
		}
		if ep < len(l.src) && l.src[ep] >= '0' && l.src[ep] <= '9' {
			isFloat = true
			l.pos = ep
			for l.pos < len(l.src) && l.src[l.pos] >= '0' && l.src[l.pos] <= '9' {
				l.pos++
			}
		}
		// else: "1e" without digits -- the 'e' is not part of this number.
	}

	// Duration suffix? Must be immediately after digits/float with no space.
	// Duration units: ns, us, ms, s, m, h, d, w.
	if unit := l.tryDurationUnit(); unit != "" {
		return Token{Kind: Duration, Start: start, End: l.pos, Text: l.src[start:l.pos]}
	}

	if isFloat {
		return Token{Kind: Float, Start: start, End: l.pos, Text: l.src[start:l.pos]}
	}
	return Token{Kind: Int, Start: start, End: l.pos, Text: l.src[start:l.pos]}
}

// tryDurationUnit checks whether the current position begins a duration unit
// suffix (immediately adjacent to digits). If so, advances pos past the unit
// and returns the unit string. Otherwise returns "".
//
// Units checked longest-first for maximal munch: ns, us, ms, then s, m, h, d, w.
func (l *Lexer) tryDurationUnit() string {
	if l.pos >= len(l.src) {
		return ""
	}

	remaining := len(l.src) - l.pos

	// Two-character units.
	if remaining >= 2 {
		two := l.src[l.pos : l.pos+2]
		switch two {
		case "ns", "us", "ms":
			// Make sure the unit isn't followed by more ident chars
			// (e.g., "5message" should not match "5m" + "essage").
			if remaining == 2 || !isIdentContinue(l.src[l.pos+2]) {
				l.pos += 2
				return two
			}
		}
	}

	// Single-character units.
	ch := l.src[l.pos]
	switch ch {
	case 's', 'm', 'h', 'd', 'w':
		if remaining == 1 || !isIdentContinue(l.src[l.pos+1]) {
			l.pos++
			return string(ch)
		}
	}

	return ""
}

// lexString lexes a double-quoted string with escape processing.
// Escapes: \" \\ \n \t \r \u{NNNN}
func (l *Lexer) lexString() Token {
	start := l.pos
	l.pos++ // skip opening "

	for l.pos < len(l.src) {
		ch := l.src[l.pos]
		if ch == '"' {
			l.pos++ // skip closing "
			return Token{Kind: String, Start: start, End: l.pos, Text: l.src[start:l.pos]}
		}
		if ch == '\\' {
			l.pos++
			if l.pos >= len(l.src) {
				return Token{Kind: Error, Start: start, End: l.pos,
					Text: "unterminated string literal"}
			}
			esc := l.src[l.pos]
			switch esc {
			case '"', '\\', 'n', 't', 'r':
				l.pos++
			case 'u':
				// Expect \u{NNNN} -- 1 to 6 hex digits inside braces.
				l.pos++
				if l.pos >= len(l.src) || l.src[l.pos] != '{' {
					return Token{Kind: Error, Start: start, End: l.pos,
						Text: `bad \u escape: expected \u{NNNN}`}
				}
				l.pos++ // skip '{'
				hexStart := l.pos
				for l.pos < len(l.src) && isHexDigit(l.src[l.pos]) {
					l.pos++
				}
				if l.pos == hexStart || l.pos >= len(l.src) || l.src[l.pos] != '}' {
					return Token{Kind: Error, Start: start, End: l.pos,
						Text: `bad \u escape: expected 1-6 hex digits inside braces \u{NNNN}`}
				}
				if l.pos-hexStart > 6 {
					return Token{Kind: Error, Start: start, End: l.pos,
						Text: `bad \u escape: too many hex digits (max 6)`}
				}
				l.pos++ // skip '}'
			default:
				return Token{Kind: Error, Start: start, End: l.pos,
					Text: "invalid escape sequence: \\" + string(esc)}
			}
			continue
		}
		l.pos++
	}
	return Token{Kind: Error, Start: start, End: l.pos,
		Text: "unterminated string literal"}
}

// lexRawString lexes r"..." -- the r must already be at l.pos, followed by ".
func (l *Lexer) lexRawString() Token {
	start := l.pos
	l.pos += 2 // skip r"
	for l.pos < len(l.src) {
		if l.src[l.pos] == '"' {
			l.pos++
			return Token{Kind: RawString, Start: start, End: l.pos, Text: l.src[start:l.pos]}
		}
		l.pos++
	}
	return Token{Kind: Error, Start: start, End: l.pos,
		Text: "unterminated raw string literal"}
}

// lexBacktickIdent lexes `...` backtick-quoted identifiers.
func (l *Lexer) lexBacktickIdent() Token {
	start := l.pos
	l.pos++ // skip opening `
	for l.pos < len(l.src) {
		if l.src[l.pos] == '`' {
			l.pos++
			if l.pos-start == 2 {
				return Token{Kind: Error, Start: start, End: l.pos,
					Text: "empty backtick-quoted identifier"}
			}
			return Token{Kind: BacktickIdent, Start: start, End: l.pos, Text: l.src[start:l.pos]}
		}
		l.pos++
	}
	return Token{Kind: Error, Start: start, End: l.pos,
		Text: "unterminated backtick-quoted identifier"}
}

// ---------------------------------------------------------------------------
// Character classification
// ---------------------------------------------------------------------------

// isIdentStart reports whether b can start a bare identifier.
// Only ASCII letters and underscore.
func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

// isIdentContinue reports whether b can continue a bare identifier.
// ASCII letters, digits, and underscore.
func isIdentContinue(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

// isHexDigit reports whether b is a valid hexadecimal digit.
func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}
