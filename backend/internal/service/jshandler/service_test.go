package jshandler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func testJSHandlerConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	return &config.Config{Pricing: config.PricingConfig{DataDir: dir}}, dir
}

type stubSettingRepo struct {
	values map[string]string
}

func (s *stubSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	if s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}

func TestApplyRequestHooks_ModifiesBody(t *testing.T) {
	cfg, _ := testJSHandlerConfig(t)
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true}`,
	}}, cfg)
	svc.InvalidateCache()
	entry, err := svc.AddScript("hook", []byte(`
function on_after_auth_request(ctx) {
  ctx.body = ctx.body + "-hooked";
  return ctx;
}
`))
	require.NoError(t, err)

	out := svc.ApplyRequestHooks(context.Background(), entry.ID, "on_after_auth_request", RequestHookInput{
		Body:         []byte("hello"),
		Headers:      http.Header{},
		Model:        "m",
		SourceFormat: "anthropic_messages",
		RequestID:    "req-1",
	})
	require.Equal(t, "hello-hooked", string(out.Body))
}

func TestApplyNonStreamResponseHooks_ModifiesBodyAndHeaders(t *testing.T) {
	cfg, _ := testJSHandlerConfig(t)
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true}`,
	}}, cfg)
	svc.InvalidateCache()
	entry, err := svc.AddScript("resp", []byte(`
function on_after_nonstream_response(ctx) {
  ctx.body = ctx.body + "-resp";
  ctx.headers = ctx.headers || {};
  ctx.headers["X-Test"] = "1";
  return ctx;
}
`))
	require.NoError(t, err)

	out := svc.ApplyNonStreamResponseHooks(context.Background(), entry.ID, ResponseHookInput{
		Body:            []byte("{}"),
		ResponseHeaders: http.Header{"Content-Type": []string{"application/json"}},
		Protocol:        "anthropic_messages",
	})
	require.Equal(t, "{}-resp", string(out.Body))
	require.Equal(t, "1", out.Headers.Get("X-Test"))
}

func TestApplyRequestHooks_MissingHookSkips(t *testing.T) {
	cfg, _ := testJSHandlerConfig(t)
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true}`,
	}}, cfg)
	svc.InvalidateCache()
	entry, err := svc.AddScript("empty", []byte(`// no hooks`))
	require.NoError(t, err)

	out := svc.ApplyRequestHooks(context.Background(), entry.ID, "on_before_request", RequestHookInput{
		Body: []byte("keep"),
	})
	require.Equal(t, "keep", string(out.Body))
}

func TestApplyRequestHooks_Disabled(t *testing.T) {
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":false}`,
	}}, nil)
	svc.InvalidateCache()
	out := svc.ApplyRequestHooks(context.Background(), "any-id", "on_before_request", RequestHookInput{Body: []byte("x")})
	require.Equal(t, "x", string(out.Body))
}

func TestApplyRequestHooks_EmptyScriptID(t *testing.T) {
	cfg, _ := testJSHandlerConfig(t)
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true}`,
	}}, cfg)
	svc.InvalidateCache()
	out := svc.ApplyRequestHooks(context.Background(), "", "on_after_auth_request", RequestHookInput{Body: []byte("x")})
	require.Equal(t, "x", string(out.Body))
}

func TestLoad_InvalidJSONReturnsError(t *testing.T) {
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{not json`,
	}}, nil)
	svc.InvalidateCache()
	_, err := svc.load(context.Background())
	require.Error(t, err)
}

func TestApplyStreamChunkHooks_ModifiesChunk(t *testing.T) {
	cfg, _ := testJSHandlerConfig(t)
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true}`,
	}}, cfg)
	svc.InvalidateCache()
	entry, err := svc.AddScript("stream", []byte(`
function on_after_stream_response(ctx) {
  if (ctx.chunk) {
    ctx.chunk = ctx.chunk.replace("old", "new");
  }
  return ctx;
}
`))
	require.NoError(t, err)

	out := svc.ApplyStreamChunkHooks(context.Background(), entry.ID, StreamChunkHookInput{
		Chunk:           `{"type":"message_start","message":{"model":"old"}}`,
		ResponseHeaders: http.Header{"X-Upstream": []string{"1"}},
		Protocol:        "anthropic_messages",
	})
	require.Contains(t, out.Chunk, "new")
}

func TestStreamSession_ReusesEngineAcrossChunks(t *testing.T) {
	cfg, _ := testJSHandlerConfig(t)
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true}`,
	}}, cfg)
	svc.InvalidateCache()
	entry, err := svc.AddScript("stream-session", []byte(`
var n = 0;
function on_after_stream_response(ctx) {
  n += 1;
  ctx.chunk = "c" + n;
  return ctx;
}
`))
	require.NoError(t, err)

	session := svc.OpenStreamSession(context.Background(), entry.ID)
	require.NotNil(t, session)
	out1, err := session.ApplyChunk(StreamChunkHookInput{Chunk: "a", Protocol: "anthropic_messages"})
	require.NoError(t, err)
	out2, err := session.ApplyChunk(StreamChunkHookInput{Chunk: "b", Protocol: "anthropic_messages"})
	require.NoError(t, err)
	require.Equal(t, "c1", out1.Chunk)
	require.Equal(t, "c2", out2.Chunk)
}

func TestEngine_TimeoutInterrupts(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "loop.js")
	require.NoError(t, os.WriteFile(script, []byte(`
function on_before_request(ctx) {
  while (true) {}
  return ctx;
}
`), 0o600))

	program, err := getJSProgram(script)
	require.NoError(t, err)
	engine := newJSEngine(nil)
	_, err = engine.runProgramAndCall(program, "on_before_request", 50*time.Millisecond, map[string]any{"body": "a"})
	require.Error(t, err)
}