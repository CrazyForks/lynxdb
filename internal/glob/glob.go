// Package glob provides RFC-style glob pattern matching used across the
// codebase. The implementation supports *, **, ?, character classes [abc],
// negated classes [!abc], and brace alternatives {a,b,c}.
//
// This package was extracted from pkg/spl2/search_eval.go so that non-query
// packages (storage, server, pipeline) can use glob matching without depending
// on the SPL2 parser.
package glob

import (
	"regexp"
	"strings"
	"sync"
)

// globRegexCache is a process-wide cache of compiled glob regexes.
// Soft-capped to avoid unbounded growth from pathological inputs;
// once full, additional patterns compile every call but are not stored.
var globRegexCache = struct {
	mu  sync.RWMutex
	m   map[string]*regexp.Regexp
	max int
}{m: make(map[string]*regexp.Regexp), max: 4096}

func compileGlobCached(key string, compile func() *regexp.Regexp) *regexp.Regexp {
	globRegexCache.mu.RLock()
	if re, ok := globRegexCache.m[key]; ok {
		globRegexCache.mu.RUnlock()

		return re
	}
	globRegexCache.mu.RUnlock()

	re := compile()
	globRegexCache.mu.Lock()
	if existing, ok := globRegexCache.m[key]; ok {
		globRegexCache.mu.Unlock()

		return existing
	}
	if len(globRegexCache.m) < globRegexCache.max {
		globRegexCache.m[key] = re
	}
	globRegexCache.mu.Unlock()

	return re
}

// ToRegex converts a glob pattern to a compiled regular expression.
// The result is anchored with ^ and $. Single * matches any character except
// /, while ** matches anything including /. When caseInsensitive is true the
// regex uses the (?i) flag.
func ToRegex(pattern string, caseInsensitive bool) *regexp.Regexp {
	return toRegex(pattern, caseInsensitive, true)
}

// Match reports whether text matches an RFC glob pattern.
// This is a convenience wrapper around ToRegex.
func Match(pattern, text string, caseInsensitive bool) bool {
	return ToRegex(pattern, caseInsensitive).MatchString(text)
}

// MatchCached reports whether text matches a glob pattern, using a
// process-wide regex cache keyed on (pattern, caseInsensitive).
func MatchCached(pattern, text string, caseInsensitive bool) bool {
	key := "s:" + pattern
	if caseInsensitive {
		key = "i:" + pattern
	}

	ci := caseInsensitive
	compiled := compileGlobCached(key, func() *regexp.Regexp {
		return ToRegex(pattern, ci)
	})

	return compiled.MatchString(text)
}

func toRegex(pattern string, caseInsensitive bool, anchored bool) *regexp.Regexp {
	return toRegexWithMode(pattern, caseInsensitive, anchored, false)
}

func toRegexWithMode(pattern string, caseInsensitive bool, anchored bool, wildcardMatchesSlash bool) *regexp.Regexp {
	var buf strings.Builder
	if caseInsensitive {
		buf.WriteString("(?i)")
	}
	if anchored {
		buf.WriteString("^")
	}
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				buf.WriteString(".*")
				i++
			} else if wildcardMatchesSlash {
				buf.WriteString(".*")
			} else {
				buf.WriteString("[^/]*")
			}
		case '?':
			if wildcardMatchesSlash {
				buf.WriteByte('.')
			} else {
				buf.WriteString("[^/]")
			}
		case '[':
			next, ok := appendGlobClass(&buf, pattern, i)
			if ok {
				i = next
			} else {
				buf.WriteString(`\[`)
			}
		case '{':
			next, ok := appendGlobAlternatives(&buf, pattern, i)
			if ok {
				i = next
			} else {
				buf.WriteString(`\{`)
			}
		case '\\':
			if i+1 < len(pattern) {
				next := pattern[i+1]
				if isEscapableGlobChar(next) {
					i++
					buf.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
				} else {
					buf.WriteString(`\\`)
				}
			} else {
				buf.WriteString(`\\`)
			}
		case '.', '(', ')', ']', '}', '+', '^', '$', '|':
			buf.WriteByte('\\')
			buf.WriteByte(ch)
		default:
			buf.WriteByte(ch)
		}
	}
	if anchored {
		buf.WriteString("$")
	}

	return regexp.MustCompile(buf.String())
}

// ToContainsRegex converts a glob pattern to a regex without anchoring
// (substring match).
func ToContainsRegex(pattern string, caseInsensitive bool) *regexp.Regexp {
	return toRegexWithMode(pattern, caseInsensitive, false, true)
}

func isEscapableGlobChar(ch byte) bool {
	switch ch {
	case '*', '?', '[', ']', '{', '}', '\\':
		return true
	default:
		return false
	}
}

func appendGlobClass(buf *strings.Builder, pattern string, start int) (int, bool) {
	i := start + 1
	if i >= len(pattern) {
		return start, false
	}

	var class strings.Builder
	class.WriteByte('[')
	switch pattern[i] {
	case '!':
		class.WriteByte('^')
		i++
	case '^':
		class.WriteByte('\\')
		class.WriteByte('^')
		i++
	}

	for ; i < len(pattern); i++ {
		ch := pattern[i]
		if ch == ']' {
			class.WriteByte(']')
			buf.WriteString(class.String())

			return i, true
		}
		if ch == '\\' {
			if i+1 >= len(pattern) {
				class.WriteString(`\\`)
				continue
			}
			i++
			ch = pattern[i]
		}
		class.WriteByte(ch)
	}

	return start, false
}

func appendGlobAlternatives(buf *strings.Builder, pattern string, start int) (int, bool) {
	i := start + 1
	var parts []string
	var part strings.Builder

	for ; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '\\':
			if i+1 < len(pattern) {
				i++
				part.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
			} else {
				part.WriteString(`\\`)
			}
		case ',':
			parts = append(parts, part.String())
			part.Reset()
		case '}':
			parts = append(parts, part.String())
			if len(parts) < 2 {
				return start, false
			}
			buf.WriteString("(?:")
			for j, alt := range parts {
				if j > 0 {
					buf.WriteByte('|')
				}
				buf.WriteString(alt)
			}
			buf.WriteByte(')')

			return i, true
		default:
			part.WriteString(regexp.QuoteMeta(pattern[i : i+1]))
		}
	}

	return start, false
}
