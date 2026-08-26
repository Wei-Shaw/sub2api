package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	kiropkg "github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
	"github.com/google/uuid"
)

// Kiro 默认 profileArn 常量（与 kiro.rs-admin 保持一致）。
// BuilderID 占位符：纯 BuilderID 账号没有真实 profile，上游 IDE 发送此占位符。
// Social 共享 ARN：Social 登录账号使用此共享 ARN。
const (
	kiroBuilderIDProfileARN = "arn:aws:codewhisperer:us-east-1:638616132270:profile/AAAACCCCXXXX"
	kiroSocialProfileARN    = "arn:aws:codewhisperer:us-east-1:699475941385:profile/EHGA3GRVQMUK"
)

// kiroIsPlaceholderProfileARN 判断给定 ARN 是否为 BuilderID 占位符（非真实可用的 profile）。
func kiroIsPlaceholderProfileARN(arn string) bool {
	return arn == kiroBuilderIDProfileARN
}

// kiroIsSocialLogin 判断账号是否为 Social 登录方式。
func kiroIsSocialLogin(account *Account) bool {
	if account == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(account.GetCredential("auth_method")), "social")
}

// kiroDefaultProfileARN 返回凭据缺少显式 profileArn 时应使用的默认 ARN：
// Social 登录 → kiroSocialProfileARN，其余（BuilderID 等）→ kiroBuilderIDProfileARN。
func kiroDefaultProfileARN(account *Account) string {
	if kiroIsSocialLogin(account) {
		return kiroSocialProfileARN
	}
	return kiroBuilderIDProfileARN
}

// kiroAccountLacksEnterpriseProfile 判断账号是否不存在企业 profile。
//
// BuilderId（个人 Builder ID）与 Enterprise（企业自建 IAM Identity Center）的 auth_method
// 都是 "idc"，无法据此区分，只能按 provider 判断。provider 取值受白名单约束
// （Google/Github/BuilderId/Enterprise/ExternalIdp，见 kiropkg.IsValidKiroProvider），
// 企业账号必为 kiropkg.ProviderEnterprise。
//
// 无企业 profile 的账号调用 ListAvailableProfiles 必然返回
// 403 AccessDeniedException: "AWS Builder ID is not supported for this operation."，
// 直接回填默认 ARN 即可，无需发起注定失败的请求。
//
// 判定为 allow-list：仅当存在企业身份证据（provider 为 Enterprise/ExternalIdp，
// 或 start_url 指向自建 IAM Identity Center）时才返回 false 去查询；
// 其余（BuilderId、Social、以及早期兜底写入 "AWS" 这类非白名单遗留值）一律返回 true 跳过。
func kiroAccountLacksEnterpriseProfile(account *Account) bool {
	if account == nil {
		return false
	}
	// ExternalIdp（企业外部 IdP）可能持有真实 profile，需要查询。
	provider := strings.TrimSpace(account.GetCredential("provider"))
	authMethod := strings.TrimSpace(account.GetCredential("auth_method"))
	if strings.EqualFold(provider, kiropkg.ProviderExternalIdp) || strings.EqualFold(authMethod, "external_idp") {
		return false
	}
	// Enterprise：企业自建 IAM Identity Center，需要查询真实 profile ARN。
	if strings.EqualFold(provider, kiropkg.ProviderEnterprise) {
		return false
	}
	// 白名单内的 BuilderId/Google/Github 明确无企业 profile。provider 经白名单校验，
	// 权威性高于 start_url，即便 start_url 看似自建也直接跳过。
	if strings.EqualFold(provider, kiropkg.ProviderBuilderId) ||
		strings.EqualFold(provider, kiropkg.ProviderGoogle) ||
		strings.EqualFold(provider, kiropkg.ProviderGithub) ||
		kiroIsSocialLogin(account) {
		return true
	}
	// provider 为遗留非白名单值（早期兜底的 "AWS" 等）时，自建 start_url 仍是企业身份的有效证据。
	// 容忍末尾斜杠差异，语义对齐 kiropkg.resolveIDCProvider。
	startURL := strings.TrimRight(strings.TrimSpace(account.GetCredential("start_url")), "/")
	if startURL != "" && !strings.EqualFold(startURL, strings.TrimRight(kiropkg.BuilderIDStartURL, "/")) {
		return false
	}
	return true
}

func buildKiroAccountKey(account *Account) string {
	if account == nil {
		return ""
	}
	return kiropkg.BuildAccountKey(
		account.GetCredential("client_id"),
		account.GetCredential("client_id_hash"),
		account.GetCredential("refresh_token"),
		account.GetCredential("profile_arn"),
		account.ID,
	)
}

func buildKiroMachineID(account *Account) string {
	if account == nil {
		return kiropkg.BuildMachineID("", "", "account:nil")
	}
	for _, key := range []string{"machine_id", "machineId"} {
		if machineID, ok := kiropkg.NormalizeMachineID(account.GetCredential(key)); ok {
			return machineID
		}
	}
	fallbackKey := buildKiroMachineIDFallbackKey(account)
	if account.Type == AccountTypeAPIKey {
		return kiropkg.BuildMachineID("", firstKiroCredential(account, "kiro_api_key", "kiroApiKey", "api_key"), fallbackKey)
	}
	return kiropkg.BuildMachineID(account.GetCredential("refresh_token"), "", fallbackKey)
}

func firstKiroCredential(account *Account, keys ...string) string {
	if account == nil {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
			return value
		}
	}
	return ""
}

func buildKiroMachineIDFallbackKey(account *Account) string {
	if account == nil {
		return "account:nil"
	}
	if account.ID > 0 {
		return fmt.Sprintf("account:%d", account.ID)
	}
	for _, key := range []string{"client_id", "profile_arn"} {
		if value := strings.TrimSpace(account.GetCredential(key)); value != "" {
			return key + ":" + value
		}
	}
	if name := strings.TrimSpace(account.Name); name != "" {
		return "name:" + name
	}
	return "account:unknown"
}

func buildKiroRequestID(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	if requestID := strings.TrimSpace(resp.Header.Get("x-request-id")); requestID != "" {
		return requestID
	}
	if requestID := strings.TrimSpace(resp.Header.Get("x-amzn-requestid")); requestID != "" {
		return requestID
	}
	return strings.TrimSpace(resp.Header.Get("x-amz-request-id"))
}

func isKiroSuspendedBody(respBody []byte) bool {
	body := string(respBody)
	return strings.Contains(body, "SUSPENDED") || strings.Contains(body, "TEMPORARILY_SUSPENDED")
}

func isKiroTokenErrorBody(respBody []byte) bool {
	lower := strings.ToLower(string(respBody))
	return strings.Contains(lower, "token") ||
		strings.Contains(lower, "expired") ||
		strings.Contains(lower, "invalid") ||
		strings.Contains(lower, "unauthorized")
}

func kiroProxyURL(account *Account) string {
	if account != nil && account.ProxyID != nil && account.Proxy != nil {
		return account.Proxy.URL()
	}
	return ""
}

// isKiroDirectModeAccount 判断账号是否走 Kiro 直连 AWS 模式。
// - OAuth 账号:直连 AWS(q.{region}.amazonaws.com 或 KRS),走 forwardKiroMessages。
// - API Key 账号:
//   - base_url 为空 → 直连 AWS(ksk_ + tokentype: API_KEY),走 forwardKiroMessages。
//   - base_url 非空 → 外部 Anthropic 兼容中转,返回 false,落回通用 buildUpstreamRequest
//     反代路径(请求 {base_url}/v1/messages,发 x-api-key),作为分组兜底/灾备账号。
func isKiroDirectModeAccount(account *Account) bool {
	if account == nil || account.Platform != PlatformKiro {
		return false
	}
	if account.Type == AccountTypeOAuth {
		return true
	}
	if account.Type == AccountTypeAPIKey {
		return strings.TrimSpace(account.GetCredential("base_url")) == ""
	}
	return false
}

func kiroAPIRegion(account *Account) string {
	if account == nil {
		return kiroDefaultRegion
	}
	region := strings.TrimSpace(account.GetCredential("api_region"))
	if region == "" {
		region = kiroDefaultRegion
	}
	return region
}

func applyKiroConditionalHeaders(req *http.Request, account *Account) {
	if req == nil || account == nil {
		return
	}
	if account.Type == AccountTypeAPIKey {
		req.Header["TokenType"] = []string{"API_KEY"}
		return
	}
	if strings.EqualFold(strings.TrimSpace(account.GetCredential("auth_method")), "external_idp") {
		req.Header.Set("TokenType", "EXTERNAL_IDP")
	}
}

func resolveKiroPayloadProfileArn(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetCredential("profile_arn"))
}

// kiroResolveProfileArnForKRS 返回 KRS endpoint 所需的 profileArn。
// KRS endpoint（runtime.us-east-1.kiro.dev）强制要求 profileArn，
// 凭据无值时 fallback 到默认 ARN（Social → Social ARN，其余 → BuilderID 占位符）。
func kiroResolveProfileArnForKRS(account *Account) string {
	arn := resolveKiroPayloadProfileArn(account)
	if arn != "" {
		return arn
	}
	return kiroDefaultProfileARN(account)
}

// kiroResolveRequestProfileArn 返回直连 AWS 请求（getUsageLimits /
// generateAssistantResponse，Q 与 KRS 端点）应携带的 profileArn。
//
// 上游现在对所有端点强制要求 profileArn：缺失 → 403 "User is not authorized to
// make this call."（或 getUsageLimits 的 400 "profileArn is required"）。
// API Key 凭据没有 profile 概念（与 kiroResolveAndPersistProfileArn 一致），返回空；
// 其余账号走 kiroResolveProfileArnForKRS（凭据真实 ARN > Social ARN > Builder ID 占位符）。
func kiroResolveRequestProfileArn(account *Account) string {
	if account != nil && account.Type == AccountTypeAPIKey {
		return ""
	}
	return kiroResolveProfileArnForKRS(account)
}

func newKiroJSONRequest(ctx context.Context, endpointURL string, payload []byte, token, accountKey, machineID, amzTarget string, account *Account) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Authorization", "Bearer "+token)
	// KRS endpoint 用 Kiro IDE 实际 UA 格式 ("KiroIDE <version> <machineId>")；
	// AWS Q endpoint 继续用 AWS SDK 风格 UA。按 URL 区分以避免再加一个参数。
	if endpointURL == kiroKRSEndpointURL {
		req.Header.Set("User-Agent", kiropkg.BuildKiroIDERuntimeUserAgent(accountKey, machineID))
	} else {
		req.Header.Set("User-Agent", kiropkg.BuildRuntimeUserAgent(accountKey, machineID))
	}
	req.Header.Set("X-Amz-User-Agent", kiropkg.BuildRuntimeAmzUserAgent(accountKey, machineID))
	req.Header.Set("x-amzn-kiro-agent-mode", "vibe")
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
	req.Header.Set("Amz-Sdk-Request", "attempt=1; max=3")
	req.Header.Set("Amz-Sdk-Invocation-Id", uuid.NewString())
	if amzTarget != "" {
		req.Header.Set("X-Amz-Target", amzTarget)
	}
	if account != nil {
		profileArn := resolveKiroPayloadProfileArn(account)
		if profileArn == "" && endpointURL == kiroKRSEndpointURL {
			profileArn = kiroResolveProfileArnForKRS(account)
		}
		if profileArn != "" {
			req.Header.Set("x-amzn-kiro-profile-arn", profileArn)
		}
	}
	applyKiroConditionalHeaders(req, account)
	return req, nil
}
