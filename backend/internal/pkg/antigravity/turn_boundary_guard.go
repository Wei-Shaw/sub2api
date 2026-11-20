package antigravity

import (
	"bytes"
	"strings"
)

// leakedUserTurnTags lists harness-injected XML tags that open a user turn in
// agent clients (Claude Code, Codex CLI). Outside a fenced code block, a
// standalone "user" line immediately followed by one of these tags indicates that
// a Gemini model failed to emit its end-of-turn token and began simulating the
// next user turn in plain text.
var leakedUserTurnTags = []string{
	"<system-reminder>",
	"<task-notification>",
	"<command-name>",
	"<command-message>",
	"<local-command-stdout>",
	"<environment_context>",
	"<user_instructions>",
	"<permissions instructions>",
}

// leakedControlTurnMarkers lists explicit chat-template turn control tokens
// that should never appear at the start of a line outside a fenced code block.
var leakedControlTurnMarkers = []string{
	"<start_of_turn>user\n",
	"<start_of_turn>user\r\n",
	"<end_of_turn>\n",
	"<end_of_turn>\r\n",
	"<end_of_turn><start_of_turn>",
	"<|end_of_turn|>\n",
	"<|end_of_turn|>\r\n",
	"<|endoftext|>\n",
	"<|endoftext|>\r\n",
	"<|user|>\n",
	"<|user|>\r\n",
}

var (
	leakedTurnMarkers        = buildLeakedTurnMarkers()
	leakedTurnMarkerStartMap = buildLeakedTurnMarkerStartMap(leakedTurnMarkers)
)

func buildLeakedTurnMarkers() []string {
	markers := make([]string, 0, len(leakedUserTurnTags)*2+len(leakedControlTurnMarkers))
	for _, tag := range leakedUserTurnTags {
		markers = append(markers, "user\n"+tag, "user\r\n"+tag)
	}
	markers = append(markers, leakedControlTurnMarkers...)
	return markers
}

func buildLeakedTurnMarkerStartMap(markers []string) [256]bool {
	var starts [256]bool
	for _, m := range markers {
		if len(m) > 0 {
			starts[m[0]] = true
		}
	}
	return starts
}

// fenceTracker tracks CommonMark fenced code blocks (``` / ~~~) across streamed
// text chunks so that quoted transcript examples inside code blocks do not
// trigger turn-boundary truncation.
//
// The zero value starts at the beginning of a line outside any fence.
type fenceTracker struct {
	midLine bool

	inFence   bool
	fenceChar byte
	fenceLen  int

	// Per-line candidate state (valid only while scanning the current line).
	lineSpaces   int
	lineChar     byte
	lineRun      int
	lineAfterRun bool
	lineInvalid  bool
}

func (f *fenceTracker) resetLine() {
	f.midLine = false
	f.lineSpaces = 0
	f.lineChar = 0
	f.lineRun = 0
	f.lineAfterRun = false
	f.lineInvalid = false
}

func (f *fenceTracker) resetBlock() {
	f.inFence = false
	f.fenceChar = 0
	f.fenceLen = 0
	f.resetLine()
}

func (f *fenceTracker) advance(s string) {
	for i := 0; i < len(s); i++ {
		f.advanceByte(s[i])
	}
}

func (f *fenceTracker) advanceByte(b byte) {
	if b == '\n' {
		if !f.lineInvalid {
			if !f.inFence && f.lineRun >= 3 {
				f.inFence = true
				f.fenceChar = f.lineChar
				f.fenceLen = f.lineRun
			} else if f.inFence && f.lineRun >= f.fenceLen {
				f.inFence = false
				f.fenceChar = 0
				f.fenceLen = 0
			}
		}
		f.resetLine()
		return
	}

	f.midLine = true
	if f.lineInvalid || b == '\r' {
		return
	}

	if !f.inFence {
		if f.lineChar == 0 {
			if b == ' ' && f.lineSpaces < 3 {
				f.lineSpaces++
				return
			}
			if b == '`' || b == '~' {
				f.lineChar = b
				f.lineRun = 1
				return
			}
			f.lineInvalid = true
			return
		}
		if !f.lineAfterRun {
			if b == f.lineChar {
				f.lineRun++
				return
			}
			if f.lineRun < 3 {
				f.lineInvalid = true
				return
			}
			f.lineAfterRun = true
		}
		// CommonMark §4.5: backtick fence info strings cannot contain backticks.
		if f.lineChar == '`' && b == '`' {
			f.lineInvalid = true
		}
		return
	}

	// Inside a fenced code block: check for a matching closing fence.
	if f.lineChar == 0 {
		if b == ' ' && f.lineSpaces < 3 {
			f.lineSpaces++
			return
		}
		if b == f.fenceChar {
			f.lineChar = b
			f.lineRun = 1
			return
		}
		f.lineInvalid = true
		return
	}
	if !f.lineAfterRun {
		if b == f.fenceChar {
			f.lineRun++
			return
		}
		if f.lineRun < f.fenceLen {
			f.lineInvalid = true
			return
		}
		f.lineAfterRun = true
	}
	if b != ' ' && b != '\t' {
		f.lineInvalid = true
	}
}

type markerMatchState int

const (
	markerNoMatch markerMatchState = iota
	markerPrefix
	markerExact
)

func matchLeakedTurnMarker(candidate []byte) markerMatchState {
	s := string(candidate)
	isPrefix := false
	for _, m := range leakedTurnMarkers {
		if s == m {
			return markerExact
		}
		if strings.HasPrefix(m, s) {
			isPrefix = true
		}
	}
	if isPrefix {
		return markerPrefix
	}
	return markerNoMatch
}

// turnBoundaryGuard detects leaked Gemini multi-turn boundaries in streamed or
// buffered assistant text. It holds back only the minimal prefix of a potential
// marker at line start outside fenced code blocks, preserving chunk-boundary
// invariance with zero buffering on ordinary text.
//
// The zero value is ready for use.
type turnBoundaryGuard struct {
	fence     fenceTracker
	pending   []byte
	triggered bool
}

// Triggered reports whether a leaked turn boundary has been detected.
func (g *turnBoundaryGuard) Triggered() bool {
	return g.triggered
}

// Feed consumes the next chunk of ordinary assistant text and returns the safe
// prefix to emit together with whether a leaked turn boundary was detected.
// Once triggered, all subsequent calls return ("", true).
func (g *turnBoundaryGuard) Feed(chunk string) (string, bool) {
	if g.triggered {
		return "", true
	}
	if chunk == "" {
		return "", false
	}

	var out strings.Builder
	if g.feedBytes([]byte(chunk), &out) {
		return out.String(), true
	}
	return out.String(), false
}

func (g *turnBoundaryGuard) feedBytes(data []byte, out *strings.Builder) bool {
	i := 0
	for i < len(data) {
		if len(g.pending) > 0 {
			g.pending = append(g.pending, data[i])
			i++
			switch matchLeakedTurnMarker(g.pending) {
			case markerExact:
				g.pending = g.pending[:0]
				g.triggered = true
				return true
			case markerPrefix:
				continue
			case markerNoMatch:
				// Release bytes up to the first newline (if any) and re-scan the
				// suffix at the new line start so patterns like "user\nuser\n<system-reminder>"
				// are still detected.
				if nl := bytes.IndexByte(g.pending, '\n'); nl >= 0 {
					prefix := string(g.pending[:nl+1])
					rem := append([]byte(nil), g.pending[nl+1:]...)
					g.pending = g.pending[:0]
					out.WriteString(prefix)
					g.fence.advance(prefix)
					if len(rem) > 0 && g.feedBytes(rem, out) {
						return true
					}
				} else {
					flushed := string(g.pending)
					g.pending = g.pending[:0]
					out.WriteString(flushed)
					g.fence.advance(flushed)
				}
			}
			continue
		}

		if !g.fence.midLine && !g.fence.inFence && leakedTurnMarkerStartMap[data[i]] {
			g.pending = append(g.pending, data[i])
			i++
			// A single-byte marker cannot exist, but check via matchLeakedTurnMarker
			// for clarity and safety if markers ever change.
			if matchLeakedTurnMarker(g.pending) == markerExact {
				g.pending = g.pending[:0]
				g.triggered = true
				return true
			}
			continue
		}

		// Fast path: mid-line, inside a code fence, or line starts with a byte
		// that cannot begin any marker. Emit through the next newline.
		nl := bytes.IndexByte(data[i:], '\n')
		var seg string
		if nl < 0 {
			seg = string(data[i:])
			i = len(data)
		} else {
			seg = string(data[i : i+nl+1])
			i += nl + 1
		}
		out.WriteString(seg)
		g.fence.advance(seg)
	}
	return false
}

// FlushAndResetBlock flushes any buffered non-matching prefix and resets block
// line/fence state before transitioning to a non-text block or ending the turn.
func (g *turnBoundaryGuard) FlushAndResetBlock() string {
	if g.triggered {
		g.pending = g.pending[:0]
		return ""
	}
	var flushed string
	if len(g.pending) > 0 {
		flushed = string(g.pending)
		g.pending = g.pending[:0]
	}
	g.fence.resetBlock()
	return flushed
}
