package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesWebSocket_NativeCurrentWindowFailover(t *testing.T) {
	for _, scenario := range []string{"repeated_failover", "same_window", "model_switch", "budget_exhausted", "unknown_chain", "visible_output"} {
		t.Run(scenario, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recovers := scenario == "repeated_failover" || scenario == "model_switch" || scenario == "same_window"
			currentModel := "gpt-5.1"
			if scenario == "model_switch" {
				currentModel = "gpt-5.2"
			}
			type received struct {
				account int
				body    []byte
			}
			requests := make(chan received, 16)
			var accounts []service.Account
			for index := range 3 {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					conn, err := coderws.Accept(w, r, nil)
					if err != nil {
						return
					}
					defer func() { _ = conn.CloseNow() }()
					ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
					defer cancel()
					for turn := 1; ; turn++ {
						_, payload, err := conn.Read(ctx)
						if err != nil {
							return
						}
						requests <- received{index, payload}
						var events []string
						switch {
						case index == 0 && turn == 1:
							events = []string{`{"type":"response.completed","response":{"id":"resp_67","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"old-window"}]}]}}`}
						case index == 0 && turn == 2:
							events = []string{
								`{"type":"response.output_item.done","item":{"type":"custom_tool_call","call_id":"call_68","name":"apply_patch","input":"patch"}}`,
								`{"type":"response.output_item.done","item":{"type":"reasoning","id":"rs_68","summary":[],"encrypted_content":"keep-cipher"}}`,
								`{"type":"response.completed","response":{"id":"resp_68","output":[{"type":"reasoning","id":"rs_68","summary":[],"encrypted_content":"keep-cipher"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"working"}]},{"type":"custom_tool_call","call_id":"call_68","name":"apply_patch","input":"patch"}]}}`,
							}
						case index < 2:
							if scenario == "visible_output" {
								events = append(events, `{"type":"response.output_text.delta","response_id":"resp_partial","delta":"visible"}`)
							}
							events = append(events, `{"type":"error","error":{"code":"rate_limit_exceeded","type":"usage_limit_reached","message":"usage limit reached"}}`)
						default:
							events = []string{`{"type":"response.completed","response":{"id":"resp_recovered","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}}`}
						}
						for _, event := range events {
							if conn.Write(ctx, coderws.MessageText, []byte(event)) != nil {
								return
							}
						}
					}
				}))
				defer upstream.Close()
				accounts = append(accounts, service.Account{
					ID: int64(9950 + index), Name: fmt.Sprintf("native-%d", index),
					Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
					Status: service.StatusActive, Schedulable: true, Concurrency: 1, Priority: index + 1,
					Credentials: map[string]any{"api_key": "sk-test", "base_url": upstream.URL},
					Extra:       map[string]any{"openai_apikey_responses_websockets_v2_enabled": true, "openai_apikey_responses_websockets_v2_mode": "ctx_pool"},
				})
				if scenario == "model_switch" && index > 0 {
					accounts[index].Credentials["model_mapping"] = map[string]any{"gpt-5.2": "gpt-5.2"}
				}
			}
			cfg := &config.Config{}
			cfg.RunMode = config.RunModeSimple
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			ws := &cfg.Gateway.OpenAIWS
			ws.Enabled, ws.APIKeyEnabled, ws.ResponsesWebsocketsV2, ws.ModeRouterV2Enabled = true, true, true, true
			ws.MaxConnsPerAccount, ws.MaxIdlePerAccount, ws.QueueLimitPerConn = 2, 2, 8
			ws.DialTimeoutSeconds, ws.ReadTimeoutSeconds, ws.WriteTimeoutSeconds = 3, 5, 3
			ws.NativeReplayMaxSessionBytes, ws.NativeReplayMaxItems = 64<<20, 32768
			if scenario == "budget_exhausted" {
				ws.NativeReplayMaxSessionBytes = 1
			}
			cfg.Gateway.MaxAccountSwitches = 3
			repo := &openAIWSFailoverHandlerAccountRepoStub{accounts: accounts}
			billing := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
			gateway := service.NewOpenAIGatewayService(repo, nil, nil, nil, nil, nil, nil, cfg, nil, nil,
				service.NewBillingService(cfg, nil), service.NewRateLimitService(repo, nil, cfg, nil, nil), billing,
				nil, &service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil)
			cache := &concurrencyCacheMock{
				acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
				acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
			}
			h := &OpenAIGatewayHandler{gatewayService: gateway, billingCacheService: billing, apiKeyService: &service.APIKeyService{},
				concurrencyHelper: NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second), maxAccountSwitches: 3}
			group := int64(4250)
			key := &service.APIKey{ID: 1850, GroupID: &group, User: &service.User{ID: 1750, Status: service.StatusActive},
				Group: &service.Group{ID: group, Platform: service.PlatformOpenAI, Status: service.StatusActive}}
			done := make(chan struct{})
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyAPIKey), key)
				c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 1750, Concurrency: 1})
				c.Next()
			})
			router.GET("/v1/responses", func(c *gin.Context) { defer close(done); h.ResponsesWebSocket(c) })
			server := httptest.NewServer(router)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			client, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
			require.NoError(t, err)
			defer func() { _ = client.CloseNow() }()
			write := func(body string) { require.NoError(t, client.Write(ctx, coderws.MessageText, []byte(body))) }
			readCompleted := func(id string) {
				for {
					_, body, err := client.Read(ctx)
					require.NoError(t, err)
					if gjson.GetBytes(body, "type").String() == "response.completed" {
						require.Equal(t, id, gjson.GetBytes(body, "response.id").String())
						return
					}
				}
			}
			write(`{"type":"response.create","model":"gpt-5.1","store":false,"input":[{"role":"user","content":"OLD-LARGE-WINDOW-67"}],"client_metadata":{"x-codex-window-id":"67"}}`)
			readCompleted("resp_67")
			window := "68"
			anchor := ""
			if scenario == "same_window" {
				window, anchor = "67", `,"previous_response_id":"resp_67"`
			}
			write(fmt.Sprintf(`{"type":"response.create","model":%q,"store":false,"input":[{"role":"user","content":"compact-summary-68"}],"client_metadata":{"x-codex-window-id":%q}%s}`, currentModel, window, anchor))
			readCompleted("resp_68")
			previous := "resp_68"
			if scenario == "unknown_chain" {
				previous = "resp_elsewhere"
			}
			write(fmt.Sprintf(`{"type":"response.create","model":%q,"store":false,"previous_response_id":%q,"input":[{"type":"custom_tool_call_output","call_id":"call_68","output":"patch-applied"}],"client_metadata":{"x-codex-window-id":%q}}`, currentModel, previous, window))
			if recovers {
				readCompleted("resp_recovered")
				require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))
			} else if scenario == "visible_output" {
				_, body, err := client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "response.output_text.delta", gjson.GetBytes(body, "type").String())
				_, body, err = client.Read(ctx)
				require.NoError(t, err)
				require.Equal(t, "error", gjson.GetBytes(body, "type").String())
				_ = client.CloseNow()
			} else {
				_, _, err = client.Read(ctx)
				require.Equal(t, coderws.StatusTryAgainLater, coderws.CloseStatus(err))
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("handler did not finish")
			}
			close(requests)
			var migrated [][]byte
			for req := range requests {
				if req.account > 0 {
					migrated = append(migrated, req.body)
				}
			}
			if !recovers {
				require.Empty(t, migrated)
				return
			}
			require.Len(t, migrated, 2)
			for _, body := range migrated {
				require.Equal(t, currentModel, gjson.GetBytes(body, "model").String())
				require.False(t, gjson.GetBytes(body, "previous_response_id").Exists())
				require.Equal(t, window, gjson.GetBytes(body, "client_metadata.x-codex-window-id").String())
				if scenario == "same_window" {
					require.Equal(t, int64(7), gjson.GetBytes(body, "input.#").Int())
					require.Contains(t, string(body), "OLD-LARGE-WINDOW-67")
				} else {
					require.Equal(t, int64(5), gjson.GetBytes(body, "input.#").Int())
					require.Equal(t, "reasoning", gjson.GetBytes(body, "input.1.type").String())
					require.Equal(t, "message", gjson.GetBytes(body, "input.2.type").String())
					require.Equal(t, "custom_tool_call", gjson.GetBytes(body, "input.3.type").String())
					require.NotContains(t, string(body), "OLD-LARGE-WINDOW-67")
					require.NotContains(t, string(body), "old-window")
				}
				require.Contains(t, string(body), "keep-cipher")
				require.Contains(t, string(body), "custom_tool_call_output")
				require.Contains(t, string(body), "compact-summary-68")
			}
			require.JSONEq(t, gjson.GetBytes(migrated[0], "input").Raw, gjson.GetBytes(migrated[1], "input").Raw)
		})
	}
}
