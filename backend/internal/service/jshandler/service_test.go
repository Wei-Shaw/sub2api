package jshandler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
	dir := t.TempDir()
	script := filepath.Join(dir, "hook.js")
	require.NoError(t, os.WriteFile(script, []byte(`
function on_before_request(ctx) {
  ctx.body = ctx.body + "-hooked";
  return ctx;
}
`), 0o600))

	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true,"script_paths":["` + script + `"]}`,
	}}, nil)
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

func TestApplyRequestHooks_MissingHookSkips(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "empty.js")
	require.NoError(t, os.WriteFile(script, []byte(`// no hooks`), 0o600))

	svc := NewService(&stubSettingRepo{values: map[string]string{
		SettingKeyJSHandlerConfig: `{"enabled":true,"script_paths":["` + script + `"]}`,
	}}, nil)
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
	require.NoError(t, engine.runProgram(program, time.Second))
	_, err = engine.callFunction("on_before_request", 50*time.Millisecond, map[string]any{"body": "a"})
	require.Error(t, err)
}