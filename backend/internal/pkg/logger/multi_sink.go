package logger

import "reflect"

// multiSink forwards each log event to all configured sinks.
// Sinks are invoked in registration order.
type multiSink struct {
	sinks []Sink
}

// NewMultiSink combines multiple sinks without changing the single-sink fast
// path. Nil sinks are ignored; no sinks returns nil.
func NewMultiSink(sinks ...Sink) Sink {
	filtered := make([]Sink, 0, len(sinks))
	for _, sink := range sinks {
		if !nilSink(sink) {
			filtered = append(filtered, sink)
		}
	}

	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		return &multiSink{sinks: filtered}
	}
}

func nilSink(sink Sink) bool {
	if sink == nil {
		return true
	}
	value := reflect.ValueOf(sink)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (s *multiSink) WriteLogEvent(event *LogEvent) {
	if s == nil || event == nil {
		return
	}
	for _, sink := range s.sinks {
		sink.WriteLogEvent(event)
	}
}
