// Package service ...
//
// oidc_signing_service.go 实现 OIDC Provider 的 RSA 签名密钥生命周期：
//   - 启动时从 security_secrets 表加载所有历史密钥到内存
//   - 首次启动时自动生成 RSA-2048 + 写表 + 设置 active kid
//   - 提供 SignIDToken / SignAccessToken (本期不签 access token，access token 是 opaque)
//   - 提供 JWKS 投影 (仅 modulus / exponent，永不暴露私钥)
//   - 提供 RotateKey / DeleteKey 由 admin 调用
//
// 私钥仅以 PKCS#1 PEM 编码持久化于 security_secrets 表，key 前缀
// "oidc_provider.signing_key.<kid>"。kid 用 UTC 时间戳 "20060102T150405Z" 自解释。
package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// SettingKeyOidcSigningKeyActiveKid 当前用于签名的 kid。
	SettingKeyOidcSigningKeyActiveKid = "oidc_provider.signing_key_active_kid"
	// SecuritySecretPrefixOidcSigningKey 私钥行 key 前缀；后接 kid。
	SecuritySecretPrefixOidcSigningKey = "oidc_provider.signing_key."
	// SettingPrefixOidcSigningKeyRetired 旧 kid 的退役 unix 秒时间戳前缀；后接 kid。
	SettingPrefixOidcSigningKeyRetired = "oidc_provider.signing_key.retired."
	// OidcSigningKeyRetireGraceSeconds 退役 kid 仍出现在 JWKS 中的宽限期 (默认 7 天)。
	OidcSigningKeyRetireGraceSeconds = 7 * 24 * 60 * 60
	// OidcSigningKeyRSABits RSA 密钥长度。
	OidcSigningKeyRSABits = 2048
)

// ErrOidcSigningKeyNotFound 内部哨兵错误 (e.g. DeleteKey 操作目标不存在时)。
var ErrOidcSigningKeyNotFound = errors.New("oidc signing key not found")

// ErrOidcSigningActiveKeyDeletion 试图删除当前 active kid 时返回。
var ErrOidcSigningActiveKeyDeletion = errors.New("active oidc signing key cannot be deleted")

// OidcSigningKeyInfo 给 admin 列表使用的 kid 元数据。
type OidcSigningKeyInfo struct {
	Kid       string
	IsActive  bool
	CreatedAt time.Time
	RetiredAt *time.Time // nil 表示尚未退役
	Removable bool       // 仅当 !IsActive 时为 true
}

// OidcSigningService RSA 签名密钥生命周期管理。
type OidcSigningService struct {
	client      *ent.Client
	settingRepo SettingRepository

	mu           sync.RWMutex
	keys         map[string]*rsa.PrivateKey // kid → 私钥 (含派生公钥)
	createdAt    map[string]time.Time       // kid → 加载/生成时间
	retiredAt    map[string]time.Time       // kid → 退役时间 (从 setting 反序列化)
	activeKid    string
	graceSeconds int64

	// 测试与 mocking 注入点
	now             func() time.Time
	rsaBits         int
	generateKeyFunc func() (*rsa.PrivateKey, error)
}

// NewOidcSigningService 构造服务。
//
// 必须在 EnsureActiveKey 调用之前完成构造。client 和 settingRepo 都必须非 nil。
func NewOidcSigningService(client *ent.Client, settingRepo SettingRepository) *OidcSigningService {
	s := &OidcSigningService{
		client:       client,
		settingRepo:  settingRepo,
		keys:         make(map[string]*rsa.PrivateKey),
		createdAt:    make(map[string]time.Time),
		retiredAt:    make(map[string]time.Time),
		graceSeconds: OidcSigningKeyRetireGraceSeconds,
		now:          func() time.Time { return time.Now().UTC() },
		rsaBits:      OidcSigningKeyRSABits,
	}
	s.generateKeyFunc = s.defaultGenerateKey
	return s
}

func (s *OidcSigningService) defaultGenerateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, s.rsaBits)
}

// EnsureActiveKey 启动时调用：
//
//   - 加载所有 oidc_provider.signing_key.* 行到内存 keys map
//   - 如 active kid 未设置或对应行不存在，则生成新 RSA-2048 并写表 + 写 active kid
//   - 加载所有退役时间戳 setting
func (s *OidcSigningService) EnsureActiveKey(ctx context.Context) error {
	if s.client == nil || s.settingRepo == nil {
		return fmt.Errorf("oidc signing service: missing dependencies")
	}

	// 1) 加载所有私钥行
	rows, err := s.client.SecuritySecret.Query().
		Where(securitysecret.KeyHasPrefix(SecuritySecretPrefixOidcSigningKey)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("oidc signing: load private keys: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, row := range rows {
		kid := strings.TrimPrefix(row.Key, SecuritySecretPrefixOidcSigningKey)
		if kid == "" {
			continue
		}
		priv, err := decodePKCS1PrivateKeyPEM(row.Value)
		if err != nil {
			return fmt.Errorf("oidc signing: decode key %q: %w", kid, err)
		}
		s.keys[kid] = priv
		s.createdAt[kid] = row.CreatedAt
	}

	// 2) 加载所有 retire 时间戳
	if err := s.loadRetiredTimestampsLocked(ctx); err != nil {
		return err
	}

	// 3) 解析 active kid
	activeKid, err := s.settingRepo.GetValue(ctx, SettingKeyOidcSigningKeyActiveKid)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("oidc signing: read active kid setting: %w", err)
	}
	activeKid = strings.TrimSpace(activeKid)

	if activeKid != "" {
		if _, ok := s.keys[activeKid]; ok {
			s.activeKid = activeKid
			return nil
		}
		// active kid 设置存在但行不存在 — 数据一致性破坏，重生成
	}

	// 4) 生成新 active key
	kid, priv, err := s.generateAndPersistLocked(ctx)
	if err != nil {
		return err
	}
	s.activeKid = kid
	s.keys[kid] = priv
	s.createdAt[kid] = s.now()

	if err := s.settingRepo.Set(ctx, SettingKeyOidcSigningKeyActiveKid, kid); err != nil {
		return fmt.Errorf("oidc signing: persist active kid: %w", err)
	}
	return nil
}

func (s *OidcSigningService) loadRetiredTimestampsLocked(ctx context.Context) error {
	all, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("oidc signing: load retired timestamps: %w", err)
	}
	for k, v := range all {
		if !strings.HasPrefix(k, SettingPrefixOidcSigningKeyRetired) {
			continue
		}
		kid := strings.TrimPrefix(k, SettingPrefixOidcSigningKeyRetired)
		secs, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil || secs <= 0 {
			continue
		}
		s.retiredAt[kid] = time.Unix(secs, 0).UTC()
	}
	return nil
}

// generateAndPersistLocked 必须在 mu 锁内调用。
func (s *OidcSigningService) generateAndPersistLocked(ctx context.Context) (string, *rsa.PrivateKey, error) {
	priv, err := s.generateKeyFunc()
	if err != nil {
		return "", nil, fmt.Errorf("oidc signing: generate rsa key: %w", err)
	}
	kid := s.now().Format("20060102T150405Z")
	// 极低概率冲突 (同秒生成两次) — 为保证唯一性追加随机后缀
	if _, exists := s.keys[kid]; exists {
		var b [4]byte
		if _, err := rand.Read(b[:]); err == nil {
			kid = kid + "-" + base64.RawURLEncoding.EncodeToString(b[:])
		} else {
			kid = kid + "-" + strconv.FormatInt(s.now().UnixNano(), 36)
		}
	}
	pemStr, err := encodePKCS1PrivateKeyPEM(priv)
	if err != nil {
		return "", nil, err
	}
	if _, err := s.client.SecuritySecret.Create().
		SetKey(SecuritySecretPrefixOidcSigningKey + kid).
		SetValue(pemStr).
		Save(ctx); err != nil {
		return "", nil, fmt.Errorf("oidc signing: persist key %q: %w", kid, err)
	}
	return kid, priv, nil
}

// SignIDToken 用当前 active kid 签 RS256 JWT。
//
// 调用方负责把所有需要的 claim (iss/sub/aud/exp/iat/nonce/...) 填好后传入。
func (s *OidcSigningService) SignIDToken(claims jwt.MapClaims) (string, error) {
	s.mu.RLock()
	kid := s.activeKid
	priv, ok := s.keys[kid]
	s.mu.RUnlock()

	if !ok || priv == nil {
		return "", fmt.Errorf("oidc signing: no active key loaded")
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		return "", fmt.Errorf("oidc signing: sign token: %w", err)
	}
	return signed, nil
}

// VerificationKey 返回某 kid 的公钥；若 kid 不存在或已过宽限期则返回 nil。
//
// 用于服务端解析自家发出的 JWT (e.g. 集成测试)。
func (s *OidcSigningService) VerificationKey(kid string) *rsa.PublicKey {
	s.mu.RLock()
	defer s.mu.RUnlock()
	priv, ok := s.keys[kid]
	if !ok || priv == nil {
		return nil
	}
	if !s.isKidVisibleLocked(kid) {
		return nil
	}
	return &priv.PublicKey
}

// ActiveKid 返回当前用于签名的 kid。
func (s *OidcSigningService) ActiveKid() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeKid
}

// JWKS 投影所有当前可见的公钥 (active + 仍在宽限期内的 retired) 为 RFC 7517 JWK。
//
// 返回 map 数组直接 JSON marshal 即得到 jwks_uri 的响应体 keys 字段值。
func (s *OidcSigningService) JWKS() []map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]map[string]any, 0, len(s.keys))
	for kid, priv := range s.keys {
		if priv == nil {
			continue
		}
		if !s.isKidVisibleLocked(kid) {
			continue
		}
		pub := priv.PublicKey
		out = append(out, map[string]any{
			"kty": "RSA",
			"kid": kid,
			"use": "sig",
			"alg": "RS256",
			"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(bigIntExponentBytes(pub.E)),
		})
	}
	return out
}

// isKidVisibleLocked 必须在 mu 读锁或写锁内调用。
//
// 当前 active kid 永远可见；其他 kid 仅在退役 ≤ 宽限期时可见 (尚未设置退役也视为可见)。
func (s *OidcSigningService) isKidVisibleLocked(kid string) bool {
	if kid == s.activeKid {
		return true
	}
	retired, ok := s.retiredAt[kid]
	if !ok {
		// 未记录退役的非 active key，按"刚加载尚未滚动"处理：可见
		return true
	}
	cutoff := s.now().Add(-time.Duration(s.graceSeconds) * time.Second)
	return retired.After(cutoff)
}

// RotateKey 生成新 RSA-2048，原 active kid 标记为退役 (now)，新 kid 设为 active。
//
// 注意：本实现**不**包在单一 DB transaction 里。原因是 ent.Client 跨 securitysecret 表
// 与 setting repo 的事务边界跨包不天然对齐；rotate 是 admin 极低频操作，失败时 admin
// 可手动核对状态。极端情况下可能出现"私钥行已写但 active kid setting 未切换" — 重启会
// 自动以现有 active kid 继续工作，新生成的孤儿 kid 留在 keys map 里但不被使用，admin
// 可调 DeleteKey 清理。
func (s *OidcSigningService) RotateKey(ctx context.Context) (string, error) {
	s.mu.Lock()
	prev := s.activeKid

	kid, priv, err := s.generateAndPersistLocked(ctx)
	if err != nil {
		s.mu.Unlock()
		return "", err
	}
	s.keys[kid] = priv
	s.createdAt[kid] = s.now()
	s.activeKid = kid

	// 记录上一代退役时间
	if prev != "" {
		s.retiredAt[prev] = s.now()
	}
	s.mu.Unlock()

	if err := s.settingRepo.Set(ctx, SettingKeyOidcSigningKeyActiveKid, kid); err != nil {
		return "", fmt.Errorf("oidc signing: persist new active kid: %w", err)
	}
	if prev != "" {
		retiredSecs := strconv.FormatInt(s.retiredAt[prev].Unix(), 10)
		if err := s.settingRepo.Set(ctx, SettingPrefixOidcSigningKeyRetired+prev, retiredSecs); err != nil {
			return "", fmt.Errorf("oidc signing: persist retired timestamp: %w", err)
		}
	}
	return kid, nil
}

// DeleteKey 删除指定 kid (必须不是 active)。同步删除 retire 时间戳 setting。
func (s *OidcSigningService) DeleteKey(ctx context.Context, kid string) error {
	kid = strings.TrimSpace(kid)
	if kid == "" {
		return ErrOidcSigningKeyNotFound
	}

	s.mu.Lock()
	if kid == s.activeKid {
		s.mu.Unlock()
		return ErrOidcSigningActiveKeyDeletion
	}
	if _, ok := s.keys[kid]; !ok {
		s.mu.Unlock()
		return ErrOidcSigningKeyNotFound
	}
	s.mu.Unlock()

	// 删 DB 行
	_, err := s.client.SecuritySecret.Delete().
		Where(securitysecret.KeyEQ(SecuritySecretPrefixOidcSigningKey + kid)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("oidc signing: delete private key row: %w", err)
	}
	if err := s.settingRepo.Delete(ctx, SettingPrefixOidcSigningKeyRetired+kid); err != nil &&
		!errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("oidc signing: delete retired timestamp: %w", err)
	}

	s.mu.Lock()
	delete(s.keys, kid)
	delete(s.createdAt, kid)
	delete(s.retiredAt, kid)
	s.mu.Unlock()
	return nil
}

// ListKeys 返回所有 kid 的元数据 (供 admin GET /signing-keys)。
func (s *OidcSigningService) ListKeys() []OidcSigningKeyInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]OidcSigningKeyInfo, 0, len(s.keys))
	for kid := range s.keys {
		info := OidcSigningKeyInfo{
			Kid:       kid,
			IsActive:  kid == s.activeKid,
			CreatedAt: s.createdAt[kid],
			Removable: kid != s.activeKid,
		}
		if r, ok := s.retiredAt[kid]; ok {
			rr := r
			info.RetiredAt = &rr
		}
		out = append(out, info)
	}
	return out
}

// ─── PEM 工具函数 ────────────────────────────────────────────────────────────

func encodePKCS1PrivateKeyPEM(priv *rsa.PrivateKey) (string, error) {
	if priv == nil {
		return "", fmt.Errorf("nil private key")
	}
	der := x509.MarshalPKCS1PrivateKey(priv)
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block)), nil
}

func decodePKCS1PrivateKeyPEM(text string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(text))
	if block == nil {
		return nil, fmt.Errorf("pem decode: empty block")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// 兼容历史上以 PKCS#8 写入的可能
		if k2, e2 := x509.ParsePKCS8PrivateKey(block.Bytes); e2 == nil {
			if rsaKey, ok := k2.(*rsa.PrivateKey); ok {
				return rsaKey, nil
			}
		}
		return nil, fmt.Errorf("pem parse pkcs1: %w", err)
	}
	return priv, nil
}

// bigIntExponentBytes 把 RSA 的 int E 编码为最短的 big-endian 字节。
//
// 标准 RSA 公钥 E 大多为 65537 (0x010001) 即 3 字节。
func bigIntExponentBytes(e int) []byte {
	if e <= 0 {
		return []byte{0}
	}
	buf := make([]byte, 0, 4)
	for e > 0 {
		buf = append([]byte{byte(e & 0xff)}, buf...)
		e >>= 8
	}
	return buf
}
