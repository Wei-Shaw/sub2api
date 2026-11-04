//go:build unit

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type webPollFailureTransport struct {
	HTTPUpstream
	statuses     []int
	challenge    bool
	emptySuccess bool
	retryAfter   string
	calls        int
}

func (f *webPollFailureTransport) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	if req.Method != http.MethodGet || !strings.HasPrefix(req.URL.Path, "/backend-api/conversations/") {
		return nil, fmt.Errorf("unexpected generation/resubmission: %s", req.URL.Path)
	}
	index := f.calls
	f.calls++
	if index >= len(f.statuses) {
		index = len(f.statuses) - 1
	}
	status := f.statuses[index]
	if status == 0 {
		return nil, errors.New("temporary network failure")
	}
	header := make(http.Header)
	if f.retryAfter != "" {
		header.Set("Retry-After", f.retryAfter)
	}
	if f.challenge {
		header.Set("cf-mitigated", "challenge")
	}
	body := `{"error":"fixture"}`
	if status == 200 && !f.emptySuccess {
		body = `{"message":{"author":{"role":"tool"},"content":{"parts":[{"asset_pointer":"file-service://result"}]}}}`
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
}
func TestChatGPTWebPollFailureClassification(t *testing.T) {
	for _, status := range []int{401, 403, 402, 429, 400} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			transport := &webPollFailureTransport{statuses: []int{status}}
			session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
			err := session.fetchConversationImages(context.Background(), "conversation", &chatGPTWebImageOutcome{})
			var upstream *chatGPTWebUpstreamError
			require.ErrorAs(t, err, &upstream)
			require.Equal(t, status, upstream.StatusCode)
			require.Equal(t, 1, transport.calls)
		})
	}
}
func TestChatGPTWebPollTransientRecoveryAndExhaustion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []int
		success  bool
	}{
		{"new conversation visibility", []int{404, 200}, true},
		{"server recovery", []int{503, 200}, true}, {"network recovery", []int{0, 200}, true},
		{"server exhaustion", []int{503}, false}, {"network exhaustion", []int{0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transport := &webPollFailureTransport{statuses: tc.statuses}
			session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
			outcome := &chatGPTWebImageOutcome{}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := session.fetchConversationImages(ctx, "conversation", outcome)
			if tc.success {
				require.NoError(t, err)
				require.Equal(t, 2, transport.calls)
				require.Equal(t, []string{"file-service://result"}, outcome.Pointers)
			} else {
				require.Error(t, err)
				require.Equal(t, 3, transport.calls)
				if tc.statuses[0] == 503 {
					var upstream *chatGPTWebUpstreamError
					require.ErrorAs(t, err, &upstream)
					require.Equal(t, 503, upstream.StatusCode)
				}
			}
		})
	}
}
func TestChatGPTWebPollChallengeAndCancellation(t *testing.T) {
	transport := &webPollFailureTransport{statuses: []int{503}, challenge: true}
	session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
	err := session.fetchConversationImages(context.Background(), "conversation", &chatGPTWebImageOutcome{})
	var upstream *chatGPTWebUpstreamError
	require.ErrorAs(t, err, &upstream)
	require.True(t, upstream.Challenge)
	require.Equal(t, 1, transport.calls)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, session.fetchConversationImages(ctx, "conversation", &chatGPTWebImageOutcome{}), context.Canceled)
	require.Equal(t, 1, transport.calls)
}

type webUploadFailureTransport struct {
	*webImageFixtureUpstream
	failPath  string
	status    int
	challenge bool
}

func (f *webUploadFailureTransport) failure() *http.Response {
	header := make(http.Header)
	if f.challenge {
		header.Set("cf-mitigated", "challenge")
	}
	return &http.Response{StatusCode: f.status, Header: header, Body: io.NopCloser(strings.NewReader(`{"error":"fixture"}`))}
}
func (f *webUploadFailureTransport) DoWithTLS(req *http.Request, p string, a int64, c int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	if req.URL.Path == f.failPath {
		return f.failure(), nil
	}
	return f.webImageFixtureUpstream.DoWithTLS(req, p, a, c, profile)
}
func (f *webUploadFailureTransport) Do(req *http.Request, p string, a int64, c int) (*http.Response, error) {
	if req.URL.Path == f.failPath {
		return f.failure(), nil
	}
	return f.webImageFixtureUpstream.Do(req, p, a, c)
}

type webFailureAccountRepo struct {
	AccountRepository
	cooled bool
}

func (r *webFailureAccountRepo) SetModelRateLimit(context.Context, int64, string, time.Time, ...string) error {
	r.cooled = true
	return nil
}
func TestChatGPTWebUploadErrorsReachFailover(t *testing.T) {
	for _, path := range []string{"/backend-api/files", "/backend-api/files/fixture-input/uploaded", "/upload"} {
		for _, status := range []int{401, 403, 429, 503} {
			for _, challenge := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/challenge=%v", path, status, challenge), func(t *testing.T) {
					var encoded bytes.Buffer
					require.NoError(t, png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))))
					transport := &webUploadFailureTransport{webImageFixtureUpstream: &webImageFixtureUpstream{}, failPath: path, status: status, challenge: challenge}
					repo := &webFailureAccountRepo{}
					svc := &OpenAIGatewayService{httpUpstream: transport, accountRepo: repo}
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest("POST", "/v1/images/edits", nil)
					account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeWebImage, Credentials: map[string]any{"access_token": "fixture-access"}}
					_, err := svc.ForwardImages(context.Background(), c, account, nil, &OpenAIImagesRequest{Endpoint: openAIImagesEditsEndpoint, Model: "gpt-image-2", N: 1, Prompt: "edit", Uploads: []OpenAIImagesUpload{{FileName: "image.png", ContentType: "image/png", Data: encoded.Bytes()}}}, "")
					require.Error(t, err)
					var failover *UpstreamFailoverError
					shouldFailover := path != "/upload" && !challenge
					require.Equal(t, shouldFailover, errors.As(err, &failover))
					require.Equal(t, shouldFailover && status != 503, repo.cooled)
					require.NotContains(t, transport.paths, "/backend-api/f/conversation")
				})
			}
		}
	}
}

func TestChatGPTWebForwardHonorsClientCancellation(t *testing.T) {
	transport := &webUploadFailureTransport{webImageFixtureUpstream: &webImageFixtureUpstream{}}
	svc := &OpenAIGatewayService{httpUpstream: transport}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeWebImage, Credentials: map[string]any{"access_token": "fixture-access"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.ForwardImages(ctx, c, account, nil, &OpenAIImagesRequest{Endpoint: openAIImagesGenerationsEndpoint, Model: "gpt-image-2", N: 1, Prompt: "generate"}, "")
	require.ErrorContains(t, err, "context canceled")
	require.NotContains(t, transport.paths, "/backend-api/f/conversation")
}
func TestChatGPTWebPollCancellationInterruptsRetryWait(t *testing.T) {
	transport := &webPollFailureTransport{statuses: []int{503}}
	session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := session.fetchConversationImages(ctx, "conversation", &chatGPTWebImageOutcome{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, transport.calls)
}

func TestChatGPTWebPollMissingConversationHonorsCancellation(t *testing.T) {
	transport := &webPollFailureTransport{statuses: []int{404}}
	session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err := session.fetchConversationImages(ctx, "new-conversation", &chatGPTWebImageOutcome{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, 1, transport.calls)
}

func TestChatGPTWebPollMissingConversationGraceExpires(t *testing.T) {
	t.Parallel()
	transport := &webPollFailureTransport{statuses: []int{404}}
	session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	err := session.fetchConversationImages(ctx, "new-conversation", &chatGPTWebImageOutcome{})
	var upstream *chatGPTWebUpstreamError
	require.ErrorAs(t, err, &upstream)
	require.Equal(t, 404, upstream.StatusCode)
	require.Greater(t, transport.calls, 1)
	require.LessOrEqual(t, transport.calls, 17)
}

func TestChatGPTWebPollVisibleConversationDoesNotRetry404(t *testing.T) {
	transport := &webPollFailureTransport{statuses: []int{200, 404}, emptySuccess: true}
	session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
	err := session.fetchConversationImages(context.Background(), "conversation", &chatGPTWebImageOutcome{})
	var upstream *chatGPTWebUpstreamError
	require.ErrorAs(t, err, &upstream)
	require.Equal(t, 404, upstream.StatusCode)
	require.Equal(t, 2, transport.calls)
}

func TestChatGPTWebPollRateLimitHonorsRetryAfter(t *testing.T) {
	t.Parallel()
	transport := &webPollFailureTransport{statuses: []int{429, 200}, retryAfter: "3"}
	session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
	started := time.Now()
	err := session.fetchConversationImages(context.Background(), "conversation", &chatGPTWebImageOutcome{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, time.Since(started), 3*time.Second)
	require.Equal(t, 2, transport.calls)
}

func TestChatGPTWebPollRateLimitBudgetAndChallenge(t *testing.T) {
	for _, tc := range []struct {
		name, retry string
		challenge   bool
	}{
		{"no delay", "", false}, {"too long", "3600", false}, {"challenge", "1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transport := &webPollFailureTransport{statuses: []int{429}, retryAfter: tc.retry, challenge: tc.challenge}
			session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
			err := session.fetchConversationImages(context.Background(), "conversation", &chatGPTWebImageOutcome{})
			var upstream *chatGPTWebUpstreamError
			require.ErrorAs(t, err, &upstream)
			require.Equal(t, 429, upstream.StatusCode)
			require.Equal(t, 1, transport.calls)
		})
	}
}

func TestChatGPTWebPollRateLimitRetriesAreBounded(t *testing.T) {
	t.Parallel()
	transport := &webPollFailureTransport{statuses: []int{429}, retryAfter: "1"}
	session := newChatGPTWebSession(transport, nil, "fixture", chatGPTWebDevice{})
	err := session.fetchConversationImages(context.Background(), "conversation", &chatGPTWebImageOutcome{})
	var upstream *chatGPTWebUpstreamError
	require.ErrorAs(t, err, &upstream)
	require.Equal(t, 429, upstream.StatusCode)
	require.Equal(t, 4, transport.calls)
}
