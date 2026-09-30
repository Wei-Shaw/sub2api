package basispoints

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// WithCompactedWindow emits the real compacted window before the continuation
// in one Responses lifecycle. Retained items must not be pruned from the window.
func WithCompactedWindow(ctx context.Context, upstream io.ReadCloser, window []any, usage map[string]any) io.ReadCloser {
	reader, writer := io.Pipe()
	go func() {
		defer func() { _ = upstream.Close() }()
		stop := context.AfterFunc(ctx, func() { _ = upstream.Close(); _ = writer.CloseWithError(ctx.Err()) })
		defer stop()
		sequence := 0
		inserted := false
		progressiveUsage := object{}
		emit := func(kind string, p object) error {
			p["type"] = kind
			p["sequence_number"] = sequence
			sequence++
			raw, e := json.Marshal(p)
			if e != nil {
				return e
			}
			_, e = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", kind, raw)
			return e
		}
		insert := func() error {
			if inserted {
				return nil
			}
			inserted = true
			for i, v := range window {
				if e := emit("response.output_item.added", object{"output_index": i, "item": v}); e != nil {
					return e
				}
				if e := emit("response.output_item.done", object{"output_index": i, "item": v}); e != nil {
					return e
				}
			}
			return nil
		}
		err := readEvents(upstream, func(event string, data []byte) error {
			if string(data) == "[DONE]" {
				return nil
			}
			var p object
			if e := decode(data, &p); e != nil {
				return e
			}
			kind := text(p["type"])
			if kind == "" {
				kind = event
			}
			if kind != "response.created" && kind != "response.in_progress" {
				if e := insert(); e != nil {
					return e
				}
			}
			if index, ok := p["output_index"]; ok {
				switch n := index.(type) {
				case json.Number:
					i, e := n.Int64()
					if e != nil {
						return e
					}
					p["output_index"] = i + int64(len(window))
				case float64:
					p["output_index"] = n + float64(len(window))
				}
			}
			response, _ := p["response"].(object)
			observed, _ := response["usage"].(object)
			if observed == nil {
				observed, _ = p["usage"].(object)
			}
			if kind != "response.completed" && kind != "response.failed" && kind != "response.cancelled" && kind != "response.incomplete" {
				copyNonzeroImageUsage(progressiveUsage, observed)
			}
			if response != nil && (kind == "response.completed" || kind == "response.failed" || kind == "response.cancelled" || kind == "response.incomplete") {
				output, _ := response["output"].([]any)
				response["output"] = append(append([]any{}, window...), output...)
				current, _ := response["usage"].(object)
				if !hasImageUsage(current) {
					current = progressiveUsage
					response["usage"] = current
				}
				mergeImageUsage(current, usage)
			}
			return emit(kind, p)
		})
		_ = writer.CloseWithError(err)
	}()
	return &imagePolicyStream{PipeReader: reader, upstream: upstream}
}

type imagePolicyStream struct {
	*io.PipeReader
	upstream io.ReadCloser
}

func (s *imagePolicyStream) Close() error { _ = s.upstream.Close(); return s.PipeReader.Close() }
func mergeImageUsage(dst, src object) {
	for k, v := range src {
		if child, ok := v.(object); ok {
			target, _ := dst[k].(object)
			if target == nil {
				target = object{}
				dst[k] = target
			}
			mergeImageUsage(target, child)
			continue
		}
		var a, b json.Number
		switch n := v.(type) {
		case json.Number:
			b = n
		case float64:
			b = json.Number(fmt.Sprint(n))
		case int:
			b = json.Number(fmt.Sprint(n))
		default:
			continue
		}
		switch n := dst[k].(type) {
		case json.Number:
			a = n
		case float64:
			a = json.Number(fmt.Sprint(n))
		case int:
			a = json.Number(fmt.Sprint(n))
		}
		x, _ := a.Int64()
		y, _ := b.Int64()
		dst[k] = x + y
	}
}

// ReadImageCompaction observes reported usage even when no successful terminal
// arrives. The caller never sees intermediate text or executable tool events.
func ReadImageCompaction(reader io.Reader, observe func([]byte)) (map[string]any, error) {
	var response object
	completed := false
	progressiveUsage := object{}
	err := readEvents(io.LimitReader(reader, 32<<20), func(event string, raw []byte) error {
		if string(raw) == "[DONE]" {
			return nil
		}
		var p object
		if e := decode(raw, &p); e != nil {
			return e
		}
		if observe != nil {
			observe(raw)
		}
		r, _ := p["response"].(object)
		u, _ := r["usage"].(object)
		if u == nil {
			u, _ = p["usage"].(object)
		}
		copyNonzeroImageUsage(progressiveUsage, u)
		kind := text(p["type"])
		if kind == "" {
			kind = event
		}
		item, _ := p["item"].(object)
		if isTool(item) || isToolEvent(kind) {
			return fmt.Errorf("image compaction returned an unexpected tool event")
		}
		switch kind {
		case "response.completed", "response.failed", "response.cancelled", "response.incomplete", "error":
			response, _ = p["response"].(object)
			completed = kind == "response.completed"
			return io.EOF
		}
		return nil
	})
	if response != nil {
		u, _ := response["usage"].(object)
		if !hasImageUsage(u) {
			response["usage"] = progressiveUsage
		}
	}
	if err != nil && err != io.EOF {
		return response, err
	}
	if !completed || response == nil {
		return response, fmt.Errorf("image history compaction did not complete")
	}
	return response, nil
}

// Positive progressive fields are snapshots; a nonzero terminal is authoritative.
func hasImageUsage(usage object) bool {
	for _, v := range usage {
		if nested, ok := v.(object); ok {
			if hasImageUsage(nested) {
				return true
			}
			continue
		}
		if n, ok := v.(json.Number); ok {
			f, _ := n.Float64()
			if f > 0 {
				return true
			}
		}
	}
	return false
}
func copyNonzeroImageUsage(dst, src object) {
	for k, v := range src {
		if nested, ok := v.(object); ok {
			target, _ := dst[k].(object)
			if target == nil {
				target = object{}
				dst[k] = target
			}
			copyNonzeroImageUsage(target, nested)
		} else if n, ok := v.(json.Number); ok {
			f, _ := n.Float64()
			if f > 0 {
				dst[k] = v
			}
		}
	}
}
