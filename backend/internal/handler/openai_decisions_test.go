//go:build unit

package handler

import (
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const decisionsBody = `{"model":"gpt-6-luna","input":"evidence","questions":[{"type":"predicate","instructions":"Test?"}]}`
const decisionsNativeReply = `{"model":"gpt-6-luna","answers":[{"type":"predicate","name":null,"probability":0.9}],"usage":{"input_tokens":4,"output_tokens":0}}`
const decisionsRouterReply = `{"model":"openai/gpt-6-luna-decisions","answers":{"q0":{"type":"noul","noul":0.9}},"usage":{"input_tokens":4,"output_tokens":0}}`

func decisionsAccount(id int64, protocol string) service.Account {
	return service.Account{ID: id, Name: protocol, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Status: service.StatusActive, Schedulable: true, Priority: int(id), Credentials: map[string]any{"api_key": "test", "openai_decisions_protocol": protocol}}
}
func TestDecisionsHandlerCrossProtocolFailover(t *testing.T) {
	for _, direction := range []struct{ first, second, reply string }{{"openrouter", "openai", decisionsNativeReply}, {"openai", "openrouter", decisionsRouterReply}} {
		t.Run(direction.first+" to "+direction.second, func(t *testing.T) {
			failed := astra403()
			failed.StatusCode = 503
			success := astra200()
			success.Body = http.NoBody
			success.Body = ioBody(direction.reply)
			upstream := newAstraProCapturedUpstream(failed, success)
			h := newOpenAIResponsesFailoverTestHandler(t, upstream, decisionsAccount(1, direction.first), decisionsAccount(2, direction.second))
			c, rec := newAstraProFailoverContext(t, decisionsBody)
			c.Request.URL.Path = "/v1/decisions"
			h.Decisions(c)
			urls, ids, bodies := upstream.snapshot()
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Equal(t, []int64{1, 2}, ids)
			for i, protocol := range []string{direction.first, direction.second} {
				if protocol == "openrouter" {
					require.Contains(t, urls[i], "/api/alpha/decisions")
					require.True(t, gjson.GetBytes(bodies[i], "questions").IsObject())
					require.False(t, gjson.GetBytes(bodies[i], "input").Exists())
				} else {
					require.Contains(t, urls[i], "/v1/decisions")
					require.JSONEq(t, decisionsBody, string(bodies[i]))
				}
			}
			require.Equal(t, "gpt-6-luna", gjson.GetBytes(rec.Body.Bytes(), "model").String())
		})
	}
}
func ioBody(body string) io.ReadCloser { return io.NopCloser(bytes.NewBufferString(body)) }

func TestDecisionsHandlerMultiImageSkipsOpenRouter(t *testing.T) {
	body := `{"model":"gpt-6-luna","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="},{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}],"questions":[{"type":"predicate","instructions":"Test?"}]}`
	for _, native := range []bool{false, true} {
		success := astra200()
		success.Body = ioBody(decisionsNativeReply)
		upstream := newAstraProCapturedUpstream(success)
		accounts := []service.Account{decisionsAccount(1, "openrouter")}
		if native {
			accounts = append(accounts, decisionsAccount(2, "openai"))
		}
		h := newOpenAIResponsesFailoverTestHandler(t, upstream, accounts...)
		c, rec := newAstraProFailoverContext(t, body)
		c.Request.URL.Path = "/v1/decisions"
		h.Decisions(c)
		_, ids, _ := upstream.snapshot()
		if native {
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Equal(t, []int64{2}, ids)
		} else {
			require.Equal(t, 503, rec.Code)
			require.Empty(t, ids)
			require.Contains(t, rec.Body.String(), "multi-image")
		}
	}
}
func TestDecisionsUsageSnapshotSurvivesContextReuse(t *testing.T) {
	c, _ := newAstraProFailoverContext(t, decisionsBody)
	c.Request.URL.Path = "/v1/decisions"
	c.Request.Header.Set("User-Agent", "original")
	key, _ := middleware.GetAPIKeyFromContext(c)
	result := &service.OpenAIForwardResult{Model: "gpt-6-luna", UpstreamModel: "openai/gpt-6-luna-decisions", UpstreamEndpoint: "/api/alpha/decisions"}
	at := time.Now()
	mapping := service.ChannelMappingResult{ChannelID: 7, MappedModel: "gpt-6-luna", BillingModelSource: service.BillingModelSourceRequested}
	input := decisionsUsageSnapshot(c, key, &service.Account{}, nil, []byte(decisionsBody), result, at, mapping)
	c.Request.URL.Path = "/changed"
	c.Request.Header.Set("User-Agent", "changed")
	require.Equal(t, "/v1/decisions", input.InboundEndpoint)
	require.Equal(t, "/api/alpha/decisions", input.UpstreamEndpoint)
	require.Equal(t, "original", input.UserAgent)
	require.Equal(t, int64(7), input.ChannelID)
	require.Equal(t, at, input.PricingAt)
	require.NotEmpty(t, input.RequestPayloadHash)
}

func TestDecisionsHandlerDeterministic400DoesNotSwitchAccounts(t *testing.T) {
	failed := astra403()
	failed.StatusCode = 400
	failed.Body = ioBody(`{"error":{"message":"invalid question"}}`)
	upstream := newAstraProCapturedUpstream(failed)
	h := newOpenAIResponsesFailoverTestHandler(t, upstream, decisionsAccount(1, "openrouter"), decisionsAccount(2, "openai"))
	c, rec := newAstraProFailoverContext(t, decisionsBody)
	c.Request.URL.Path = "/v1/decisions"
	h.Decisions(c)
	_, ids, _ := upstream.snapshot()
	require.Equal(t, 400, rec.Code)
	require.Equal(t, []int64{1}, ids)
}
