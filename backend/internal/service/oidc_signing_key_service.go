// Package service ...
//
// oidc_signing_key_service.go 实现 OIDC 签名密钥的生成、管理和 JWT 签名功能。
// 负责 ID Token 和 UserInfo 响应的 JWT 签名，以及 JWKS 端点的密钥集提供。
package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/oidc_signature_key"
	"github.com/dgrijalva/jwt-go"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var (
	ErrOIDCSigningKeyNotFound = infraerrors.Internal("OIDC_SIGNING_KEY_NOT_FOUND", "No active signing key found")
	ErrOIDCInvalidKeyFormat  = infraerrors.Internal("OIDC_INVALID_KEY_FORMAT", "Invalid key format")
)

// OIDCSigningKeyService 处理 OIDC 签名密钥的生成和管理
type OIDCSigningKeyService struct {
	entClient       *dbent.Client
	auditLogService *AuditLogService
}

// NewOIDCSigningKeyService 创建 OIDC Signing Key Service 实例
func NewOIDCSigningKeyService(entClient *dbent.Client, auditLogService *AuditLogService) *OIDCSigningKeyService {
	return &OIDCSigningKeyService{
		entClient:       entClient,
		auditLogService: auditLogService,
	}
}

// JWK 表示 JSON Web Key
type JWK struct {
	KTY string `json:"kty"`
	USE string `json:"use"`
	KID string `json:"kid"`
	ALG string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS 表示 JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// SignIDTokenParams ID Token 签名参数
type SignIDTokenParams struct {
	Issuer     string
	Subject    string
	Audience   string
	ExpiresAt  time.Time
	IssuedAt   time.Time
	Nonce      string
	AuthTime   *time.Time
	ACR        string
	AMR        []string
	Claims     map[string]interface{}
}

// GenerateSigningKey 生成新的 RSA 签名密钥
func (s *OIDCSigningKeyService) GenerateSigningKey(ctx context.Context) (string, error) {
	// 生成 RSA 密钥对
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// 将私钥编码为 PEM 格式
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// 生成密钥 ID
	kidBytes := make([]byte, 16)
	if _, err := rand.Read(kidBytes); err != nil {
		return "", fmt.Errorf("failed to generate KID: %w", err)
	}
	kid := base64.RawURLEncoding.EncodeToString(kidBytes)

	// 保存到数据库
	_, err = s.entClient.OidcSignatureKey.Create().
		SetKid(kid).
		SetPrivateKey(string(privateKeyPEM)).
		SetIsActive(true).
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to save signing key: %w", err)
	}

	// 记录审计日志
	s.auditLogService.WriteOidcSigningKeyAuditLog(ctx, kid, AuditActionOidcSigningKeyGenerated, "admin", map[string]any{
		"key_size": 2048,
		"algorithm": "RS256",
	})

	return kid, nil
}

// GetActiveSigningKey 获取当前活跃的签名密钥
func (s *OIDCSigningKeyService) GetActiveSigningKey(ctx context.Context) (*rsa.PrivateKey, string, error) {
	key, err := s.entClient.OidcSignatureKey.Query().
		Where(oidc_signature_key.IsActive(true)).
		Order(dbent.Desc(oidc_signature_key.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		return nil, "", ErrOIDCSigningKeyNotFound
	}

	// 解析 PEM 格式的私钥
	block, _ := pem.Decode([]byte(key.PrivateKey))
	if block == nil {
		return nil, "", ErrOIDCInvalidKeyFormat
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse private key: %w", err)
	}

	return privateKey, key.Kid, nil
}

// SignIDToken 签名 ID Token
func (s *OIDCSigningKeyService) SignIDToken(ctx context.Context, params SignIDTokenParams) (string, error) {
	privateKey, kid, err := s.GetActiveSigningKey(ctx)
	if err != nil {
		return "", err
	}

	// 构建 JWT claims
	claims := jwt.MapClaims{
		"iss": params.Issuer,
		"sub": params.Subject,
		"aud": params.Audience,
		"exp": params.ExpiresAt.Unix(),
		"iat": params.IssuedAt.Unix(),
		"kid": kid,
	}

	// 添加可选声明
	if params.Nonce != "" {
		claims["nonce"] = params.Nonce
	}
	if params.AuthTime != nil {
		claims["auth_time"] = params.AuthTime.Unix()
	}
	if params.ACR != "" {
		claims["acr"] = params.ACR
	}
	if len(params.AMR) > 0 {
		claims["amr"] = params.AMR
	}

	// 添加自定义声明
	for k, v := range params.Claims {
		claims[k] = v
	}

	// 创建并签名 JWT
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	signedToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign ID token: %w", err)
	}

	return signedToken, nil
}

// JWKS 返回 JWK Set
func (s *OIDCSigningKeyService) JWKS(ctx context.Context) (*JWKS, error) {
	// 获取所有活跃和已退休但仍在宽限期内的密钥
	keys, err := s.entClient.OidcSignatureKey.Query().
		Where(
			oidc_signature_key.Or(
				oidc_signature_key.IsActive(true),
				// 添加退休密钥的宽限期条件
				oidc_signature_key.RetiredAtGT(time.Now().Add(-7*24*time.Hour)),
			),
		).
		Order(dbent.Desc(oidc_signature_key.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query signing keys: %w", err)
	}

	jwks := &JWKS{Keys: make([]JWK, 0, len(keys))}

	for _, key := range keys {
		// 解析私钥以获取公钥信息
		block, _ := pem.Decode([]byte(key.PrivateKey))
		if block == nil {
			continue
		}

		privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			continue
		}

		publicKey := privateKey.Public().(*rsa.PublicKey)

		jwk := JWK{
			KTY: "RSA",
			USE: "sig",
			KID: key.Kid,
			ALG: "RS256",
			N:   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString([]byte{byte(publicKey.E >> 24), byte(publicKey.E >> 16), byte(publicKey.E >> 8), byte(publicKey.E)}),
		}

		jwks.Keys = append(jwks.Keys, jwk)
	}

	return jwks, nil
}

// RotateSigningKey 轮换签名密钥
func (s *OIDCSigningKeyService) RotateSigningKey(ctx context.Context) (string, error) {
	// 标记当前活跃密钥为退休
	activeKeys, err := s.entClient.OidcSignatureKey.Query().
		Where(oidc_signature_key.IsActive(true)).
		All(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to query active keys: %w", err)
	}

	for _, key := range activeKeys {
		_, err = s.entClient.OidcSignatureKey.UpdateOne(key).
			SetIsActive(false).
			SetRetiredAt(time.Now().UTC()).
			Save(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to retire key: %w", err)
		}
	}

	// 生成新密钥
	kid, err := s.GenerateSigningKey(ctx)
	if err != nil {
		return "", err
	}

	// 记录审计日志
	s.auditLogService.WriteOidcSigningKeyAuditLog(ctx, kid, AuditActionOidcSigningKeyRotated, "admin", map[string]any{
		"retired_keys_count": len(activeKeys),
	})

	return kid, nil
}

// ListSigningKeys 列出所有签名密钥
func (s *OIDCSigningKeyService) ListSigningKeys(ctx context.Context) ([]*dbent.OidcSignatureKey, error) {
	keys, err := s.entClient.OidcSignatureKey.Query().
		Order(dbent.Desc(oidc_signature_key.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list signing keys: %w", err)
	}
	return keys, nil
}

// DeleteSigningKey 删除指定的签名密钥
func (s *OIDCSigningKeyService) DeleteSigningKey(ctx context.Context, kid string) error {
	// 检查是否为活跃密钥
	activeKey, err := s.entClient.OidcSignatureKey.Query().
		Where(oidc_signature_key.IsActive(true)).
		First(ctx)
	if err == nil && activeKey.Kid == kid {
		return fmt.Errorf("cannot delete active signing key")
	}

	err = s.entClient.OidcSignatureKey.
		Delete().
		Where(oidc_signature_key.Kid(kid)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete signing key: %w", err)
	}

	// 记录审计日志
	s.auditLogService.WriteOidcSigningKeyAuditLog(ctx, kid, AuditActionOidcSigningKeyDeleted, "admin", map[string]any{
		"was_active": false,
	})

	return nil
}