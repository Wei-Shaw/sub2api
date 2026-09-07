package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

// kiroDefaultRegion 与 pkg/kiro 的默认区域保持一致。
const kiroDefaultRegion = "us-east-1"

// KiroAuthMethod 返回账号的凭证接入方式。缺省为 social。
func (a *Account) KiroAuthMethod() kiro.AuthMethod {
	if a == nil {
		return kiro.AuthSocial
	}
	return kiro.ParseAuthMethod(a.GetCredential("auth_method"))
}

// IsKiroAPIKeyAccount 判断是否为 API Key 账号。
// 这类账号走 Kiro CLI runtime 端点、带 tokentype 头、且不使用 profileArn。
func (a *Account) IsKiroAPIKeyAccount() bool {
	return a.KiroAuthMethod() == kiro.AuthAPIKey
}

// KiroRegion 返回账号所属区域，缺省 us-east-1。
func (a *Account) KiroRegion() string {
	if a == nil {
		return kiroDefaultRegion
	}
	if region := strings.TrimSpace(a.GetCredential("region")); region != "" {
		return region
	}
	return kiroDefaultRegion
}

// KiroProfileArn 返回 profileArn。刷新 token 时必须回写此字段，
// 否则账号运行一段时间后会 403。
func (a *Account) KiroProfileArn() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("profile_arn"))
}

// KiroAPIKey 返回 API Key 账号的密钥。
func (a *Account) KiroAPIKey() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("api_key"))
}

// KiroAccessToken 返回当前的访问令牌。
func (a *Account) KiroAccessToken() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("access_token"))
}

// KiroRefreshToken 返回刷新令牌。
func (a *Account) KiroRefreshToken() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("refresh_token"))
}

// KiroClientCredentials 返回 OIDC 动态注册得到的客户端凭据。
// 仅 builder_id / idc 两种方式有值。
func (a *Account) KiroClientCredentials() (string, string) {
	if a == nil {
		return "", ""
	}
	return strings.TrimSpace(a.GetCredential("client_id")),
		strings.TrimSpace(a.GetCredential("client_secret"))
}

// KiroIssuerURL 返回 SSO 门户地址（IdC 为组织自有 start URL）。
func (a *Account) KiroIssuerURL() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("issuer_url"))
}

// KiroMachineID 返回设备指纹。参与 User-Agent 构造，必须稳定。
func (a *Account) KiroMachineID() string {
	if a == nil {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("machine_id"))
}

// KiroBearerToken 返回用于 Authorization 头的令牌。
// API Key 账号用 api_key，OAuth 账号用 access_token。
func (a *Account) KiroBearerToken() string {
	if a == nil {
		return ""
	}
	if a.IsKiroAPIKeyAccount() {
		if key := a.KiroAPIKey(); key != "" {
			return key
		}
	}
	return a.KiroAccessToken()
}

// KiroFakeThinking 返回是否为该账号启用假思考。默认开启 ——
// 客户端携带的推理强度诉求（thinking.budget_tokens / output_config.effort）
// 必须在 Kiro 端真实生效，不能因为没有显式配置就被这道总闸静默挡住、让
// 客户端的请求诉求凭空消失。代价是会往每个请求注入数百 token 的指令，且
// 产出的是模型自写文本而非真 reasoning（见 kiroFakeThinkingPlan 文档）——
// 管理员需要为不接受这个代价的账号显式设置 fake_thinking:false 关闭。
func (a *Account) KiroFakeThinking() bool {
	if a == nil {
		return false
	}
	raw, ok := a.Credentials["fake_thinking"]
	if !ok {
		return true
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return true
		}
		return parsed
	default:
		return true
	}
}

// GenerateKiroMachineID 生成一个新的设备指纹，形态与 Kiro IDE 一致
// （64 位十六进制）。
func GenerateKiroMachineID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate kiro machine id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// EnsureKiroMachineID 保证 creds 里有 machine_id，返回该值与「是否本次新建」。
//
// 返回 true 时调用方**必须**把 creds 落库 —— machine_id 参与上游的设备指纹，
// 每次请求重新生成等于每次都是新设备，有触发风控的风险。
// 生成失败时返回空串与 false，调用方应降级为不带 machineId 的 User-Agent。
func EnsureKiroMachineID(creds map[string]any) (string, bool) {
	if creds == nil {
		return "", false
	}
	if existing, ok := creds["machine_id"].(string); ok {
		if trimmed := strings.TrimSpace(existing); trimmed != "" {
			return trimmed, false
		}
	}

	id, err := GenerateKiroMachineID()
	if err != nil {
		return "", false
	}
	creds["machine_id"] = id
	return id, true
}

// KiroTokenCacheKey 返回分布式刷新锁使用的缓存键，形态对齐其他平台。
func KiroTokenCacheKey(account *Account) string {
	if account == nil {
		return "kiro:account:0"
	}
	return "kiro:account:" + strconv.FormatInt(account.ID, 10)
}
