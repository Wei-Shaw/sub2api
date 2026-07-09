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
	cfg, dir := testJSHandlerConfig(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hook.js"), []byte(`
function on_before_request(ctx) {
  ctx.body = ctx.body + "-hooked";
  return ctx;
}
`), 0o600))

	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true,"script_paths":["hook.js"]}`,
	}}, cfg)
	svc.InvalidateCache()

	out := svc.ApplyRequestHooks(context.Background(), "on_before_request", RequestHookInput{
		Body:         []byte("hello"),
		Headers:      http.Header{},
		Model:        "m",
		SourceFormat: "anthropic_messages",
		RequestID:    "req-1",
	})
	require.Equal(t, "hello-hooked", string(out.Body))
}

func TestApplyNonStreamResponseHooks_ModifiesBodyAndHeaders(t *testing.T) {
	cfg, dir := testJSHandlerConfig(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resp.js"), []byte(`
function on_after_nonstream_response(ctx) {
  ctx.body = ctx.body + "-resp";
  ctx.headers = ctx.headers || {};
  ctx.headers["X-Test"] = "1";
  return ctx;
}
`), 0o600))

	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true,"script_paths":["resp.js"]}`,
	}}, cfg)
	svc.InvalidateCache()

	out := svc.ApplyNonStreamResponseHooks(context.Background(), ResponseHookInput{
		Body:            []byte("{}"),
		ResponseHeaders: http.Header{"Content-Type": []string{"application/json"}},
		Protocol:        "anthropic_messages",
	})
	require.Equal(t, "{}-resp", string(out.Body))
	require.Equal(t, "1", out.Headers.Get("X-Test"))
}

func TestApplyRequestHooks_MissingHookSkips(t *testing.T) {
	cfg, dir := testJSHandlerConfig(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.js"), []byte(`// no hooks`), 0o600))

	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true,"script_paths":["empty.js"]}`,
	}}, cfg)
	svc.InvalidateCache()

	out := svc.ApplyRequestHooks(context.Background(), "on_before_request", RequestHookInput{
		Body: []byte("keep"),
	})
	require.Equal(t, "keep", string(out.Body))
}

func TestApplyRequestHooks_Disabled(t *testing.T) {
	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":false}`,
	}}, nil)
	svc.InvalidateCache()
	out := svc.ApplyRequestHooks(context.Background(), "on_before_request", RequestHookInput{Body: []byte("x")})
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
	cfg, dir := testJSHandlerConfig(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stream.js"), []byte(`
function on_after_stream_response(ctx) {
  if (ctx.chunk && ctx.chunk.indexOf("data:") >= 0) {
    ctx.chunk = ctx.chunk.replace("old", "new");
  }
  return ctx;
}
`), 0o600))

	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true,"script_paths":["stream.js"]}`,
	}}, cfg)
	svc.InvalidateCache()

	out := svc.ApplyStreamChunkHooks(context.Background(), StreamChunkHookInput{
		Chunk:    `data: {"type":"message_start","message":{"model":"old"}}`,
		Protocol: "anthropic_messages",
	})
	require.Contains(t, out.Chunk, "new")
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