package service

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// refusalHoldTimeout is the max wall-clock the buffered-hold may withhold client
// output while waiting to classify a refusal (shared by all streaming paths).
func refusalHoldTimeout(cfg *config.Config) time.Duration {
	ms := 15000
	if cfg != nil && cfg.Gateway.RefusalHoldTimeoutMs > 0 {
		ms = cfg.Gateway.RefusalHoldTimeoutMs
	}
	return time.Duration(ms) * time.Millisecond
}

// refusalHoldMaxBytes is the max buffered bytes before the hold force-releases.
func refusalHoldMaxBytes(cfg *config.Config) int {
	n := 64 * 1024
	if cfg != nil && cfg.Gateway.RefusalHoldMaxBytes > 0 {
		n = cfg.Gateway.RefusalHoldMaxBytes
	}
	return n
}

// refusalRetryEnabled reports whether silent-refusal retry is enabled on the
// native Anthropic streaming path.
func (s *GatewayService) refusalRetryEnabled() bool {
	return s.cfg != nil && s.cfg.Gateway.RefusalRetryEnabled
}

// anthropicRefusalDetector decides whether a native Anthropic (non-Gemini)
// streaming response is a silent refusal: the upstream emitted
// stop_reason="refusal" (Claude 4 streaming classifier) without producing any
// meaningful content. It observes the parsed SSE events the native handlers
// already decode, so it never re-parses raw bytes.
type anthropicRefusalDetector struct {
	enabled              bool
	sawMeaningfulContent bool
	sawRefusalStop       bool
}

func newAnthropicRefusalDetector(enabled bool) *anthropicRefusalDetector {
	return &anthropicRefusalDetector{enabled: enabled}
}

func (d *anthropicRefusalDetector) Enabled() bool {
	return d != nil && d.enabled
}

// ObserveEvent inspects one decoded Anthropic SSE event. eventType is the
// event's "type" field; event is the decoded JSON object (may be nil for
// non-JSON frames like [DONE]).
func (d *anthropicRefusalDetector) ObserveEvent(eventType string, event map[string]any) {
	if d == nil || !d.enabled {
		return
	}
	switch eventType {
	case "content_block_start":
		// text / tool_use / thinking blocks all count as meaningful output.
		d.sawMeaningfulContent = true
	case "content_block_delta":
		d.sawMeaningfulContent = true
	case "message_delta":
		if delta, ok := event["delta"].(map[string]any); ok {
			if sr, ok := delta["stop_reason"].(string); ok && sr == "refusal" {
				d.sawRefusalStop = true
			}
		}
	}
}

// HasMeaningfulContent reports whether any content block was produced. Used by
// the buffered hold to release output as soon as a real response begins.
func (d *anthropicRefusalDetector) HasMeaningfulContent() bool {
	return d != nil && d.sawMeaningfulContent
}

// IsSilentRefusal reports whether the finished stream was a refusal with no
// meaningful content. A content-bearing turn is never a silent refusal.
func (d *anthropicRefusalDetector) IsSilentRefusal() bool {
	if d == nil || !d.enabled {
		return false
	}
	return d.sawRefusalStop && !d.sawMeaningfulContent
}
