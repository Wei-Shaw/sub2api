package repository

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type lifecycleUsageLogRepo struct {
	service.UsageLogRepository
	logs []*service.UsageLog
}

func (r *lifecycleUsageLogRepo) Create(ctx context.Context, log *service.UsageLog) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	r.logs = append(r.logs, log)
	return true, nil
}

type lifecycleBillingRepo struct {
	service.UsageBillingRepository
	commands []*service.UsageBillingCommand
}

func (r *lifecycleBillingRepo) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (*service.UsageBillingApplyResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.commands = append(r.commands, cmd)
	return &service.UsageBillingApplyResult{Applied: true}, nil
}

type lifecycleDisconnectedWriter struct {
	gin.ResponseWriter
	disconnect context.CancelFunc
}

func (w *lifecycleDisconnectedWriter) Write([]byte) (int, error) {
	w.disconnect()
	return 0, errors.New("client disconnected")
}

// Exercise the real HTTP cancellation boundary and RecordUsage together. Usage
// already observed before delivery fails remains billable; unobserved terminal
// usage is never invented after cancellation.
func TestHTTPUpstreamCancellationPreservesOnlyObservedConsumption(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, observed := range []bool{false, true} {
		name := "unobserved"
		if observed {
			name = "observed"
		}
		t.Run(name, func(t *testing.T) {
			clientCtx, disconnect := context.WithCancel(t.Context())
			defer disconnect()
			upstreamCancelled := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("X-Request-Id", "lifecycle-billing")
				event := `{"type":"response.output_text.delta","delta":"hello"}`
				if observed {
					event = `{"type":"response.completed","response":{"id":"resp_lifecycle","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"input_tokens":17,"output_tokens":8,"total_tokens":25}}}`
				}
				_, _ = io.WriteString(w, "data: "+event+"\n\n")
				_ = http.NewResponseController(w).Flush()
				<-r.Context().Done()
				close(upstreamCancelled)
			}))
			defer srv.Close()
			defer srv.CloseClientConnections()
			cfg := &config.Config{}
			cfg.Default.RateMultiplier = 1
			cfg.Security.URLAllowlist.AllowPrivateHosts = true
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.StreamKeepaliveInterval = 1
			upstream := NewHTTPUpstream(cfg)
			usageRepo := &lifecycleUsageLogRepo{}
			billingRepo := &lifecycleBillingRepo{}
			svc := service.NewOpenAIGatewayService(
				nil, usageRepo, billingRepo, nil, nil, nil, nil, cfg, nil, nil,
				service.NewBillingService(cfg, nil), nil, nil, upstream,
				&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
			)
			account := &service.Account{ID: 1, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
				Concurrency: 1, Credentials: map[string]any{"base_url": srv.URL, "api_key": "test-key"}}
			body := []byte(`{"model":"gpt-5.1","stream":true,"input":"hello"}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(string(body))).WithContext(clientCtx)
			c.Writer = &lifecycleDisconnectedWriter{ResponseWriter: c.Writer, disconnect: disconnect}
			type forwardResult struct {
				result *service.OpenAIForwardResult
				err    error
			}
			done := make(chan forwardResult, 1)
			go func() { result, err := svc.Forward(clientCtx, c, account, body); done <- forwardResult{result, err} }()
			select {
			case got := <-done:
				var deliveryErr *service.OpenAIDeliveryError
				require.ErrorAs(t, got.err, &deliveryErr)
				require.Equal(t, service.ResponseOutcomeClientCancelled, deliveryErr.Outcome)
				var failoverErr *service.UpstreamFailoverError
				require.False(t, errors.As(got.err, &failoverErr))
				require.NotNil(t, got.result)
				require.Equal(t, observed, got.result.HasObservedConsumption())
				if observed {
					require.Equal(t, 17, got.result.Usage.InputTokens)
					require.Equal(t, 8, got.result.Usage.OutputTokens)
					// RecordUsage detaches persistence from the cancelled client.
					require.NoError(t, svc.RecordUsage(clientCtx, &service.OpenAIRecordUsageInput{
						Result: got.result, APIKey: &service.APIKey{ID: 2}, User: &service.User{ID: 3}, Account: account,
					}))
				} else {
					require.Zero(t, got.result.Usage.InputTokens)
					require.Zero(t, got.result.Usage.OutputTokens)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("forward did not stop after the downstream disconnected")
			}
			select {
			case <-upstreamCancelled:
			case <-time.After(5 * time.Second):
				t.Fatal("downstream cancellation did not reach the HTTP upstream")
			}
			if observed {
				require.Len(t, billingRepo.commands, 1)
				require.Len(t, usageRepo.logs, 1)
				require.Equal(t, 17, usageRepo.logs[0].InputTokens)
				require.Equal(t, 8, usageRepo.logs[0].OutputTokens)
				require.Equal(t, "client_cancelled", *usageRepo.logs[0].ResponseOutcome)
				require.Greater(t, usageRepo.logs[0].ActualCost, 0.0)
			} else {
				require.Empty(t, billingRepo.commands)
				require.Empty(t, usageRepo.logs)
			}
			s, ok := upstream.(*httpUpstreamService)
			require.True(t, ok)
			requireNoUpstreamInFlight(t, s)
			for _, entry := range s.clients {
				entry.client.CloseIdleConnections()
			}
		})
	}
}
