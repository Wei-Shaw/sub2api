package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"
)

func TestDeriveOpenAIContentSessionSeed_ResponsesLeadingPrefix(t *testing.T) {
	makeBody := func(items ...string) []byte {
		return []byte(`{"model":"gpt-5.4","instructions":"Be concise.","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],"input":[` + strings.Join(items, ",") + `]}`)
	}
	prefix := []string{
		`{"role":"system","content":"System A"}`,
		`{"role":"developer","content":[{"type":"input_text","text":"Developer B"}]}`,
	}
	boundaries := []struct {
		name string
		item string
	}{
		{"user", `{"role":"user","content":"First question"}`},
		{"assistant", `{"role":"assistant","content":"Earlier answer"}`},
		{"function_call", `{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"}`},
		{"function_call_output", `{"type":"function_call_output","call_id":"call_1","output":"Result"}`},
		{"input_text", `{"type":"input_text","text":"First question"}`},
	}
	for _, boundary := range boundaries {
		for _, role := range []string{"system", "developer"} {
			t.Run(boundary.name+"/"+role, func(t *testing.T) {
				items := append(append([]string(nil), prefix...), boundary.item)
				baseline := makeBody(append(items, `{"role":"user","content":"First question"}`)...)
				reminder := `{"role":"` + role + `","content":[{"type":"input_text","text":"Dynamic reminder"}]}`
				continued := makeBody(append(items, reminder, `{"role":"user","content":"First question"}`, `{"role":"user","content":"Follow-up"}`)...)
				seed := deriveOpenAIContentSessionSeed(baseline)
				require.Contains(t, seed, "System A")
				require.Contains(t, seed, "Developer B")
				require.Contains(t, seed, "First question")
				require.Equal(t, seed, deriveOpenAIContentSessionSeed(continued), "later reminders must not change the session identity")
			})
		}
	}

	t.Run("stable fields still distinguish sessions", func(t *testing.T) {
		body := makeBody(append(prefix, `{"role":"user","content":"First question"}`)...)
		seed := deriveOpenAIContentSessionSeed(body)
		for _, change := range [][2]string{
			{"gpt-5.4", "gpt-5.5"},
			{"Be concise.", "Be detailed."},
			{"lookup", "search"},
			{"System A", "System C"},
			{"Developer B", "Developer C"},
			{"First question", "Compacted question"},
		} {
			t.Run(change[0], func(t *testing.T) {
				require.NotEqual(t, seed, deriveOpenAIContentSessionSeed([]byte(strings.ReplaceAll(string(body), change[0], change[1]))))
			})
		}
	})
}

func TestOpenAIGatewayService_ResponsesToolLoopKeepsStickyAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	first := []byte(`{"model":"gpt-5.4","input":[{"role":"system","content":"Be helpful."},{"role":"user","content":"Look up the weather"}]}`)
	continued := []byte(`{"model":"gpt-5.4","input":[{"role":"system","content":"Be helpful."},{"role":"user","content":"Look up the weather"},{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"Sunny"},{"role":"developer","content":"Answer using the tool result."},{"role":"user","content":"And tomorrow?"}]}`)
	for _, mode := range []string{"false", "true"} {
		for _, identity := range []string{"content", "header", "prompt_cache_key"} {
			t.Run("advanced="+mode+"/"+identity, func(t *testing.T) {
				t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				firstBody, nextBody := first, continued
				switch identity {
				case "header":
					c.Request.Header.Set("session-id", "stable-conversation")
				case "prompt_cache_key":
					var err error
					firstBody, err = sjson.SetBytes(first, "prompt_cache_key", "stable-conversation")
					require.NoError(t, err)
					nextBody, err = sjson.SetBytes(continued, "prompt_cache_key", "stable-conversation")
					require.NoError(t, err)
				}
				require.True(t, json.Valid(firstBody))
				require.True(t, json.Valid(nextBody))
				groupID := int64(10)
				cfg := &config.Config{}
				cfg.Gateway.OpenAIWS.LBTopK = 1
				svc := &OpenAIGatewayService{
					accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{
						{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 100, GroupIDs: []int64{groupID}},
						{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0, GroupIDs: []int64{groupID}},
					}},
					cfg:                cfg,
					rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(mode),
					concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
				}
				firstHash := svc.GenerateSessionHash(c, firstBody)
				require.NotEmpty(t, firstHash)
				svc.cache = &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:" + firstHash: 1}}
				selection, decision, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", svc.GenerateSessionHash(c, nextBody), "gpt-5.4", nil, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				require.NotNil(t, selection)
				if selection.ReleaseFunc != nil {
					defer selection.ReleaseFunc()
				}
				require.Equal(t, int64(1), selection.Account.ID, "a reminder must not lose a healthy existing binding to the higher-priority account")
				require.True(t, decision.StickySessionHit)
			})
		}
	}
}
