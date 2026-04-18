package main

import (
	"bufio"
	"bytes"
	ctls "crypto/tls"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

const h2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

// bodyPreviewLimit caps per-stream body bytes captured for preview.
const bodyPreviewLimit = 512

// h2IdleTimeout is the per-frame read deadline; long enough for Claude Code
// to finish streaming a response before the next request, short enough that
// a hung session dumps its capture in reasonable wall-clock time.
const h2IdleTimeout = 30 * time.Second

// h2Session holds the per-connection state for an HTTP/2 capture session.
type h2Session struct {
	framer           *http2.Framer
	capture          *Capture
	bodies           map[uint32]*bytes.Buffer
	firstHeadersSeen bool
}

// serveH2 handles an HTTP/2 session in "discovery" mode: it captures the
// TLS/H2 fingerprint on the first request and then keeps the connection open
// responding to every subsequent request with a path-appropriate stub, so we
// can observe the full set of endpoints a real Claude Code CLI hits in one
// session (not just /v1/messages).
//
// Exit conditions: client GOAWAY, read deadline (idle), or explicit error.
// The overall session is bounded by the raw-conn deadline set in
// handleConn (currently 30s); per-frame iteration we reset to
// h2IdleTimeout so a hung stream doesn't block past that ceiling.
func serveH2(tlsConn *ctls.Conn, c *Capture) error {
	c.HTTP2 = &H2Capture{ClientSettings: map[string]uint32{}}
	reader := bufio.NewReader(tlsConn)
	if err := readH2Preface(reader, c); err != nil {
		return err
	}
	sess := newH2Session(tlsConn, reader, c)
	if err := sess.framer.WriteSettings(); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}
	return sess.loop(tlsConn)
}

func readH2Preface(reader *bufio.Reader, c *Capture) error {
	preface := make([]byte, len(h2Preface))
	if _, err := io.ReadFull(reader, preface); err != nil {
		return fmt.Errorf("read preface: %w", err)
	}
	if string(preface) != h2Preface {
		return fmt.Errorf("bad preface: %q", preface)
	}
	c.HTTP2.PrefaceSeen = true
	return nil
}

func newH2Session(tlsConn *ctls.Conn, reader *bufio.Reader, c *Capture) *h2Session {
	framer := http2.NewFramer(tlsConn, reader)
	framer.SetMaxReadFrameSize(16384)
	framer.ReadMetaHeaders = hpack.NewDecoder(4096, nil)
	return &h2Session{
		framer:  framer,
		capture: c,
		bodies:  map[uint32]*bytes.Buffer{},
	}
}

func (s *h2Session) loop(tlsConn *ctls.Conn) error {
	for {
		_ = tlsConn.SetDeadline(time.Now().Add(h2IdleTimeout))
		frame, err := s.framer.ReadFrame()
		if err != nil {
			// Idle timeout or connection close is the expected happy path
			// after the CLI is done. Report as nil error so the capture
			// dump still happens.
			return nil
		}
		s.capture.HTTP2.FrameSequence = append(s.capture.HTTP2.FrameSequence, frame.Header().Type.String())
		if done, err := s.dispatch(frame); done || err != nil {
			return err
		}
	}
}

func (s *h2Session) dispatch(frame http2.Frame) (done bool, err error) {
	switch f := frame.(type) {
	case *http2.SettingsFrame:
		return false, s.handleSettings(f)
	case *http2.WindowUpdateFrame:
		s.handleWindowUpdate(f)
	case *http2.MetaHeadersFrame:
		s.handleHeaders(f)
	case *http2.DataFrame:
		s.handleData(f)
	case *http2.PingFrame:
		s.handlePing(f)
	case *http2.GoAwayFrame:
		return true, nil
	}
	return false, nil
}

func (s *h2Session) handleSettings(f *http2.SettingsFrame) error {
	if f.IsAck() {
		return nil
	}
	_ = f.ForeachSetting(func(st http2.Setting) error {
		s.capture.HTTP2.ClientSettings[settingName(st.ID)] = st.Val
		return nil
	})
	if err := s.framer.WriteSettingsAck(); err != nil {
		return fmt.Errorf("write settings ack: %w", err)
	}
	return nil
}

func (s *h2Session) handleWindowUpdate(f *http2.WindowUpdateFrame) {
	inc := f.Increment
	if s.capture.HTTP2.WindowUpdate == nil {
		s.capture.HTTP2.WindowUpdate = &inc
	}
}

func (s *h2Session) handleHeaders(f *http2.MetaHeadersFrame) {
	req, pseudo, all := buildH2Request(f)
	// Record the first request's header ordering for fingerprint
	// comparison (backwards compatible with the old capture format).
	if !s.firstHeadersSeen {
		s.capture.HTTP2.FirstHeadersPseudoOrder = pseudo
		s.capture.HTTP2.FirstHeadersAll = all
		s.firstHeadersSeen = true
	}
	s.capture.HTTP2.Requests = append(s.capture.HTTP2.Requests, req)
	// Emit the stub response immediately. For requests with bodies the
	// DataFrame arrives after our response header block — that's fine,
	// we just absorb the DATA frames in handleData. Don't bail on write
	// errors: the client may have already closed the stream.
	if err := writeStubResponseForPath(s.framer, f.StreamID, req.Path); err != nil {
		log.Printf("write stub for %s: %v", req.Path, err)
	}
}

func buildH2Request(f *http2.MetaHeadersFrame) (H2Request, []string, []string) {
	req := H2Request{StreamID: f.StreamID}
	var pseudo, all []string
	for _, hf := range f.Fields {
		all = append(all, hf.Name+": "+hf.Value)
		if hf.IsPseudo() {
			pseudo = append(pseudo, hf.Name)
		}
		switch hf.Name {
		case ":method":
			req.Method = hf.Value
		case ":path":
			req.Path = hf.Value
		case ":authority":
			req.Authority = hf.Value
		}
	}
	req.Headers = all
	return req, pseudo, all
}

func (s *h2Session) handleData(f *http2.DataFrame) {
	buf, ok := s.bodies[f.StreamID]
	if !ok {
		buf = &bytes.Buffer{}
		s.bodies[f.StreamID] = buf
	}
	if buf.Len() < bodyPreviewLimit {
		need := bodyPreviewLimit - buf.Len()
		data := f.Data()
		if len(data) > need {
			data = data[:need]
		}
		buf.Write(data)
	}
	if f.StreamEnded() {
		// Attach body preview back to the matching request.
		for i := range s.capture.HTTP2.Requests {
			if s.capture.HTTP2.Requests[i].StreamID == f.StreamID {
				s.capture.HTTP2.Requests[i].BodyPreview = buf.String()
				break
			}
		}
	}
}

func (s *h2Session) handlePing(f *http2.PingFrame) {
	// Reply to client pings so idle-detection keepalives work.
	if !f.IsAck() {
		_ = s.framer.WritePing(true, f.Data)
	}
}

// writeStubResponseForPath dispatches to a path-appropriate stub. Unknown
// paths get a generic 404 so the CLI visibly errors on them — which is fine
// for a diagnostic tool; the path is already recorded in the capture.
func writeStubResponseForPath(framer *http2.Framer, streamID uint32, path string) error {
	switch {
	case path == "" || path == "/":
		return writeFakeJSONResponse(framer, streamID, 404, `{"error":"empty path"}`)
	case strings.HasSuffix(path, "/count_tokens") || strings.Contains(path, "/count_tokens?"):
		return writeFakeJSONResponse(framer, streamID, 200, `{"input_tokens":1}`)
	case strings.HasPrefix(path, "/v1/messages"):
		return writeFakeH2Response(framer, streamID)
	case strings.HasPrefix(path, "/api/oauth/usage") || strings.HasPrefix(path, "/v1/organizations"):
		return writeFakeJSONResponse(framer, streamID, 200, `{"five_hour":{"utilization":0.05,"resets_at":"2099-01-01T00:00:00Z"},"seven_day":{"utilization":0.05,"resets_at":"2099-01-01T00:00:00Z"},"seven_day_sonnet":{"utilization":0.05,"resets_at":"2099-01-01T00:00:00Z"}}`)
	case strings.HasPrefix(path, "/v1/models"):
		return writeFakeJSONResponse(framer, streamID, 200, `{"data":[],"has_more":false,"first_id":null,"last_id":null}`)
	default:
		return writeFakeJSONResponse(framer, streamID, 404, `{"type":"error","error":{"type":"not_found_error","message":"capture stub has no handler for this path"}}`)
	}
}

// writeFakeJSONResponse sends a single-frame HTTP/2 JSON response with the
// given status and body. Used for non-streaming endpoints like
// count_tokens, /api/oauth/usage, /v1/models.
func writeFakeJSONResponse(framer *http2.Framer, streamID uint32, status int, body string) error {
	if streamID == 0 {
		streamID = 1
	}
	var hdrBuf bytes.Buffer
	enc := hpack.NewEncoder(&hdrBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":status", Value: fmt.Sprintf("%d", status)})
	_ = enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "application/json"})
	_ = enc.WriteField(hpack.HeaderField{Name: "x-request-id", Value: "req_capture_stub"})

	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: hdrBuf.Bytes(),
		EndStream:     false,
		EndHeaders:    true,
	}); err != nil {
		return err
	}
	return framer.WriteData(streamID, true, []byte(body))
}

func writeFakeH2Response(framer *http2.Framer, streamID uint32) error {
	if streamID == 0 {
		streamID = 1
	}
	// Encode a minimal response header block via hpack.
	var hdrBuf bytes.Buffer
	enc := hpack.NewEncoder(&hdrBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
	_ = enc.WriteField(hpack.HeaderField{Name: "content-type", Value: "text/event-stream"})
	_ = enc.WriteField(hpack.HeaderField{Name: "cache-control", Value: "no-cache"})
	_ = enc.WriteField(hpack.HeaderField{Name: "x-request-id", Value: "req_capture_001"})

	if err := framer.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      streamID,
		BlockFragment: hdrBuf.Bytes(),
		EndStream:     false,
		EndHeaders:    true,
	}); err != nil {
		return err
	}

	// Minimal SSE body that Claude Code will parse as a valid stream.
	body := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_capture","type":"message","role":"assistant","model":"claude-sonnet-4-5-20250929","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_start\n" +
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}` + "\n\n" +
		"event: content_block_stop\n" +
		`data: {"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":1}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	if err := framer.WriteData(streamID, true, []byte(body)); err != nil {
		return err
	}
	return nil
}

func settingName(id http2.SettingID) string {
	switch id {
	case http2.SettingHeaderTableSize:
		return "HEADER_TABLE_SIZE"
	case http2.SettingEnablePush:
		return "ENABLE_PUSH"
	case http2.SettingMaxConcurrentStreams:
		return "MAX_CONCURRENT_STREAMS"
	case http2.SettingInitialWindowSize:
		return "INITIAL_WINDOW_SIZE"
	case http2.SettingMaxFrameSize:
		return "MAX_FRAME_SIZE"
	case http2.SettingMaxHeaderListSize:
		return "MAX_HEADER_LIST_SIZE"
	}
	return fmt.Sprintf("SETTING_0x%04x", uint16(id))
}
