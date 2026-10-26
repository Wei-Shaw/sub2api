//go:build unit

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakeCodexCapabilityProber 记录触发点调用，不发起任何上游请求。
type fakeCodexCapabilityProber struct {
	mu          sync.Mutex
	probed      []int64
	async       []int64
	asyncReason []string
	recorded    []int64
	versions    []string
	bodies      [][]byte
}

func (f *fakeCodexCapabilityProber) Probe(_ context.Context, account *Account) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.probed = append(f.probed, account.ID)
	return nil
}

func (f *fakeCodexCapabilityProber) ProbeAsync(accountID int64, reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.async = append(f.async, accountID)
	f.asyncReason = append(f.asyncReason, reason)
}

func (f *fakeCodexCapabilityProber) RecordFromManifest(_ context.Context, account *Account, clientVersion string, body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = append(f.recorded, account.ID)
	f.versions = append(f.versions, clientVersion)
	f.bodies = append(f.bodies, body)
}

func TestCreateAccountTriggersCodexCapabilityProbeForOAuthLike(t *testing.T) {
	for _, accountType := range []string{AccountTypeOAuth, AccountTypeSetupToken} {
		t.Run(accountType, func(t *testing.T) {
			repo := &upstreamBillingProbeAccountRepo{}
			prober := &fakeCodexCapabilityProber{}
			svc := &adminServiceImpl{accountRepo: repo, codexCapabilityProber: prober}
			created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
				Name: "oauth", Platform: PlatformOpenAI, Type: accountType,
				Credentials: map[string]any{"access_token": "tok"}, SkipDefaultGroupBind: true,
			})
			require.NoError(t, err)
			require.Equal(t, []int64{created.ID}, prober.async, "创建响应返回时探测已异步触发，不阻塞创建")
			require.Equal(t, []string{codexCapabilityProbeReasonAccountCreated}, prober.asyncReason)
			require.Empty(t, prober.probed)
		})
	}
}

func TestCreateAccountDoesNotProbeAPIKeyOrOtherPlatforms(t *testing.T) {
	for name, input := range map[string]*CreateAccountInput{
		"openai apikey":   {Name: "k", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "sk"}, SkipDefaultGroupBind: true},
		"anthropic oauth": {Name: "a", Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "tok"}, SkipDefaultGroupBind: true},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &upstreamBillingProbeAccountRepo{}
			prober := &fakeCodexCapabilityProber{}
			svc := &adminServiceImpl{accountRepo: repo, codexCapabilityProber: prober}
			_, err := svc.CreateAccount(context.Background(), input)
			require.NoError(t, err)
			require.Empty(t, prober.async)
		})
	}
}

func TestUpdateAccountProbesOnlyWhenCredentialsChange(t *testing.T) {
	newRepo := func() *upstreamBillingProbeAccountRepo {
		return &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			31: {ID: 31, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"access_token": "old", "refresh_token": "r1"}},
		}}
	}
	t.Run("name only does not probe", func(t *testing.T) {
		repo := newRepo()
		prober := &fakeCodexCapabilityProber{}
		svc := &adminServiceImpl{accountRepo: repo, codexCapabilityProber: prober}
		_, err := svc.UpdateAccount(context.Background(), 31, &UpdateAccountInput{Name: "renamed"})
		require.NoError(t, err)
		require.Empty(t, prober.async)
	})
	t.Run("re-auth credentials probe", func(t *testing.T) {
		repo := newRepo()
		prober := &fakeCodexCapabilityProber{}
		svc := &adminServiceImpl{accountRepo: repo, codexCapabilityProber: prober}
		_, err := svc.UpdateAccount(context.Background(), 31, &UpdateAccountInput{
			Type:        AccountTypeOAuth,
			Credentials: map[string]any{"access_token": "new", "refresh_token": "r2"},
		})
		require.NoError(t, err)
		require.Equal(t, []int64{31}, prober.async)
		require.Equal(t, []string{codexCapabilityProbeReasonCredentialsUpdated}, prober.asyncReason)
	})
	t.Run("identical credentials do not probe", func(t *testing.T) {
		repo := newRepo()
		prober := &fakeCodexCapabilityProber{}
		svc := &adminServiceImpl{accountRepo: repo, codexCapabilityProber: prober}
		_, err := svc.UpdateAccount(context.Background(), 31, &UpdateAccountInput{
			Credentials: map[string]any{"access_token": "old", "refresh_token": "r1"},
		})
		require.NoError(t, err)
		require.Empty(t, prober.async)
	})
	t.Run("nil prober is skipped", func(t *testing.T) {
		repo := newRepo()
		svc := &adminServiceImpl{accountRepo: repo}
		_, err := svc.UpdateAccount(context.Background(), 31, &UpdateAccountInput{
			Credentials: map[string]any{"access_token": "new"},
		})
		require.NoError(t, err)
	})
}

// 作用域管理员 API 把创建和凭据更新包在事务里。ProbeAsync 用独立上下文读账号，
// 必须等提交成功后再启动，回滚或提交失败都不能探测。
func TestCodexCapabilityProbeWaitsForAccountCommit(t *testing.T) {
	newTx := func(t *testing.T) (*dbent.Tx, sqlmock.Sqlmock, func()) {
		t.Helper()
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
		mock.ExpectBegin()
		tx, err := client.Tx(context.Background())
		require.NoError(t, err)
		return tx, mock, func() { _ = db.Close() }
	}
	createInput := &CreateAccountInput{
		Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "tok"}, SkipDefaultGroupBind: true,
	}

	t.Run("create probes after commit", func(t *testing.T) {
		tx, mock, cleanup := newTx(t)
		defer cleanup()
		repo := &upstreamBillingProbeAccountRepo{}
		prober := &fakeCodexCapabilityProber{}
		svc := &adminServiceImpl{accountRepo: repo, codexCapabilityProber: prober}
		created, err := svc.CreateAccount(dbent.NewTxContext(context.Background(), tx), createInput)
		require.NoError(t, err)
		require.Empty(t, prober.async)
		mock.ExpectCommit()
		require.NoError(t, tx.Commit())
		require.Equal(t, []int64{created.ID}, prober.async)
		require.Equal(t, []string{codexCapabilityProbeReasonAccountCreated}, prober.asyncReason)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("create skips probe when commit fails", func(t *testing.T) {
		tx, mock, cleanup := newTx(t)
		defer cleanup()
		prober := &fakeCodexCapabilityProber{}
		svc := &adminServiceImpl{accountRepo: &upstreamBillingProbeAccountRepo{}, codexCapabilityProber: prober}
		_, err := svc.CreateAccount(dbent.NewTxContext(context.Background(), tx), createInput)
		require.NoError(t, err)
		mock.ExpectCommit().WillReturnError(errors.New("提交失败"))
		require.Error(t, tx.Commit())
		require.Empty(t, prober.async)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("create skips probe on rollback", func(t *testing.T) {
		tx, mock, cleanup := newTx(t)
		defer cleanup()
		prober := &fakeCodexCapabilityProber{}
		svc := &adminServiceImpl{accountRepo: &upstreamBillingProbeAccountRepo{}, codexCapabilityProber: prober}
		_, err := svc.CreateAccount(dbent.NewTxContext(context.Background(), tx), createInput)
		require.NoError(t, err)
		mock.ExpectRollback()
		require.NoError(t, tx.Rollback())
		require.Empty(t, prober.async)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("credential update probes after commit", func(t *testing.T) {
		tx, mock, cleanup := newTx(t)
		defer cleanup()
		repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
			31: {ID: 31, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"access_token": "old"}},
		}}
		prober := &fakeCodexCapabilityProber{}
		svc := &adminServiceImpl{accountRepo: repo, codexCapabilityProber: prober}
		_, err := svc.UpdateAccount(dbent.NewTxContext(context.Background(), tx), 31, &UpdateAccountInput{
			Credentials: map[string]any{"access_token": "new"},
		})
		require.NoError(t, err)
		require.Empty(t, prober.async)
		mock.ExpectCommit()
		require.NoError(t, tx.Commit())
		require.Equal(t, []int64{31}, prober.async)
		require.Equal(t, []string{codexCapabilityProbeReasonCredentialsUpdated}, prober.asyncReason)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTokenRefreshPostRefreshActionsProbesCodexCapabilitiesOnce(t *testing.T) {
	repo := &tokenRefreshAccountRepo{}
	cfg := &config.Config{TokenRefresh: config.TokenRefreshConfig{MaxRetries: 1}}
	svc := NewTokenRefreshService(repo, nil, nil, nil, nil, nil, nil, cfg, nil)
	prober := &fakeCodexCapabilityProber{}
	svc.SetCodexCapabilityProber(prober)

	oauth := &Account{ID: 41, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{"access_token": "tok"}}
	svc.postRefreshActions(context.Background(), oauth)
	require.Equal(t, []int64{41}, prober.probed)

	gemini := &Account{ID: 42, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusActive}
	svc.postRefreshActions(context.Background(), gemini)
	require.Equal(t, []int64{41}, prober.probed, "非 OpenAI OAuth-like 账号不探测")
}

func TestAccountTestOpenAIConnectionProbesAfterSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()
	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n"))
	prober := &fakeCodexCapabilityProber{}
	svc := &AccountTestService{accountRepo: &openAIAccountTestRepo{}, httpUpstream: &queuedHTTPUpstream{responses: []*http.Response{resp}}}
	svc.SetCodexCapabilityProber(prober)
	account := &Account{ID: 89, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "test-token"}}

	require.NoError(t, svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", ""))
	require.Equal(t, []int64{89}, prober.probed)
}

func TestAccountTestOpenAIConnectionDoesNotProbeAfterFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := newTestContext()
	resp := newJSONResponse(http.StatusOK, "")
	resp.Body = io.NopCloser(strings.NewReader("data: {\"type\":\"response.created\"}\n\n"))
	prober := &fakeCodexCapabilityProber{}
	svc := &AccountTestService{accountRepo: &openAIAccountTestRepo{}, httpUpstream: &queuedHTTPUpstream{responses: []*http.Response{resp}}}
	svc.SetCodexCapabilityProber(prober)
	account := &Account{ID: 90, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "test-token"}}

	require.Error(t, svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "", ""))
	require.Empty(t, prober.probed)
}

func TestSyncUpstreamModelCatalogRecordsCodexCapabilitiesWithoutExtraRequest(t *testing.T) {
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(codexCapabilitiesCompleteManifest)),
	}}
	prober := &fakeCodexCapabilityProber{}
	svc := &AccountTestService{accountRepo: &upstreamModelMetadataRepoStub{}, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	svc.SetCodexCapabilityProber(prober)
	account := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "openai-oauth-token"}}

	_, err := svc.SyncUpstreamModelCatalog(context.Background(), account)
	require.NoError(t, err)
	require.Len(t, upstream.requests, 1, "同步只发生一次上游请求")
	require.Equal(t, []int64{12}, prober.recorded)
	require.Equal(t, []string{CodexCanonicalClientVersion()}, prober.versions)
	require.JSONEq(t, codexCapabilitiesCompleteManifest, string(prober.bodies[0]))
}

// 带完整元数据的目录，避免同步路径为补齐能力元数据再去请求 models.dev 注册表。
const codexCapabilitiesCompleteManifest = `{"models":[{
  "slug":"gpt-6-sol","reasoning":true,"supported_reasoning_levels":[{"effort":"high"}],
  "input_modalities":["text","image"],"context_window":272000,
  "service_tiers":[{"id":"priority","name":"Fast"}],
  "available_access_programs":{"cyber":["standard","daybreak_blue"]}
}]}`
