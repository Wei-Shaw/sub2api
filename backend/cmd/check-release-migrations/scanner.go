package main

import (
	"bytes"
	"fmt"
	"strings"
)

const reviewedCompatibleAnnotation = "sub2api-managed-update: reviewed-compatible"
const postgresIdentifierMaxBytes = 63

type tokenKind uint8

const (
	tokenWord tokenKind = iota
	tokenQuotedIdentifier
	tokenUnicodeIdentifier
	tokenString
	tokenNumber
	tokenSymbol
)

type sqlToken struct {
	kind tokenKind
	text string
	raw  string
}

type sqlStatement struct {
	tokens             []sqlToken
	startLine          int
	reviewedCompatible bool
}

// scanSQLStatements splits PostgreSQL statements without interpreting quoted
// content or comments. Dollar-quoted function bodies and nested block comments
// are consumed as single lexical regions, so their semicolons cannot escape
// into the top-level policy classifier.
func scanSQLStatements(src []byte) ([]sqlStatement, error) {
	var (
		statements []sqlStatement
		current    sqlStatement
		line       = 1
		started    bool
	)

	addToken := func(tok sqlToken) {
		if !started {
			current.startLine = line
			started = true
		}
		current.tokens = append(current.tokens, tok)
	}
	finishStatement := func() {
		if len(current.tokens) > 0 {
			statements = append(statements, current)
		}
		current = sqlStatement{}
		started = false
	}

	for i := 0; i < len(src); {
		switch {
		case isSQLSpace(src[i]):
			if src[i] == '\n' {
				line++
			}
			i++

		case hasPrefixAt(src, i, "--"):
			start := i + 2
			i = start
			for i < len(src) && src[i] != '\n' && src[i] != '\r' {
				i++
			}
			if !started && strings.TrimSpace(string(src[start:i])) == reviewedCompatibleAnnotation {
				current.reviewedCompatible = true
			}

		case hasPrefixAt(src, i, "/*"):
			startLine := line
			depth := 1
			i += 2
			for i < len(src) && depth > 0 {
				switch {
				case hasPrefixAt(src, i, "/*"):
					depth++
					i += 2
				case hasPrefixAt(src, i, "*/"):
					depth--
					i += 2
				default:
					if src[i] == '\n' {
						line++
					}
					i++
				}
			}
			if depth != 0 {
				return nil, fmt.Errorf("line %d: unterminated block comment", startLine)
			}

		case src[i] == ';':
			finishStatement()
			i++

		case src[i] == '\'':
			start := i
			startLine := line
			next, err := consumeSingleQuoted(src, i, false, &line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", startLine, err)
			}
			addToken(sqlToken{kind: tokenString, text: "<string>", raw: string(src[start:next])})
			i = next

		case isDigit(src[i]) || src[i] == '.' && i+1 < len(src) && isDigit(src[i+1]):
			start := i
			i = consumeNumericLiteral(src, i)
			addToken(sqlToken{kind: tokenNumber, text: string(src[start:i])})

		case isWordStart(src[i]):
			start := i
			i++
			for i < len(src) && isWordContinue(src[i]) {
				i++
			}
			word := string(src[start:i])
			if len(word) > postgresIdentifierMaxBytes {
				return nil, fmt.Errorf("line %d: unquoted identifier exceeds PostgreSQL's %d-byte limit", line, postgresIdentifierMaxBytes)
			}

			// PostgreSQL recognizes these prefixes only when immediately
			// adjacent to a quote. Treat the whole literal as opaque.
			if i < len(src) && src[i] == '\'' && isSingleQuotePrefix(word) {
				startLine := line
				next, err := consumeSingleQuoted(src, i, strings.EqualFold(word, "e"), &line)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", startLine, err)
				}
				addToken(sqlToken{kind: tokenString, text: "<string>", raw: string(src[start:next])})
				i = next
				continue
			}
			if i+1 < len(src) && src[i] == '&' && src[i+1] == '\'' && strings.EqualFold(word, "u") {
				startLine := line
				// UESCAPE changes how Unicode escapes inside the value are
				// decoded; it does not let a backslash escape the SQL quote.
				next, err := consumeSingleQuoted(src, i+1, false, &line)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", startLine, err)
				}
				addToken(sqlToken{kind: tokenString, text: "<string>", raw: string(src[start:next])})
				i = next
				continue
			}
			if i+1 < len(src) && src[i] == '&' && src[i+1] == '"' && strings.EqualFold(word, "u") {
				next, value, err := consumeDoubleQuoted(src, i+1, &line)
				if err != nil {
					return nil, fmt.Errorf("line %d: %w", line, err)
				}
				addToken(sqlToken{kind: tokenUnicodeIdentifier, text: value})
				i = next
				continue
			}

			addToken(sqlToken{kind: tokenWord, text: word})

		case src[i] == '"':
			startLine := line
			next, value, err := consumeDoubleQuoted(src, i, &line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", startLine, err)
			}
			if len(value) > postgresIdentifierMaxBytes {
				return nil, fmt.Errorf("line %d: quoted identifier exceeds PostgreSQL's %d-byte limit", startLine, postgresIdentifierMaxBytes)
			}
			addToken(sqlToken{kind: tokenQuotedIdentifier, text: value})
			i = next

		case src[i] >= utf8RuneSelf:
			return nil, fmt.Errorf("line %d: non-ASCII unquoted identifiers are not supported by the managed-update policy", line)

		case src[i] == '$':
			if delimiter, ok := dollarQuoteDelimiter(src[i:]); ok {
				startLine := line
				bodyStart := i + len(delimiter)
				endOffset := bytes.Index(src[bodyStart:], delimiter)
				if endOffset < 0 {
					return nil, fmt.Errorf("line %d: unterminated dollar-quoted string %q", startLine, delimiter)
				}
				end := bodyStart + endOffset + len(delimiter)
				line += bytes.Count(src[i:end], []byte{'\n'})
				addToken(sqlToken{kind: tokenString, text: "<dollar-string>", raw: string(src[i:end])})
				i = end
				continue
			}
			addToken(sqlToken{kind: tokenSymbol, text: string(src[i])})
			i++

		default:
			addToken(sqlToken{kind: tokenSymbol, text: string(src[i])})
			i++
		}
	}

	finishStatement()
	return statements, nil
}

func consumeNumericLiteral(src []byte, start int) int {
	i := start
	if src[i] == '.' {
		i++
		for i < len(src) && isDigit(src[i]) {
			i++
		}
	} else {
		for i < len(src) && isDigit(src[i]) {
			i++
		}
		if i < len(src) && src[i] == '.' {
			i++
			for i < len(src) && isDigit(src[i]) {
				i++
			}
		}
	}
	if i >= len(src) || src[i] != 'e' && src[i] != 'E' {
		return i
	}
	exponent := i + 1
	if exponent < len(src) && (src[exponent] == '+' || src[exponent] == '-') {
		exponent++
	}
	digits := exponent
	for exponent < len(src) && isDigit(src[exponent]) {
		exponent++
	}
	if exponent == digits {
		return i
	}
	return exponent
}

func consumeSingleQuoted(src []byte, quote int, backslashEscapes bool, line *int) (int, error) {
	for i := quote + 1; i < len(src); i++ {
		if src[i] == '\n' {
			(*line)++
		}
		if backslashEscapes && src[i] == '\\' {
			if i+1 < len(src) {
				if src[i+1] == '\n' {
					(*line)++
				}
				i++
			}
			continue
		}
		if src[i] != '\'' {
			continue
		}
		if i+1 < len(src) && src[i+1] == '\'' {
			i++
			continue
		}
		return i + 1, nil
	}
	return 0, fmt.Errorf("unterminated quoted string")
}

func consumeDoubleQuoted(src []byte, quote int, line *int) (int, string, error) {
	var value strings.Builder
	for i := quote + 1; i < len(src); i++ {
		if src[i] == '\n' {
			(*line)++
		}
		if src[i] != '"' {
			value.WriteByte(src[i])
			continue
		}
		if i+1 < len(src) && src[i+1] == '"' {
			value.WriteByte('"')
			i++
			continue
		}
		return i + 1, value.String(), nil
	}
	return 0, "", fmt.Errorf("unterminated quoted identifier")
}

func dollarQuoteDelimiter(src []byte) ([]byte, bool) {
	if len(src) < 2 || src[0] != '$' {
		return nil, false
	}
	if src[1] == '$' {
		return src[:2], true
	}
	if !isWordStart(src[1]) {
		return nil, false
	}
	for i := 2; i < len(src); i++ {
		if src[i] == '$' {
			return src[:i+1], true
		}
		if !isWordContinue(src[i]) {
			return nil, false
		}
	}
	return nil, false
}

func isSingleQuotePrefix(word string) bool {
	return strings.EqualFold(word, "e") || strings.EqualFold(word, "b") || strings.EqualFold(word, "x")
}

func isSQLSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '\f'
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isWordStart(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func isWordContinue(b byte) bool {
	return isWordStart(b) || b >= '0' && b <= '9' || b == '$'
}

func hasPrefixAt(src []byte, offset int, prefix string) bool {
	return len(src)-offset >= len(prefix) && string(src[offset:offset+len(prefix)]) == prefix
}

const utf8RuneSelf = 0x80
