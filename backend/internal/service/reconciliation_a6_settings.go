package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// ==================== 键名 ====================

// A6 上游凭据在面板上可覆盖的键，存进 reconciliation_sync_state 这张键值表。
//
// 键名是接口契约的一部分（运维会直接用 SQL 查这几行），不要改名。
// 令牌的键名以 _enc 结尾不是装饰：它同时提醒后来者这一列存的是密文，
// 任何「顺手把明文写进去」的改动都会在 diff 里一眼看出来。
const (
	ReconciliationStateKeyA6BaseURLOverride     = "a6_base_url_override"
	ReconciliationStateKeyA6UserIDOverride      = "a6_user_id_override"
	ReconciliationStateKeyA6AccessTokenOverride = "a6_access_token_override_enc"
)

// override_keys 里出现的键名。
//
// 刻意用前端表单的字段名（a6_base_url）而不是数据库键名（a6_base_url_override）：
// 前端拿到这个列表后直接对应到需要标记「已覆盖」的输入框，不必再做一次映射。
const (
	ReconciliationOverrideKeyA6BaseURL     = "a6_base_url"
	ReconciliationOverrideKeyA6UserID      = "a6_user_id"
	ReconciliationOverrideKeyA6AccessToken = "a6_access_token"
	ReconciliationOverrideKeyFxRate        = "fx_usd_cny_rate"
)

// reconciliationSettingsLogComponent 本文件所有日志的组件名。
const reconciliationSettingsLogComponent = "service.reconciliation_settings"

// reconciliationMaxFxRate 是汇率覆盖值的上限。
//
// 汇率是记账口径：比 10 万还大的数字一定是把「每百万 token 单价」之类的东西误填
// 进来了，让它落库只会把全部金额算成天文数字，所以在这里就挡掉。
const reconciliationMaxFxRate = 100000

// ==================== 错误 ====================

var (
	// ErrReconciliationInvalidBaseURL 面板提交的 A6 站点基址不是合法的 http/https URL。
	ErrReconciliationInvalidBaseURL = errors.New("RECONCILIATION_INVALID_A6_BASE_URL")
	// ErrReconciliationInvalidFxRate 面板提交的汇率越界（<= 0、超上限、NaN/Inf）。
	ErrReconciliationInvalidFxRate = errors.New("RECONCILIATION_INVALID_FX_RATE")
	// ErrReconciliationSecretEncryptorUnavailable 加密器缺席，拒绝保存令牌。
	//
	// 宁可返回错误也不落明文：这张表是普通键值表，明文令牌会被任何一次库备份带走。
	ErrReconciliationSecretEncryptorUnavailable = errors.New("RECONCILIATION_SECRET_ENCRYPTOR_UNAVAILABLE")
	// ErrReconciliationSettingsStoreUnavailable 覆盖值存储（同步状态表）缺席，无法读写。
	ErrReconciliationSettingsStoreUnavailable = errors.New("RECONCILIATION_SETTINGS_STORE_UNAVAILABLE")
)

// ==================== 生效配置 ====================

// ReconciliationA6Settings 是解析后的生效 A6 配置。
//
// 生效值 = 面板覆盖 → config.reconciliation.* → 默认（基址/用户标识/令牌默认空串）。
// 除值本身外还带上「哪些键来自面板覆盖」，供接口层告诉前端该标记哪几个输入框。
type ReconciliationA6Settings struct {
	BaseURL     string
	UserID      string
	AccessToken string
	// Timeout 单次访问上游的超时，只来自配置文件：面板没有这个入口，
	// 留在这里是为了让 Resolve 出来的配置能直接喂给 A6 客户端。
	Timeout      time.Duration
	FxUSDCNYRate float64
	// OverrideKeys 当前来自面板覆盖的键，按固定顺序，永不为 nil（前端直接遍历）。
	OverrideKeys []string
}

// Configured 报告基址、用户标识与令牌三者齐备且基址可用。
func (s ReconciliationA6Settings) Configured() bool {
	return a6ConfigUsable(s.ClientConfig())
}

// ClientConfig 把生效值装成 A6 客户端配置。
func (s ReconciliationA6Settings) ClientConfig() ReconciliationA6Config {
	return ReconciliationA6Config{
		BaseURL:     s.BaseURL,
		AccessToken: s.AccessToken,
		UserID:      s.UserID,
		Timeout:     s.Timeout,
	}
}

// View 返回脱敏视图，供管理端接口直接序列化。
func (s ReconciliationA6Settings) View() ReconciliationA6SettingsView {
	keys := make([]string, len(s.OverrideKeys))
	copy(keys, s.OverrideKeys)
	return ReconciliationA6SettingsView{
		A6BaseURL:         s.BaseURL,
		A6UserID:          s.UserID,
		A6TokenConfigured: strings.TrimSpace(s.AccessToken) != "",
		A6TokenMask:       maskReconciliationA6Token(s.AccessToken),
		FxUSDCNYRate:      s.FxUSDCNYRate,
		OverrideKeys:      keys,
	}
}

// ReconciliationA6SettingsView 是 A6 配置的脱敏视图。
//
// 这个结构体会被 JSON 序列化后发到浏览器，所以它**只有**令牌的「是否已配置」与
// 脱敏提示，没有任何字段能承载明文。要加字段时请先想清楚这一条。
type ReconciliationA6SettingsView struct {
	A6BaseURL         string
	A6UserID          string
	A6TokenConfigured bool
	A6TokenMask       string
	FxUSDCNYRate      float64
	OverrideKeys      []string
}

// ReconciliationA6SettingsInput 是一次面板保存的入参。
//
// 字段全可选（nil = 不改动）：用指针而不是零值，才能把「没传这个字段」与
// 「传了空串（= 清除该覆盖）」区分开——前端「留空 = 不改」的语义全靠它。
type ReconciliationA6SettingsInput struct {
	BaseURL      *string
	UserID       *string
	AccessToken  *string
	FxUSDCNYRate *float64
	// ClearAccessToken 为 true 时清掉令牌覆盖，回落到配置/环境变量里的值。
	ClearAccessToken bool
}

// ==================== 服务 ====================

// ReconciliationA6SettingsService 负责 A6 上游凭据的运行时覆盖。
//
// 覆盖值存在 reconciliation_sync_state 里（优先级高于配置文件），因此管理员在
// 「上游 A6 配置」页面改完立刻生效，既不用重建容器也不用重启进程；令牌一律
// 加密后落库，这个服务是唯一有权限写这一行的代码路径。
type ReconciliationA6SettingsService struct {
	stateRepo ReconciliationSyncStateRepository
	encryptor SecretEncryptor
	// defaults 是配置层（环境变量 / 配置文件）注入的默认值。
	defaults ReconciliationA6Config
	// fxDefault 是配置层的汇率默认值，已归一化。
	fxDefault float64
}

// NewReconciliationA6SettingsService 创建设置服务。
//
// encryptor 可以为 nil，但此时保存令牌会直接报错而不是落明文。
func NewReconciliationA6SettingsService(
	stateRepo ReconciliationSyncStateRepository,
	encryptor SecretEncryptor,
	defaults ReconciliationA6Config,
	fxDefault float64,
) *ReconciliationA6SettingsService {
	defaults.BaseURL = strings.TrimSpace(defaults.BaseURL)
	defaults.UserID = strings.TrimSpace(defaults.UserID)
	defaults.AccessToken = strings.TrimSpace(defaults.AccessToken)
	return &ReconciliationA6SettingsService{
		stateRepo: stateRepo,
		encryptor: encryptor,
		defaults:  defaults,
		fxDefault: normalizeReconciliationFxDefault(fxDefault),
	}
}

// Effective 解析当前生效的 A6 配置。
//
// 读库失败或密文解不开都不报错，而是退回配置默认值并告警：设置页与 /status 的
// 健康指示不该因为一次读库抖动整页失败，这与汇率覆盖的既有处理保持一致。
func (s *ReconciliationA6SettingsService) Effective(ctx context.Context) ReconciliationA6Settings {
	settings := ReconciliationA6Settings{
		BaseURL:     s.defaults.BaseURL,
		UserID:      s.defaults.UserID,
		AccessToken: s.defaults.AccessToken,
		Timeout:     s.defaults.Timeout,
		// 汇率复用同步服务那条「覆盖 → 配置 → 1」的既有逻辑。
		FxUSDCNYRate: s.fxDefault,
		OverrideKeys: make([]string, 0, 4),
	}
	if s.stateRepo == nil {
		return settings
	}

	values, err := s.stateRepo.GetMultiple(ctx, []string{
		ReconciliationStateKeyA6BaseURLOverride,
		ReconciliationStateKeyA6UserIDOverride,
		ReconciliationStateKeyA6AccessTokenOverride,
		ReconciliationStateKeyFxRateOverride,
	})
	if err != nil {
		logger.LegacyPrintf(reconciliationSettingsLogComponent, "settings_read_failed: err=%v", err)
		return settings
	}

	// 覆盖值为空串表示「已清除」而不是「覆盖成空」：这一条必须与 Update 的写入、
	// 以及汇率覆盖的既有判定完全一致，否则清除操作会在页面上表现为「填了个空值」。
	if raw := strings.TrimSpace(values[ReconciliationStateKeyA6BaseURLOverride]); raw != "" {
		settings.BaseURL = raw
		settings.OverrideKeys = append(settings.OverrideKeys, ReconciliationOverrideKeyA6BaseURL)
	}
	if raw := strings.TrimSpace(values[ReconciliationStateKeyA6UserIDOverride]); raw != "" {
		settings.UserID = raw
		settings.OverrideKeys = append(settings.OverrideKeys, ReconciliationOverrideKeyA6UserID)
	}
	if raw := strings.TrimSpace(values[ReconciliationStateKeyA6AccessTokenOverride]); raw != "" {
		token, decryptErr := s.decryptAccessToken(raw)
		if decryptErr != nil {
			// 解不开（例如 TOTP_ENCRYPTION_KEY 被轮换过）时退回配置里的令牌。
			// 绝不能把密文当令牌发出去，也绝不能把这份失败当成「已配置覆盖」。
			logger.LegacyPrintf(reconciliationSettingsLogComponent, "token_decrypt_failed: err=%v", decryptErr)
		} else {
			settings.AccessToken = token
			settings.OverrideKeys = append(settings.OverrideKeys, ReconciliationOverrideKeyA6AccessToken)
		}
	}
	if rate, ok := parseReconciliationFxRate(values[ReconciliationStateKeyFxRateOverride]); ok {
		settings.FxUSDCNYRate = rate
		settings.OverrideKeys = append(settings.OverrideKeys, ReconciliationOverrideKeyFxRate)
	}

	return settings
}

// Update 保存面板提交的覆盖值，返回保存后重新解析的生效配置。
//
// 保存成功后重新解析（而不是就地拼一份返回值）：接口层返回给前端的必须是
// 「库里现在到底是什么」，读回来一次才能保证这一点。
func (s *ReconciliationA6SettingsService) Update(ctx context.Context, input ReconciliationA6SettingsInput) (ReconciliationA6Settings, error) {
	if s.stateRepo == nil {
		return ReconciliationA6Settings{}, ErrReconciliationSettingsStoreUnavailable
	}

	// 校验全部字段之后再落库：一个非法字段不该让同一请求里的其它字段被半保存。
	baseURL := ""
	if input.BaseURL != nil {
		baseURL = strings.TrimSpace(*input.BaseURL)
		if baseURL != "" && !isValidReconciliationA6BaseURL(baseURL) {
			return ReconciliationA6Settings{}, ErrReconciliationInvalidBaseURL
		}
	}
	if input.FxUSDCNYRate != nil {
		if !isValidReconciliationFxRate(*input.FxUSDCNYRate) {
			return ReconciliationA6Settings{}, ErrReconciliationInvalidFxRate
		}
	}

	// 空串 = 清除该覆盖，与 Effective 里「空串不算覆盖」的判定严格对应。
	if input.BaseURL != nil {
		if err := s.stateRepo.Set(ctx, ReconciliationStateKeyA6BaseURLOverride, baseURL); err != nil {
			return ReconciliationA6Settings{}, err
		}
	}
	if input.UserID != nil {
		if err := s.stateRepo.Set(ctx, ReconciliationStateKeyA6UserIDOverride, strings.TrimSpace(*input.UserID)); err != nil {
			return ReconciliationA6Settings{}, err
		}
	}
	if err := s.applyAccessToken(ctx, input); err != nil {
		return ReconciliationA6Settings{}, err
	}
	if input.FxUSDCNYRate != nil {
		if err := setReconciliationFxRateOverride(ctx, s.stateRepo, *input.FxUSDCNYRate); err != nil {
			return ReconciliationA6Settings{}, err
		}
	}

	return s.Effective(ctx), nil
}

// applyAccessToken 处理令牌的三条路径，判定顺序不可调换。
//
// 显式非空令牌优先：管理员刚粘进来的令牌，不该因为同一个请求里带了 clear 标志
// 就被丢掉（丢令牌不可恢复，漏清一次覆盖只是下次再点一下）。
// 其次才是 clear 标志；两者都没给就原样保留——前端「留空 = 不改」的语义。
func (s *ReconciliationA6SettingsService) applyAccessToken(ctx context.Context, input ReconciliationA6SettingsInput) error {
	token := ""
	if input.AccessToken != nil {
		token = strings.TrimSpace(*input.AccessToken)
	}
	if token != "" {
		return s.saveAccessTokenOverride(ctx, token)
	}
	if input.ClearAccessToken {
		return s.stateRepo.Set(ctx, ReconciliationStateKeyA6AccessTokenOverride, "")
	}
	return nil
}

// saveAccessTokenOverride 加密后写入令牌覆盖。
//
// 密文形态是 base64(nonce + 密文 + tag)，比明文长；本表的 value 列是 TEXT，
// 没有长度上限问题，因此不需要为「令牌特别长」另做处理。
func (s *ReconciliationA6SettingsService) saveAccessTokenOverride(ctx context.Context, token string) error {
	if s.encryptor == nil {
		return ErrReconciliationSecretEncryptorUnavailable
	}
	ciphertext, err := s.encryptor.Encrypt(token)
	if err != nil {
		return fmt.Errorf("encrypt A6 access token: %w", err)
	}
	return s.stateRepo.Set(ctx, ReconciliationStateKeyA6AccessTokenOverride, ciphertext)
}

// decryptAccessToken 解开令牌覆盖值。
func (s *ReconciliationA6SettingsService) decryptAccessToken(ciphertext string) (string, error) {
	if s.encryptor == nil {
		return "", ErrReconciliationSecretEncryptorUnavailable
	}
	token, err := s.encryptor.Decrypt(ciphertext)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

// ==================== 校验与小工具 ====================

// isValidReconciliationA6BaseURL 校验站点基址是绝对的 http/https URL。
//
// 与 A6 客户端发请求前的校验同一口径：面板能存进去的地址，采集器一定用得了，
// 不会出现「页面显示已配置、采集器却一直跳过」的矛盾。
func isValidReconciliationA6BaseURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

// isValidReconciliationFxRate 校验汇率覆盖值：必须为正、有限、且不超过上限。
func isValidReconciliationFxRate(rate float64) bool {
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return false
	}
	return rate > 0 && rate <= reconciliationMaxFxRate
}

// reconciliationTokenMaskMinLen 是允许部分揭示的最短令牌长度。
//
// 门槛按「揭示比例」定，而不是随手取个小数字：首尾各露 4 位时，长度 20 的令牌
// 露出 40%，再短就会一次露出大半（长度 9 时高达 89%）。真实 A6 令牌都在 40 位以上，
// 因此这个门槛不牺牲「靠前缀认出是哪一套凭据」的实际价值，却把最短可揭示情形的
// 暴露比例压到了 40% 以下。
const reconciliationTokenMaskMinLen = 20

// maskReconciliationA6Token 给出令牌的脱敏提示：足够长时首尾各留 4 个字符。
//
// 与 maskSecretTail（全遮 + 尾 4 位）的区别是这里多露一个前 4 位：A6 令牌的前缀
// 常能区分「是哪一套凭据」。短于 reconciliationTokenMaskMinLen 的令牌整体涂黑，
// 避免把短令牌的大半字符露出去。
func maskReconciliationA6Token(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	runes := []rune(token)
	if len(runes) < reconciliationTokenMaskMinLen {
		return "****"
	}
	return string(runes[:4]) + "…" + string(runes[len(runes)-4:])
}
