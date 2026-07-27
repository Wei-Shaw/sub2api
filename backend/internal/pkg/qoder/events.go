package qoder

import (
	"strings"

	"github.com/tidwall/gjson"
)

// SSE data payload "type" values emitted on the session event stream.
const (
	// EventDelta carries an incremental chunk of agent output.
	EventDelta = "event_delta"
	// EventSessionStatusIdle marks the session returning to idle; combined with
	// a stop_reason of end_turn it signals the current turn is complete.
	EventSessionStatusIdle = "session.status_idle"
	// StopReasonEndTurn is the stop_reason.type value that ends a turn.
	StopReasonEndTurn = "end_turn"
)

// Frame is one assembled Server-Sent Event (its event/id/data fields joined
// across the raw wire lines that precede a blank separator line).
type Frame struct {
	Event string
	ID    string
	Data  string
}

// FrameAccumulator assembles SSE wire lines into complete Frames. Feed it one
// line at a time (without the trailing newline); Line returns a non-nil Frame
// when a blank separator line completes the current event.
type FrameAccumulator struct {
	event string
	id    string
	data  strings.Builder
	dirty bool
}

// Line consumes a single SSE wire line. It returns the completed Frame when the
// line is blank (event boundary), otherwise nil.
func (a *FrameAccumulator) Line(line string) *Frame {
	// Comments / heartbeats begin with ':' and are ignored.
	if strings.HasPrefix(line, ":") {
		return nil
	}
	if line == "" {
		if !a.dirty {
			return nil
		}
		frame := &Frame{Event: a.event, ID: a.id, Data: a.data.String()}
		a.reset()
		return frame
	}
	a.dirty = true
	switch {
	case strings.HasPrefix(line, "event:"):
		a.event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
	case strings.HasPrefix(line, "id:"):
		a.id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
	case strings.HasPrefix(line, "data:"):
		if a.data.Len() > 0 {
			a.data.WriteByte('\n')
		}
		a.data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
	}
	return nil
}

func (a *FrameAccumulator) reset() {
	a.event = ""
	a.id = ""
	a.data.Reset()
	a.dirty = false
}

// DeltaText extracts the incremental assistant text from a frame's data JSON.
// Returns an empty string when the frame carries no text delta.
func (f *Frame) DeltaText() string {
	if f == nil || f.Data == "" {
		return ""
	}
	if t := gjson.Get(f.Data, "delta.content.text"); t.Exists() {
		return t.String()
	}
	if t := gjson.Get(f.Data, "content.text"); t.Exists() {
		return t.String()
	}
	return ""
}

// dataType returns the frame's semantic type, preferring the data payload's
// "type" field and falling back to the SSE event name.
func (f *Frame) dataType() string {
	if f == nil {
		return ""
	}
	if f.Data != "" {
		if t := gjson.Get(f.Data, "type"); t.Exists() {
			return t.String()
		}
	}
	return f.Event
}

// IsTurnEnd reports whether the frame signals the end of the current turn
// (session returned to idle with an end_turn stop reason). The stop reason is
// treated as satisfied when absent, since idle alone terminates the stream.
func (f *Frame) IsTurnEnd() bool {
	if f == nil {
		return false
	}
	if f.dataType() != EventSessionStatusIdle {
		return false
	}
	reason := gjson.Get(f.Data, "stop_reason.type")
	if !reason.Exists() {
		return true
	}
	return reason.String() == StopReasonEndTurn
}
