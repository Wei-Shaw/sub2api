package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// fanoutUpstreamStub answers each upstream call through respond, which receives
// the zero-based order in which the call arrived. It records every request and
// the highest number of calls in flight at once.
type fanoutUpstreamStub struct {
	respond func(call int, req *http.Request) (*http.Response, error)

	mu        sync.Mutex
	requests  []*http.Request
	bodies    [][]byte
	inFlight  int
	maxFlight int
}

func (u *fanoutUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()

	u.mu.Lock()
	call := len(u.requests)
	u.requests = append(u.requests, req)
	u.bodies = append(u.bodies, body)
	u.inFlight++
	u.maxFlight = max(u.maxFlight, u.inFlight)
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.inFlight--
		u.mu.Unlock()
	}()

	return u.respond(call, req)
}

func (u *fanoutUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, accountConcurrency)
}

func fanoutSSEResponse(requestID, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{requestID},
		},
		Body: io.NopCloser(strings.NewReader(body)),
	}
}

func fanoutImageBody(call int) string {
	images := []string{"aW1hZ2UtMQ==", "aW1hZ2UtMg==", "aW1hZ2UtMw=="}
	return "data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000000,\"usage\":{\"input_tokens\":11,\"output_tokens\":22,\"input_tokens_details\":{\"cached_tokens\":3},\"output_tokens_details\":{\"image_tokens\":7}},\"tool_usage\":{\"image_gen\":{\"input_tokens\":46,\"output_tokens\":2459,\"output_tokens_details\":{\"image_tokens\":2459},\"images\":1}},\"output\":[{\"type\":\"image_generation_call\",\"result\":\"" +
		images[call] + "\",\"revised_prompt\":\"draw a cat " + fmt.Sprint(call+1) + "\",\"output_format\":\"png\",\"quality\":\"high\",\"size\":\"1024x1024\"}]}}\n\n" +
		"data: [DONE]\n\n"
}

// waitOrCancelled blocks for d, or returns the request's context error if the
// request is cancelled first, the way a real transport does.
func waitOrCancelled(req *http.Request, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-req.Context().Done():
		return req.Context().Err()
	}
}

func newFanoutImagesTest(t *testing.T, body string, upstream *fanoutUpstreamStub) (*OpenAIGatewayService, *gin.Context, *httptest.ResponseRecorder, *OpenAIImagesRequest, *Account) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set("api_key", &APIKey{ID: 42})

	svc := &OpenAIGatewayService{httpUpstream: upstream}
	parsed, err := svc.ParseOpenAIImagesRequest(c, []byte(body))
	require.NoError(t, err)
	account := &Account{
		ID:       1,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "token-123",
			"chatgpt_account_id": "acct-123",
		},
	}
	return svc, c, rec, parsed, account
}

func TestOpenAIGatewayServiceForwardImages_OAuthFansOutNAndReturnsAllImages(t *testing.T) {
	body := `{"model":"gpt-image-1","prompt":"draw a cat","size":"1024x1024","quality":"high","n":3}`
	upstream := &fanoutUpstreamStub{respond: func(call int, req *http.Request) (*http.Response, error) {
		if err := waitOrCancelled(req, 50*time.Millisecond); err != nil {
			return nil, err
		}
		return fanoutSSEResponse(fmt.Sprintf("req_img_%d", call+1), fanoutImageBody(call)), nil
	}}
	svc, c, rec, parsed, account := newFanoutImagesTest(t, body, upstream)

	result, err := svc.ForwardImages(context.Background(), c, account, []byte(body), parsed, "")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "gpt-image-1", result.Model)
	require.Equal(t, "gpt-image-1", result.UpstreamModel)
	require.False(t, result.Stream)
	require.True(t, result.ImageEndpoint)
	require.Equal(t, 3, result.ImageCount)
	require.Equal(t, []string{"1024x1024", "1024x1024", "1024x1024"}, result.ImageOutputSizes)
	require.Equal(t, 138, result.Usage.InputTokens)
	require.Equal(t, 7377, result.Usage.OutputTokens)
	require.Equal(t, 7377, result.Usage.ImageOutputTokens)
	require.Regexp(t, `^req_img_[123]$`, result.RequestID)

	require.Len(t, upstream.requests, 3)
	require.GreaterOrEqual(t, upstream.maxFlight, 2)
	sessionIDs := make(map[string]struct{})
	for i, upstreamReq := range upstream.requests {
		require.Equal(t, chatgptCodexURL, upstreamReq.URL.String())
		require.Equal(t, "application/json", upstreamReq.Header.Get("Content-Type"))
		require.Equal(t, "text/event-stream", upstreamReq.Header.Get("Accept"))
		require.Equal(t, "acct-123", upstreamReq.Header.Get("chatgpt-account-id"))
		require.Equal(t, "responses=experimental", upstreamReq.Header.Get("OpenAI-Beta"))
		sessionIDs[upstreamReq.Header.Get("session_id")] = struct{}{}

		upstreamBody := upstream.bodies[i]
		require.Equal(t, "image_generation", gjson.GetBytes(upstreamBody, "tools.0.type").String())
		require.Equal(t, "generate", gjson.GetBytes(upstreamBody, "tools.0.action").String())
		require.Equal(t, "gpt-image-1", gjson.GetBytes(upstreamBody, "tools.0.model").String())
		require.Equal(t, "1024x1024", gjson.GetBytes(upstreamBody, "tools.0.size").String())
		require.Equal(t, "high", gjson.GetBytes(upstreamBody, "tools.0.quality").String())
		require.False(t, gjson.GetBytes(upstreamBody, "tools.0.n").Exists())
		require.Equal(t, "draw a cat", gjson.GetBytes(upstreamBody, "input.0.content.0.text").String())
	}
	require.Len(t, sessionIDs, 3, "each call carries its own session")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "gpt-image-1", gjson.Get(rec.Body.String(), "model").String())
	data := gjson.Get(rec.Body.String(), "data").Array()
	require.Len(t, data, 3)
	var images, revisedPrompts []string
	for _, item := range data {
		images = append(images, item.Get("b64_json").String())
		revisedPrompts = append(revisedPrompts, item.Get("revised_prompt").String())
	}
	require.ElementsMatch(t, []string{"aW1hZ2UtMQ==", "aW1hZ2UtMg==", "aW1hZ2UtMw=="}, images)
	require.ElementsMatch(t, []string{"draw a cat 1", "draw a cat 2", "draw a cat 3"}, revisedPrompts)
	require.Equal(t, int64(138), gjson.Get(rec.Body.String(), "usage.input_tokens").Int())
	require.Equal(t, int64(7377), gjson.Get(rec.Body.String(), "usage.output_tokens").Int())
	require.Equal(t, int64(7515), gjson.Get(rec.Body.String(), "usage.total_tokens").Int())
	require.Equal(t, int64(7377), gjson.Get(rec.Body.String(), "usage.output_tokens_details.image_tokens").Int())
}

func TestOpenAIGatewayServiceForwardImages_OAuthFanoutRejectsStreamingN(t *testing.T) {
	body := `{"model":"gpt-image-1","prompt":"draw a cat","stream":true,"n":2}`
	upstream := &fanoutUpstreamStub{}
	svc, c, rec, parsed, account := newFanoutImagesTest(t, body, upstream)

	result, err := svc.ForwardImages(context.Background(), c, account, []byte(body), parsed, "")
	require.Nil(t, result)
	var upstreamErr *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Empty(t, upstream.requests)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_request_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Equal(t, "unsupported_parameter", gjson.Get(rec.Body.String(), "error.code").String())
	require.Equal(t, "n", gjson.Get(rec.Body.String(), "error.param").String())
}

func TestOpenAIGatewayServiceForwardImages_OAuthFanoutRejectsNAboveLimit(t *testing.T) {
	body := fmt.Sprintf(`{"model":"gpt-image-1","prompt":"draw a cat","n":%d}`, openAIImagesOAuthFanoutMaxN+1)
	upstream := &fanoutUpstreamStub{}
	svc, c, rec, parsed, account := newFanoutImagesTest(t, body, upstream)

	result, err := svc.ForwardImages(context.Background(), c, account, []byte(body), parsed, "")
	require.Nil(t, result)
	var upstreamErr *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Empty(t, upstream.requests)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "invalid_value", gjson.Get(rec.Body.String(), "error.code").String())
	require.Equal(t, "n", gjson.Get(rec.Body.String(), "error.param").String())
}

func TestOpenAIGatewayServiceForwardImages_OAuthFanoutHTTPErrorFailsOverAndCancelsOtherCalls(t *testing.T) {
	body := `{"model":"gpt-image-1","prompt":"draw a cat","n":3}`
	var cancelled sync.WaitGroup
	cancelled.Add(2)
	upstream := &fanoutUpstreamStub{respond: func(call int, req *http.Request) (*http.Response, error) {
		if call == 0 {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     http.Header{"Content-Type": []string{"application/json"}, "X-Request-Id": []string{"req_img_failed"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"server_error","message":"The image service is temporarily unavailable."}}`)),
			}, nil
		}
		defer cancelled.Done()
		// Slower than any test timeout: these calls end only by cancellation.
		if err := waitOrCancelled(req, time.Minute); err != nil {
			return nil, err
		}
		return fanoutSSEResponse("req_img_late", fanoutImageBody(call)), nil
	}}
	svc, c, rec, parsed, account := newFanoutImagesTest(t, body, upstream)

	started := time.Now()
	result, err := svc.ForwardImages(context.Background(), c, account, []byte(body), parsed, "")
	require.Less(t, time.Since(started), 10*time.Second)
	cancelled.Wait()

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusInternalServerError, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "temporarily unavailable")
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())

	rawEvents, ok := c.Get(OpsUpstreamErrorsKey)
	require.True(t, ok)
	events, ok := rawEvents.([]*OpsUpstreamErrorEvent)
	require.True(t, ok)
	require.Len(t, events, 1)
	require.Equal(t, "failover", events[0].Kind)
	require.Equal(t, http.StatusInternalServerError, events[0].UpstreamStatusCode)
	require.Equal(t, "req_img_failed", events[0].UpstreamRequestID)
}

func TestOpenAIGatewayServiceForwardImages_OAuthFanoutModerationBlockedReturnsClientError(t *testing.T) {
	body := `{"model":"gpt-image-1","prompt":"draw blocked image","n":2}`
	upstream := &fanoutUpstreamStub{respond: func(call int, req *http.Request) (*http.Response, error) {
		if call == 0 {
			return fanoutSSEResponse("req_img_blocked",
				"data: {\"type\":\"response.created\",\"response\":{\"created_at\":1710000020}}\n\n"+
					"data: {\"type\":\"error\",\"error\":{\"type\":\"image_generation_user_error\",\"code\":\"moderation_blocked\",\"message\":\"Your request was rejected by the safety system. safety_violations=[sexual].\"}}\n\n"), nil
		}
		if err := waitOrCancelled(req, time.Minute); err != nil {
			return nil, err
		}
		return fanoutSSEResponse("req_img_late", fanoutImageBody(call)), nil
	}}
	svc, c, rec, parsed, account := newFanoutImagesTest(t, body, upstream)

	result, err := svc.ForwardImages(context.Background(), c, account, []byte(body), parsed, "")
	require.Nil(t, result)
	var upstreamErr *OpenAIImagesUpstreamError
	require.ErrorAs(t, err, &upstreamErr)
	require.Equal(t, http.StatusBadRequest, upstreamErr.StatusCode)
	require.Equal(t, "moderation_blocked", upstreamErr.Code)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "image_generation_user_error", gjson.Get(rec.Body.String(), "error.type").String())
	require.Equal(t, "moderation_blocked", gjson.Get(rec.Body.String(), "error.code").String())
	require.Contains(t, gjson.Get(rec.Body.String(), "error.message").String(), "safety system")
}

func TestOpenAIGatewayServiceForwardImages_OAuthFanoutEmptyResponseRetriesOnSameAccount(t *testing.T) {
	body := `{"model":"gpt-image-1","prompt":"draw a cat","n":2}`
	upstream := &fanoutUpstreamStub{respond: func(call int, req *http.Request) (*http.Response, error) {
		if call == 0 {
			return fanoutSSEResponse("req_img_empty",
				"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000030,\"status\":\"completed\",\"output\":[]}}\n\n"), nil
		}
		if err := waitOrCancelled(req, time.Minute); err != nil {
			return nil, err
		}
		return fanoutSSEResponse("req_img_late", fanoutImageBody(call)), nil
	}}
	svc, c, rec, parsed, account := newFanoutImagesTest(t, body, upstream)

	result, err := svc.ForwardImages(context.Background(), c, account, []byte(body), parsed, "")
	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.True(t, failoverErr.RetryableOnSameAccount)
	require.False(t, c.Writer.Written())
	require.Empty(t, rec.Body.String())
}
