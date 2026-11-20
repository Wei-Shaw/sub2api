//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestDeepseekSparseToolsOrigin(t *testing.T) {
	for _, protocol := range []string{APIProtocolResponses, APIProtocolAnthropic} {
		account := newDeepseekToolsTestAccount("")
		account.Credentials["api_protocol"] = APIProtocolAdaptive
		account.Credentials["api_base_urls"] = map[string]any{protocol: "https://custom.example/provider"}
		account.Extra = map[string]any{"deepseek_tools_supported": true}
		require.False(t, account.supportsDeepseekTools())
		for _, op := range []string{"search", "files_create"} {
			_, err := buildDeepseekToolsURL(account, op, "", false, (&OpenAIGatewayService{}).validateUpstreamBaseURL)
			require.ErrorContains(t, err, "unambiguous")
		}
	}
}

func TestDeepseekOwnershipFollowsWireShapeAndOrigin(t *testing.T) {
	store := &testDeepseekFileCache{}
	account := newDeepseekToolsTestAccount(DefaultDeepseekBaseURL)
	account.Credentials["api_protocol"] = APIProtocolAdaptive
	gateway := &OpenAIGatewayService{cache: store}
	require.NoError(t, store.SaveDeepseekFile(t.Context(), 1, DeepseekFile{ID: "own", AccountID: account.ID, Format: "openai", Origin: DefaultDeepseekBaseURL}))
	for _, tc := range []struct{ endpoint, body string }{
		{APIProtocolChatCompletions, `{"model":"deepseek-flash","input":[{"role":"user","content":[{"type":"input_file","file_id":"own"}]}]}`},
		{APIProtocolResponses, `{"model":"deepseek-flash","messages":[{"role":"user","content":[{"type":"file","file":{"file_id":"own"}}]}]}`},
	} {
		ctx, normalized, err := gateway.PrepareDeepseekFileIngress(t.Context(), PlatformDeepseek, tc.endpoint, 1, []byte(tc.body))
		require.NoError(t, err)
		require.NotEmpty(t, gjson.GetBytes(normalized, "input.0.content.0.file_id").String())
		require.Equal(t, account.ID, ctx.Value(deepseekFileAccountKey{}))
		_, _, err = gateway.PrepareDeepseekFileIngress(t.Context(), PlatformDeepseek, tc.endpoint, 2, []byte(tc.body))
		require.ErrorContains(t, err, "Referenced file not found")
	}
	body := []byte(`{"model":"deepseek-flash","messages":[{"content":[{"type":"file","file_id":"own"}]}]}`)
	ctx, _, err := gateway.PrepareDeepseekFileIngress(t.Context(), PlatformOpenAI, APIProtocolChatCompletions, 2, body)
	require.NoError(t, err)
	mapped := *account
	mapped.Platform = PlatformOpenAI
	req := httptest.NewRequest("POST", DefaultDeepseekBaseURL+"/v1/chat/completions", nil).WithContext(ctx)
	_, err = gateway.doOpenAIUpstream(req, "", &mapped)
	require.ErrorContains(t, err, "Referenced file not found")
	ctx, _, err = gateway.PrepareDeepseekFileIngress(t.Context(), PlatformDeepseek, APIProtocolChatCompletions, 1, body)
	require.NoError(t, err)
	req = httptest.NewRequest("POST", "https://different.example/anthropic/v1/messages", nil).WithContext(ctx)
	_, err = gateway.doOpenAIUpstream(req, "", account)
	require.ErrorContains(t, err, "Referenced file not found")
	// Both wire families can use the same storage namespace.
	req = httptest.NewRequest("POST", DefaultDeepseekBaseURL+"/anthropic/v1/messages", nil).WithContext(ctx)
	require.NoError(t, verifyDeepseekFileDestination(ctx, store, account, req.URL))
	account.Credentials["api_base_urls"] = map[string]any{APIProtocolAnthropic: "https://different.example/anthropic"}
	for _, op := range []string{"files_get/own", "files_delete/own"} {
		c, _ := newDeepseekToolsTestContext("GET", "/v1/files/own", nil)
		c.Request.Header.Set("anthropic-beta", deepseekFilesBetaHeader)
		_, err := forwardDeepseekToolsTest(gateway, c, account, 1, op, nil, "")
		require.ErrorIs(t, err, errDeepseekFileNotFound)
	}
}

func TestDeepseekToolPriceSkipsTokenWildcard(t *testing.T) {
	model := "deepseek-tools-search"
	channelPrice := 0.03
	groupPrice := 0.07
	resolver := newResolverWithChannelForPlatform(t, PlatformDeepseek, []ChannelModelPricing{{Platform: PlatformDeepseek, Models: []string{model}, BillingMode: BillingModePerRequest, PerRequestPrice: &channelPrice}})
	group := &Group{ID: *groupIDPtr(), Platform: PlatformDeepseek, ModelPricing: []ChannelModelPricing{{Platform: PlatformDeepseek, Models: []string{"deepseek-*"}, BillingMode: BillingModeToken, InputPrice: testPtrFloat64(1)}}}
	usage := DeepseekToolsUsage{Endpoint: "search", Units: 2}
	price := func(want float64) {
		t.Helper()
		cost, err := resolver.billingService.calculateDeepseekToolCost(t.Context(), resolver, &APIKey{Group: group}, usage, 3)
		require.NoError(t, err)
		require.InDelta(t, 6*want, cost.ActualCost, 1e-12)
	}
	price(channelPrice)
	group.ModelPricing = append(group.ModelPricing, ChannelModelPricing{Platform: PlatformDeepseek, Models: []string{model}, BillingMode: BillingModePerRequest, PerRequestPrice: &groupPrice})
	_, err := normalizeGroupModelPricing(PlatformDeepseek, group.ModelPricing)
	require.NoError(t, err)
	price(groupPrice)
}

func TestQueuedGatewayValidationFrames(t *testing.T) {
	for _, tc := range []struct {
		name   string
		write  compatErrorWriter
		marker string
	}{
		{"chat", writeChatCompletionsError, "data: "},
		{"responses", writeResponsesError, "event: response.failed"},
		{"responses fallback", writeOpenAIResponsesFallbackError, "event: response.failed"},
		{"messages", writeAnthropicError, "event: error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Header("Content-Type", "text/event-stream")
			_, _ = c.Writer.WriteString(": queue\n\n")
			c.Writer.Flush()
			err := rejectGatewayRequest(c, tc.write, "bad input", errors.New("bad input"))
			require.ErrorIs(t, err, errInvalidGatewayRequest)
			require.True(t, IsResponseCommitted(c))
			require.Equal(t, 200, c.Writer.Status())
			require.Contains(t, c.Writer.Header().Get("Content-Type"), "text/event-stream")
			frame := strings.TrimPrefix(rec.Body.String(), ": queue\n\n")
			require.True(t, strings.HasPrefix(frame, tc.marker), frame)
			require.Contains(t, frame, "bad input")
			require.Equal(t, 1, strings.Count(frame, "data: "))
		})
	}
}

func TestDeepseekASRFilePartBoundary(t *testing.T) {
	for _, extra := range []int{0, 1} {
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		part, err := w.CreateFormFile("file", "near-limit.wav")
		require.NoError(t, err)
		_, err = part.Write(make([]byte, DeepseekASRFileLimit+extra))
		require.NoError(t, err)
		require.NoError(t, w.Close())
		require.Greater(t, body.Len(), DeepseekASRFileLimit)
		c, _ := newDeepseekToolsTestContext(http.MethodPost, "/v1/tools/asr", nil)
		_, err = (&OpenAIGatewayService{}).PrepareDeepseekToolsRequest(t.Context(), c, 1, "asr", body.Bytes(), w.FormDataContentType())
		if extra == 0 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "size limit")
		}
	}
}

type retryDeleteTestStore struct {
	testDeepseekFileCache
	failures int
	attempts int
}

func (s *retryDeleteTestStore) DeleteDeepseekFile(ctx context.Context, user, account int64, id string) error {
	s.attempts++
	if s.attempts <= s.failures {
		return errors.New("temporary redis error")
	}
	return s.testDeepseekFileCache.DeleteDeepseekFile(ctx, user, account, id)
}

func TestDeepseekDeleteReconcilesMissingFile(t *testing.T) {
	store := &retryDeleteTestStore{failures: 3}
	account := newDeepseekToolsTestAccount(DefaultDeepseekBaseURL)
	require.NoError(t, store.SaveDeepseekFile(t.Context(), 7, DeepseekFile{ID: "f", AccountID: account.ID, Origin: DefaultDeepseekBaseURL}))
	calls := 0
	gateway := &OpenAIGatewayService{cache: store, httpUpstream: &deepseekToolsHTTPUpstream{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"type":"not_found_error","message":"File not found"}}`)
	})}}
	for i := range 2 {
		c, _ := newDeepseekToolsTestContext("DELETE", "/v1/files/f", nil)
		res, err := forwardDeepseekToolsTest(gateway, c, account, 7, "files_delete/f", nil, "")
		if i == 0 {
			require.ErrorIs(t, err, errDeepseekFileStore)
			require.Equal(t, 3, store.attempts)
		} else {
			require.NoError(t, err)
			require.Equal(t, 204, res.StatusCode)
		}
	}
	require.Empty(t, store.files[7])
	require.Equal(t, 2, calls)
}

func TestDeepseekDrainingSlotRenewsUntilRelease(t *testing.T) {
	parent, cancel := context.WithCancel(t.Context())
	ctx := WithDrainingSlotTTL(parent, 15*time.Millisecond)
	renewed := make(chan struct{}, 1)
	released := false
	done := keepDrainingSlot(ctx, func(attempt context.Context) (bool, error) {
		if err := attempt.Err(); err != nil {
			return false, err
		}
		select {
		case renewed <- struct{}{}:
		default:
		}
		return true, nil
	}, func() { released = true })
	cancel()
	select {
	case <-renewed:
	case <-time.After(time.Second):
		t.Fatal("slot was not renewed after disconnect")
	}
	done()
	require.True(t, released)
}
