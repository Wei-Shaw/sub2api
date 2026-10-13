//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxM31LegacyGatewayRejectsInvalidThinking(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []string{APIProtocolAnthropic, APIProtocolChatCompletions, APIProtocolResponses} {
		for _, model := range []string{minimaxM31TestModel, "client-alias"} {
			for _, preference := range []string{"disabled", "none", "auto"} {
				t.Run(protocol+"/"+model+"/"+preference, func(t *testing.T) {
					effort, thinking := preference, ""
					if preference == "disabled" {
						effort, thinking = "max", "disabled"
						if protocol == APIProtocolResponses {
							effort = "none"
						}
					}
					body := minimaxM31TestBody(model, protocol, effort, thinking, false)
					account := &Account{ID: 1, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Credentials: map[string]any{
						"api_key": "fixture-key", "base_url": "http://minimax.example", "model_mapping": map[string]any{"client-alias": minimaxM31TestModel},
					}}
					upstream := &httpUpstreamRecorder{err: errors.New("unexpected upstream request")}
					svc := &GatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
					c := adaptiveProtocolTestContext("/v1/test", body)
					var err error
					switch protocol {
					case APIProtocolAnthropic:
						parsed, parseErr := ParseGatewayRequest(NewRequestBodyRef(body), "")
						require.NoError(t, parseErr)
						_, err = svc.Forward(context.Background(), c, account, parsed)
					case APIProtocolChatCompletions:
						_, err = svc.ForwardAsChatCompletions(context.Background(), c, account, body, nil)
					default:
						_, err = svc.ForwardAsResponses(context.Background(), c, account, body, nil)
					}
					require.Error(t, err)
					require.Equal(t, http.StatusBadRequest, c.Writer.Status())
					require.Nil(t, upstream.lastReq)
				})
			}
		}
	}
}
