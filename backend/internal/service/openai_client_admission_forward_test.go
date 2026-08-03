package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCodexAdmissionForwardGuardFixture(t *testing.T) (*OpenAIGatewayService, context.Context, *Account, *httpUpstreamRecorder) {
	t.Helper()
	selected := codexAdmissionAccount(8901, true)
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{
		cfg:           &config.Config{},
		httpUpstream:  upstream,
		codexDetector: &accountAwareCodexAdmissionDetector{},
	}
	return svc, newCodexAdmissionContext(t, svc), &selected, upstream
}

func newCodexAdmissionForwardGinContext(method, target string, body []byte) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, bytes.NewReader(body))
	return c
}

func TestCodexClientAdmissionFinalGuardsDoNotReachHTTPUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("alpha search", func(t *testing.T) {
		svc, ctx, account, upstream := newCodexAdmissionForwardGuardFixture(t)
		body := []byte(`{"model":"gpt-5.1-codex","input":"test"}`)
		_, err := svc.ForwardAlphaSearch(ctx, newCodexAdmissionForwardGinContext(http.MethodPost, "/v1/alpha/search", body), account, body)
		require.ErrorIs(t, err, ErrCodexClientRestricted)
		require.Nil(t, upstream.lastReq)
	})

	t.Run("embeddings", func(t *testing.T) {
		svc, ctx, account, upstream := newCodexAdmissionForwardGuardFixture(t)
		body := []byte(`{"model":"text-embedding-3-small","input":"test"}`)
		_, err := svc.ForwardEmbeddings(ctx, newCodexAdmissionForwardGinContext(http.MethodPost, "/v1/embeddings", body), account, body, "")
		require.ErrorIs(t, err, ErrCodexClientRestricted)
		require.Nil(t, upstream.lastReq)
	})

	t.Run("images", func(t *testing.T) {
		svc, ctx, account, upstream := newCodexAdmissionForwardGuardFixture(t)
		body := []byte(`{"model":"gpt-image-1","prompt":"test"}`)
		_, err := svc.ForwardImages(
			ctx,
			newCodexAdmissionForwardGinContext(http.MethodPost, "/v1/images/generations", body),
			account,
			body,
			&OpenAIImagesRequest{Model: "gpt-image-1", Prompt: "test"},
			"",
		)
		require.ErrorIs(t, err, ErrCodexClientRestricted)
		require.Nil(t, upstream.lastReq)
	})

	t.Run("codex models", func(t *testing.T) {
		svc, ctx, account, upstream := newCodexAdmissionForwardGuardFixture(t)
		_, err := svc.FetchCodexModelsManifest(ctx, account, "0.145.0", "")
		require.ErrorIs(t, err, ErrCodexClientRestricted)
		require.Nil(t, upstream.lastReq)
	})

	t.Run("live create", func(t *testing.T) {
		svc, ctx, account, upstream := newCodexAdmissionForwardGuardFixture(t)
		_, err := svc.createUpstreamLiveCall(ctx, account, &LiveCallRequest{
			SDP:     "v=0\r\n",
			Session: json.RawMessage(`{"model":"gpt-live"}`),
		}, "attestation")
		require.ErrorIs(t, err, ErrCodexClientRestricted)
		require.Nil(t, upstream.lastReq)
	})
}

func TestCodexClientAdmissionFinalGuardAllowsUnchangedAccount(t *testing.T) {
	account := codexAdmissionAccount(8902, false)
	cache := &codexAdmissionSnapshotCache{account: &account}
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{account.ID: &account}}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	svc := &OpenAIGatewayService{schedulerSnapshot: snapshot, accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	latest, err := svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, &account)
	require.NoError(t, err)
	require.NotNil(t, latest)
	require.False(t, errors.Is(err, ErrCodexClientRestricted))
}

func TestOpenAITerminalAdmissionKeepsDBSelectionWhenTimestampTies(t *testing.T) {
	selected := codexAdmissionAccount(8903, true)
	selected.UpdatedAt = time.Now().UTC()
	cached := codexAdmissionAccount(selected.ID, false)
	cached.UpdatedAt = selected.UpdatedAt
	cache := &codexAdmissionSnapshotCache{account: &cached}
	snapshot := NewSchedulerSnapshotService(cache, nil, &codexAdmissionAccountRepo{}, nil, nil)
	svc := &OpenAIGatewayService{schedulerSnapshot: snapshot, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	result := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.Same(t, &selected, result.Account, "相同时间戳的缓存对象不得覆盖刚完成 DB 复检的选择对象")
	require.True(t, result.ClientVetoed)
}
