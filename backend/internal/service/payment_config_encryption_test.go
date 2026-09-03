//go:build unit

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paymentTestKey 是一把固定的 32 字节 AES-256 密钥。
func paymentTestKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

// C4 的核心断言：支付服务商配置绝不能以明文落库。
// 单行泄露即可暴露 Stripe secretKey/webhookSecret、EasyPay pkey、wxpay
// apiV3Key/privateKey、Alipay privateKey、Airwallex apiKey/webhookSecret，
// 拿到它们的攻击者可以为任意订单签发合法的成功回调。
func TestEncryptConfigDoesNotStorePlaintext(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{encryptionKey: paymentTestKey()}
	cfg := map[string]string{
		"secretKey":     "sk_live_super_secret",
		"webhookSecret": "whsec_super_secret",
	}

	stored, err := svc.encryptConfig(cfg)
	require.NoError(t, err)

	// 明文格式（json.Marshal 的输出）必须解析失败，否则就是回归重现。
	var probe map[string]string
	require.Error(t, json.Unmarshal([]byte(stored), &probe),
		"stored config must not be parseable as plaintext JSON")

	// 密钥的字面值不得出现在存储值中。
	assert.NotContains(t, stored, "sk_live_super_secret")
	assert.NotContains(t, stored, "whsec_super_secret")

	// 格式必须是 iv:authTag:ciphertext，与回归窗口之前的历史密文一致，
	// 这样老行不需要任何迁移就能继续读。
	assert.Len(t, strings.Split(stored, ":"), 3)
}

func TestEncryptDecryptConfigRoundTrip(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{encryptionKey: paymentTestKey()}

	tests := []struct {
		name string
		cfg  map[string]string
	}{
		{"stripe", map[string]string{"secretKey": "sk_live_x", "webhookSecret": "whsec_y"}},
		{"empty_map", map[string]string{}},
		{"unicode_values", map[string]string{"name": "支付宝正式通道", "privateKey": "-----BEGIN-----"}},
		{"long_value", map[string]string{"privateKey": strings.Repeat("A", 4096)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stored, err := svc.encryptConfig(tt.cfg)
			require.NoError(t, err)

			got, err := svc.decryptConfig(stored)
			require.NoError(t, err)
			assert.Equal(t, tt.cfg, got)
		})
	}
}

// 没有密钥时必须拒绝写入。退回明文正是这次回归的做法，而且它失败得毫无声响。
func TestEncryptConfigRefusesWithoutKey(t *testing.T) {
	t.Parallel()

	for _, key := range [][]byte{nil, {}, []byte("too-short")} {
		svc := &PaymentConfigService{encryptionKey: key}
		stored, err := svc.encryptConfig(map[string]string{"secretKey": "sk_live_x"})
		require.Error(t, err, "must not fall back to plaintext")
		assert.Empty(t, stored)
		assert.Contains(t, err.Error(), "SECURITY_SECRET_ENCRYPTION_KEY",
			"the error must name the variable the operator has to set")
	}
}

// decryptConfig 的明文分支是回归窗口写入行的读兼容 shim。它必须留到启动回填
// 在所有部署上跑过为止：过早删掉会把一行可读的配置变成静默的空配置。
func TestDecryptConfigReadsRegressionWindowPlaintext(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{encryptionKey: paymentTestKey()}
	legacy := `{"secretKey":"sk_live_written_during_regression"}`

	got, err := svc.decryptConfig(legacy)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"secretKey": "sk_live_written_during_regression"}, got)
}

// 回归窗口之前的历史密文（同一格式、同一密钥）必须继续可读。
func TestDecryptConfigReadsPreRegressionCiphertext(t *testing.T) {
	t.Parallel()

	key := paymentTestKey()
	historical, err := payment.Encrypt(`{"pkey":"pkey-1001"}`, key)
	require.NoError(t, err)

	svc := &PaymentConfigService{encryptionKey: key}
	got, err := svc.decryptConfig(historical)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"pkey": "pkey-1001"}, got)
}

// 密钥被轮换或丢失后，密文解不开：按空配置处理，让管理员在后台重新录入，
// 而不是让整个启动或整个支付页面炸掉。
func TestDecryptConfigTreatsUnreadableCiphertextAsEmpty(t *testing.T) {
	t.Parallel()

	ciphertext, err := payment.Encrypt(`{"secretKey":"sk_live_x"}`, paymentTestKey())
	require.NoError(t, err)

	other := []byte("ffffffffffffffffffffffffffffffff")
	svc := &PaymentConfigService{encryptionKey: other}

	got, err := svc.decryptConfig(ciphertext)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDecryptConfigEmptyStored(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{encryptionKey: paymentTestKey()}
	got, err := svc.decryptConfig("")
	require.NoError(t, err)
	assert.Nil(t, got)
}

// 每次加密使用新的随机 nonce，相同配置不会产生相同密文——否则密文相等本身
// 就泄露了「两个通道用了同一份凭据」。
func TestEncryptConfigUsesFreshNonce(t *testing.T) {
	t.Parallel()

	svc := &PaymentConfigService{encryptionKey: paymentTestKey()}
	cfg := map[string]string{"secretKey": "sk_live_x"}

	seen := make(map[string]struct{}, 20)
	for i := 0; i < 20; i++ {
		stored, err := svc.encryptConfig(cfg)
		require.NoError(t, err)
		seen[stored] = struct{}{}
	}
	assert.Len(t, seen, 20)
}
