package apicompat

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

// customToolInputStream decodes the string inside the function adapter's
// {"input":"..."} wrapper without waiting for the closing JSON document.
// offset advances over complete JSON string units only, so split escapes,
// surrogate pairs and UTF-8 characters never leak into client deltas.
type customToolInputStream struct {
	offset  int
	started bool
	stopped bool
	emitted strings.Builder
}

func (s *customToolInputStream) delta(arguments string) string {
	if s.stopped {
		return ""
	}
	if !s.started {
		offset := customToolInputStart(arguments)
		if offset < 0 {
			return ""
		}
		s.offset, s.started = offset, true
	}
	var out strings.Builder
	for s.offset < len(arguments) {
		rest := arguments[s.offset:]
		switch rest[0] {
		case '"':
			s.stopped = true
		case '\\':
			n := 2
			if len(rest) < n {
				return s.record(out.String())
			}
			if rest[1] == 'u' {
				n = 6
				if len(rest) < n {
					return s.record(out.String())
				}
				code, err := strconv.ParseUint(rest[2:6], 16, 16)
				if err != nil {
					s.stopped = true
					break
				}
				if code >= 0xd800 && code <= 0xdbff {
					// A high surrogate may be followed by a low surrogate in
					// another delta. Wait until that decision is unambiguous.
					if len(rest) == 6 || (rest[6] == '\\' && len(rest) == 7) {
						return s.record(out.String())
					}
					if strings.HasPrefix(rest[6:], `\u`) {
						if len(rest) < 12 {
							return s.record(out.String())
						}
						low, err := strconv.ParseUint(rest[8:12], 16, 16)
						if err == nil && low >= 0xdc00 && low <= 0xdfff {
							n = 12
						}
					}
				}
			}
			var decoded string
			if err := json.Unmarshal([]byte(`"`+rest[:n]+`"`), &decoded); err != nil {
				s.stopped = true
				break
			}
			out.WriteString(decoded)
			s.offset += n
		default:
			if rest[0] < 0x20 {
				s.stopped = true
				break
			}
			if !utf8.FullRuneInString(rest) {
				return s.record(out.String())
			}
			r, n := utf8.DecodeRuneInString(rest)
			out.WriteRune(r)
			s.offset += n
		}
		if s.stopped {
			break
		}
	}
	return s.record(out.String())
}

func (s *customToolInputStream) record(delta string) string {
	s.emitted.WriteString(delta)
	return delta
}

// Only the wrapper prefix is decoded here; the potentially large input string
// is scanned once by delta. Other fields before input are allowed. Non-wrapper
// arguments retain the existing final-payload fallback.
func customToolInputStart(arguments string) int {
	d := json.NewDecoder(strings.NewReader(arguments))
	open, err := d.Token()
	if err != nil || open != json.Delim('{') {
		return -1
	}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return -1
		}
		if key == "input" {
			i := int(d.InputOffset())
			for i < len(arguments) && strings.ContainsRune(" \t\r\n", rune(arguments[i])) {
				i++
			}
			if i == len(arguments) || arguments[i] != ':' {
				return -1
			}
			i++
			for i < len(arguments) && strings.ContainsRune(" \t\r\n", rune(arguments[i])) {
				i++
			}
			if i < len(arguments) && arguments[i] == '"' {
				return i + 1
			}
			return -1
		}
		var ignored json.RawMessage
		if err := d.Decode(&ignored); err != nil {
			return -1
		}
	}
	return -1
}
