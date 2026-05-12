package spl2

import "strings"

// QueryLint is a post-parse warning for syntactically valid queries.
type QueryLint struct {
	Code     string
	Message  string
	Position int
}

const (
	LintCountWithoutParens = "L013"
)

// LintQuery parses input and returns RFC lint warnings for valid queries.
func LintQuery(input string) ([]QueryLint, error) {
	prog, err := ParseProgram(input)
	if err != nil {
		return nil, err
	}

	return LintProgram(input, prog)
}

// LintProgram returns RFC lint warnings for an already parsed program.
func LintProgram(input string, prog *Program) ([]QueryLint, error) {
	if prog == nil {
		return nil, nil
	}

	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, err
	}

	return lintCountWithoutParens(tokens), nil
}

func lintCountWithoutParens(tokens []Token) []QueryLint {
	var lints []QueryLint
	inAggCommand := false
	afterBy := false

	for i, tok := range tokens {
		switch tok.Type {
		case TokenPipe, TokenRBracket, TokenEOF:
			inAggCommand = false
			afterBy = false
			continue
		}

		if isAggregateCommandToken(tok.Type) {
			inAggCommand = true
			afterBy = false
			continue
		}

		if !inAggCommand {
			continue
		}

		if tok.Type == TokenBy {
			afterBy = true
			continue
		}

		if afterBy {
			continue
		}

		if strings.EqualFold(tok.Literal, "count") && peekTokenType(tokens, i+1) != TokenLParen {
			lints = append(lints, QueryLint{
				Code:     LintCountWithoutParens,
				Message:  "`count` is a function; use `count()`",
				Position: tok.Pos,
			})
		}
	}

	return lints
}

func isAggregateCommandToken(t TokenType) bool {
	switch t {
	case TokenStats, TokenTimechart, TokenStreamstats, TokenEventstats,
		TokenRunning, TokenEnrich, TokenEvery, TokenImpact:
		return true
	default:
		return false
	}
}

func peekTokenType(tokens []Token, idx int) TokenType {
	if idx < 0 || idx >= len(tokens) {
		return TokenEOF
	}

	return tokens[idx].Type
}
