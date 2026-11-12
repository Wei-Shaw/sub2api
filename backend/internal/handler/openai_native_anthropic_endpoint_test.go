//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type nativeEndpointUpstream struct {
	service.HTTPUpstream
	mode  string
	paths []string
}

func (u *nativeEndpointUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	u.paths = append(u.paths, req.URL.Path)
	if u.mode == "transport error" {
		return nil, errors.New("synthetic upstream transport failure")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}}
	switch u.mode {
	case "HTTP error":
		response.StatusCode = http.StatusBadRequest
		response.Body = io.NopCloser(strings.NewReader(`{"error":{"type":"invalid_request_error","message":"synthetic rejection"}}`))
	case "body read error":
		response.Body = io.NopCloser(iotest.ErrReader(errors.New("synthetic body read failure")))
	default:
		if gjson.GetBytes(body, "stream").Bool() {
			response.Header.Set("Content-Type", "text/event-stream")
			response.Body = io.NopCloser(strings.NewReader(compatStreamUsagePartialStream + compatStreamUsageCompletion))
		} else {
			response.Body = io.NopCloser(strings.NewReader(`{"id":"msg_1","type":"message","role":"assistant","model":"k3","content":[{"type":"text","text":"partial"}],"stop_reason":"end_turn","usage":{"input_tokens":12,"output_tokens":7}}`))
		}
	}
	return response, nil
}

func (u *nativeEndpointUpstream) DoWithTLS(req *http.Request, proxy string, accountID int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxy, accountID, concurrency)
}

func TestOpenAINativeAnthropicEndpointDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, route := range []struct {
		name     string
		path     string
		protocol string
		platform string
	}{
		{"messages", "/v1/messages", service.APIProtocolAnthropic, service.PlatformKimi},
		{"chat", "/v1/chat/completions", service.APIProtocolAnthropic, service.PlatformKimi},
		{"responses", "/v1/responses", service.APIProtocolAnthropic, service.PlatformKimi},
		{"adaptive messages", "/v1/messages", service.APIProtocolAdaptive, service.PlatformZhipu},
	} {
		for _, stream := range []bool{false, true} {
			for _, mode := range []string{"transport error", "HTTP error", "body read error", "success"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", route.name, stream, mode), func(t *testing.T) {
					upstream := &nativeEndpointUpstream{mode: mode}
					cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{AllowInsecureHTTP: true}}}
					gateway := service.NewOpenAIGatewayService(
						nil, nil, nil, nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, upstream,
						nil, nil, nil, nil, nil, nil, nil, nil,
					)
					account := &service.Account{
						ID: 9501, Platform: route.platform, Type: service.AccountTypeAPIKey,
						Credentials: map[string]any{
							"api_key": "test-key", "api_protocol": route.protocol,
							"base_url": "http://anthropic.example",
							"api_base_urls": map[string]any{
								service.APIProtocolAnthropic:       "http://anthropic.example",
								service.APIProtocolChatCompletions: "http://chat.example",
							},
						},
					}
					model := "k3"
					if route.protocol == service.APIProtocolAdaptive {
						require.False(t, account.SupportsNativeCNResponses())
						model = "glm-5.3"
					}
					request := map[string]any{"model": model, "stream": stream}
					if route.path == "/v1/responses" {
						request["input"] = "hello"
					} else {
						request["messages"] = []map[string]string{{"role": "user", "content": "hello"}}
						request["max_tokens"] = 32
					}
					body, err := json.Marshal(request)
					require.NoError(t, err)
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, route.path, strings.NewReader(string(body)))
					// The same Gin context can retain an earlier attempt's protocol.
					service.SetActualOpenAIUpstreamEndpoint(c, EndpointChatCompletions)
					var result *service.OpenAIForwardResult
					switch route.path {
					case "/v1/messages":
						result, err = gateway.ForwardAsAnthropic(context.Background(), c, account, body, "", "")
					case "/v1/chat/completions":
						result, err = gateway.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")
					default:
						result, err = gateway.Forward(context.Background(), c, account, body)
					}
					if mode == "success" {
						require.NoError(t, err)
						require.NotNil(t, result)
						require.Equal(t, EndpointMessages, result.UpstreamEndpoint)
						require.Contains(t, recorder.Body.String(), "partial")
					} else {
						require.Error(t, err)
						if mode != "body read error" {
							require.Nil(t, result, "error diagnostics must work without a forward result")
						}
					}
					require.Len(t, upstream.paths, 1)
					require.True(t, strings.HasSuffix(upstream.paths[0], EndpointMessages), "the request actually used native Anthropic: %v", upstream.paths)
					// This is the endpoint reader used when writing operations errors and result-less usage.
					require.Equal(t, EndpointMessages, GetUpstreamEndpoint(c, account.Platform))
					require.Equal(t, EndpointMessages, service.GetActualOpenAIUpstreamEndpoint(c))
				})
			}
		}
	}
}
