package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service/jshandler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyOpenAIWSAccountAfterAuth_RewritesBody(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{Pricing: config.PricingConfig{DataDir: dir}}
	js := jshandler.NewService(&jshandlerSettingStub{values: map[string]string{
		jshandler.SettingKeyJSHandlerConfig: `{"enabled":true}`,
	}}, cfg)
	js.InvalidateCache()
	entry, err := js.AddScript("after", []byte(`
function on_after_auth_request(ctx) {
  try {
    var o = JSON.parse(ctx.body || "{}");
    o.model = (o.model || "") + "-after";
    ctx.body = JSON.stringify(o);
  } catch (e) {}
  return ctx;
}
`))
	require.NoError(t, err)

	svc := &OpenAIGatewayService{jsHandler: js}
	account := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			AccountExtraJShandlerScriptIDs: []string{entry.ID},
		},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)

	body := []byte(`{"type":"response.create","model":"gpt-4"}`)
	out := svc.applyOpenAIWSAccountAfterAuth(context.Background(), c, account, body, "gpt-4", "gpt-4")
	require.Equal(t, "gpt-4-after", gjson.GetBytes(out, "model").String())
}

func TestApplyOpenAIWSFollowupHookOrder_BeforeThenAfter(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	cfg := &config.Config{Pricing: config.PricingConfig{DataDir: dir}}
	js := jshandler.NewService(&jshandlerSettingStub{values: map[string]string{
		jshandler.SettingKeyJSHandlerConfig: `{"enabled":true}`,
	}}, cfg)
	js.InvalidateCache()

	beforeEntry, err := js.AddScript("before", []byte(`
function on_before_request(ctx) {
  try {
    var o = JSON.parse(ctx.body || "{}");
    o.model = (o.model || "") + "-before";
    o.service_tier = "priority";
    ctx.body = JSON.stringify(o);
  } catch (e) {}
  return ctx;
}
`))
	require.NoError(t, err)
	afterEntry, err := js.AddScript("after", []byte(`
function on_after_auth_request(ctx) {
  try {
    var o = JSON.parse(ctx.body || "{}");
    o.model = (o.model || "") + "-after";
    ctx.body = JSON.stringify(o);
  } catch (e) {}
  return ctx;
}
`))
	require.NoError(t, err)

	// Order must be: group before → account after (base-before-after).
	body := []byte(`{"type":"response.create","model":"base"}`)
	beforeOut := js.ApplyRequestHooksChain(context.Background(), []string{beforeEntry.ID}, "on_before_request", jshandler.RequestHookInput{
		Body:         body,
		Headers:      http.Header{},
		Model:        "base",
		SourceFormat: "openai_responses",
	})
	require.Equal(t, "base-before", gjson.GetBytes(beforeOut.Body, "model").String())
	require.Equal(t, "priority", gjson.GetBytes(beforeOut.Body, "service_tier").String())

	svc := &OpenAIGatewayService{jsHandler: js}
	account := &Account{
		Platform: PlatformOpenAI,
		Extra: map[string]any{
			AccountExtraJShandlerScriptIDs: []string{afterEntry.ID},
		},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request, _ = http.NewRequest(http.MethodPost, "/", nil)

	final := svc.applyOpenAIWSAccountAfterAuth(context.Background(), c, account, beforeOut.Body, "base-before", "base-before")
	require.Equal(t, "base-before-after", gjson.GetBytes(final, "model").String(),
		"after-auth must see body already rewritten by on_before_request")
	require.NotEqual(t, "base-after-before", gjson.GetBytes(final, "model").String())

	// Fast Policy evaluates the post-hook frame (service_tier set by before).
	policyOut, blocked, policyErr := svc.applyOpenAIFastPolicyToWSResponseCreate(
		context.Background(),
		&Account{Platform: PlatformOpenAI},
		"base-before-after",
		final,
	)
	require.NoError(t, policyErr)
	require.Nil(t, blocked)
	require.Equal(t, "priority", gjson.GetBytes(policyOut, "service_tier").String())
}

func TestApplyOpenAIFastPolicyToWSResponseCreate_SeesServiceTier(t *testing.T) {
	t.Parallel()
	svc := &OpenAIGatewayService{}
	frame := []byte(`{"type":"response.create","model":"gpt-4","service_tier":"priority"}`)
	out, blocked, err := svc.applyOpenAIFastPolicyToWSResponseCreate(context.Background(), &Account{Platform: PlatformOpenAI}, "gpt-4", frame)
	require.NoError(t, err)
	require.Nil(t, blocked)
	require.Equal(t, "priority", gjson.GetBytes(out, "service_tier").String())
}

type jshandlerSettingStub struct {
	values map[string]string
}

func (s *jshandlerSettingStub) GetValue(_ context.Context, key string) (string, error) {
	if s.values == nil {
		return "", nil
	}
	return s.values[key], nil
}
