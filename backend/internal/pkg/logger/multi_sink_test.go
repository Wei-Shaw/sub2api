package logger

import (
	"testing"
	"time"
)

type recordingSink struct {
	events []*LogEvent
}

func (s *recordingSink) WriteLogEvent(event *LogEvent) {
	s.events = append(s.events, event)
}

func TestNewMultiSink(t *testing.T) {
	first := &recordingSink{}
	second := &recordingSink{}
	event := &LogEvent{Time: time.Now(), Message: "hello"}

	sink := NewMultiSink(nil, first, second)
	if sink == nil {
		t.Fatal("expected combined sink")
	}
	sink.WriteLogEvent(event)

	for name, recorder := range map[string]*recordingSink{"first": first, "second": second} {
		if len(recorder.events) != 1 || recorder.events[0] != event {
			t.Fatalf("%s sink did not receive the event", name)
		}
	}
}

func TestNewMultiSinkFastPaths(t *testing.T) {
	if sink := NewMultiSink(nil); sink != nil {
		t.Fatalf("expected nil sink, got %T", sink)
	}

	only := &recordingSink{}
	var typedNil *recordingSink
	if sink := NewMultiSink(nil, only, typedNil); sink != only {
		t.Fatalf("expected the original single sink, got %T", sink)
	}
}
