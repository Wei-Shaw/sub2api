//go:build unit

package repository

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

// 本用例刻意放在 repository 包：depguard 禁止 internal/service/** 引入 internal/repository，
// 而「A6 令牌必须以密文落库」这条安全约束必须用**真实**的 AES-256-GCM 加密器验一次。
// service 包里的假加密器只能证明调用路径，证明不了算法可用、也证明不了密文可逆。
//
// 这里只桩掉 reconciliation_sync_state 的读写口（真实 SQL 行为由 integration 测试覆盖），
// 加密器用 NewAESEncryptor 造出来的真家伙，与被 wire 注入生产的那一个完全同源。

// a6SettingsStateStub 是 reconciliation_sync_state 的内存桩。
type a6SettingsStateStub struct {
	values map[string]string
}

func newA6SettingsStateStub() *a6SettingsStateStub {
	return &a6SettingsStateStub{values: map[string]string{}}
}

func (s *a6SettingsStateStub) Get(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}

func (s *a6SettingsStateStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *a6SettingsStateStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func a6SettingsStringPtr(value string) *string { return &value }

func a6SettingsFloatPtr(value float64) *float64 { return &value }

func a6SettingsServiceWithRealEncryptor(t *testing.T, encryptor service.SecretEncryptor) (*service.ReconciliationA6SettingsService, *a6SettingsStateStub) {
	t.Helper()
	state := newA6SettingsStateStub()
	svc := service.NewReconciliationA6SettingsService(state, encryptor, service.ReconciliationA6Config{
		BaseURL: "https://a6.example.com",
		UserID:  "123",
		Timeout: time.Minute,
	}, 7.2)
	return svc, state
}

// TestReconciliationA6SettingsRealEncryptorPersistsCiphertext 断言令牌经真实
// AES-256-GCM 加密后落库，并且能原文读回。
func TestReconciliationA6SettingsRealEncryptorPersistsCiphertext(t *testing.T) {
	ctx := context.Background()
	encryptor := aesEncryptor(t)
	svc, state := a6SettingsServiceWithRealEncryptor(t, encryptor)

	token := "a6-production-token-9f8e7d6c5b4a"
	settings, err := svc.Update(ctx, service.ReconciliationA6SettingsInput{AccessToken: &token})
	require.NoError(t, err)

	stored := state.values[service.ReconciliationStateKeyA6AccessTokenOverride]
	require.NotEmpty(t, stored, "令牌覆盖必须落库")
	require.NotEqual(t, token, stored, "落库的必须是密文，不能是明文")
	require.NotContains(t, stored, token, "密文里不能夹带明文")

	// 密文形态：base64(nonce(12 字节) + 密文 + GCM tag(16 字节))，必然长于明文。
	raw, err := base64.StdEncoding.DecodeString(stored)
	require.NoError(t, err, "落库值必须是合法 base64（AES-GCM 输出形态）")
	require.Greater(t, len(raw), len(token), "含 nonce 与 tag 的密文必须长于明文")

	// 解密读回：保存返回值与重新解析的生效值都必须是原文。
	require.Equal(t, token, settings.AccessToken)
	require.Equal(t, token, svc.Effective(ctx).AccessToken)
	require.Equal(t, "a6-p…5b4a", svc.Effective(ctx).View().A6TokenMask)

	// 随机 nonce：同一明文再存一次得到不同密文，但都能解回同一个令牌。
	_, err = svc.Update(ctx, service.ReconciliationA6SettingsInput{AccessToken: &token})
	require.NoError(t, err)
	require.NotEqual(t, stored, state.values[service.ReconciliationStateKeyA6AccessTokenOverride],
		"随机 nonce：两次加密同一明文不应产生相同密文")
	require.Equal(t, token, svc.Effective(ctx).AccessToken)
}

// TestReconciliationA6SettingsRealEncryptorRotatedKeyFallsBack 复现 TOTP_ENCRYPTION_KEY
// 轮换后的读路径：密文解不开时必须退回配置里的令牌，绝不能把密文当令牌发往上游。
func TestReconciliationA6SettingsRealEncryptorRotatedKeyFallsBack(t *testing.T) {
	ctx := context.Background()

	writer, state := a6SettingsServiceWithRealEncryptor(t, aesEncryptor(t))
	token := "a6-token-written-with-old-key"
	_, err := writer.Update(ctx, service.ReconciliationA6SettingsInput{AccessToken: &token})
	require.NoError(t, err)

	// 换一把密钥（模拟密钥轮换）后重新构造服务，共用同一份"库"。
	rotated, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0x99)))
	require.NoError(t, err)
	reader := service.NewReconciliationA6SettingsService(state, rotated, service.ReconciliationA6Config{
		BaseURL:     "https://a6.example.com",
		UserID:      "123",
		AccessToken: "config-token-000000",
		Timeout:     time.Minute,
	}, 7.2)

	settings := reader.Effective(ctx)
	require.Equal(t, "config-token-000000", settings.AccessToken, "解不开就退回配置令牌")
	require.NotContains(t, settings.OverrideKeys, service.ReconciliationOverrideKeyA6AccessToken,
		"解不开的密文不能算作「已配置覆盖」")
	require.NotContains(t, settings.AccessToken, state.values[service.ReconciliationStateKeyA6AccessTokenOverride],
		"绝不能把密文当令牌")
}

// TestReconciliationA6SettingsRealEncryptorStoresNothingElseInPlaintext 兜底断言：
// 保存令牌之后，整张表里任何一行的值都不该包含明文令牌。
func TestReconciliationA6SettingsRealEncryptorStoresNothingElseInPlaintext(t *testing.T) {
	ctx := context.Background()
	svc, state := a6SettingsServiceWithRealEncryptor(t, aesEncryptor(t))

	token := "a6-leak-canary-token-xyz"
	_, err := svc.Update(ctx, service.ReconciliationA6SettingsInput{
		BaseURL:      a6SettingsStringPtr("https://panel.example.com"),
		UserID:       a6SettingsStringPtr("panel-user"),
		AccessToken:  &token,
		FxUSDCNYRate: a6SettingsFloatPtr(6.71),
	})
	require.NoError(t, err)

	for key, value := range state.values {
		require.NotContainsf(t, value, token, "键 %s 的落库值包含明文令牌", key)
	}
}
