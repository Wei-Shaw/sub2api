package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// ErrInnerAPIAppTokenNotConfigured 表示 inner_api_rpc.encryption_key 未配置/非法。
var ErrInnerAPIAppTokenNotConfigured = errors.New("inner api rpc encryption key not configured")

// errInnerAPITokenInvalid token 解密/解析失败（伪造、篡改、密钥不符）。
var errInnerAPITokenInvalid = errors.New("invalid inner api app token")

// InnerAPITokenCodec 用本地 32 字节密钥对接入方 token 做 AES-256-GCM 加解密。
// token = base64(nonce + ciphertext + tag)，明文为 {app_id}。
// 鉴权即「解密成功」：GCM 认证标签保证只有持密钥方能造出可干净解密的密文。
type InnerAPITokenCodec struct {
	key    []byte
	keyErr error // 密钥缺失/非法时记录，Mint/Parse 时返回
}

type innerAPIAppTokenPayload struct {
	AppID string `json:"aid"`
	Ver   int    `json:"ver"` // token 版本，与 inner_api_apps.token_version 比对；刷新后旧版本失效
	V     int    `json:"v"`   // payload schema 版本
}

// NewInnerAPITokenCodec 从 hex 密钥构造 codec。空/非法密钥不导致启动失败（返回一个
// 在 Mint/Parse 时报 ErrInnerAPIAppTokenNotConfigured 的 codec），便于未启用内部 API RPC 的
// 部署无需配置密钥。
func NewInnerAPITokenCodec(cfg *config.Config) *InnerAPITokenCodec {
	raw := strings.TrimSpace(cfg.InnerAPIRPC.EncryptionKey)
	if raw == "" {
		return &InnerAPITokenCodec{keyErr: ErrInnerAPIAppTokenNotConfigured}
	}
	key, err := hex.DecodeString(raw)
	if err != nil {
		return &InnerAPITokenCodec{keyErr: fmt.Errorf("%w: %v", ErrInnerAPIAppTokenNotConfigured, err)}
	}
	if len(key) != 32 {
		return &InnerAPITokenCodec{keyErr: fmt.Errorf("%w: must be 32 bytes (64 hex), got %d", ErrInnerAPIAppTokenNotConfigured, len(key))}
	}
	return &InnerAPITokenCodec{key: key}
}

// Configured 返回密钥是否已正确配置。
func (c *InnerAPITokenCodec) Configured() bool {
	return c != nil && c.keyErr == nil
}

// Mint 为 (app_id, tokenVersion) 生成一个 token（密文）。
func (c *InnerAPITokenCodec) Mint(appID string, tokenVersion int) (string, error) {
	if c.keyErr != nil {
		return "", c.keyErr
	}
	plaintext, err := json.Marshal(innerAPIAppTokenPayload{AppID: appID, Ver: tokenVersion, V: 1})
	if err != nil {
		return "", err
	}
	gcm, err := c.gcm()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Parse 解密 token 并返回其 app_id 与 token 版本。解密/解析失败返回 errInnerAPITokenInvalid。
func (c *InnerAPITokenCodec) Parse(token string) (appID string, version int, err error) {
	if c.keyErr != nil {
		return "", 0, c.keyErr
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil {
		return "", 0, errInnerAPITokenInvalid
	}
	gcm, err := c.gcm()
	if err != nil {
		return "", 0, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return "", 0, errInnerAPITokenInvalid
	}
	nonce, ciphertext := data[:ns], data[ns:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		// GCM 标签校验失败 → 伪造/篡改/密钥不符。
		return "", 0, errInnerAPITokenInvalid
	}
	var payload innerAPIAppTokenPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil || payload.AppID == "" {
		return "", 0, errInnerAPITokenInvalid
	}
	return payload.AppID, payload.Ver, nil
}

func (c *InnerAPITokenCodec) gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
