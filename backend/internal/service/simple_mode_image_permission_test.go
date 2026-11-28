package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGroupAllowsImageGeneration(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cfg   *config.Config
		group *Group
		want  bool
	}{
		{"simple disabled group", &config.Config{RunMode: config.RunModeSimple}, &Group{}, true},
		{"simple enabled group", &config.Config{RunMode: config.RunModeSimple}, &Group{AllowImageGeneration: true}, true},
		{"simple ungrouped", &config.Config{RunMode: config.RunModeSimple}, nil, true},
		{"standard disabled group", &config.Config{RunMode: config.RunModeStandard}, &Group{}, false},
		{"standard enabled group", &config.Config{RunMode: config.RunModeStandard}, &Group{AllowImageGeneration: true}, true},
		{"standard ungrouped", &config.Config{RunMode: config.RunModeStandard}, nil, true},
		{"default mode disabled group", &config.Config{}, &Group{}, false},
		{"nil config disabled group", nil, &Group{}, false},
		{"nil config enabled group", nil, &Group{AllowImageGeneration: true}, true},
		{"nil config ungrouped", nil, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, GroupAllowsImageGeneration(tc.cfg, tc.group))
		})
	}
}

func TestOpenAIGatewayServiceForward_ImagePermissionRunMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	requests := []struct {
		name        string
		body        string
		mappedModel bool
	}{
		{"image model", `{"model":"gpt-image-2","input":"draw"}`, false},
		{"native tool", `{"model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation","model":"gpt-image-2"}]}`, false},
		{"Codex namespace", `{"model":"gpt-5.4","input":"draw","tools":[{"type":"namespace","name":"image_gen","tools":[{"type":"function","name":"imagegen"}]}]}`, false},
		{"Responses Lite declaration", `{"model":"gpt-5.4","input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"image_gen"}]}]}`, false},
		{"mapped image model", `{"model":"draw-alias","input":"draw"}`, true},
	}
	for _, mode := range []string{config.RunModeStandard, config.RunModeSimple} {
		for _, passthrough := range []bool{false, true} {
			for _, request := range requests {
				// API-key passthrough deliberately preserves the body; account model
				// mappings are not applied on that path.
				if passthrough && request.mappedModel {
					continue
				}
				name := mode + "/normal/" + request.name
				if passthrough {
					name = mode + "/passthrough/" + request.name
				}
				t.Run(name, func(t *testing.T) {
					upstream := &httpUpstreamRecorder{resp: &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"output":[{"id":"ig_1","type":"image_generation_call","result":"image"}],"usage":{"input_tokens":1,"output_tokens":2}}`)),
					}}
					svc := newOpenAIImageGenerationControlTestService(upstream)
					svc.cfg.RunMode = mode
					c, recorder := newOpenAIImageGenerationControlTestContext(false, "curl/8.0")
					account := newOpenAIImageGenerationControlTestAccount()
					account.Extra = map[string]any{"openai_passthrough": passthrough}
					if request.mappedModel {
						account.Credentials["model_mapping"] = map[string]any{"draw-alias": "gpt-image-2"}
					}
					result, err := svc.Forward(context.Background(), c, account, []byte(request.body))
					if mode == config.RunModeStandard {
						require.Error(t, err)
						require.Nil(t, result)
						require.Equal(t, http.StatusForbidden, recorder.Code)
						require.Equal(t, "permission_error", gjson.GetBytes(recorder.Body.Bytes(), "error.type").String())
						require.Nil(t, upstream.lastReq)
						return
					}
					require.NoError(t, err)
					require.NotNil(t, result)
					require.Equal(t, http.StatusOK, recorder.Code)
					require.NotNil(t, upstream.lastReq)
					require.Equal(t, 1, result.ImageCount)
					if request.name == "native tool" || request.name == "image model" || request.mappedModel {
						require.Equal(t, "gpt-image-2", result.BillingModel)
					} else {
						require.Equal(t, "gpt-5.4", result.BillingModel)
					}
					if request.mappedModel {
						require.Equal(t, "gpt-image-2", gjson.GetBytes(upstream.lastBody, `tools.#(type=="image_generation").model`).String())
					}
					require.False(t, getAPIKeyFromContext(c).Group.AllowImageGeneration, "bypass must not mutate the stored flag")
				})
			}
		}
	}
}

func TestOpenAIWSIngress_ImagePermissionRunMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{config.RunModeStandard, config.RunModeSimple} {
		for _, imageTurn := range []int{1, 2} {
			for _, mapped := range []bool{false, true} {
				name := mode + "/first/tool"
				if imageTurn == 2 {
					name = mode + "/follow-up/tool"
				}
				if mapped {
					name += "/mapped-model"
				}
				t.Run(name, func(t *testing.T) {
					cfg := newOpenAIWSExecutionScopeTestConfig()
					cfg.RunMode = mode
					captureConn := &openAIWSCaptureConn{events: [][]byte{
						[]byte(`{"type":"response.completed","response":{"id":"resp_1","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`),
						[]byte(`{"type":"response.completed","response":{"id":"resp_2","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`),
					}}
					pool := newOpenAIWSConnPool(cfg)
					pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
					defer pool.Close()
					svc := newOpenAIImageGenerationControlTestService(&httpUpstreamRecorder{})
					svc.cfg = cfg
					svc.openaiWSResolver = NewOpenAIWSProtocolResolver(cfg)
					svc.openaiWSPool = pool
					account := newOpenAIImageGenerationControlTestAccount()
					account.Extra = map[string]any{"responses_websockets_v2_enabled": true}
					if mapped {
						account.Credentials["model_mapping"] = map[string]any{"draw-alias": "gpt-image-2"}
					}
					serverErrCh := make(chan error, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						conn, err := coderws.Accept(w, r, nil)
						if err != nil {
							serverErrCh <- err
							return
						}
						defer func() { _ = conn.CloseNow() }()
						c, _ := newOpenAIImageGenerationControlTestContext(false, "curl/8.0")
						c.Request = r
						ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
						defer cancel()
						_, first, err := conn.Read(ctx)
						if err == nil {
							err = svc.ProxyResponsesWebSocketFromClient(ctx, c, conn, account, "test-token", first, nil)
						}
						serverErrCh <- err
					}))
					defer server.Close()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
					require.NoError(t, err)
					defer func() { _ = client.CloseNow() }()
					for turn := 1; turn <= imageTurn; turn++ {
						body := `{"type":"response.create","model":"gpt-5.4","input":"hello"}`
						if turn == imageTurn {
							body = `{"type":"response.create","model":"gpt-5.4","input":"draw","tools":[{"type":"image_generation","model":"gpt-image-2"}]}`
							if mapped {
								body = `{"type":"response.create","model":"draw-alias","input":"draw"}`
							}
						}
						require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(body)))
						_, event, readErr := client.Read(ctx)
						if turn == imageTurn && mode == config.RunModeStandard {
							require.Error(t, readErr)
							select {
							case proxyErr := <-serverErrCh:
								require.ErrorContains(t, proxyErr, ImageGenerationPermissionMessage())
							case <-ctx.Done():
								t.Fatal("proxy did not return the permission error")
							}
							require.Len(t, captureConn.writes, imageTurn-1, "denied frame must not reach upstream")
							return
						}
						require.NoError(t, readErr)
						require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
					}
					_ = client.Close(coderws.StatusNormalClosure, "done")
					select {
					case <-serverErrCh:
					case <-ctx.Done():
						t.Fatal("proxy did not stop after client close")
					}
					require.Len(t, captureConn.writes, imageTurn)
					if mapped {
						require.Equal(t, "gpt-image-2", gjson.Get(requestToJSONString(captureConn.writes[imageTurn-1]), "model").String())
					}
				})
			}
		}
	}
}
