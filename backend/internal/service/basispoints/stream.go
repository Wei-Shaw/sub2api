package basispoints

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

type protocolError struct{ error }

func (e protocolError) Unwrap() error { return e.error }

type streamBody struct {
	*io.PipeReader
	upstream io.ReadCloser
	once     sync.Once
	err      error
	cancel   context.CancelFunc
}

func (b *streamBody) closeUpstream() error {
	b.once.Do(func() { b.err = b.upstream.Close() })
	return b.err
}

func (b *streamBody) Close() error {
	b.cancel()
	readerErr := b.PipeReader.Close()
	return errors.Join(readerErr, b.closeUpstream())
}

// Stream keeps ordinary text incremental while withholding native tool events
// and structured final answers until validated.
// Closing the downstream body interrupts an upstream read or a blocked pipe write.
func (b *Bridge) Stream(upstream io.ReadCloser) io.ReadCloser {
	return b.StreamWithToolRepair(context.Background(), upstream, nil)
}

// StreamWithToolRepair permits bounded native tool-error continuations before
// dispatch. Closing the stream cancels both the active request and correction.
func (b *Bridge) StreamWithToolRepair(ctx context.Context, upstream io.ReadCloser, repair ToolRepairFunc) io.ReadCloser {
	return b.StreamWithRepairs(ctx, upstream, repair, nil)
}

// StreamWithRepair adds only first-turn unknown-target recovery.
func (b *Bridge) StreamWithRepair(ctx context.Context, upstream io.ReadCloser, repair RepairToolCall) io.ReadCloser {
	return b.StreamWithRepairs(ctx, upstream, nil, repair)
}

// StreamWithRepairs keeps known-target transport corrections and first-turn
// unknown-target regeneration separate; neither can dispatch unvalidated tools.
func (b *Bridge) StreamWithRepairs(ctx context.Context, upstream io.ReadCloser, repair ToolRepairFunc, unknown RepairToolCall) io.ReadCloser {
	ctx, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	body := &streamBody{PipeReader: reader, upstream: upstream, cancel: cancel}
	var continueTool ToolRepairFunc
	if repair != nil {
		continueTool = func(ctx context.Context, response object, validation error) (object, error) {
			// Release the first HTTP response's concurrency lease before issuing
			// another request, including accounts with concurrency set to one.
			_ = body.closeUpstream()
			return repair(ctx, response, validation)
		}
	}
	var regenerate RepairToolCall
	if unknown != nil {
		regenerate = func(ctx context.Context) (io.ReadCloser, error) {
			_ = body.closeUpstream()
			return unknown(ctx)
		}
	}
	go func() {
		defer cancel()
		stop := context.AfterFunc(ctx, func() {
			_ = writer.CloseWithError(ctx.Err())
			_ = body.closeUpstream()
		})
		defer stop()
		err := b.transformWithRepairs(ctx, upstream, writer, continueTool, regenerate)
		_ = body.closeUpstream()
		_ = writer.CloseWithError(err)
	}()
	return body
}

func (b *Bridge) transformWithRepairs(ctx context.Context, reader io.Reader, writer io.Writer, repair ToolRepairFunc, unknown RepairToolCall) error {
	sequence := 0
	terminal := false
	visibleContent := false
	var terminalResponse object
	emitted := make(map[string]bool)
	pendingTools := make(map[string]bool)
	emit := func(kind string, payload object) error {
		payload["type"] = kind
		payload["sequence_number"] = sequence
		sequence++
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", kind, raw)
		if err == nil && b.clientStream && hasVisibleStreamContent(kind, payload) {
			visibleContent = true
		}
		return err
	}
	emitTool := func(item object, index any) error {
		id := text(item["id"])
		if emitted[id] {
			return nil
		}
		emitted[id] = true
		field, prefix := "arguments", "response.function_call_arguments"
		if text(item["type"]) == "custom_tool_call" {
			field, prefix = "input", "response.custom_tool_call_input"
		}
		added := make(object, len(item))
		for k, v := range item {
			added[k] = v
		}
		added[field], added["status"] = "", "in_progress"
		if err := emit("response.output_item.added", object{"output_index": index, "item": added}); err != nil {
			return err
		}
		if err := emit(prefix+".delta", object{"output_index": index, "item_id": id, "delta": item[field]}); err != nil {
			return err
		}
		if err := emit(prefix+".done", object{"output_index": index, "item_id": id, field: item[field]}); err != nil {
			return err
		}
		return emit("response.output_item.done", object{"output_index": index, "item": item})
	}
	process := func(event string, data []byte) error {
		if string(data) == "[DONE]" {
			return nil
		}
		var payload object
		if decode(data, &payload) != nil || payload == nil {
			return fmt.Errorf("invalid Basispoints SSE event")
		}
		kind := text(payload["type"])
		if kind == "" {
			kind = event
		}
		if b.structured != nil && kind == "response.completed" {
			if response, ok := payload["response"].(object); !ok || response == nil {
				return fmt.Errorf("basispoints structured output is missing its terminal response")
			}
		}
		if isToolEvent(kind) {
			return nil
		}
		item, _ := payload["item"].(object)
		if b.structured != nil && isStructuredMessageEvent(kind, item) {
			return nil
		}
		if kind == "response.output_item.added" && isTool(item) {
			return nil
		}
		if kind == "response.output_item.done" && isTool(item) {
			// Only the terminal response contains the authoritative native item.
			// Text keeps streaming; tool calls wait until the whole response validates.
			if len(pendingTools) >= 1024 {
				return fmt.Errorf("basispoints response contains too many tool items")
			}
			pendingTools[text(item["call_id"])+"\x00"+text(item["id"])] = true
			return nil
		}
		if response, ok := payload["response"].(object); ok {
			if b.structured != nil {
				config, _ := response["text"].(object)
				if config == nil {
					config = make(object)
				}
				config["format"] = b.structured.format
				response["text"] = config
			}
			if kind == "response.completed" {
				terminalResponse = response
				if b.structured != nil {
					if err := b.structured.validate(response); err != nil {
						return err
					}
				}
				output, _ := response["output"].([]any)
				for _, raw := range output {
					item, _ := raw.(object)
					if isTool(item) {
						delete(pendingTools, text(item["call_id"])+"\x00"+text(item["id"]))
					}
				}
				if len(pendingTools) != 0 {
					return fmt.Errorf("basispoints completed response omitted an original tool item")
				}
				repairStart := -1
				validation := b.validateToolResponse(response)
				if unknown != nil && b.canRepair(response, validation) {
					// Regeneration can change the answer after the client has read it.
					// Known-target corrections below only replace withheld tool slots.
					if visibleContent {
						return validation
					}
					callback := unknown
					unknown = nil
					fixed, index, err := b.repairResponse(ctx, response, callback)
					if err != nil {
						return err
					}
					response = fixed
					payload["response"] = fixed
					terminalResponse = fixed
					repairStart = index
					if b.structured != nil {
						if err := b.structured.validate(response); err != nil {
							return err
						}
					}
				} else if err := b.translateCompleted(ctx, response, repair); err != nil {
					return err
				}

				output, _ = response["output"].([]any)
				for i, raw := range output {
					item, _ := raw.(object)
					if isTool(item) {
						if err := emitTool(item, i); err != nil {
							return err
						}
					} else if (b.structured != nil || repairStart >= 0 && i >= repairStart) && text(item["type"]) == "message" {
						if err := emitStructuredMessage(item, i, emit); err != nil {
							return err
						}
					}
				}
			} else {
				// Never expose native or incomplete tool arguments to the client.
				output, _ := response["output"].([]any)
				filtered := make([]any, 0, len(output))
				for _, raw := range output {
					item, _ := raw.(object)
					if !isTool(item) && (b.structured == nil || text(item["type"]) != "message") {
						filtered = append(filtered, raw)
					}
				}
				response["output"] = filtered
				response["reasoning"] = object{"effort": b.Effort}
			}
		}
		terminal = kind == "response.completed" || kind == "response.incomplete" || kind == "response.failed" || kind == "response.cancelled" || kind == "error"
		return emit(kind, normalizeFailureEvent(kind, payload))
	}
	err := readEvents(reader, func(event string, data []byte) error {
		if terminal {
			return io.EOF
		}
		if err := process(event, data); err != nil {
			return protocolError{err}
		}
		if terminal {
			return io.EOF
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		var invalid protocolError
		if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &invalid) {
			return err
		}
		failed := object{
			"status": "failed", "output": []any{},
			"error": object{"code": "basispoints_protocol_error", "message": err.Error()},
		}
		var upstreamFailure *UpstreamFailure
		if errors.As(err, &upstreamFailure) {
			failed["error"] = upstreamFailure.Details()
		}
		for _, field := range []string{"id", "model", "usage"} {
			if value := terminalResponse[field]; value != nil {
				failed[field] = value
			}
		}
		return emit("response.failed", object{"response": failed})
	}
	if !terminal {
		return io.ErrUnexpectedEOF
	}
	return nil
}

// Empty lifecycle events and opaque reasoning do not commit visible content.
// Also recognize content carried by done/item events when deltas are absent.
func hasVisibleStreamContent(kind string, payload object) bool {
	partHasText := func(part object) bool {
		return text(part["text"]) != "" || text(part["refusal"]) != ""
	}
	switch kind {
	case "response.output_text.delta", "response.reasoning_summary_text.delta", "response.refusal.delta":
		return text(payload["delta"]) != ""
	case "response.output_text.done", "response.reasoning_summary_text.done", "response.refusal.done":
		return partHasText(payload)
	case "response.content_part.added", "response.content_part.done", "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		part, _ := payload["part"].(object)
		return partHasText(part)
	case "response.output_item.added", "response.output_item.done":
		item, _ := payload["item"].(object)
		field := "content"
		switch text(item["type"]) {
		case "message":
		case "reasoning":
			field = "summary"
		default:
			return false
		}
		parts, _ := item[field].([]any)
		for _, raw := range parts {
			part, _ := raw.(object)
			if partHasText(part) {
				return true
			}
		}
	}
	return false
}

func readEvents(reader io.Reader, consume func(string, []byte) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	var data strings.Builder
	event := ""
	flush := func() error {
		if data.Len() == 0 {
			event = ""
			return nil
		}
		err := consume(event, []byte(strings.TrimSuffix(data.String(), "\n")))
		data.Reset()
		event = ""
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
		} else if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			_, _ = data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			_ = data.WriteByte('\n')
			if data.Len() > 16<<20 {
				return protocolError{fmt.Errorf("basispoints SSE event exceeds 16 MiB")}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return protocolError{fmt.Errorf("basispoints SSE line exceeds 16 MiB")}
		}
		return err
	}
	return flush()
}
