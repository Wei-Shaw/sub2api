//go:build unit

package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 本文件覆盖 A6 上游凭据的运行时覆盖（面板配置）：
//   - 令牌必须加密落库，且任何响应里都不得出现明文；
//   - 生效值解析顺序固定为「面板覆盖 → config.reconciliation.* → 默认」；
//   - 校验失败必须报错，且不落半个字段；
//   - 面板改完，账单来源下一轮拉取就要用上新凭据。

const a6SettingsCipherPrefix = "enc:v1:"

// a6SettingsMaskableToken 是恰好达到「可部分揭示」门槛（20 位）的令牌，
// 首尾各 4 位正是接口契约里举的例子 abcd…wxyz。
const a6SettingsMaskableToken = "abcd123456789012wxyz"

// a6SettingsStateRepoStub 是 reconciliation_sync_state 的内存桩。
//
// 带锁：设置服务会被 HTTP 处理器并发调用，桩跟不上就会在 -race 下炸掉。
type a6SettingsStateRepoStub struct {
	mu     sync.Mutex
	values map[string]string
	getErr error
	setErr error
	// writes 记录写过的键，用于断言「校验失败时一个字段都没落库」。
	writes []string
}

func newA6SettingsStateRepoStub() *a6SettingsStateRepoStub {
	return &a6SettingsStateRepoStub{values: map[string]string{}}
}

func (s *a6SettingsStateRepoStub) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return "", s.getErr
	}
	return s.values[key], nil
}

func (s *a6SettingsStateRepoStub) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setErr != nil {
		return s.setErr
	}
	s.values[key] = value
	s.writes = append(s.writes, key)
	return nil
}

func (s *a6SettingsStateRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return nil, s.getErr
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *a6SettingsStateRepoStub) seed(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
}

// raw 读回某个键当前落库的原始字符串（就是断言「不是明文」时看的东西）。
func (s *a6SettingsStateRepoStub) raw(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[key]
}

func (s *a6SettingsStateRepoStub) snapshot() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out
}

func (s *a6SettingsStateRepoStub) writeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.writes)
}

// allValues 把所有落库值拼成一串，用于断言「明文不在任何一行里」。
func (s *a6SettingsStateRepoStub) allValues() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	parts := make([]string, 0, len(s.values))
	for _, value := range s.values {
		parts = append(parts, value)
	}
	return strings.Join(parts, "|")
}

// a6SettingsEncryptorStub 是可逆的假加密器。
//
// 它不提供任何机密性，只用来断言调用契约（「服务写进库里的不是明文」）。
// 真实 AES-256-GCM 的落库形态由 internal/repository 的用例覆盖：
// depguard 禁止 service 包 import repository，真实加密器只能在那里验。
type a6SettingsEncryptorStub struct {
	encryptErr error
	decryptErr error
}

func (e *a6SettingsEncryptorStub) Encrypt(plaintext string) (string, error) {
	if e.encryptErr != nil {
		return "", e.encryptErr
	}
	return a6SettingsCipherPrefix + base64.StdEncoding.EncodeToString([]byte(plaintext)), nil
}

func (e *a6SettingsEncryptorStub) Decrypt(ciphertext string) (string, error) {
	if e.decryptErr != nil {
		return "", e.decryptErr
	}
	if !strings.HasPrefix(ciphertext, a6SettingsCipherPrefix) {
		return "", errors.New("ciphertext has no encryption prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ciphertext, a6SettingsCipherPrefix))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// a6SettingsDefaults 是配置层默认值，测试里统一用它起手。
func a6SettingsDefaults() ReconciliationA6Config {
	return ReconciliationA6Config{
		BaseURL:     "https://config.example.com",
		UserID:      "config-user",
		AccessToken: "config-token-000000",
		Timeout:     30 * time.Second,
	}
}

func newA6SettingsHarness(t *testing.T) (*ReconciliationA6SettingsService, *a6SettingsStateRepoStub, *a6SettingsEncryptorStub) {
	t.Helper()
	state := newA6SettingsStateRepoStub()
	encryptor := &a6SettingsEncryptorStub{}
	return NewReconciliationA6SettingsService(state, encryptor, a6SettingsDefaults(), 7.2), state, encryptor
}

func a6SettingsStringPtr(value string) *string { return &value }

func a6SettingsFloatPtr(value float64) *float64 { return &value }

// ==================== 1. 令牌加密落库与解密读回 ====================

func TestReconciliationA6SettingsStoresTokenEncrypted(t *testing.T) {
	svc, state, _ := newA6SettingsHarness(t)
	ctx := context.Background()
	token := "a6-panel-token-0123456789"

	saved, err := svc.Update(ctx, ReconciliationA6SettingsInput{AccessToken: &token})
	require.NoError(t, err)

	stored := state.raw(ReconciliationStateKeyA6AccessTokenOverride)
	require.NotEmpty(t, stored, "令牌覆盖必须落库")
	require.NotEqual(t, token, stored, "落库的必须是密文，不能是明文")
	require.NotContains(t, stored, token, "密文里不能夹带明文")
	require.True(t, strings.HasPrefix(stored, a6SettingsCipherPrefix), "看起来根本没走加密器: %q", stored)

	// 明文不得出现在这张表的任何一行里。
	for key, value := range state.snapshot() {
		require.NotContainsf(t, value, token, "键 %s 落库内容是明文", key)
	}

	// 读回：生效配置拿到的必须是原文。
	require.Equal(t, token, saved.AccessToken)
	require.Contains(t, saved.OverrideKeys, ReconciliationOverrideKeyA6AccessToken)
	require.Equal(t, token, svc.Effective(ctx).AccessToken, "解密读回必须还原原始令牌")
}

func TestReconciliationA6SettingsTokenMaskCoversOnlyHeadAndTail(t *testing.T) {
	svc, _, _ := newA6SettingsHarness(t)
	ctx := context.Background()
	// 与接口契约里的示例一致：abcd…wxyz（20 位，恰好达到部分揭示的门槛）。
	token := a6SettingsMaskableToken

	_, err := svc.Update(ctx, ReconciliationA6SettingsInput{AccessToken: &token})
	require.NoError(t, err)

	view := svc.Effective(ctx).View()
	require.True(t, view.A6TokenConfigured)
	require.Equal(t, "abcd…wxyz", view.A6TokenMask)

	// 未配置令牌时既没有 mask，也不该说「已配置」。
	empty := NewReconciliationA6SettingsService(
		newA6SettingsStateRepoStub(), &a6SettingsEncryptorStub{},
		ReconciliationA6Config{BaseURL: "https://config.example.com", UserID: "u"}, 7.2,
	)
	emptyView := empty.Effective(ctx).View()
	require.False(t, emptyView.A6TokenConfigured)
	require.Empty(t, emptyView.A6TokenMask)
}

func TestReconciliationA6SettingsMaskBoundaries(t *testing.T) {
	// 门槛以下一律整体涂黑：短令牌一旦做「首尾各 4 位」，暴露比例会高得离谱
	// （长度 9 露 89%、长度 12 露 67%）。
	for _, size := range []int{1, 4, 8, 9, 12, 19} {
		token := strings.Repeat("a", size)
		require.Equalf(t, "****", maskReconciliationA6Token(token), "长度 %d 必须整体涂黑", size)
	}

	// 恰好到门槛才开始部分揭示。
	require.Equal(t, "1234…7890", maskReconciliationA6Token("12345678901234567890"), "长度 20 露出 40%")
	require.Equal(t, "abcd…wxyz", maskReconciliationA6Token("abcd123456789012wxyz"))

	// 真实长度（A6 令牌 40 位以上）的暴露比例远低于 20%。
	require.Equal(t, "a6-s…f9e8", maskReconciliationA6Token("a6-secret-token-0123456789abcdef-f9e8"))

	require.Empty(t, maskReconciliationA6Token("   "), "空白令牌不是已配置")
	require.Empty(t, maskReconciliationA6Token(""))
}

// ==================== 2. 生效值解析顺序 ====================

func TestReconciliationA6SettingsResolutionOrder(t *testing.T) {
	ctx := context.Background()

	t.Run("配置层默认值", func(t *testing.T) {
		svc, _, _ := newA6SettingsHarness(t)

		settings := svc.Effective(ctx)
		require.Equal(t, "https://config.example.com", settings.BaseURL)
		require.Equal(t, "config-user", settings.UserID)
		require.Equal(t, "config-token-000000", settings.AccessToken)
		require.Equal(t, 7.2, settings.FxUSDCNYRate)
		require.Empty(t, settings.OverrideKeys, "没有覆盖时 override_keys 必须是空列表")
		require.NotNil(t, settings.OverrideKeys, "override_keys 不能是 nil：前端直接遍历")
	})

	t.Run("面板覆盖优先于配置", func(t *testing.T) {
		svc, state, encryptor := newA6SettingsHarness(t)
		ciphertext, err := encryptor.Encrypt("panel-token-abcdefgh")
		require.NoError(t, err)
		state.seed(ReconciliationStateKeyA6BaseURLOverride, "https://panel.example.com")
		state.seed(ReconciliationStateKeyA6UserIDOverride, "panel-user")
		state.seed(ReconciliationStateKeyA6AccessTokenOverride, ciphertext)
		state.seed(ReconciliationStateKeyFxRateOverride, "6.71")

		settings := svc.Effective(ctx)
		require.Equal(t, "https://panel.example.com", settings.BaseURL)
		require.Equal(t, "panel-user", settings.UserID)
		require.Equal(t, "panel-token-abcdefgh", settings.AccessToken)
		require.Equal(t, 6.71, settings.FxUSDCNYRate)
		require.Equal(t, []string{
			ReconciliationOverrideKeyA6BaseURL,
			ReconciliationOverrideKeyA6UserID,
			ReconciliationOverrideKeyA6AccessToken,
			ReconciliationOverrideKeyFxRate,
		}, settings.OverrideKeys, "override_keys 顺序固定，前端按它标记输入框")
	})

	t.Run("空串覆盖视为已清除", func(t *testing.T) {
		svc, state, _ := newA6SettingsHarness(t)
		state.seed(ReconciliationStateKeyA6BaseURLOverride, "")
		state.seed(ReconciliationStateKeyA6UserIDOverride, "")
		state.seed(ReconciliationStateKeyA6AccessTokenOverride, "")
		state.seed(ReconciliationStateKeyFxRateOverride, "")

		settings := svc.Effective(ctx)
		require.Equal(t, "https://config.example.com", settings.BaseURL)
		require.Equal(t, "config-user", settings.UserID)
		require.Equal(t, "config-token-000000", settings.AccessToken)
		require.Equal(t, 7.2, settings.FxUSDCNYRate)
		require.Empty(t, settings.OverrideKeys)
	})

	t.Run("覆盖值写坏时退回配置", func(t *testing.T) {
		svc, state, _ := newA6SettingsHarness(t)
		// 汇率写坏：绝不能把全部金额算成 0。
		state.seed(ReconciliationStateKeyFxRateOverride, "not-a-number")
		require.Equal(t, 7.2, svc.Effective(ctx).FxUSDCNYRate)

		state.seed(ReconciliationStateKeyFxRateOverride, "-3")
		require.Equal(t, 7.2, svc.Effective(ctx).FxUSDCNYRate)
	})

	t.Run("读库失败时退回配置默认值", func(t *testing.T) {
		state := newA6SettingsStateRepoStub()
		state.getErr = assert.AnError
		svc := NewReconciliationA6SettingsService(state, &a6SettingsEncryptorStub{}, a6SettingsDefaults(), 7.2)

		settings := svc.Effective(ctx)
		require.Equal(t, "https://config.example.com", settings.BaseURL)
		require.Equal(t, "config-token-000000", settings.AccessToken)
		require.Equal(t, 7.2, settings.FxUSDCNYRate)
		require.Empty(t, settings.OverrideKeys)
	})

	t.Run("密文解不开时退回配置且不算已覆盖", func(t *testing.T) {
		svc, state, _ := newA6SettingsHarness(t)
		// 例如 TOTP_ENCRYPTION_KEY 被轮换过：密文再也解不开。
		state.seed(ReconciliationStateKeyA6AccessTokenOverride, "not-a-valid-ciphertext")

		settings := svc.Effective(ctx)
		require.Equal(t, "config-token-000000", settings.AccessToken, "解不开就退回配置令牌")
		require.NotContains(t, settings.OverrideKeys, ReconciliationOverrideKeyA6AccessToken,
			"解不开的密文不能算作「已配置覆盖」")
	})
}

func TestReconciliationA6SettingsViewNeverCarriesPlaintext(t *testing.T) {
	svc, _, _ := newA6SettingsHarness(t)
	ctx := context.Background()
	token := "a6-view-token-should-not-leak"
	_, err := svc.Update(ctx, ReconciliationA6SettingsInput{AccessToken: &token})
	require.NoError(t, err)

	view := svc.Effective(ctx).View()
	raw, err := json.Marshal(view)
	require.NoError(t, err)
	require.NotContains(t, string(raw), token, "视图结构体被序列化后绝不允许出现明文令牌")

	require.Equal(t, "a6-v…leak", view.A6TokenMask)
	require.True(t, view.A6TokenConfigured)
}

// ==================== 4. 保存语义 ====================

func TestReconciliationA6SettingsUpdateKeepsTokenWhenBlank(t *testing.T) {
	svc, state, _ := newA6SettingsHarness(t)
	ctx := context.Background()
	token := "a6-keep-me-token-1234"
	_, err := svc.Update(ctx, ReconciliationA6SettingsInput{AccessToken: &token})
	require.NoError(t, err)
	storedBefore := state.raw(ReconciliationStateKeyA6AccessTokenOverride)

	t.Run("只改基址时令牌不变", func(t *testing.T) {
		_, err := svc.Update(ctx, ReconciliationA6SettingsInput{BaseURL: a6SettingsStringPtr("https://panel.example.com")})
		require.NoError(t, err)
		require.Equal(t, storedBefore, state.raw(ReconciliationStateKeyA6AccessTokenOverride))
		require.Equal(t, token, svc.Effective(ctx).AccessToken)
	})

	t.Run("令牌传空串等于不改", func(t *testing.T) {
		_, err := svc.Update(ctx, ReconciliationA6SettingsInput{AccessToken: a6SettingsStringPtr("   ")})
		require.NoError(t, err)
		require.Equal(t, storedBefore, state.raw(ReconciliationStateKeyA6AccessTokenOverride))
		require.Equal(t, token, svc.Effective(ctx).AccessToken)
	})

	t.Run("全字段缺省等于什么都不改", func(t *testing.T) {
		before := state.snapshot()
		_, err := svc.Update(ctx, ReconciliationA6SettingsInput{})
		require.NoError(t, err)
		require.Equal(t, before, state.snapshot())
	})
}

func TestReconciliationA6SettingsUpdateClearTokenFallsBackToConfig(t *testing.T) {
	svc, state, _ := newA6SettingsHarness(t)
	ctx := context.Background()
	token := "a6-clear-me-token-1234"
	_, err := svc.Update(ctx, ReconciliationA6SettingsInput{AccessToken: &token})
	require.NoError(t, err)
	require.Contains(t, svc.Effective(ctx).OverrideKeys, ReconciliationOverrideKeyA6AccessToken)

	settings, err := svc.Update(ctx, ReconciliationA6SettingsInput{ClearAccessToken: true})
	require.NoError(t, err)

	require.Empty(t, state.raw(ReconciliationStateKeyA6AccessTokenOverride), "清除后该键必须是空串而不是被删行之外的东西")
	require.Equal(t, "config-token-000000", settings.AccessToken, "清除覆盖后回落到配置/环境变量")
	require.NotContains(t, settings.OverrideKeys, ReconciliationOverrideKeyA6AccessToken)
	require.True(t, settings.View().A6TokenConfigured, "配置里有令牌，所以仍然算已配置")
}

func TestReconciliationA6SettingsUpdateExplicitTokenWinsOverClearFlag(t *testing.T) {
	svc, state, _ := newA6SettingsHarness(t)
	ctx := context.Background()
	token := "a6-typed-right-now-1234"

	// 同一个请求里既有新令牌又有 clear 标志：宁可漏清一次覆盖，也不能丢掉刚粘进来的令牌。
	settings, err := svc.Update(ctx, ReconciliationA6SettingsInput{
		AccessToken:      &token,
		ClearAccessToken: true,
	})
	require.NoError(t, err)
	require.Equal(t, token, settings.AccessToken)
	require.True(t, strings.HasPrefix(state.raw(ReconciliationStateKeyA6AccessTokenOverride), a6SettingsCipherPrefix))
	require.Equal(t, token, svc.Effective(ctx).AccessToken)
}

func TestReconciliationA6SettingsUpdateEmptyStringsClearOverrides(t *testing.T) {
	svc, state, _ := newA6SettingsHarness(t)
	ctx := context.Background()

	_, err := svc.Update(ctx, ReconciliationA6SettingsInput{
		BaseURL: a6SettingsStringPtr("https://panel.example.com"),
		UserID:  a6SettingsStringPtr("panel-user"),
	})
	require.NoError(t, err)
	require.Equal(t, "https://panel.example.com", svc.Effective(ctx).BaseURL)

	settings, err := svc.Update(ctx, ReconciliationA6SettingsInput{
		BaseURL: a6SettingsStringPtr(""),
		UserID:  a6SettingsStringPtr("   "),
	})
	require.NoError(t, err)
	require.Equal(t, "https://config.example.com", settings.BaseURL, "空串表示清除覆盖，回落到配置")
	require.Equal(t, "config-user", settings.UserID)
	require.Equal(t, "", state.raw(ReconciliationStateKeyA6BaseURLOverride))
	require.Equal(t, "", state.raw(ReconciliationStateKeyA6UserIDOverride))
	require.Empty(t, settings.OverrideKeys)
}

func TestReconciliationA6SettingsUpdateTrimsWhitespace(t *testing.T) {
	svc, state, _ := newA6SettingsHarness(t)
	ctx := context.Background()

	settings, err := svc.Update(ctx, ReconciliationA6SettingsInput{
		BaseURL: a6SettingsStringPtr("  https://panel.example.com  "),
		UserID:  a6SettingsStringPtr("  panel-user  "),
	})
	require.NoError(t, err)
	require.Equal(t, "https://panel.example.com", settings.BaseURL)
	require.Equal(t, "panel-user", settings.UserID)
	require.Equal(t, "https://panel.example.com", state.raw(ReconciliationStateKeyA6BaseURLOverride))
	require.Equal(t, "panel-user", state.raw(ReconciliationStateKeyA6UserIDOverride))
}

// ==================== 5. 校验 ====================

func TestReconciliationA6SettingsUpdateValidation(t *testing.T) {
	ctx := context.Background()

	badBaseURLs := []string{"not a url", "ftp://a6.example.com", "/relative/path", "a6.example.com", "https://"}
	for _, raw := range badBaseURLs {
		t.Run("非法基址_"+raw, func(t *testing.T) {
			svc, state, _ := newA6SettingsHarness(t)

			_, err := svc.Update(ctx, ReconciliationA6SettingsInput{
				BaseURL: a6SettingsStringPtr(raw),
				UserID:  a6SettingsStringPtr("should-not-be-saved"),
			})
			require.ErrorIs(t, err, ErrReconciliationInvalidBaseURL)
			require.Zero(t, state.writeCount(), "校验失败不允许落任何字段")
			require.Equal(t, "config-user", svc.Effective(ctx).UserID)
		})
	}

	goodBaseURLs := []string{"https://a6.example.com", "http://127.0.0.1:8080", "https://a6.example.com/base"}
	for _, raw := range goodBaseURLs {
		t.Run("合法基址_"+raw, func(t *testing.T) {
			svc, _, _ := newA6SettingsHarness(t)
			_, err := svc.Update(ctx, ReconciliationA6SettingsInput{BaseURL: a6SettingsStringPtr(raw)})
			require.NoError(t, err)
		})
	}

	badRates := []struct {
		name string
		rate float64
	}{
		{"零", 0},
		{"负数", -1},
		{"极小负数", -0.01},
		{"超过上限", 100000.5},
		{"天文数字", 1e9},
		{"NaN", math.NaN()},
		{"正无穷", math.Inf(1)},
	}
	for _, tc := range badRates {
		t.Run("非法汇率_"+tc.name, func(t *testing.T) {
			svc, state, _ := newA6SettingsHarness(t)

			_, err := svc.Update(ctx, ReconciliationA6SettingsInput{
				FxUSDCNYRate: a6SettingsFloatPtr(tc.rate),
				BaseURL:      a6SettingsStringPtr("https://panel.example.com"),
			})
			require.ErrorIs(t, err, ErrReconciliationInvalidFxRate)
			require.Zero(t, state.writeCount(), "校验失败不允许落任何字段")
		})
	}

	t.Run("汇率边界值可用", func(t *testing.T) {
		svc, _, _ := newA6SettingsHarness(t)
		settings, err := svc.Update(ctx, ReconciliationA6SettingsInput{FxUSDCNYRate: a6SettingsFloatPtr(reconciliationMaxFxRate)})
		require.NoError(t, err)
		require.Equal(t, float64(reconciliationMaxFxRate), settings.FxUSDCNYRate)
		require.Equal(t, float64(reconciliationMaxFxRate), svc.Effective(ctx).FxUSDCNYRate)
	})
}

func TestReconciliationA6SettingsWithoutEncryptorRefusesToStoreToken(t *testing.T) {
	ctx := context.Background()
	state := newA6SettingsStateRepoStub()
	svc := NewReconciliationA6SettingsService(state, nil, a6SettingsDefaults(), 7.2)

	token := "a6-must-not-land-in-plaintext"
	_, err := svc.Update(ctx, ReconciliationA6SettingsInput{AccessToken: &token})
	require.ErrorIs(t, err, ErrReconciliationSecretEncryptorUnavailable)
	require.Zero(t, state.writeCount(), "没有加密器时连别的字段都不该写")
	require.NotContains(t, state.allValues(), token)
}

func TestReconciliationA6SettingsEncryptFailureIsSurfaced(t *testing.T) {
	ctx := context.Background()
	state := newA6SettingsStateRepoStub()
	encryptor := &a6SettingsEncryptorStub{encryptErr: assert.AnError}
	svc := NewReconciliationA6SettingsService(state, encryptor, a6SettingsDefaults(), 7.2)

	token := "a6-encrypt-fails"
	_, err := svc.Update(ctx, ReconciliationA6SettingsInput{AccessToken: &token})
	require.Error(t, err)
	require.NotContains(t, state.allValues(), token, "加密失败绝不能退化成落明文")
}

func TestReconciliationA6SettingsPropagatesStoreErrors(t *testing.T) {
	ctx := context.Background()
	state := newA6SettingsStateRepoStub()
	state.setErr = assert.AnError
	svc := NewReconciliationA6SettingsService(state, &a6SettingsEncryptorStub{}, a6SettingsDefaults(), 7.2)

	_, err := svc.Update(ctx, ReconciliationA6SettingsInput{BaseURL: a6SettingsStringPtr("https://panel.example.com")})
	require.ErrorIs(t, err, assert.AnError, "写库失败必须原样上报，接口层再翻成 500")
}

func TestReconciliationA6SettingsConfiguredNeedsAllThree(t *testing.T) {
	ctx := context.Background()

	t.Run("三者齐备且基址合法才算已配置", func(t *testing.T) {
		svc := NewReconciliationA6SettingsService(newA6SettingsStateRepoStub(), &a6SettingsEncryptorStub{}, a6SettingsDefaults(), 7.2)
		require.True(t, svc.Effective(ctx).Configured())
	})

	t.Run("基址不合法视为未配置", func(t *testing.T) {
		svc := NewReconciliationA6SettingsService(newA6SettingsStateRepoStub(), &a6SettingsEncryptorStub{},
			ReconciliationA6Config{BaseURL: "a6.example.com", UserID: "u", AccessToken: "t"}, 7.2)
		require.False(t, svc.Effective(ctx).Configured(), "与 A6 客户端的可用性判定必须是同一口径")
	})

	t.Run("缺令牌视为未配置", func(t *testing.T) {
		svc := NewReconciliationA6SettingsService(newA6SettingsStateRepoStub(), &a6SettingsEncryptorStub{},
			ReconciliationA6Config{BaseURL: "https://a6.example.com", UserID: "u"}, 7.2)
		require.False(t, svc.Effective(ctx).Configured())
	})

	t.Run("缺用户标识视为未配置", func(t *testing.T) {
		svc := NewReconciliationA6SettingsService(newA6SettingsStateRepoStub(), &a6SettingsEncryptorStub{},
			ReconciliationA6Config{BaseURL: "https://a6.example.com", AccessToken: "t"}, 7.2)
		require.False(t, svc.Effective(ctx).Configured())
	})
}
