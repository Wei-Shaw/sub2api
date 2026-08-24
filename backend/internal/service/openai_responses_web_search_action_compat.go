package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// defaultOpenAIResponsesWebSearchActionCompatBufferedBytes bounds the amount of
// an upstream Responses stream that may wait for a later output_item.done.
// Crossing the bound fails open for the remainder of that stream.
const defaultOpenAIResponsesWebSearchActionCompatBufferedBytes = 512 * 1024

type openAIResponsesWebSearchActionCompatBody struct {
	*io.PipeReader
	source io.Closer
}

func (b *openAIResponsesWebSearchActionCompatBody) Close() error {
	readerErr := b.PipeReader.Close()
	sourceErr := b.source.Close()
	if readerErr != nil {
		return readerErr
	}
	return sourceErr
}

// newOpenAIResponsesWebSearchActionCompatStreamBody restores actions omitted
// from web_search_call added/completed events by copying the raw JSON value from
// the corresponding output_item.done. It deliberately works at the SSE frame
// boundary so unknown action fields survive unchanged.
func newOpenAIResponsesWebSearchActionCompatStreamBody(source io.ReadCloser, maxLineSize, maxBufferedBytes int) io.ReadCloser {
	reader, writer := io.Pipe()
	body := &openAIResponsesWebSearchActionCompatBody{PipeReader: reader, source: source}
	go transformOpenAIResponsesWebSearchActionCompatStream(source, writer, maxLineSize, maxBufferedBytes)
	return body
}

type openAIWebSearchActionCompatFrame struct {
	raw     []byte
	pending *openAIWebSearchActionCompatPending
}

type openAIWebSearchActionCompatPending struct {
	frame       *openAIWebSearchActionCompatFrame
	itemID      string
	outputIndex string
	actionPath  string
	resolved    bool
}

func transformOpenAIResponsesWebSearchActionCompatStream(source io.ReadCloser, destination *io.PipeWriter, maxLineSize, maxBufferedBytes int) {
	defer func() { _ = source.Close() }()
	if maxLineSize <= 0 {
		maxLineSize = defaultMaxLineSize
	}
	if maxBufferedBytes <= 0 {
		maxBufferedBytes = defaultOpenAIResponsesWebSearchActionCompatBufferedBytes
	}

	buffered := bufio.NewWriterSize(destination, 4*1024)
	write := func(raw []byte) error {
		_, err := buffered.Write(raw)
		return err
	}
	flush := func() error { return buffered.Flush() }

	var queued []*openAIWebSearchActionCompatFrame
	var pending []*openAIWebSearchActionCompatPending
	actionsByItemID := make(map[string]string)
	actionsByOutputIndex := make(map[string]string)
	bufferedBytes := 0
	disabled := false
	rememberAction := func(itemID, outputIndex, actionRaw string) {
		if itemID != "" {
			actionsByItemID[itemID] = actionRaw
		}
		if outputIndex != "" {
			actionsByOutputIndex[outputIndex] = actionRaw
		}
	}
	knownAction := func(itemID, outputIndex string) string {
		if itemID != "" {
			return actionsByItemID[itemID]
		}
		return actionsByOutputIndex[outputIndex]
	}
	forgetAction := func(itemID, outputIndex string) {
		if itemID != "" {
			delete(actionsByItemID, itemID)
		}
		if outputIndex != "" {
			delete(actionsByOutputIndex, outputIndex)
		}
	}

	removePending := func(target *openAIWebSearchActionCompatPending) {
		for i, candidate := range pending {
			if candidate == target {
				pending = append(pending[:i], pending[i+1:]...)
				return
			}
		}
	}
	flushQueued := func() error {
		for _, frame := range queued {
			if err := write(frame.raw); err != nil {
				return err
			}
		}
		queued = queued[:0]
		pending = pending[:0]
		bufferedBytes = 0
		return flush()
	}
	flushReady := func() error {
		for len(queued) > 0 {
			frame := queued[0]
			if frame.pending != nil && !frame.pending.resolved {
				break
			}
			if err := write(frame.raw); err != nil {
				return err
			}
			queued = queued[1:]
			bufferedBytes -= len(frame.raw)
			if frame.pending != nil {
				removePending(frame.pending)
			}
		}
		if len(queued) == 0 {
			return flush()
		}
		return nil
	}
	queue := func(frame *openAIWebSearchActionCompatFrame) error {
		queued = append(queued, frame)
		bufferedBytes += len(frame.raw)
		if bufferedBytes > maxBufferedBytes {
			disabled = true
			return flushQueued()
		}
		return nil
	}
	failOpenRaw := func(parts ...[]byte) error {
		disabled = true
		if len(queued) > 0 {
			if err := flushQueued(); err != nil {
				return err
			}
		}
		for _, part := range parts {
			if len(part) == 0 {
				continue
			}
			if err := write(part); err != nil {
				return err
			}
		}
		return flush()
	}

	processFrame := func(raw []byte) error {
		if disabled {
			if err := write(raw); err != nil {
				return err
			}
			return flush()
		}

		payload, validSingleData := openAIWebSearchActionCompatSingleData(raw)
		if !validSingleData {
			if len(queued) == 0 {
				if err := write(raw); err != nil {
					return err
				}
				return flush()
			}
			return queue(&openAIWebSearchActionCompatFrame{raw: raw})
		}
		if bytes.Equal(bytes.TrimSpace(payload), []byte("[DONE]")) {
			if len(queued) == 0 {
				if err := write(raw); err != nil {
					return err
				}
				return flush()
			}
			if err := queue(&openAIWebSearchActionCompatFrame{raw: raw}); err != nil {
				return err
			}
			return flushQueued()
		}
		if !json.Valid(payload) {
			if len(queued) == 0 {
				if err := write(raw); err != nil {
					return err
				}
				return flush()
			}
			return queue(&openAIWebSearchActionCompatFrame{raw: raw})
		}

		eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
		if openAIWebSearchActionCompatTerminal(eventType) {
			if len(queued) == 0 {
				if err := write(raw); err != nil {
					return err
				}
				return flush()
			}
			if err := queue(&openAIWebSearchActionCompatFrame{raw: raw}); err != nil {
				return err
			}
			return flushQueued()
		}

		if eventType == "response.output_item.added" && strings.TrimSpace(gjson.GetBytes(payload, "item.type").String()) == "web_search_call" {
			if actionRaw := openAIWebSearchActionCompatActionRaw(gjson.GetBytes(payload, "item.action")); actionRaw != "" {
				rememberAction(strings.TrimSpace(gjson.GetBytes(payload, "item.id").String()), openAIWebSearchActionCompatIndex(payload), actionRaw)
			}
		}

		if itemID, outputIndex, actionPath, missingAction := openAIWebSearchActionCompatMissingActionTarget(eventType, payload); missingAction {
			frame := &openAIWebSearchActionCompatFrame{raw: raw}
			if actionRaw := knownAction(itemID, outputIndex); actionRaw != "" {
				if patched, err := openAIWebSearchActionCompatInsertRawAction(frame.raw, actionPath, actionRaw); err == nil {
					frame.raw = patched
				}
			}
			if eventType == "response.web_search_call.completed" {
				forgetAction(itemID, outputIndex)
			}
			if !bytes.Equal(frame.raw, raw) {
				if len(queued) == 0 {
					if err := write(frame.raw); err != nil {
						return err
					}
					return flush()
				}
				if err := queue(frame); err != nil {
					return err
				}
				return flushReady()
			}
			pendingItem := &openAIWebSearchActionCompatPending{
				frame:       frame,
				itemID:      itemID,
				outputIndex: outputIndex,
				actionPath:  actionPath,
			}
			frame.pending = pendingItem
			pending = append(pending, pendingItem)
			return queue(frame)
		}
		if eventType == "response.web_search_call.completed" {
			forgetAction(strings.TrimSpace(gjson.GetBytes(payload, "item_id").String()), openAIWebSearchActionCompatIndex(payload))
		}

		frame := &openAIWebSearchActionCompatFrame{raw: raw}
		if eventType == "response.output_item.done" {
			itemID := strings.TrimSpace(gjson.GetBytes(payload, "item.id").String())
			outputIndex := openAIWebSearchActionCompatIndex(payload)
			actionRaw := openAIWebSearchActionCompatActionRaw(gjson.GetBytes(payload, "item.action"))
			for _, target := range openAIWebSearchActionCompatMatchingPending(pending, itemID, outputIndex) {
				if actionRaw != "" {
					if patched, err := openAIWebSearchActionCompatInsertRawAction(target.frame.raw, target.actionPath, actionRaw); err == nil {
						target.frame.raw = patched
					}
				}
				// A matching done item is the last authoritative chance to obtain
				// its action. If it is absent or unusable, release the original
				// frame instead of stalling the rest of the response.
				target.resolved = true
			}
			forgetAction(itemID, outputIndex)
		}

		if len(queued) == 0 {
			if err := write(frame.raw); err != nil {
				return err
			}
			return flush()
		}
		if err := queue(frame); err != nil {
			return err
		}
		return flushReady()
	}

	readerSize := 64 * 1024
	if maxLineSize < readerSize {
		readerSize = maxLineSize
	}
	if readerSize < 1 {
		readerSize = 1
	}
	reader := bufio.NewReaderSize(source, readerSize)
	var frame []byte
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > 0 {
			if disabled {
				if writeErr := failOpenRaw(fragment); writeErr != nil {
					_ = destination.CloseWithError(writeErr)
					return
				}
			} else if len(fragment) > maxLineSize-len(line) {
				if writeErr := failOpenRaw(frame, line, fragment); writeErr != nil {
					_ = destination.CloseWithError(writeErr)
					return
				}
				frame = nil
				line = nil
			} else {
				line = append(line, fragment...)
				if err != bufio.ErrBufferFull {
					completeLine := line
					line = nil
					if len(completeLine) > maxBufferedBytes-len(frame) {
						if writeErr := failOpenRaw(frame, completeLine); writeErr != nil {
							_ = destination.CloseWithError(writeErr)
							return
						}
						frame = nil
					} else {
						frame = append(frame, completeLine...)
						if openAIWebSearchActionCompatBlankLine(completeLine) {
							if processErr := processFrame(frame); processErr != nil {
								_ = destination.CloseWithError(processErr)
								return
							}
							frame = nil
						}
					}
				}
			}
		}
		if err != nil && err != bufio.ErrBufferFull {
			if err != io.EOF {
				if len(frame) > 0 {
					if processErr := processFrame(frame); processErr != nil {
						_ = destination.CloseWithError(processErr)
						return
					}
				}
				if len(queued) > 0 {
					if processErr := flushQueued(); processErr != nil {
						_ = destination.CloseWithError(processErr)
						return
					}
				}
				_ = destination.CloseWithError(err)
				return
			}
			break
		}
	}
	if !disabled && len(frame) > 0 {
		if err := processFrame(frame); err != nil {
			_ = destination.CloseWithError(err)
			return
		}
	}
	if !disabled && len(queued) > 0 {
		if err := flushQueued(); err != nil {
			_ = destination.CloseWithError(err)
			return
		}
	}
	_ = destination.Close()
}

func openAIWebSearchActionCompatBlankLine(line []byte) bool {
	return bytes.Equal(line, []byte("\n")) || bytes.Equal(line, []byte("\r\n"))
}

// openAIWebSearchActionCompatSingleData accepts only one ordinary data line.
// Comment-only, multi-data, malformed and non-data frames are all transparent.
func openAIWebSearchActionCompatSingleData(raw []byte) ([]byte, bool) {
	lines := bytes.SplitAfter(raw, []byte("\n"))
	var payload []byte
	dataCount := 0
	for _, line := range lines {
		line = bytes.TrimSuffix(line, []byte("\n"))
		line = bytes.TrimSuffix(line, []byte("\r"))
		if bytes.HasPrefix(line, []byte("data:")) {
			dataCount++
			if dataCount > 1 {
				return nil, false
			}
			payload = bytes.TrimSpace(line[len("data:"):])
		}
	}
	return payload, dataCount == 1
}

func openAIWebSearchActionCompatTerminal(eventType string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.incomplete", "response.done", "response.cancelled", "response.canceled", "error":
		return true
	default:
		return false
	}
}

func openAIWebSearchActionCompatIndex(payload []byte) string {
	result := gjson.GetBytes(payload, "output_index")
	if !result.Exists() {
		return ""
	}
	return result.Raw
}

func openAIWebSearchActionCompatMatchingPending(pending []*openAIWebSearchActionCompatPending, itemID, outputIndex string) []*openAIWebSearchActionCompatPending {
	if itemID != "" {
		var matched []*openAIWebSearchActionCompatPending
		for _, candidate := range pending {
			if candidate.itemID == itemID {
				matched = append(matched, candidate)
			}
		}
		return matched
	}
	if outputIndex == "" {
		return nil
	}
	var matched []*openAIWebSearchActionCompatPending
	for _, candidate := range pending {
		if candidate.outputIndex == outputIndex {
			matched = append(matched, candidate)
		}
	}
	return matched
}

func openAIWebSearchActionCompatMissingActionTarget(eventType string, payload []byte) (itemID, outputIndex, actionPath string, missing bool) {
	outputIndex = openAIWebSearchActionCompatIndex(payload)
	switch eventType {
	case "response.web_search_call.completed":
		if openAIWebSearchActionCompatActionRaw(gjson.GetBytes(payload, "action")) != "" {
			return "", "", "", false
		}
		return strings.TrimSpace(gjson.GetBytes(payload, "item_id").String()), outputIndex, "action", true
	case "response.output_item.added":
		if strings.TrimSpace(gjson.GetBytes(payload, "item.type").String()) != "web_search_call" || openAIWebSearchActionCompatActionRaw(gjson.GetBytes(payload, "item.action")) != "" {
			return "", "", "", false
		}
		return strings.TrimSpace(gjson.GetBytes(payload, "item.id").String()), outputIndex, "item.action", true
	default:
		return "", "", "", false
	}
}

func openAIWebSearchActionCompatActionRaw(result gjson.Result) string {
	actionRaw := strings.TrimSpace(result.Raw)
	if !result.Exists() || !openAIWebSearchActionCompatUsableActionRaw(actionRaw) {
		return ""
	}
	return actionRaw
}

func openAIWebSearchActionCompatUsableActionRaw(actionRaw string) bool {
	actionRaw = strings.TrimSpace(actionRaw)
	if len(actionRaw) < 2 || actionRaw[0] != '{' || actionRaw[len(actionRaw)-1] != '}' || !json.Valid([]byte(actionRaw)) {
		return false
	}
	actionType := gjson.Get(actionRaw, "type")
	return actionType.Type == gjson.String && strings.TrimSpace(actionType.String()) != ""
}

func openAIWebSearchActionCompatInsertRawAction(rawFrame []byte, actionPath, actionRaw string) ([]byte, error) {
	payload, ok := openAIWebSearchActionCompatSingleData(rawFrame)
	if !ok || !gjson.ValidBytes(payload) || !openAIWebSearchActionCompatUsableActionRaw(actionRaw) {
		return nil, fmt.Errorf("invalid web search completed frame")
	}
	patched, err := sjson.SetRawBytes(payload, actionPath, []byte(actionRaw))
	if err != nil {
		return nil, err
	}
	dataOffset, lineEnd, ok := openAIWebSearchActionCompatDataLineBounds(rawFrame)
	if !ok {
		return nil, fmt.Errorf("missing data line")
	}
	prefixEnd := dataOffset + len("data:")
	for prefixEnd < lineEnd && (rawFrame[prefixEnd] == ' ' || rawFrame[prefixEnd] == '\t') {
		prefixEnd++
	}
	out := make([]byte, 0, len(rawFrame)+len(patched)-len(payload))
	out = append(out, rawFrame[:prefixEnd]...)
	out = append(out, patched...)
	out = append(out, rawFrame[lineEnd:]...)
	return out, nil
}

func openAIWebSearchActionCompatDataLineBounds(raw []byte) (start, end int, ok bool) {
	for start < len(raw) {
		end = bytes.IndexByte(raw[start:], '\n')
		if end < 0 {
			end = len(raw)
		} else {
			end += start
		}
		lineEnd := end
		if lineEnd > start && raw[lineEnd-1] == '\r' {
			lineEnd--
		}
		if bytes.HasPrefix(raw[start:lineEnd], []byte("data:")) {
			return start, lineEnd, true
		}
		if end == len(raw) {
			break
		}
		start = end + 1
	}
	return 0, 0, false
}
