package service

import "strings"

// Default Gemini finishReason values that indicate the upstream blocked the
// response (documented Gemini values). Configurable via settings so the set can
// be corrected against the empirically captured refusal shapes (Step 0).
var defaultAntigravityBlockingFinishReasons = map[string]struct{}{
	"SAFETY":             {},
	"RECITATION":         {},
	"PROHIBITED_CONTENT": {},
	"BLOCKLIST":          {},
	"SPII":               {},
}

// antigravityRefusalDetector decides whether an antigravity (Gemini-backed
// Claude) stream is a silent refusal. It reads signals from the StreamingProcessor
// (raw Gemini finishReason + whether any meaningful content was produced) rather
// than re-parsing the converted Claude SSE.
type antigravityRefusalDetector struct {
	enabled        bool
	blockingFinish map[string]struct{}
}

func newAntigravityRefusalDetector(enabled bool, blockingFinish []string) *antigravityRefusalDetector {
	set := defaultAntigravityBlockingFinishReasons
	if len(blockingFinish) > 0 {
		set = make(map[string]struct{}, len(blockingFinish))
		for _, r := range blockingFinish {
			set[strings.ToUpper(strings.TrimSpace(r))] = struct{}{}
		}
	}
	return &antigravityRefusalDetector{enabled: enabled, blockingFinish: set}
}

func (d *antigravityRefusalDetector) Enabled() bool {
	return d != nil && d.enabled
}

// IsSilentRefusal reports whether a finished stream is a refusal: a blocking
// finishReason with no meaningful content produced. sawMeaningfulContent must
// be false (no visible text, thinking, or tool use). A turn that produced any
// real content is never a silent refusal, even if it later blocks.
func (d *antigravityRefusalDetector) IsSilentRefusal(finishReason string, sawMeaningfulContent bool) bool {
	if d == nil || !d.enabled {
		return false
	}
	if sawMeaningfulContent {
		return false
	}
	_, blocking := d.blockingFinish[strings.ToUpper(strings.TrimSpace(finishReason))]
	return blocking
}
