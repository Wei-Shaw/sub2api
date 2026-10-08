package service

import (
	"net/http"
	"strings"

	"github.com/tidwall/gjson"
)

// openAICacheWriteOutputCapture keeps only hashes and structural progress. It
// deliberately never reconstructs terminal output from progressive events.
type openAICacheWriteOutputCapture struct {
	evidence       openAICacheWriteOutputEvidence
	disabled       bool
	invalid        bool
	terminalSeen   bool
	sawOutput      bool
	sawOutputIndex bool
	maxOutputIndex int64
	completedItems map[int64]string
}

func newOpenAICacheWriteOutputCapture(resp *http.Response) openAICacheWriteOutputCapture {
	var capture openAICacheWriteOutputCapture
	if resp != nil {
		// These bodies already contain client-facing tool rewrites. They cannot
		// establish the original upstream lineage, even if the terminal is full.
		_, capture.disabled = resp.Body.(*responsesClientToolStreamBody)
	}
	return capture
}

func (c *openAICacheWriteOutputCapture) invalidate() {
	c.invalid = true
	c.evidence = openAICacheWriteOutputEvidence{}
	c.completedItems = nil
}

func (c *openAICacheWriteOutputCapture) observeResponse(body []byte) {
	if c == nil || c.disabled || c.invalid {
		return
	}
	c.evidence = buildOpenAICacheWriteOutputEvidence(body)
}

func (c *openAICacheWriteOutputCapture) observe(payload []byte, eventType string) {
	if c == nil || c.disabled || c.invalid {
		return
	}
	if len(payload) > openAICacheWriteEvidenceMaxBodyBytes {
		c.invalidate()
		return
	}
	eventType = effectiveOpenAISSEEventType(payload, eventType)
	switch eventType {
	case "response.completed", "response.done":
		if c.terminalSeen {
			c.invalidate()
			return
		}
		c.terminalSeen = true
		output := gjson.GetBytes(payload, "response.output")
		if !output.IsArray() {
			c.invalidate()
			return
		}
		count := int64(len(output.Array()))
		if (c.sawOutput && count == 0) || (c.sawOutputIndex && count <= c.maxOutputIndex) {
			// Some upstreams omit all or part of terminal output and expect the
			// client to reconstruct it. Such a terminal is not complete evidence.
			c.invalidate()
			return
		}
		payload = []byte(openAICompatPayloadWithEventType(string(payload), eventType))
		c.evidence = buildOpenAICacheWriteOutputEvidence(payload)
		if !c.evidence.Valid {
			return
		}
		for index, hash := range c.completedItems {
			if index >= int64(len(c.evidence.OutputHashes)) || c.evidence.OutputHashes[index] != hash {
				c.invalidate()
				return
			}
		}
		return
	case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
		c.invalidate()
		return
	}
	index := gjson.GetBytes(payload, "output_index")
	output := gjson.GetBytes(payload, "response.output")
	progress := index.Exists() || len(output.Array()) > 0
	switch eventType {
	case "", "response.created", "response.in_progress", "response.queued":
		if c.terminalSeen && eventType != "" {
			c.invalidate()
			return
		}
	default:
		// Unknown Responses events may carry output too. In particular, tool
		// argument and reasoning deltas without indexes must not make an empty
		// terminal look complete.
		progress = progress || strings.HasPrefix(eventType, "response.")
	}
	if !progress {
		return
	}
	if c.terminalSeen {
		c.invalidate()
		return
	}
	c.sawOutput = true
	if index.Exists() {
		if index.Type != gjson.Number || index.Int() < 0 || index.Int() >= openAICacheWriteEvidenceMaxItems || float64(index.Int()) != index.Float() {
			c.invalidate()
			return
		}
		if !c.sawOutputIndex || index.Int() > c.maxOutputIndex {
			c.maxOutputIndex = index.Int()
		}
		c.sawOutputIndex = true
	}
	if eventType == "response.output_item.done" {
		item := gjson.GetBytes(payload, "item")
		const prefix, suffix = `{"output":[`, `]}`
		if !index.Exists() || !item.IsObject() || len(item.Raw) > openAICacheWriteEvidenceMaxBodyBytes-len(prefix)-len(suffix) {
			c.invalidate()
			return
		}
		completed := buildOpenAICacheWriteOutputEvidence([]byte(prefix + item.Raw + suffix))
		if !completed.Valid || len(completed.OutputHashes) != 1 {
			c.invalidate()
			return
		}
		if prior, exists := c.completedItems[index.Int()]; exists && prior != completed.OutputHashes[0] {
			c.invalidate()
			return
		}
		if c.completedItems == nil {
			c.completedItems = make(map[int64]string)
		}
		c.completedItems[index.Int()] = completed.OutputHashes[0]
	}
	if count := int64(len(output.Array())); count > 0 {
		if !c.sawOutputIndex || count-1 > c.maxOutputIndex {
			c.maxOutputIndex = count - 1
		}
		c.sawOutputIndex = true
	}
}

func captureOpenAICacheWriteSSEOutput(resp *http.Response, body string) openAICacheWriteOutputEvidence {
	capture := newOpenAICacheWriteOutputCapture(resp)
	forEachOpenAISSEFrame(body, func(eventType string, payload []byte) {
		capture.observe(payload, eventType)
	})
	return capture.evidence
}
