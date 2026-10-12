//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type openAIVideoTransportFailure struct {
	HTTPUpstream
	err   error
	calls int
}

func (s *openAIVideoTransportFailure) Do(*http.Request, string, int64, int) (*http.Response, error) {
	s.calls++
	return nil, s.err
}

type openAIVideoFailedRead struct{}

func (openAIVideoFailedRead) Read([]byte) (int, error) { return 0, context.DeadlineExceeded }

func TestOpenAIVideoCreateUnknownNeverReturnsFailover(t *testing.T) {
	for _, cause := range []error{context.DeadlineExceeded, context.Canceled, io.ErrUnexpectedEOF, errors.New("connection reset by peer")} {
		upstream := &openAIVideoTransportFailure{err: cause}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
		_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, openAIVideoTestAccount(),
			[]byte(`{"model":"video","seconds":"6","size":"1280x720"}`), "application/json",
			func(*OpenAIForwardResult) error { t.Fatal("unknown task must not be saved as successful"); return nil })
		var unknown *OpenAIVideoCreateUnknownError
		require.ErrorAs(t, err, &unknown)
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover))
		require.Equal(t, 1, upstream.calls)
		require.False(t, c.Writer.Written())
	}
}

func TestOpenAIVideoAmbiguousResponseNeverReturnsFailover(t *testing.T) {
	for _, status := range []int{http.StatusRequestTimeout, http.StatusInternalServerError, http.StatusBadGateway, http.StatusGatewayTimeout, http.StatusTemporaryRedirect, http.StatusOK} {
		response := &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"status":"queued"}`))}
		upstream := &grokMediaContentUpstreamStub{response: response}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
		_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, openAIVideoTestAccount(),
			[]byte(`{"model":"video","seconds":"6","size":"1280x720"}`), "application/json", func(*OpenAIForwardResult) error { return nil })
		var unknown *OpenAIVideoCreateUnknownError
		require.ErrorAs(t, err, &unknown, "status=%d", status)
		var failover *UpstreamFailoverError
		require.False(t, errors.As(err, &failover))
		require.Len(t, upstream.requests, 1)
	}
}

func TestOpenAIVideoReadTimeoutAndPersistenceFailureRemainUnknown(t *testing.T) {
	for _, readFails := range []bool{true, false} {
		response := grokMediaContentStatusResponse(`{"id":"accepted-1","status":"queued"}`)
		if readFails {
			response.Body = io.NopCloser(openAIVideoFailedRead{})
		}
		upstream := &grokMediaContentUpstreamStub{response: response}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
		c, _ := grokMediaContentTestContext(http.MethodPost, "https://api.example/v1/videos", nil)
		_, err := svc.ForwardOpenAIVideoCreate(context.Background(), c, openAIVideoTestAccount(),
			[]byte(`{"model":"video","seconds":"6","size":"1280x720"}`), "application/json", func(*OpenAIForwardResult) error { return errors.New("database unavailable") })
		var unknown *OpenAIVideoCreateUnknownError
		require.ErrorAs(t, err, &unknown)
		if !readFails {
			require.Equal(t, "accepted-1", unknown.TaskID)
		}
		require.False(t, c.Writer.Written())
	}
}

func TestOpenAIVideoLookupUsesOwnerDespiteCreationCooldown(t *testing.T) {
	owner := openAIVideoTestAccount()
	owner.ID = 313
	owner.Schedulable = false
	until := time.Now().Add(time.Hour)
	owner.RateLimitResetAt = &until
	svc := &OpenAIGatewayService{accountRepo: stubOpenAIAccountRepo{accounts: []Account{*owner, {ID: 493, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Schedulable: true}}}}
	got, err := svc.GetOpenAIVideoLookupAccount(context.Background(), 313)
	require.NoError(t, err)
	require.Equal(t, int64(313), got.ID)
	got, err = svc.GetOpenAIVideoLookupAccount(context.Background(), 999)
	require.Error(t, err)
	require.Nil(t, got)
	owner.Platform = PlatformGrok
	svc.accountRepo = stubOpenAIAccountRepo{accounts: []Account{*owner}}
	_, err = svc.GetOpenAIVideoLookupAccount(context.Background(), 313)
	require.Error(t, err)
}
