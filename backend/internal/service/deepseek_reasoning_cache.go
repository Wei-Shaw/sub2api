package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	deepSeekReplayMaxBytes = 256 * 1024
	deepSeekReplayMaxItems = 128
	deepSeekReplayBudget   = 200 * time.Millisecond
)

// This cache is deliberately separate from the legacy, globally keyed Chat
// fallback cache. Never recover another API key's reasoning or invent missing
// text. Model scope uses the client model so account failover keeps working.
type deepSeekReasoningCache struct {
	svc                 *OpenAIGatewayService
	scope               string
	remaining           time.Duration
	operations          int
	seen                map[string]bool
	text                string
	done                map[int]string
	doneBytes           int
	doneTruncated       bool
	deltas              *apicompat.BufferedResponseAccumulator
	deltaBytes          int
	deltaReasoningIndex int
}

func (s *OpenAIGatewayService) newDeepSeekReasoningCache(c *gin.Context, account *Account, model, upstreamModel string) *deepSeekReasoningCache {
	// Passthrough handlers leave mappedModel empty when no mapping is needed.
	if upstreamModel == "" {
		upstreamModel = model
	}
	if s == nil || s.cache == nil || c == nil || account == nil || account.Type != AccountTypeAPIKey ||
		!isDeepSeekReasoningReplayModel(upstreamModel) || isOpenAIResponsesCompactPath(c) {
		return nil
	}
	keyID := getAPIKeyIDFromContext(c)
	if keyID <= 0 {
		return nil
	}
	return &deepSeekReasoningCache{svc: s, scope: fmt.Sprintf("deepseek-replay-v1:%d:%x:", keyID, sha256.Sum256([]byte(model))), remaining: deepSeekReplayBudget, seen: make(map[string]bool), deltaReasoningIndex: -1}
}

func (r *deepSeekReasoningCache) key(kind, identity string) string {
	return r.scope + kind + ":" + fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}

func (r *deepSeekReasoningCache) access(kind, identity, value string, write bool) string {
	if r == nil || identity == "" || r.remaining <= 0 || r.operations >= deepSeekReplayMaxItems || len(value) > deepSeekReplayMaxBytes {
		return ""
	}
	r.operations++
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), r.remaining)
	defer func() { cancel(); r.remaining -= time.Since(start) }()
	key := r.key(kind, identity)
	if write {
		if value != "" && !r.seen[key] {
			r.seen[key] = true
			_ = r.svc.cache.SetReasoningContent(ctx, key, value, responsesReasoningCacheTTL)
		}
		return ""
	}
	text, err := r.svc.cache.GetReasoningContent(ctx, key)
	if err != nil || len(text) > deepSeekReplayMaxBytes {
		return ""
	}
	return text
}

// Prefer the provider's full reasoning content over its optional summary.
func deepSeekReplayText(item gjson.Result) string {
	for _, field := range []string{"content", "summary"} {
		if text := deepSeekReplayPartText(item, field); text != "" {
			return text
		}
	}
	return ""
}

func deepSeekReplayPartText(item gjson.Result, field string) string {
	var parts []string
	for _, part := range item.Get(field).Array() {
		if text := part.Get("text").String(); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

// Call IDs alone can be reused by compatible providers; bind them to the tool
// name and arguments as well, preventing accidental reuse across tool calls.
func deepSeekReplayCallIdentity(item gjson.Result) string {
	if item.Get("type").String() != "function_call" && item.Get("type").String() != "custom_tool_call" {
		return ""
	}
	if item.Get("call_id").String() == "" {
		return ""
	}
	values := []string{item.Get("call_id").String(), item.Get("name").String(), item.Get("namespace").String(), item.Get("arguments").String(), item.Get("input").String()}
	encoded, _ := json.Marshal(values)
	return string(encoded)
}

func (s *OpenAIGatewayService) restoreDeepSeekReasoning(c *gin.Context, account *Account, body []byte, model, upstreamModel string) []byte {
	r := s.newDeepSeekReasoningCache(c, account, model, upstreamModel)
	if r == nil || !gjson.ValidBytes(body) {
		return body
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body
	}
	items := input.Array()
	out := make([]json.RawMessage, 0, len(items))
	changed, hasReasoning := false, false
	missingIndex := -1
	addedBytes := 0
	for _, item := range items {
		raw := []byte(item.Raw)
		typ := item.Get("type").String()
		if item.Get("role").String() == "user" || item.Get("role").String() == "system" || item.Get("role").String() == "developer" {
			hasReasoning, missingIndex = false, -1
		}
		if typ == "function_call_output" || typ == "custom_tool_call_output" {
			hasReasoning, missingIndex = false, -1
		}
		if typ == "reasoning" {
			// A client may keep the summary while dropping reasoning_text.
			// Preserve its summary but restore missing full content if cached.
			hasReasoning = deepSeekReplayPartText(item, "content") != ""
			missingIndex = -1
			if !hasReasoning {
				if text := r.access("item", item.Get("id").String(), "", false); text != "" && addedBytes+len(text) <= deepSeekReplayMaxBytes {
					patched, err := sjson.SetBytes(raw, "content", []map[string]string{{"type": "reasoning_text", "text": text}})
					if err == nil {
						raw, changed, hasReasoning = patched, true, true
						addedBytes += len(text)
					}
				}
				if !hasReasoning {
					missingIndex = len(out)
				}
			}
		}
		if identity := deepSeekReplayCallIdentity(item); identity != "" && !hasReasoning {
			if text := r.access("call", identity, "", false); text != "" && addedBytes+len(text) <= deepSeekReplayMaxBytes {
				content := []map[string]string{{"type": "reasoning_text", "text": text}}
				if missingIndex >= 0 {
					patched, err := sjson.SetBytes(out[missingIndex], "content", content)
					if err != nil {
						return body
					}
					out[missingIndex] = patched
				} else {
					patched, _ := json.Marshal(map[string]any{"type": "reasoning", "content": content})
					out = append(out, patched)
				}
				changed, hasReasoning = true, true
				addedBytes += len(text)
			}
		}
		out = append(out, raw)
	}
	if !changed {
		return body
	}
	patched, err := sjson.SetBytes(body, "input", out)
	if err != nil {
		return body
	}
	return patched
}

// Observe only completed reasoning items. Partial deltas must not be mistaken
// for full reasoning. Terminal output also covers non-streaming responses and
// providers that omit output_item.done. Existing stream reconstruction supplies
// complete terminal output for delta-only providers.
func (r *deepSeekReasoningCache) observe(payload []byte) {
	if r == nil {
		return
	}
	root := gjson.ParseBytes(payload)
	switch root.Get("type").String() {
	case "response.output_item.done":
		item := root.Get("item")
		index := root.Get("output_index")
		if !index.Exists() || index.Int() < 0 || (item.Get("type").String() != "reasoning" && deepSeekReplayCallIdentity(item) == "") {
			return
		}
		if r.done == nil {
			r.done = make(map[int]string)
		}
		i := int(index.Int())
		size := r.doneBytes - len(r.done[i]) + len(item.Raw)
		if len(r.done) >= deepSeekReplayMaxItems || size > deepSeekReplayMaxBytes {
			r.doneTruncated = true
			return
		}
		r.done[i], r.doneBytes = strings.Clone(item.Raw), size
	case "response.completed", "response.done":
		if status := root.Get("response.status").String(); status != "" && status != "completed" {
			return
		}
		r.observeOutput(root.Get("response.output"))
	case "response.output_item.added", "response.function_call_arguments.delta", "response.function_call_arguments.done", "response.reasoning_text.delta":
		// Bound delta-only providers as well. Only full reasoning deltas are
		// accumulated: combining summary and full text would duplicate thought.
		r.deltaBytes += len(payload)
		if root.Get("type").String() == "response.reasoning_text.delta" {
			index := int(root.Get("output_index").Int())
			if r.deltaReasoningIndex >= 0 && r.deltaReasoningIndex != index {
				// The generic accumulator joins reasoning items. Do not cache
				// ambiguous multi-item deltas as one tool turn's thought.
				r.deltaBytes = deepSeekReplayMaxBytes + 1
			}
			r.deltaReasoningIndex = index
		}
		if r.deltaBytes > deepSeekReplayMaxBytes {
			r.deltas = nil
			return
		}
		if r.deltas == nil {
			r.deltas = apicompat.NewBufferedResponseAccumulator()
		}
		var event apicompat.ResponsesStreamEvent
		if json.Unmarshal(payload, &event) == nil {
			r.deltas.ProcessEvent(&event)
		}
	case "response":
		if root.Get("status").String() == "completed" {
			r.observeOutput(root.Get("output"))
		}
	default:
		if root.Get("status").String() == "completed" && root.Get("output").IsArray() {
			r.observeOutput(root.Get("output"))
		}
	}
}

func (r *deepSeekReasoningCache) observeOutput(output gjson.Result) {
	r.text = ""
	items := output.Array()
	if len(items) == 0 && len(r.done) == 0 && r.deltas != nil {
		encoded, err := json.Marshal(r.deltas.BuildOutput())
		if err == nil {
			items = gjson.ParseBytes(encoded).Array()
		}
	}
	if len(items) < len(r.done) || len(items) == 0 {
		if r.doneTruncated {
			return
		}
		indices := make([]int, 0, len(r.done))
		for i := range r.done {
			indices = append(indices, i)
		}
		sort.Ints(indices)
		for _, i := range indices {
			r.observeItem(gjson.Parse(r.done[i]))
		}
		return
	}
	for i, item := range items {
		// A terminal snapshot may omit reasoning text already delivered in done.
		if saved := gjson.Parse(r.done[i]); item.Get("type").String() == "reasoning" && saved.Get("id").String() == item.Get("id").String() && deepSeekReplayText(saved) != "" {
			item = saved
		}
		r.observeItem(item)
	}
}

func (r *deepSeekReasoningCache) observeItem(item gjson.Result) {
	if item.Get("type").String() == "reasoning" {
		r.text = deepSeekReplayText(item)
		if len(r.text) > deepSeekReplayMaxBytes {
			r.text = ""
		}
		r.access("item", item.Get("id").String(), r.text, true)
	} else if r.text != "" {
		r.access("call", deepSeekReplayCallIdentity(item), r.text, true)
	}
}
