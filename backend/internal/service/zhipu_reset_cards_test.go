package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

const (
	zhipuTestZCode = "zcode-token-for-test"
	zhipuTestMaaS  = "maas-token-for-test"
	zhipuTestCards = `{"code":0,"data":{"available_five_hour_resets":[{"expire_at":1890000000000,"ignored":"private"}],"available_week_resets":[],"latest_five_hour_reset_history":null,"latest_week_reset_history":{"used_at":1780000000000}}}`
)

type zhipuTestRepo struct {
	AccountRepository
	account  *Account
	conflict bool
}

func (r *zhipuTestRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if id != r.account.ID {
		return nil, ErrAccountNotFound
	}
	return r.account, nil
}

func (r *zhipuTestRepo) CompareAndSwapZhipuResetCards(_ context.Context, _ *Account, next json.RawMessage) (bool, error) {
	if r.conflict {
		return false, nil
	}
	if len(next) == 0 {
		delete(r.account.Extra, ZhipuResetCardsExtraKey)
	} else {
		r.account.Extra[ZhipuResetCardsExtraKey] = next
	}
	return true, nil
}

type zhipuTestCipher struct{}

func (zhipuTestCipher) Encrypt(value string) (string, error) {
	return base64.StdEncoding.EncodeToString([]byte(value)), nil
}

func (zhipuTestCipher) Decrypt(value string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	return string(raw), err
}

type zhipuTestClient struct {
	HTTPUpstream
	t          *testing.T
	pollAuth   string
	status     string
	action     string
	actionCode int
	actionFail bool
	readFail   bool
	paths      []string
	writes     []map[string]string
}

func (c *zhipuTestClient) Do(req *http.Request, proxy string, id int64, limit int) (*http.Response, error) {
	t := c.t
	t.Helper()
	require.Equal(t, zhipuControlOrigin, req.URL.Scheme+"://"+req.URL.Host)
	require.Empty(t, proxy)
	require.Equal(t, int64(7), id)
	require.Equal(t, 1, limit)
	c.paths = append(c.paths, req.Method+" "+req.URL.Path)
	raw, httpStatus := zhipuTestCards, http.StatusOK
	switch req.URL.Path {
	case zhipuDevicePath + "/init":
		var data map[string]string
		require.NoError(t, json.NewDecoder(req.Body).Decode(&data))
		require.Equal(t, "bigmodel", data["provider"])
		c.pollAuth = req.Header.Get("Authorization")
		require.Regexp(t, `^Bearer [a-f0-9]{64}$`, c.pollAuth)
		init := zhipuDeviceInit{
			FlowID: "upstream-flow-fixture", PollInterval: 2, ServerTime: 1780000000, ExpiresAt: 1780000120,
			AuthorizeURL: "https://bigmodel.cn/login?redirect=https%3A%2F%2Fzcode.z.ai%2Fapi%2Fv1%2Foauth%2Fcli%2Fcallback%2Fbigmodel&state=state",
		}
		encoded, err := json.Marshal(init)
		require.NoError(t, err)
		raw = `{"code":0,"data":` + string(encoded) + `}`
	case zhipuDevicePath + "/poll/upstream-flow-fixture":
		require.Equal(t, c.pollAuth, req.Header.Get("Authorization"))
		raw = `{"code":0,"data":{"status":"pending"}}`
		if c.status == "ready" {
			raw = `{"code":0,"data":{"status":"ready","token":"` + zhipuTestZCode +
				`","user":{"user_id":"upstream-person"},"bigmodel":{"access_token":"` + zhipuTestMaaS + `"}}}`
		}
	case zhipuResetPath + "/status", zhipuResetPath + "/use", zhipuResetPath + "/opportunity":
		require.Equal(t, "Bearer "+zhipuTestZCode, req.Header.Get("Authorization"))
		require.Equal(t, zhipuTestMaaS, req.Header.Get("X-Bigmodel-Authorization"))
		require.Equal(t, "PERSONAL", req.Header.Get("Bigmodel-Target-Type"))
		require.Empty(t, req.Header.Get("Bigmodel-Organization"))
		if req.Method == http.MethodPost {
			var body map[string]string
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			require.Regexp(t, `^[a-f0-9]{64}$`, body["idempotency_key"])
			c.writes = append(c.writes, body)
			if c.actionFail {
				return nil, errors.New("transport failure " + zhipuTestMaaS)
			}
			raw = c.action
			if c.actionCode != 0 {
				httpStatus = c.actionCode
			}
		} else if c.readFail {
			return nil, errors.New("readback failed")
		}
	default:
		t.Fatalf("unexpected upstream path %s", req.URL.Path)
	}
	return &http.Response{StatusCode: httpStatus, Body: io.NopCloser(strings.NewReader(raw)), Request: req}, nil
}

func newZhipuTestService(t *testing.T) (*ZhipuResetCardsService, *zhipuTestRepo, *zhipuTestClient) {
	repo := &zhipuTestRepo{account: &Account{ID: 7, Platform: PlatformZhipu, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "inference key stays unchanged", "account_mode": AccountModeCoding},
		Extra:       map[string]any{},
	}}
	client := &zhipuTestClient{t: t, action: `{"code":0,"data":{"used":true}}`}
	cfg := &config.Config{}
	cfg.Totp.EncryptionKeyConfigured = true
	return NewZhipuResetCardsService(repo, client, zhipuTestCipher{}, cfg), repo, client
}

func initZhipuTestBinding(t *testing.T, svc *ZhipuResetCardsService, client *zhipuTestClient) *ZhipuResetState {
	t.Helper()
	ctx := context.Background()
	prompt, err := svc.Start(ctx, 7, 11)
	require.NoError(t, err)
	require.NotContains(t, prompt.AuthorizeURL, strings.TrimPrefix(client.pollAuth, "Bearer "))
	_, err = svc.Poll(ctx, 7, 99, prompt.AttemptID)
	require.ErrorIs(t, err, errZhipuDeviceExpired)
	svc.attempts[7].nextPoll = time.Time{}
	pending, err := svc.Poll(ctx, 7, 11, prompt.AttemptID)
	require.NoError(t, err)
	require.Equal(t, "pending", pending.Status)
	svc.attempts[7].nextPoll = time.Time{}
	client.status = "ready"
	polled, err := svc.Poll(ctx, 7, 11, prompt.AttemptID)
	require.NoError(t, err)
	require.Equal(t, "bound", polled.Status)
	require.Len(t, polled.State.Cards.FiveHour, 1)
	require.Empty(t, client.writes)
	raw, err := json.Marshal(polled)
	require.NoError(t, err)
	for _, secret := range []string{zhipuTestZCode, zhipuTestMaaS, "upstream-person", "private", "ciphertext"} {
		require.NotContains(t, string(raw), secret)
	}
	return polled.State
}

func TestZhipuResetCardsAuthorizationAndConfirmedUse(t *testing.T) {
	svc, repo, client := newZhipuTestService(t)
	ctx := context.Background()
	state := initZhipuTestBinding(t, svc, client)
	require.Equal(t, "inference key stays unchanged", repo.account.Credentials["api_key"])
	raw, ok := repo.account.Extra[ZhipuResetCardsExtraKey].(json.RawMessage)
	require.True(t, ok, "expected a serialized reset-card binding")
	require.NotContains(t, string(raw), zhipuTestMaaS)
	_, err := svc.Start(ctx, 7, 11)
	require.ErrorIs(t, err, errZhipuResetBound)

	key := "00000000-1000-4000-a000-100000000000"
	_, err = svc.Act(ctx, 7, "different-binding", "use", zhipuWeek, key)
	require.ErrorIs(t, err, errZhipuResetChanged)
	require.Empty(t, client.writes)
	got, err := svc.Act(ctx, 7, state.BindingID, "use", zhipuWeek, key)
	require.NoError(t, err)
	require.Equal(t, "used", got.Outcome)
	require.NotNil(t, got.Cards)
	require.Equal(t, zhipuWeek, client.writes[0]["reset_type"])
	require.Equal(t, "GET "+zhipuResetPath+"/status", client.paths[len(client.paths)-1])

	client.readFail = true
	got, err = svc.Act(ctx, 7, state.BindingID, "use", zhipuWeek, key)
	require.NoError(t, err)
	require.Equal(t, "used", got.Outcome)
	require.Nil(t, got.Cards)
	require.Equal(t, client.writes[0], client.writes[1])
	require.NoError(t, svc.Unbind(ctx, 7))
	require.NotContains(t, repo.account.Extra, ZhipuResetCardsExtraKey)
}

func TestZhipuResetCardsDeniedAndAmbiguousActions(t *testing.T) {
	svc, repo, client := newZhipuTestService(t)
	ctx := context.Background()
	state := initZhipuTestBinding(t, svc, client)
	key := "e0000000-1000-4000-a000-100000000000"
	client.action = `{"code":3301,"data":{"granted":false,"next_try_at":1890000000000}}`
	client.actionCode = http.StatusBadRequest
	res, err := svc.Act(ctx, 7, state.BindingID, "opportunity", "", key)
	require.NoError(t, err)
	require.Equal(t, "not_eligible", res.Outcome)
	require.EqualValues(t, 1890000000000, res.NextTryAt)
	client.actionFail = true
	_, err = svc.Act(ctx, 7, state.BindingID, "use", zhipuFiveHour, "another-idempotency-key")
	require.ErrorIs(t, err, errZhipuResetUnknown)
	require.NotContains(t, err.Error(), zhipuTestMaaS)
	require.Len(t, client.writes, 2)
	repo.account.Credentials["api_key"] = "new inference key"
	_, err = svc.Act(ctx, 7, state.BindingID, "use", zhipuFiveHour, key)
	require.ErrorIs(t, err, errZhipuResetAuth)
	require.Len(t, client.writes, 2)
}

func TestZhipuResetCardsRejectsTeamInternationalAndCallbackRedirects(t *testing.T) {
	svc, repo, client := newZhipuTestService(t)
	ctx := context.Background()
	repo.account.Credentials["base_url"] = "https://api.z.ai/api/coding/paas/v4"
	_, err := svc.Start(ctx, 7, 11)
	require.ErrorIs(t, err, errZhipuResetAccount)
	delete(repo.account.Credentials, "base_url")
	repo.account.Credentials["zhipu_organization"] = "team"
	_, err = svc.Start(ctx, 7, 11)
	require.ErrorIs(t, err, errZhipuResetAccount)
	require.Empty(t, client.paths)
	require.False(t, safeZhipuAuthorizeURL("https://bigmodel.cn/login?redirect=https://other.example/api/v1/oauth/cli/callback/bigmodel"))
	require.False(t, safeZhipuAuthorizeURL("https://bigmodel.cn.evil.example/login?redirect=https://zcode.z.ai/api/v1/oauth/cli/callback/bigmodel"))
}

func TestZhipuResetCardsAuthorizationDoesNotOverwriteOnConflict(t *testing.T) {
	svc, repo, client := newZhipuTestService(t)
	ctx := context.Background()
	prompt, err := svc.Start(ctx, 7, 11)
	require.NoError(t, err)
	svc.attempts[7].nextPoll = time.Time{}
	repo.conflict = true
	client.status = "ready"
	_, err = svc.Poll(ctx, 7, 11, prompt.AttemptID)
	require.ErrorIs(t, err, errZhipuResetChanged)
	require.Empty(t, repo.account.Extra)
	require.Empty(t, client.writes)
}
