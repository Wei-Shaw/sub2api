package service

import (
	"strings"
	"sync"

	"github.com/tidwall/gjson"
)

// A passthrough connection can carry session/conversation changes outside a
// response.create payload. Once that happens, payload-only prompt evidence is
// unavailable for the remainder of the connection. Store only hashes, never the
// stateful frames or inherited session data.
type openAIWSCacheWritePromptCapture struct {
	mu       sync.RWMutex
	disabled bool
	evidence openAICacheWritePromptEvidence
}

func (c *openAIWSCacheWritePromptCapture) observeOutbound(payload []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disabled {
		return
	}
	switch strings.TrimSpace(gjson.GetBytes(payload, "type").String()) {
	case "response.create":
		c.evidence = buildOpenAICacheWritePromptEvidence(payload)
	case "response.cancel":
		// Cancellation does not add prompt state. The failed/incomplete turn
		// is independently excluded by terminal/output evidence validation.
	default:
		// Includes session.update, conversation.item.*, audio-buffer updates,
		// and unknown frames whose effects are not covered by response.create.
		c.disabled = true
		c.evidence = openAICacheWritePromptEvidence{}
	}
}

func (c *openAIWSCacheWritePromptCapture) snapshot() openAICacheWritePromptEvidence {
	if c == nil {
		return openAICacheWritePromptEvidence{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.disabled {
		return openAICacheWritePromptEvidence{}
	}
	return c.evidence
}
