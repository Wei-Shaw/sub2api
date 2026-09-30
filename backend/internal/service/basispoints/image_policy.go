package basispoints

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ImageProgress stores only a digest and a position, never conversation content.
type ImageProgress struct {
	Count  int
	Digest string
}
type ImageHistory struct {
	source object
	Input  []any
	Count  int
}

func InspectImageHistory(raw []byte) (*ImageHistory, error) {
	var source object
	if err := decode(raw, &source); err != nil || source == nil {
		return nil, fmt.Errorf("invalid image history request")
	}
	input, ok := source["input"].([]any)
	if !ok {
		return &ImageHistory{source: source}, nil
	}
	return &ImageHistory{source: source, Input: input, Count: CountInlineImages(input)}, nil
}
func CountInlineImages(input []any) int {
	count := 0
	for _, raw := range input {
		item, _ := raw.(object)
		field := "content"
		switch text(item["type"]) {
		case "", "message", "agent_message":
		case "function_call_output", "custom_tool_call_output":
			field = "output"
		default:
			continue
		}
		parts, _ := item[field].([]any)
		for _, p := range parts {
			part, _ := p.(object)
			if text(part["type"]) == "input_image" && strings.HasPrefix(strings.ToLower(text(part["image_url"])), "data:") {
				count++
			}
		}
	}
	return count
}
func (h *ImageHistory) Progress() ImageProgress {
	return ImageProgress{Count: len(h.Input), Digest: imageHistoryDigest(h.Input)}
}

// ImageCheckpoint authenticates a compact window echoed by the client without
// persisting the window, images, or conversation contents.
type ImageCheckpoint struct {
	Prefix ImageProgress
	Split  int
	Window ImageProgress
}

func (h *ImageHistory) Checkpoint(split int, window []any) (ImageCheckpoint, error) {
	if split <= 0 || split >= len(h.Input) || len(window) == 0 {
		return ImageCheckpoint{}, fmt.Errorf("invalid compaction boundary")
	}
	return ImageCheckpoint{Prefix: h.Progress(), Split: split, Window: ImageProgress{Count: len(window), Digest: imageHistoryDigest(window)}}, nil
}

// Reconcile consumes authenticated windows in order. New input was recorded
// before the emitted window, so it must be moved after it.
func (h *ImageHistory) Reconcile(checkpoints []ImageCheckpoint) (bool, error) {
	changed := false
	for _, cp := range checkpoints {
		n := cp.Prefix.Count
		w := cp.Window.Count
		if n <= 0 || cp.Split <= 0 || cp.Split >= n || w <= 0 {
			return false, fmt.Errorf("invalid image checkpoint; run compact manually")
		}
		if len(h.Input) < n || imageHistoryDigest(h.Input[:n]) != cp.Prefix.Digest {
			continue
		}
		if len(h.Input) == n {
			continue
		}
		if len(h.Input) < n+w || imageHistoryDigest(h.Input[n:n+w]) != cp.Window.Digest {
			return false, fmt.Errorf("compacted history does not match its checkpoint; run compact manually")
		}
		next := append([]any{}, h.Input[n:n+w]...)
		next = append(next, h.Input[cp.Split:n]...)
		next = append(next, h.Input[n+w:]...)
		h.Input = next
		changed = true
	}
	h.Count = CountInlineImages(h.Input)
	return changed, nil
}
func (h *ImageHistory) Body() ([]byte, error) { return h.WithInput(h.Input, false) }
func imageHistoryDigest(input []any) string {
	b, _ := json.Marshal(input)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Split retains the complete unconsumed tail. A saved prefix is authoritative;
// bootstrap accepts only an unambiguous user tail or complete terminal tool batch.
func (h *ImageHistory) Split(previous *ImageProgress) (int, error) {
	if previous != nil && previous.Count > 0 && previous.Count < len(h.Input) && imageHistoryDigest(h.Input[:previous.Count]) == previous.Digest {
		return previous.Count, nil
	}
	i := len(h.Input)
	for i > 0 {
		m, _ := h.Input[i-1].(object)
		if text(m["role"]) != "user" || (text(m["type"]) != "" && text(m["type"]) != "message") {
			break
		}
		i--
	}
	if i < len(h.Input) {
		return i, nil
	}
	// Tool batches must end in outputs and contain each matching complete call.
	needed := map[string]bool{}
	i = len(h.Input)
	for i > 0 {
		m, _ := h.Input[i-1].(object)
		k := text(m["type"])
		if k != "function_call_output" && k != "custom_tool_call_output" {
			break
		}
		id := text(m["call_id"])
		if id == "" || needed[id] {
			return 0, fmt.Errorf("ambiguous image tool history; run compact manually")
		}
		needed[id] = true
		i--
	}
	if len(needed) == 0 {
		return 0, fmt.Errorf("image history boundary unavailable; run compact manually")
	}
	for i > 0 && len(needed) > 0 {
		m, _ := h.Input[i-1].(object)
		k := text(m["type"])
		if k == "reasoning" {
			i--
			continue
		}
		if k != "function_call" && k != "custom_tool_call" {
			return 0, fmt.Errorf("ambiguous image tool history; run compact manually")
		}
		id := text(m["call_id"])
		if !needed[id] {
			return 0, fmt.Errorf("incomplete image tool batch; run compact manually")
		}
		delete(needed, id)
		i--
	}
	if len(needed) > 0 {
		return 0, fmt.Errorf("image tool call is unavailable; run compact manually")
	}
	return i, nil
}
func (h *ImageHistory) WithInput(input []any, compact bool) ([]byte, error) {
	source := make(object, len(h.source))
	for k, v := range h.source {
		source[k] = v
	}
	source["input"] = input
	if compact {
		source["input"] = append(append([]any{}, input...), object{"type": "compaction_trigger"})
		source["tool_choice"] = "none"
		delete(source, "text")
		delete(source, "response_format")
	}
	return json.Marshal(source)
}

// CompactWindow accepts an actual upstream compaction, not a fabricated summary.
// Retained messages remain part of the canonical window for the continuation.
func CompactWindow(response map[string]any) ([]any, []any, error) {
	if text(response["status"]) != "completed" {
		return nil, nil, fmt.Errorf("image compaction did not complete")
	}
	output, ok := response["output"].([]any)
	if !ok {
		return nil, nil, fmt.Errorf("image compaction returned no window")
	}
	var markers []any
	for _, raw := range output {
		m, ok := raw.(object)
		if !ok {
			return nil, nil, fmt.Errorf("invalid image compaction item")
		}
		switch text(m["type"]) {
		case "compaction":
			if text(m["encrypted_content"]) == "" {
				return nil, nil, fmt.Errorf("image compaction returned an empty state")
			}
			markers = append(markers, m)
		case "message":
		default:
			return nil, nil, fmt.Errorf("unexpected image compaction output item")
		}
	}
	if len(markers) != 1 {
		return nil, nil, fmt.Errorf("image compaction must return one state item")
	}
	return output, markers, nil
}

// ValidateImageBudget preserves raw upload byte protections even when old
// verified images are omitted from the active upstream window.
func ValidateImageBudget(raw []byte, maxImageMiB, maxTotalMiB int) error {
	h, err := InspectImageHistory(raw)
	if err != nil {
		return err
	}
	var total int64
	for _, v := range h.Input {
		item, _ := v.(object)
		field := "content"
		switch text(item["type"]) {
		case "", "message", "agent_message":
		case "function_call_output", "custom_tool_call_output":
			field = "output"
		default:
			continue
		}
		parts, _ := item[field].([]any)
		for _, v := range parts {
			part, _ := v.(object)
			url := text(part["image_url"])
			if text(part["type"]) != "input_image" || !strings.HasPrefix(strings.ToLower(url), "data:") {
				continue
			}
			_, payload, e := relayImagePayload(url, maxImageMiB)
			if e != nil {
				return e
			}
			size, e := io.Copy(io.Discard, io.LimitReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload)), int64(maxImageMiB)<<20+1))
			if e != nil || size == 0 || size > int64(maxImageMiB)<<20 {
				return fmt.Errorf("invalid or oversized inline image")
			}
			total += size
			if total > int64(maxTotalMiB)<<20 {
				return fmt.Errorf("inline images exceed the configured %d MiB raw request limit; run compact manually", maxTotalMiB)
			}
		}
	}
	return nil
}
