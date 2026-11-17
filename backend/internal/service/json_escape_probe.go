package service

import "bytes"

// JSONMayEscapePlainASCII reports whether body contains a \u escape that spells a
// printable ASCII character no JSON encoder needs to escape (letters, digits,
// underscore, most punctuation). Only such an escape can hide a keyword made of
// those characters from a literal byte search, so when this returns false a
// bytes.Contains miss for such a keyword is a reliable negative. Keywords that
// contain an ignored character below need their own check.
//
// Escapes that encoders do emit on real traffic are ignored: control
// characters (the ANSI escape in shell output), quote, backslash, apostrophe,
// Go's HTML-safe escapes of <, > and &, and every non-ASCII rune. Large agent
// requests therefore keep their fast paths instead of being fully decoded. A
// malformed or truncated escape is reported as true to stay conservative.
//
// Escapes are consumed pairwise, so an escaped backslash followed by a literal
// u (Windows paths, regexes, code samples) is plain text, not an escape.
func JSONMayEscapePlainASCII(body []byte) bool {
	for i := 0; ; {
		j := bytes.IndexByte(body[i:], '\\')
		if j < 0 {
			return false
		}
		next := i + j + 1
		if next >= len(body) {
			return true
		}
		if body[next] != 'u' {
			i = next + 1
			continue
		}
		if next+5 > len(body) {
			return true
		}
		r, ok := parseJSONHex4(body[next+1 : next+5])
		if !ok || jsonEscapeHidesPlainASCII(r) {
			return true
		}
		i = next + 5
	}
}

func parseJSONHex4(b []byte) (rune, bool) {
	var r rune
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9':
			r = r<<4 | rune(c-'0')
		case c >= 'a' && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case c >= 'A' && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}

func jsonEscapeHidesPlainASCII(r rune) bool {
	if r < 0x20 || r >= 0x7f {
		return false
	}
	switch r {
	case '"', '\\', '<', '>', '&', '\'':
		return false
	}
	return true
}
