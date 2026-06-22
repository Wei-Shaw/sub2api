package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// ─── 测试基础设施 ────────────────────────────────────────────────────────────

func newOidcSigningTestClient(t *testing.T) *dbent.Client {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", name)

	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// signingMemSettingRepo 实现 SettingRepository 接口的内存版（仅供本测试文件使用）。
type signingMemSettingRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func newSigningMemSettingRepo() *signingMemSettingRepo {
	return &signingMemSettingRepo{values: map[string]string{}}
}

func (r *signingMemSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: v}, nil
}

func (r *signingMemSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}

func (r *signingMemSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *signingMemSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := r.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

func (r *signingMemSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range settings {
		r.values[k] = v
	}
	return nil
}

func (r *signingMemSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func (r *signingMemSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.values[key]; !ok {
		return ErrSettingNotFound
	}
	delete(r.values, key)
	return nil
}

// 一个 1024-bit 的随机 RSA 密钥（仅测试用，签验过得了）—— 加快单测速度。
func newTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	return priv
}

// 建立一个用 1024-bit 测试密钥的 service 实例（避免 2048-bit 跑得慢）。
func newTestSigningService(t *testing.T, client *dbent.Client) (*OidcSigningService, *signingMemSettingRepo) {
	t.Helper()
	repo := newSigningMemSettingRepo()
	svc := NewOidcSigningService(client, repo)
	svc.rsaBits = 1024
	svc.generateKeyFunc = func() (*rsa.PrivateKey, error) {
		return newTestRSAKey(t), nil
	}
	return svc, repo
}

// ─── 用例 ───────────────────────────────────────────────────────────────────

func TestOidcSigning_EnsureActiveKey_GeneratesOnFirstCall(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, repo := newTestSigningService(t, client)

	require.NoError(t, svc.EnsureActiveKey(context.Background()))

	kid := svc.ActiveKid()
	require.NotEmpty(t, kid)

	// security_secrets 表里有一行 oidc_provider.signing_key.<kid>
	row, err := client.SecuritySecret.Query().
		Where(securitysecret.KeyEQ(SecuritySecretPrefixOidcSigningKey + kid)).
		Only(context.Background())
	require.NoError(t, err)
	require.Contains(t, row.Value, "RSA PRIVATE KEY")

	// active kid setting 已写入
	v, ok := repo.values[SettingKeyOidcSigningKeyActiveKid]
	require.True(t, ok)
	require.Equal(t, kid, v)
}

func TestOidcSigning_EnsureActiveKey_Idempotent(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)

	require.NoError(t, svc.EnsureActiveKey(context.Background()))
	first := svc.ActiveKid()

	// 重新构造一个 service 模拟"重启"
	repo2 := newSigningMemSettingRepo()
	// 把 active kid 从持久层"恢复"
	require.NoError(t, repo2.Set(context.Background(), SettingKeyOidcSigningKeyActiveKid, first))
	svc2 := NewOidcSigningService(client, repo2)
	svc2.rsaBits = 1024
	require.NoError(t, svc2.EnsureActiveKey(context.Background()))

	require.Equal(t, first, svc2.ActiveKid(), "active kid must persist across restarts")

	// 表里仍然只有 1 行私钥
	count, err := client.SecuritySecret.Query().
		Where(securitysecret.KeyHasPrefix(SecuritySecretPrefixOidcSigningKey)).
		Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestOidcSigning_SignIDToken_AndVerifyWithJWKS(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)
	require.NoError(t, svc.EnsureActiveKey(context.Background()))

	claims := jwt.MapClaims{
		"iss": "https://test.example",
		"sub": "42",
		"aud": "rp_abc",
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	signed, err := svc.SignIDToken(claims)
	require.NoError(t, err)
	require.NotEmpty(t, signed)

	// JWKS 里至少有一个 RSA RS256 公钥
	jwks := svc.JWKS()
	require.Len(t, jwks, 1)
	require.Equal(t, "RSA", jwks[0]["kty"])
	require.Equal(t, "RS256", jwks[0]["alg"])
	require.Equal(t, "sig", jwks[0]["use"])
	require.Equal(t, svc.ActiveKid(), jwks[0]["kid"])

	// 直接用导出的公钥验签
	pub := svc.VerificationKey(svc.ActiveKid())
	require.NotNil(t, pub)
	parsed, err := jwt.Parse(signed, func(tok *jwt.Token) (any, error) {
		require.Equal(t, "RS256", tok.Method.Alg())
		require.Equal(t, svc.ActiveKid(), tok.Header["kid"])
		return pub, nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
}

func TestOidcSigning_JWKS_NeverLeaksPrivateMaterial(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)
	require.NoError(t, svc.EnsureActiveKey(context.Background()))

	jwks := svc.JWKS()
	for _, k := range jwks {
		for _, forbidden := range []string{"d", "p", "q", "dp", "dq", "qi", "PRIVATE"} {
			_, found := k[forbidden]
			require.False(t, found, "JWKS must not contain %q", forbidden)
		}
	}
}

func TestOidcSigning_RotateKey_AddsNewActiveAndKeepsOld(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)
	require.NoError(t, svc.EnsureActiveKey(context.Background()))
	old := svc.ActiveKid()

	// 让 now() 步进 1 秒，避免 kid 时间戳冲突
	base := time.Now().UTC()
	step := 0
	svc.now = func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Second)
	}

	newKid, err := svc.RotateKey(context.Background())
	require.NoError(t, err)
	require.NotEqual(t, old, newKid)
	require.Equal(t, newKid, svc.ActiveKid())

	jwks := svc.JWKS()
	require.Len(t, jwks, 2, "old kid must still appear within grace window")
	kidsInJWKS := map[string]bool{}
	for _, k := range jwks {
		kid, ok := k["kid"].(string)
		require.True(t, ok)
		kidsInJWKS[kid] = true
	}
	require.True(t, kidsInJWKS[old])
	require.True(t, kidsInJWKS[newKid])
}

func TestOidcSigning_RotateKey_OldKidStillVerifiesOldTokens(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)
	require.NoError(t, svc.EnsureActiveKey(context.Background()))

	claims := jwt.MapClaims{"sub": "1", "exp": time.Now().Add(time.Hour).Unix()}
	oldToken, err := svc.SignIDToken(claims)
	require.NoError(t, err)
	oldKid := svc.ActiveKid()

	base := time.Now().UTC()
	step := 0
	svc.now = func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Second)
	}
	_, err = svc.RotateKey(context.Background())
	require.NoError(t, err)

	// 用旧 kid 的公钥仍能验旧 token
	pub := svc.VerificationKey(oldKid)
	require.NotNil(t, pub)
	parsed, err := jwt.Parse(oldToken, func(tok *jwt.Token) (any, error) {
		return pub, nil
	})
	require.NoError(t, err)
	require.True(t, parsed.Valid)
}

func TestOidcSigning_RotateKey_OldKidExpiresAfterGrace(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)
	require.NoError(t, svc.EnsureActiveKey(context.Background()))

	base := time.Now().UTC()
	step := 0
	svc.now = func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Second)
	}

	old := svc.ActiveKid()
	_, err := svc.RotateKey(context.Background())
	require.NoError(t, err)

	// 让 now() 跳到宽限期之后
	svc.now = func() time.Time {
		return base.Add(time.Duration(OidcSigningKeyRetireGraceSeconds+10) * time.Second)
	}

	jwks := svc.JWKS()
	for _, k := range jwks {
		require.NotEqual(t, old, k["kid"], "old kid must be filtered out after grace window")
	}
	require.Nil(t, svc.VerificationKey(old))
}

func TestOidcSigning_DeleteKey_RefusesActive(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)
	require.NoError(t, svc.EnsureActiveKey(context.Background()))

	err := svc.DeleteKey(context.Background(), svc.ActiveKid())
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrOidcSigningActiveKeyDeletion))
}

func TestOidcSigning_DeleteKey_RemovesNonActive(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)
	require.NoError(t, svc.EnsureActiveKey(context.Background()))

	base := time.Now().UTC()
	step := 0
	svc.now = func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Second)
	}

	old := svc.ActiveKid()
	_, err := svc.RotateKey(context.Background())
	require.NoError(t, err)

	require.NoError(t, svc.DeleteKey(context.Background(), old))

	// security_secrets 中已无 old 的私钥行
	count, err := client.SecuritySecret.Query().
		Where(securitysecret.KeyEQ(SecuritySecretPrefixOidcSigningKey + old)).
		Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, count)

	// JWKS 中已无该 kid
	for _, k := range svc.JWKS() {
		require.NotEqual(t, old, k["kid"])
	}
}

func TestOidcSigning_ListKeys_ReportsActiveAndRetired(t *testing.T) {
	client := newOidcSigningTestClient(t)
	svc, _ := newTestSigningService(t, client)
	require.NoError(t, svc.EnsureActiveKey(context.Background()))

	base := time.Now().UTC()
	step := 0
	svc.now = func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Second)
	}

	old := svc.ActiveKid()
	newKid, err := svc.RotateKey(context.Background())
	require.NoError(t, err)

	infos := svc.ListKeys()
	require.Len(t, infos, 2)
	byKid := map[string]OidcSigningKeyInfo{}
	for _, i := range infos {
		byKid[i.Kid] = i
	}
	require.True(t, byKid[newKid].IsActive)
	require.False(t, byKid[newKid].Removable)
	require.False(t, byKid[old].IsActive)
	require.True(t, byKid[old].Removable)
	require.NotNil(t, byKid[old].RetiredAt)
}

func TestOidcSigning_BigIntExponentBytes_StandardE65537(t *testing.T) {
	got := bigIntExponentBytes(65537)
	require.Equal(t, []byte{0x01, 0x00, 0x01}, got)
}
