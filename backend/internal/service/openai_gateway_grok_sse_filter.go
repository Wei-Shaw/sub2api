package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// OpenAI Responses SSE event types are a closed enum for strict clients
// (grok CLI, Codex CLI): an unknown `event: ping` frame aborts the whole
// turn. Vendor gateways behind Grok subscriptions inject such frames for
// billing/keepalive, so ping frames are rewritten into an SSE comment that
// every parser ignores while the connection still looks alive downstream.
var grokResponsesPingComment = []byte(": ping\n\n")

// A vendor ping frame is one event line plus one small data line. Cap what is
// buffered while deciding, so an upstream streaming a frame that never ends
// cannot grow gateway memory; frames over the cap are passed through as-is.
const (
	grokResponsesPingFrameMaxLines = 16
	grokResponsesPingFrameMaxBytes = 16 * 1024
)

type grokResponsesBillingPingFilterBody struct {
	*io.PipeReader
	source    io.Closer
	closeOnce sync.Once
	closeErr  error
}

func (b *grokResponsesBillingPingFilterBody) Close() error {
	readerErr := b.PipeReader.Close()
	sourceErr := b.closeSource()
	if readerErr != nil {
		return readerErr
	}
	return sourceErr
}

func (b *grokResponsesBillingPingFilterBody) closeSource() error {
	b.closeOnce.Do(func() { b.closeErr = b.source.Close() })
	return b.closeErr
}

func newGrokResponsesBillingPingFilterBody(source io.ReadCloser, account *Account, maxLineSize int) io.ReadCloser {
	if account == nil || account.Platform != PlatformGrok {
		return source
	}
	reader, writer := io.Pipe()
	body := &grokResponsesBillingPingFilterBody{PipeReader: reader, source: source}
	go filterGrokResponsesBillingPings(source, writer, body.closeSource, maxLineSize)
	return body
}

func filterGrokResponsesBillingPings(
	source io.Reader,
	destination *io.PipeWriter,
	closeSource func() error,
	maxLineSize int,
) {
	defer func() { _ = closeSource() }()
	if maxLineSize <= 0 {
		maxLineSize = defaultMaxLineSize
	}

	scanner := bufio.NewScanner(source)
	scanBuf := getSSEScannerBuf64K()
	defer putSSEScannerBuf64K(scanBuf)
	initialBufferSize := len(scanBuf)
	if maxLineSize < initialBufferSize {
		initialBufferSize = maxLineSize
	}
	scanner.Buffer(scanBuf[:0:initialBufferSize], maxLineSize)
	scanner.Split(scanSSELinesPreservingEndings)

	// Only frames opened by an `event: ping` line are buffered (pingFrame);
	// every other frame streams through line by line without copying.
	pingFrame := make([][]byte, 0, 3)
	pingFrameBytes := 0
	inPassthroughFrame := false

	replayPingFrame := func() error {
		for _, line := range pingFrame {
			if _, err := destination.Write(line); err != nil {
				return err
			}
		}
		pingFrame = pingFrame[:0]
		pingFrameBytes = 0
		return nil
	}
	// endPingFrame decides a complete buffered candidate: vendor ping frames
	// become an SSE comment, everything else is replayed verbatim. blankLine
	// is nil when the stream ends inside the frame.
	endPingFrame := func(blankLine []byte) error {
		if isGrokResponsesPingEventFrame(pingFrame) {
			pingFrame = pingFrame[:0]
			pingFrameBytes = 0
			_, err := destination.Write(grokResponsesPingComment)
			return err
		}
		if err := replayPingFrame(); err != nil {
			return err
		}
		if blankLine == nil {
			return nil
		}
		_, err := destination.Write(blankLine)
		return err
	}
	abort := func(err error) { _ = destination.CloseWithError(err) }

	for scanner.Scan() {
		line := scanner.Bytes()
		isBlank := len(trimSSELineEnding(line)) == 0

		if inPassthroughFrame {
			if _, err := destination.Write(line); err != nil {
				abort(err)
				return
			}
			if isBlank {
				inPassthroughFrame = false
			}
			continue
		}

		if len(pingFrame) > 0 {
			if isBlank {
				if err := endPingFrame(line); err != nil {
					abort(err)
					return
				}
				continue
			}
			if canExtendGrokResponsesPingFrame(line) &&
				len(pingFrame) < grokResponsesPingFrameMaxLines &&
				pingFrameBytes+len(line) <= grokResponsesPingFrameMaxBytes {
				pingFrame = append(pingFrame, append([]byte(nil), line...))
				pingFrameBytes += len(line)
				continue
			}
			// Not a filterable ping frame after all (unexpected field line,
			// or past the buffering caps): replay it and stream the rest of
			// the frame through unchanged.
			if err := replayPingFrame(); err != nil {
				abort(err)
				return
			}
			if _, err := destination.Write(line); err != nil {
				abort(err)
				return
			}
			inPassthroughFrame = true
			continue
		}

		// Frame start: only `event: ping` opens a buffered candidate.
		if !isBlank {
			if value, ok := extractOpenAISSEEventLine(string(trimSSELineEnding(line))); ok && value == "ping" {
				pingFrame = append(pingFrame, append([]byte(nil), line...))
				pingFrameBytes = len(line)
				continue
			}
		}
		if _, err := destination.Write(line); err != nil {
			abort(err)
			return
		}
		inPassthroughFrame = !isBlank
	}
	if len(pingFrame) > 0 {
		if err := endPingFrame(nil); err != nil {
			abort(err)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		abort(fmt.Errorf("filter Grok Responses billing ping: %w", err))
		return
	}
	_ = destination.Close()
}

func scanSSELinesPreservingEndings(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for index, value := range data {
		switch value {
		case '\n':
			return index + 1, data[:index+1], nil
		case '\r':
			if index+1 == len(data) && !atEOF {
				return 0, nil, nil
			}
			if index+1 < len(data) && data[index+1] == '\n' {
				return index + 2, data[:index+2], nil
			}
			return index + 1, data[:index+1], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func trimSSELineEnding(line []byte) []byte {
	return bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
}

// canExtendGrokResponsesPingFrame reports whether a line may still belong to a
// vendor ping frame: only data lines and SSE comments. Any other field (a
// second event line, id, retry, ...) means the frame is not a plain ping.
func canExtendGrokResponsesPingFrame(rawLine []byte) bool {
	line := trimSSELineEnding(rawLine)
	if len(line) > 0 && line[0] == ':' {
		return true
	}
	_, ok := extractOpenAISSEDataLine(string(line))
	return ok
}

// isGrokResponsesPingEventFrame decides a buffered candidate whose first line
// is already `event: ping`. The only candidates replayed verbatim are frames
// whose data payload declares a different event type than the SSE event line;
// every other shape (billing cost, keepalive, no data, malformed JSON) would
// break strict Responses clients and is rewritten into a comment.
func isGrokResponsesPingEventFrame(rawLines [][]byte) bool {
	dataParts := make([]string, 0, 1)
	for _, rawLine := range rawLines[1:] {
		if value, ok := extractOpenAISSEDataLine(string(trimSSELineEnding(rawLine))); ok {
			dataParts = append(dataParts, value)
		}
	}
	if len(dataParts) == 0 {
		return true
	}
	var payload struct {
		Type *string `json:"type"`
	}
	if err := json.Unmarshal([]byte(strings.Join(dataParts, "\n")), &payload); err != nil || payload.Type == nil {
		return true
	}
	return *payload.Type == "ping"
}
