package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClientGeoPrivacyProviderFormats(t *testing.T) {
	for _, tt := range []struct {
		name       string
		body       map[string]any
		path, kept string
	}{
		{"responses", map[string]any{"instructions": "Rules\n" + geoEnvironment, "input": []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": geoEnvironment}}}, map[string]any{"type": "function_call_output", "call_id": "c", "output": geoEnvironment}}}, "input.0.content.0.text", "input.1"},
		{"chat", map[string]any{"messages": []any{map[string]any{"role": "developer", "content": geoEnvironment}, map[string]any{"role": "tool", "content": geoEnvironment}}}, "messages.0.content", "messages.1"},
		{"gemini", map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": geoEnvironment}}}, "contents": []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": geoEnvironment}, map[string]any{"functionResponse": map[string]any{"name": "read", "response": map[string]any{"text": geoEnvironment}}}}}}}, "contents.0.parts.0.text", "contents.0.parts.1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, wrapper := range []string{"", "request", "response"} {
				body := tt.body
				prefix := ""
				if wrapper != "" {
					body = map[string]any{wrapper: body}
					prefix = wrapper + "."
				}
				raw, err := json.Marshal(body)
				require.NoError(t, err)
				got, err := RedactClientGeoMetadata(raw)
				require.NoError(t, err)
				require.Equal(t, cleanEnvironment, gjson.GetBytes(got, prefix+tt.path).String())
				require.Equal(t, gjson.GetBytes(raw, prefix+tt.kept).Raw, gjson.GetBytes(got, prefix+tt.kept).Raw)
			}
		})
	}
}

func TestClientGeoPrivacyHTTPRequestReplayAndUploads(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{RedactClientGeoMetadata: true}}
	body, err := json.Marshal(map[string]any{"input": geoEnvironment})
	require.NoError(t, err)
	for _, replayable := range []bool{false, true} {
		req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/responses", bytes.NewReader(body))
		require.NoError(t, err)
		if !replayable {
			req.GetBody = nil
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		require.NoError(t, ApplyClientGeoPrivacy(req, cfg))
		got, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Equal(t, cleanEnvironment, gjson.GetBytes(got, "input").String())
		require.Equal(t, int64(len(got)), req.ContentLength)
		retry, err := req.GetBody()
		require.NoError(t, err)
		retryBody, err := io.ReadAll(retry)
		require.NoError(t, err)
		require.NoError(t, retry.Close())
		require.Equal(t, got, retryBody)
	}
	req, err := http.NewRequest(http.MethodPost, "https://example.com/v1/images/edits", strings.NewReader(geoEnvironment))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=example")
	originalBody := req.Body
	require.NoError(t, ApplyClientGeoPrivacy(req, cfg))
	require.Equal(t, originalBody, req.Body, "uploads must remain untouched streams")
	upload, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, geoEnvironment, string(upload))
}

func TestClientGeoPrivacyBeforeBedrockSigning(t *testing.T) {
	body, err := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": geoEnvironment}}})
	require.NoError(t, err)
	svc := privacyTestService()
	signer := NewBedrockSigner("test-access-key", "test-secret-key", "", "us-east-1")
	req, err := svc.buildUpstreamRequestBedrock(context.Background(), body, "test-model", "us-east-1", false, signer)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(req.Header.Get("Authorization"), "AWS4-HMAC-SHA256 "))
	signedBody := req.Body
	signedHeaders := req.Header.Clone()
	require.NoError(t, ApplyClientGeoPrivacy(req, svc.cfg))
	require.Equal(t, signedBody, req.Body, "egress must retain the body that was signed")
	require.Equal(t, signedHeaders, req.Header)
	got, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NoError(t, req.Body.Close())
	require.Equal(t, cleanEnvironment, gjson.GetBytes(got, "messages.0.content").String())

	// A caller that signs before filtering must fail rather than send a stale signature.
	req, err = http.NewRequest(http.MethodPost, "https://example.com", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	require.NoError(t, signer.SignRequest(context.Background(), req, body))
	require.ErrorContains(t, ApplyClientGeoPrivacy(req, svc.cfg), "before signing")
	require.NoError(t, req.Body.Close())
}

func TestClientGeoPrivacyPreservesUnknownContentShapes(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"contents": map[string]any{"custom": map[string]any{"role": "user", "parts": []any{map[string]any{"text": geoEnvironment}}}},
	})
	require.NoError(t, err)
	got, err := RedactClientGeoMetadata(body)
	require.NoError(t, err)
	require.Equal(t, body, got)
}

func TestClientGeoPrivacyWebSocketEveryFrame(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	type received struct {
		header http.Header
		bodies [][]byte
		err    error
	}
	observed := make(chan received, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			observed <- received{err: err}
			return
		}
		defer func() { _ = conn.CloseNow() }()
		item := received{header: r.Header.Clone()}
		for i := 0; i < 3; i++ {
			_, payload, err := conn.Read(ctx)
			if err != nil {
				item.err = err
				break
			}
			item.bodies = append(item.bodies, payload)
		}
		observed <- item
	}))
	defer srv.Close()
	cfg := &config.Config{Gateway: config.GatewayConfig{RedactClientGeoMetadata: true}}
	dialer := newDefaultOpenAIWSClientDialer(cfg)
	conn, _, _, err := dialer.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), http.Header{"X-Timezone": {"Asia/Shanghai"}, "X-Forwarded-For": {"192.0.2.10"}}, "")
	require.NoError(t, err)
	frameConn, ok := conn.(*coderOpenAIWSClientConn)
	require.True(t, ok)
	defer func() { _ = frameConn.CloseNow() }()
	payload := map[string]any{"type": "response.create", "input": []any{map[string]any{"role": "user", "content": geoEnvironment}, map[string]any{"type": "function_call_output", "call_id": "c", "output": geoEnvironment}}}
	require.NoError(t, conn.WriteJSON(ctx, payload))
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, frameConn.WriteFrame(ctx, coderws.MessageText, raw))
	require.NoError(t, frameConn.WriteFrame(ctx, coderws.MessageBinary, raw))
	select {
	case got := <-observed:
		require.NoError(t, got.err)
		require.Empty(t, got.header.Get("X-Timezone"))
		require.Empty(t, got.header.Get("X-Forwarded-For"))
		require.Len(t, got.bodies, 3)
		for _, body := range got.bodies {
			require.Equal(t, cleanEnvironment, gjson.GetBytes(body, "input.0.content").String())
			require.Equal(t, geoEnvironment, gjson.GetBytes(body, "input.1.output").String())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for synthetic upstream frames")
	}
}
