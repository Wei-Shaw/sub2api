package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oidcaccesstoken"
	"github.com/Wei-Shaw/sub2api/ent/oidcauthorizationcode"
	"github.com/Wei-Shaw/sub2api/ent/oidcconsent"
	"github.com/Wei-Shaw/sub2api/ent/oidcrefreshtoken"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// ─── helper：bcrypt 测试加速 ────────────────────────────────────────────────

func newOidcClientServiceForTest(t *testing.T, client *ent.Client) *OidcClientService {
	t.Helper()
	svc := NewOidcClientService(client)
	// MinCost = 4，让 hash 在测试里跑得快
	svc.bcryptCost = bcrypt.MinCost
	return svc
}

// ─── Create ──────────────────────────────────────────────────────────────────

func TestOidcClient_Create_HappyPath(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	view, plain, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:      "Acme RP",
		RedirectURIs:    []string{"https://acme.example.com/callback"},
		AllowedScopes:   []string{"openid", "profile"},
		ConsentRequired: true,
		Enabled:         true,
	})
	require.NoError(t, err)
	require.NotNil(t, view)
	require.True(t, strings.HasPrefix(view.ClientID, OidcClientIDPrefix))
	require.GreaterOrEqual(t, len(view.ClientID), len(OidcClientIDPrefix)+20)
	require.NotEmpty(t, plain, "plaintext secret must be returned once")
	require.Equal(t, []string{"authorization_code", "refresh_token"}, view.GrantTypes)

	// secret 不被持久化 view (View 不暴露 hash)
	row, err := client.OidcClient.Get(context.Background(), view.ID)
	require.NoError(t, err)
	require.NotEmpty(t, row.ClientSecretHash)
	require.NoError(t, bcrypt.CompareHashAndPassword([]byte(row.ClientSecretHash), []byte(plain)))
}

func TestOidcClient_Create_RejectsEmptyName(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	_, _, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:   "  ",
		RedirectURIs: []string{"https://x.example.com/cb"},
	})
	require.True(t, errors.Is(err, ErrOidcClientNameRequired))
}

func TestOidcClient_Create_RejectsHTTPNonLocalhostRedirect(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	_, _, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:   "Acme",
		RedirectURIs: []string{"http://acme.example.com/cb"},
	})
	require.True(t, errors.Is(err, ErrOidcClientInvalidRedirectURI), "got %v", err)
}

func TestOidcClient_Create_AllowsHTTPLocalhost(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	_, _, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:   "Local Dev",
		RedirectURIs: []string{"http://localhost:3000/cb", "http://127.0.0.1:8080/cb"},
	})
	require.NoError(t, err)
}

func TestOidcClient_Create_RejectsFragmentInRedirect(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	_, _, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:   "Acme",
		RedirectURIs: []string{"https://acme.example.com/cb#frag"},
	})
	require.True(t, errors.Is(err, ErrOidcClientInvalidRedirectURI))
}

func TestOidcClient_Create_RejectsDisallowedScope(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	_, _, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:    "Acme",
		RedirectURIs:  []string{"https://x.example.com/cb"},
		AllowedScopes: []string{"openid", "internal:admin"},
	})
	require.True(t, errors.Is(err, ErrOidcClientInvalidScope), "got %v", err)
}

func TestOidcClient_Create_NoRedirectURIs_Fails(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	_, _, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:    "Acme",
		AllowedScopes: []string{"openid"},
	})
	require.True(t, errors.Is(err, ErrOidcClientRedirectURIsRequired))
}

// ─── Authenticate ────────────────────────────────────────────────────────────

func TestOidcClient_Authenticate_HappyAndWrongSecret(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	view, plain, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:    "Acme",
		RedirectURIs:  []string{"https://acme.example.com/cb"},
		AllowedScopes: []string{"openid"},
		Enabled:       true,
	})
	require.NoError(t, err)

	got, err := svc.Authenticate(context.Background(), view.ClientID, plain)
	require.NoError(t, err)
	require.Equal(t, view.ID, got.ID)

	_, err = svc.Authenticate(context.Background(), view.ClientID, "wrong")
	require.True(t, errors.Is(err, ErrOidcClientWrongSecret))
}

func TestOidcClient_Authenticate_UnknownClientReturnsNotFound(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	_, err := svc.Authenticate(context.Background(), "rp_doesnotexist", "irrelevant")
	require.True(t, errors.Is(err, ErrOidcClientNotFound))
}

func TestOidcClient_Authenticate_DisabledClient(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	view, plain, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:    "Acme",
		RedirectURIs:  []string{"https://acme.example.com/cb"},
		AllowedScopes: []string{"openid"},
		Enabled:       false, // 关闭
	})
	require.NoError(t, err)

	_, err = svc.Authenticate(context.Background(), view.ClientID, plain)
	require.True(t, errors.Is(err, ErrOidcClientDisabled))
}

// ─── ResetSecret ─────────────────────────────────────────────────────────────

func TestOidcClient_ResetSecret_InvalidatesOldSecret(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	view, oldPlain, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:    "Acme",
		RedirectURIs:  []string{"https://acme.example.com/cb"},
		AllowedScopes: []string{"openid"},
		Enabled:       true,
	})
	require.NoError(t, err)

	newPlain, err := svc.ResetSecret(context.Background(), view.ID)
	require.NoError(t, err)
	require.NotEqual(t, oldPlain, newPlain)

	// 旧 secret 失效
	_, err = svc.Authenticate(context.Background(), view.ClientID, oldPlain)
	require.True(t, errors.Is(err, ErrOidcClientWrongSecret))

	// 新 secret 通过
	_, err = svc.Authenticate(context.Background(), view.ClientID, newPlain)
	require.NoError(t, err)
}

func TestOidcClient_ResetSecret_UnknownIDReturnsNotFound(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	_, err := svc.ResetSecret(context.Background(), 999)
	require.True(t, errors.Is(err, ErrOidcClientNotFound))
}

// ─── Update ──────────────────────────────────────────────────────────────────

func TestOidcClient_Update_PartialPatch(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	view, _, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:    "Acme",
		RedirectURIs:  []string{"https://acme.example.com/cb"},
		AllowedScopes: []string{"openid"},
		Enabled:       false,
	})
	require.NoError(t, err)

	newName := "Acme V2"
	enabled := true
	updated, err := svc.Update(context.Background(), view.ID, UpdateOidcClientPatch{
		ClientName: &newName,
		Enabled:    &enabled,
	})
	require.NoError(t, err)
	require.Equal(t, "Acme V2", updated.ClientName)
	require.True(t, updated.Enabled)
	// 未触及的字段保持不变
	require.Equal(t, []string{"https://acme.example.com/cb"}, updated.RedirectURIs)
	require.Equal(t, []string{"openid"}, updated.AllowedScopes)
}

func TestOidcClient_Update_RejectsBadScope(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	view, _, err := svc.Create(context.Background(), CreateOidcClientRequest{
		ClientName:    "Acme",
		RedirectURIs:  []string{"https://acme.example.com/cb"},
		AllowedScopes: []string{"openid"},
		Enabled:       true,
	})
	require.NoError(t, err)

	bad := []string{"openid", "evil"}
	_, err = svc.Update(context.Background(), view.ID, UpdateOidcClientPatch{
		AllowedScopes: &bad,
	})
	require.True(t, errors.Is(err, ErrOidcClientInvalidScope))
}

// ─── Delete cascade ──────────────────────────────────────────────────────────

func TestOidcClient_Delete_CascadesDependentRows(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)
	ctx := context.Background()

	// 必须先建 user (consent / authorization_code 等都引用 user_id；FK on 部分表)
	_, err := client.User.Create().
		SetEmail("c@x.com").SetUsername("c").SetPasswordHash("x").SetRole("user").
		Save(ctx)
	require.NoError(t, err)
	userID := int64(1)

	view, _, err := svc.Create(ctx, CreateOidcClientRequest{
		ClientName:    "Acme",
		RedirectURIs:  []string{"https://acme.example.com/cb"},
		AllowedScopes: []string{"openid"},
		Enabled:       true,
	})
	require.NoError(t, err)
	clientID := view.ClientID

	// 注入若干依赖行 (consent / authorization_code / refresh_token / access_token)
	_, err = client.OidcConsent.Create().
		SetUserID(userID).SetClientID(clientID).
		SetGrantedScopes([]string{"openid"}).
		SetGrantedAt(time.Now().UTC()).
		SetLastUsedAt(time.Now().UTC()).
		Save(ctx)
	require.NoError(t, err)

	now := time.Now().UTC()
	_, err = client.OidcAuthorizationCode.Create().
		SetCode("code-1").SetClientID(clientID).SetUserID(userID).
		SetRedirectURI("https://acme.example.com/cb").SetScopes([]string{"openid"}).
		SetCodeChallenge("ccc").SetCodeChallengeMethod("S256").
		SetExpiresAt(now.Add(60 * time.Second)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.OidcRefreshToken.Create().
		SetToken("rt-1").SetFamilyID("fam-1").SetClientID(clientID).SetUserID(userID).
		SetScopes([]string{"openid"}).SetExpiresAt(now.Add(60 * time.Second)).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.OidcAccessToken.Create().
		SetToken("at-1").SetClientID(clientID).SetUserID(userID).
		SetScopes([]string{"openid"}).SetExpiresAt(now.Add(60 * time.Second)).
		Save(ctx)
	require.NoError(t, err)

	// 执行 cascade delete
	require.NoError(t, svc.Delete(ctx, view.ID))

	// 全部依赖行清空
	cnt, err := client.OidcConsent.Query().Where(oidcconsent.ClientIDEQ(clientID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, cnt)

	cnt, err = client.OidcAuthorizationCode.Query().Where(oidcauthorizationcode.ClientIDEQ(clientID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, cnt)

	cnt, err = client.OidcRefreshToken.Query().Where(oidcrefreshtoken.ClientIDEQ(clientID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, cnt)

	cnt, err = client.OidcAccessToken.Query().Where(oidcaccesstoken.ClientIDEQ(clientID)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, cnt)

	// client 行本身也被删
	_, err = svc.Get(ctx, view.ID)
	require.True(t, errors.Is(err, ErrOidcClientNotFound))
}

func TestOidcClient_Delete_UnknownIDReturnsNotFound(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)

	err := svc.Delete(context.Background(), 999)
	require.True(t, errors.Is(err, ErrOidcClientNotFound))
}

// ─── List ────────────────────────────────────────────────────────────────────

func TestOidcClient_List_Filters(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc := newOidcClientServiceForTest(t, client)
	ctx := context.Background()

	for _, c := range []CreateOidcClientRequest{
		{ClientName: "Alpha", RedirectURIs: []string{"https://a.example.com/cb"}, AllowedScopes: []string{"openid"}, Enabled: true},
		{ClientName: "Beta", RedirectURIs: []string{"https://b.example.com/cb"}, AllowedScopes: []string{"openid"}, Enabled: false},
		{ClientName: "Alpaca", RedirectURIs: []string{"https://c.example.com/cb"}, AllowedScopes: []string{"openid"}, Enabled: true},
	} {
		_, _, err := svc.Create(ctx, c)
		require.NoError(t, err)
	}

	all, err := svc.List(ctx, OidcClientListFilters{})
	require.NoError(t, err)
	require.Len(t, all, 3)

	enabled, err := svc.List(ctx, OidcClientListFilters{OnlyEnabled: true})
	require.NoError(t, err)
	require.Len(t, enabled, 2)

	alpha, err := svc.List(ctx, OidcClientListFilters{NameLike: "alp"})
	require.NoError(t, err)
	require.Len(t, alpha, 2, "Alpha + Alpaca 命中")
}

// ─── ValidateOidcIssuerURL (来自 oidc_provider_settings.go) ──────────────────

func TestValidateOidcIssuerURL_AllRules(t *testing.T) {
	require.True(t, errors.Is(ValidateOidcIssuerURL(""), ErrOidcProviderIssuerURLEmpty))
	require.True(t, errors.Is(ValidateOidcIssuerURL("http://a.com"), ErrOidcProviderIssuerURLNotHTTPS))
	require.True(t, errors.Is(ValidateOidcIssuerURL("https://a.com/"), ErrOidcProviderIssuerURLTrailingSlash))
	require.True(t, errors.Is(ValidateOidcIssuerURL("https://a.com?x=1"), ErrOidcProviderIssuerURLContainsQueryOrFragment))
	require.True(t, errors.Is(ValidateOidcIssuerURL("https://a.com#frag"), ErrOidcProviderIssuerURLContainsQueryOrFragment))

	require.NoError(t, ValidateOidcIssuerURL("https://a.com"))
	require.NoError(t, ValidateOidcIssuerURL("https://api.sub2api.com"))
}

// ─── helper ──────────────────────────────────────────────────────────────────

func init() {
	// 无操作，仅确保引用本包 service 类型 (避免编译器抱怨)
	_ = (*OidcClientService)(nil)
}
