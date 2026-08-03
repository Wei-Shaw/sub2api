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
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{selected.ID: &selected}}
	upstream := &httpUpstreamRecorder{}
	svc := &OpenAIGatewayService{
		accountRepo:   repo,
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
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
		require.Nil(t, upstream.lastReq)
	})

	t.Run("embeddings", func(t *testing.T) {
		svc, ctx, account, upstream := newCodexAdmissionForwardGuardFixture(t)
		body := []byte(`{"model":"text-embedding-3-small","input":"test"}`)
		_, err := svc.ForwardEmbeddings(ctx, newCodexAdmissionForwardGinContext(http.MethodPost, "/v1/embeddings", body), account, body, "")
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
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
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
		require.Nil(t, upstream.lastReq)
	})

	t.Run("codex models", func(t *testing.T) {
		svc, ctx, account, upstream := newCodexAdmissionForwardGuardFixture(t)
		_, err := svc.FetchCodexModelsManifest(ctx, account, "0.145.0", "")
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
		require.Nil(t, upstream.lastReq)
	})

	t.Run("live create", func(t *testing.T) {
		svc, ctx, account, upstream := newCodexAdmissionForwardGuardFixture(t)
		_, err := svc.createUpstreamLiveCall(ctx, account, &LiveCallRequest{
			SDP:     "v=0\r\n",
			Session: json.RawMessage(`{"model":"gpt-live"}`),
		}, "attestation")
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
		require.Nil(t, upstream.lastReq)
	})
}

func TestCodexClientAdmissionFinalGuardWithoutGrantIsUnavailable(t *testing.T) {
	account := codexAdmissionAccount(8902, false)
	cache := &codexAdmissionSnapshotCache{account: &account}
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{account.ID: &account}}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	svc := &OpenAIGatewayService{schedulerSnapshot: snapshot, accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	latest, err := svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, &account)
	require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
	require.NotNil(t, latest)
	require.False(t, errors.Is(err, ErrCodexClientRestricted))
	require.Zero(t, repo.getCalls.Load(), "发送前保护缺 grant 时不得再次访问数据库")
}

func TestCodexClientAdmissionFinalGuardReusesTerminalAdmission(t *testing.T) {
	selected := codexAdmissionAccount(8905, false)
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{selected.ID: &selected}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	admission, admissionErr := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, admissionErr)
	require.False(t, admission.ClientVetoed)
	require.Equal(t, int64(1), repo.getCalls.Load())

	latest, err := svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, admission.Account)
	require.NoError(t, err)
	require.Equal(t, admission.Account.ID, latest.ID)
	require.Equal(t, int64(1), repo.getCalls.Load(), "发送前防线应复用本请求已完成的权威终检")
}

func TestCodexClientAdmissionProofDoesNotApplyToDifferentObjectWithSameID(t *testing.T) {
	selected := codexAdmissionAccount(8906, false)
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{selected.ID: &selected}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	admission, admissionErr := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, admissionErr)
	require.Equal(t, int64(1), repo.getCalls.Load())

	restricted := codexAdmissionAccount(selected.ID, true)
	repo.byID[selected.ID] = &restricted
	copyWithSameID := *admission.Account
	latest, err := svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, &copyWithSameID)
	require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
	require.False(t, errors.Is(err, ErrCodexClientRestricted))
	require.Equal(t, copyWithSameID.ID, latest.ID)
	require.Equal(t, int64(1), repo.getCalls.Load(), "不匹配 grant 时不得在发送前重新查询数据库")
}

func TestCodexClientAdmissionWSRegrantsStableBoundObject(t *testing.T) {
	bound := codexAdmissionAccount(8914, false)
	authoritative := bound
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{bound.ID: &authoritative}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	admission, err := svc.OpenAITerminalAdmissionLatest(ctx, &bound)
	require.NoError(t, err)
	require.NotSame(t, &bound, admission.Account)
	require.True(t, svc.GrantOpenAIWSTerminalAdmission(ctx, &bound, admission.Account))

	latest, err := svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, &bound)
	require.NoError(t, err)
	require.Same(t, &bound, latest)
	require.Equal(t, int64(1), repo.getCalls.Load(), "WS 绑定对象获得 grant 后发送前不得再次访问数据库")
}

func TestCodexClientAdmissionWSRejectsBindingIdentityChange(t *testing.T) {
	oldParentID := int64(8915)
	newParentID := int64(8916)
	bound := codexAdmissionAccount(8917, false)
	bound.ParentAccountID = &oldParentID
	authoritative := bound
	authoritative.ParentAccountID = &newParentID
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{
		bound.ID:    &authoritative,
		newParentID: func() *Account { parent := codexAdmissionAccount(newParentID, false); return &parent }(),
	}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	admission, err := svc.OpenAITerminalAdmissionLatest(ctx, &bound)
	require.NoError(t, err)
	require.False(t, svc.GrantOpenAIWSTerminalAdmission(ctx, &bound, admission.Account))

	_, err = svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, &bound)
	require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
}

func TestCodexClientAdmissionNewTerminalAttemptClearsOldGrant(t *testing.T) {
	first := codexAdmissionAccount(8907, false)
	second := codexAdmissionAccount(8908, false)
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{first.ID: &first, second.ID: &second}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	firstAdmission, err := svc.OpenAITerminalAdmissionLatest(ctx, &first)
	require.NoError(t, err)
	repo.getErrByID = map[int64]error{second.ID: errors.New("database unavailable")}
	_, err = svc.OpenAITerminalAdmissionLatest(ctx, &second)
	require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)

	_, err = svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, firstAdmission.Account)
	require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable, "新的终检开始后旧账号 grant 必须立即失效")
}

func TestCodexClientAdmissionEveryTerminalAttemptClearsOldGrant(t *testing.T) {
	first := codexAdmissionAccount(8912, false)
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{first.ID: &first}}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	t.Run("nil selection", func(t *testing.T) {
		admission, err := svc.OpenAITerminalAdmissionLatest(ctx, &first)
		require.NoError(t, err)
		_, err = svc.OpenAITerminalAdmissionLatest(ctx, nil)
		require.NoError(t, err)

		_, err = svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, admission.Account)
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
	})

	t.Run("non applicable account", func(t *testing.T) {
		admission, err := svc.OpenAITerminalAdmissionLatest(ctx, &first)
		require.NoError(t, err)
		apiKey := &Account{ID: 8913, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		_, err = svc.OpenAITerminalAdmissionLatest(ctx, apiKey)
		require.NoError(t, err)

		_, err = svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, admission.Account)
		require.ErrorIs(t, err, ErrCodexClientAdmissionUnavailable)
	})
}

func TestCodexClientAdmissionInactiveForwardSkipsDB(t *testing.T) {
	parentID := int64(8909)
	shadow := codexAdmissionAccount(8910, false)
	shadow.ParentAccountID = &parentID
	repo := &codexAdmissionAccountRepo{getErr: errors.New("must not be called")}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: alwaysMatchedCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)
	require.False(t, codexClientAdmissionActive(ctx))
	c := newCodexAdmissionForwardGinContext(http.MethodPost, "/v1/responses", nil)

	err := svc.enforceOpenAICodexClientAdmissionForForward(ctx, c, &shadow, nil)
	require.NoError(t, err)
	require.Zero(t, repo.getCalls.Load())
}

func TestCodexClientAdmissionActiveForwardSkipsGrantForAPIKeyAccount(t *testing.T) {
	repo := &codexAdmissionAccountRepo{getErr: errors.New("must not query repository")}
	svc := &OpenAIGatewayService{accountRepo: repo, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)
	account := Account{ID: 8911, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	latest, err := svc.enforceOpenAICodexClientAdmissionBeforeUpstream(ctx, &account)
	require.NoError(t, err)
	require.Same(t, &account, latest)
	require.Zero(t, repo.getCalls.Load())
}

func TestOpenAITerminalAdmissionKeepsDBSelectionWhenTimestampTies(t *testing.T) {
	selected := codexAdmissionAccount(8903, true)
	selected.UpdatedAt = time.Now().UTC()
	cached := codexAdmissionAccount(selected.ID, false)
	cached.UpdatedAt = selected.UpdatedAt
	cache := &codexAdmissionSnapshotCache{account: &cached}
	repo := &codexAdmissionAccountRepo{byID: map[int64]*Account{selected.ID: &selected}}
	snapshot := NewSchedulerSnapshotService(cache, nil, repo, nil, nil)
	svc := &OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: snapshot, codexDetector: &accountAwareCodexAdmissionDetector{}}
	ctx := newCodexAdmissionContext(t, svc)

	result, err := svc.OpenAITerminalAdmissionLatest(ctx, &selected)
	require.NoError(t, err)
	require.NotSame(t, &cached, result.Account, "相同时间戳的陈旧缓存不得覆盖数据库权威对象")
	require.True(t, result.Account.IsCodexCLIOnlyEnabled())
	require.Zero(t, cache.calls.Load())
	require.True(t, result.ClientVetoed)
}
