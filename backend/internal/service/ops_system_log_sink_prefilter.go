package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/logger"

// AcceptsLogEntry implements logger.SinkEntryFilter so entries shouldIndex
// would drop anyway are skipped before the logger encodes their fields.
// WriteLogEvent still runs the full shouldIndex check on accepted entries.
func (s *OpsSystemLogSink) AcceptsLogEntry(level logger.Level, component string) bool {
	return s != nil && s.shouldIndex(&logger.LogEvent{Level: level.String(), Component: component})
}
