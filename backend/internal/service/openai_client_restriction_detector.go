package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

const (
	// CodexClientRestrictionReasonDisabled 表示账号未开启 codex_cli_only。
	CodexClientRestrictionReasonDisabled = "codex_cli_only_disabled"
	// CodexClientRestrictionReasonMatchedUA 表示请求命中官方客户端 UA 白名单。
	CodexClientRestrictionReasonMatchedUA = "official_client_user_agent_matched"
	// CodexClientRestrictionReasonMatchedOriginator 表示请求命中官方客户端 originator 白名单。
	CodexClientRestrictionReasonMatchedOriginator = "official_client_originator_matched"
	// CodexClientRestrictionReasonNotMatchedUA 表示请求未命中任何允许的客户端身份。
	CodexClientRestrictionReasonNotMatchedUA = "official_client_user_agent_not_matched"
	// CodexClientRestrictionReasonForceCodexCLI 表示通过 ForceCodexCLI 配置兜底放行。
	CodexClientRestrictionReasonForceCodexCLI = "force_codex_cli_enabled"
	// CodexClientRestrictionReasonBlacklisted 表示请求命中全局黑名单（门内 deny 最先，OR 语义）。
	CodexClientRestrictionReasonBlacklisted = "blacklist_matched"
	// CodexClientRestrictionReasonMatchedWhitelistClient 表示请求命中全局自由白名单条目（双因子 AND）。
	CodexClientRestrictionReasonMatchedWhitelistClient = "whitelist_client_matched"
	// CodexClientRestrictionReasonVersionTooLow 表示 UA 解析出的 Codex 引擎版本低于最低要求。
	CodexClientRestrictionReasonVersionTooLow = "codex_version_too_low"
	// CodexClientRestrictionReasonMissingEngineFingerprint 表示 strict 指纹门下缺少 codex 引擎指纹头。
	CodexClientRestrictionReasonMissingEngineFingerprint = "missing_engine_fingerprint"
	// CodexClientRestrictionReasonVersionUndetectable 表示 codex_cli_only 下无法从 UA 解析出 Codex 引擎版本。
	CodexClientRestrictionReasonVersionUndetectable = "codex_version_undetectable"
	// CodexClientRestrictionReasonVersionTooHigh 表示 UA 解析出的 Codex 引擎版本高于最高允许版本。
	CodexClientRestrictionReasonVersionTooHigh = "codex_version_too_high"
	// CodexClientRestrictionReasonMatchedAppServerClient 表示 App Server 开关开启时对未列名客户端开闸放行（仍过引擎门）。
	CodexClientRestrictionReasonMatchedAppServerClient = "app_server_client_matched"
)

// CodexRestrictionPolicy 是 codex_cli_only 判定所需的全局策略快照，由调用方从全局设置解析注入（global-only）。
// 账号侧只有 codex_cli_only 开关本身；黑/白名单、最低版本、指纹门均为全局设置。
type CodexRestrictionPolicy struct {
	Whitelist                []openai.AllowedClientEntry      // 全局自由白名单（双因子 AND，放行官方集未覆盖的 app-server client）
	Blacklist                []openai.AllowedClientEntry      // 全局自由黑名单（OR，宽 deny）
	MinCodexVersion          string                           // 最低 Codex 引擎版本 semver；""=不校验
	MaxCodexVersion          string                           // 最高 Codex 引擎版本 semver；""=不校验
	AllowAppServerClients    bool                             // App Server 开关：对未列名客户端开闸（仍受引擎门约束）
	EngineFingerprintSignals []openai.EngineFingerprintSignal // 引擎指纹门信号列表（勾选 AND / 行内变体 OR）；缺省=默认种子(只勾 x-codex-)
}

// CodexClientRestrictionDetectionResult 是 codex_cli_only 统一检测入口结果。
type CodexClientRestrictionDetectionResult struct {
	Enabled bool
	Matched bool
	Reason  string
}

// CodexClientRestrictionDetector 定义 codex_cli_only 统一检测入口。
type CodexClientRestrictionDetector interface {
	Detect(c *gin.Context, account *Account, policy CodexRestrictionPolicy, body []byte) CodexClientRestrictionDetectionResult
}

// OpenAICodexClientRestrictionDetector 为 OpenAI OAuth codex_cli_only 的默认实现。
type OpenAICodexClientRestrictionDetector struct {
	cfg *config.Config
}

func NewOpenAICodexClientRestrictionDetector(cfg *config.Config) *OpenAICodexClientRestrictionDetector {
	return &OpenAICodexClientRestrictionDetector{cfg: cfg}
}

// Detect 门控顺序（每步可短路）：
//  1. 账号未开 codex_cli_only → 不限制（Disabled）。
//  2. gateway.force_codex_cli → 全局旁路放行（ForceCodexCLI）。
//  3. 黑名单命中 → 立即拒（门内 deny 最先，OR 语义）。
//  4. 身份候选：官方 UA / 官方 originator / 全局白名单 / App Server 开闸（全局开关 OR 账号开关）；都不命中 → 拒（NotMatchedUA）。
//  5. Codex 版本（仅官方候选）：版本必须可解析（否则 VersionUndetectable）；< Min → 拒（TooLow）；> Max → 拒（TooHigh）。
//  6. 引擎指纹 AND 硬门：按 EngineFingerprintSignals 列表勾选 AND 判定（无任何 Required 信号→放行，即「关闭指纹门」=取消所有勾选）；白名单条目可显式 skip。
func (d *OpenAICodexClientRestrictionDetector) Detect(c *gin.Context, account *Account, policy CodexRestrictionPolicy, body []byte) CodexClientRestrictionDetectionResult {
	if account == nil || !account.IsCodexCLIOnlyEnabled() {
		return CodexClientRestrictionDetectionResult{Enabled: false, Matched: false, Reason: CodexClientRestrictionReasonDisabled}
	}

	if d != nil && d.cfg != nil && d.cfg.Gateway.ForceCodexCLI {
		return CodexClientRestrictionDetectionResult{Enabled: true, Matched: true, Reason: CodexClientRestrictionReasonForceCodexCLI}
	}

	userAgent := ""
	originator := ""
	var header http.Header
	if c != nil {
		userAgent = c.GetHeader("User-Agent")
		originator = c.GetHeader("originator")
		if c.Request != nil {
			header = c.Request.Header
		}
	}

	// 3. 黑名单优先（门内 deny 最先，OR：任一已声明字段命中即拒）。
	if openai.MatchDenyEntries(userAgent, originator, policy.Blacklist) {
		return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonBlacklisted}
	}

	// 4. 身份候选（优先级：官方 > 全局白名单 > App Server 开闸：全局开关 OR 账号开关）。
	reason := ""
	skipFingerprint := false
	switch {
	case openai.IsCodexOfficialClientRequestStrict(userAgent):
		reason = CodexClientRestrictionReasonMatchedUA
	case openai.IsCodexOfficialClientOriginator(originator):
		reason = CodexClientRestrictionReasonMatchedOriginator
	default:
		if entry, ok := openai.MatchClientEntry(userAgent, originator, policy.Whitelist); ok {
			reason = CodexClientRestrictionReasonMatchedWhitelistClient
			skipFingerprint = entry.SkipEngineFingerprint
		} else if policy.AllowAppServerClients || account.IsCodexCLIOnlyAppServerAllowed() {
			reason = CodexClientRestrictionReasonMatchedAppServerClient
		}
	}
	if reason == "" {
		return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonNotMatchedUA}
	}

	// 5. Codex 版本：仅对官方候选（官方 UA / 官方 originator）。版本必须可识别（需求②），再校验 [min,max]。
	//    白名单/账号预设/App Server 候选可能不带可解析引擎版本，整块跳过。
	if reason == CodexClientRestrictionReasonMatchedUA || reason == CodexClientRestrictionReasonMatchedOriginator {
		ver, ok := openai.ParseCodexEngineVersion(userAgent)
		if !ok {
			return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonVersionUndetectable}
		}
		if policy.MinCodexVersion != "" && CompareVersions(ver, policy.MinCodexVersion) < 0 {
			return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonVersionTooLow}
		}
		if policy.MaxCodexVersion != "" && CompareVersions(ver, policy.MaxCodexVersion) > 0 {
			return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonVersionTooHigh}
		}
	}

	// 6. 引擎指纹 AND 硬门。对所有候选生效;唯一例外:命中的白名单条目显式 SkipEngineFingerprint。
	//    按全局信号列表判定:所有勾选(Required)信号都命中即放行,每条命中任一变体即满足(行内 OR);
	//    无任何勾选信号 → 视为无要求放行(即「关闭指纹门」=取消所有勾选)。ForceCodexCLI 与黑名单不经此门。
	if !skipFingerprint {
		if !openai.EvaluateEngineFingerprint(header, body, policy.EngineFingerprintSignals) {
			return CodexClientRestrictionDetectionResult{Enabled: true, Matched: false, Reason: CodexClientRestrictionReasonMissingEngineFingerprint}
		}
	}

	return CodexClientRestrictionDetectionResult{Enabled: true, Matched: true, Reason: reason}
}
