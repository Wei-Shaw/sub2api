package service

import (
	"context"
	"fmt"
	"maps"
)

var openAIShadowUpstreamProfileExtraKeys = [...]string{
	openAILongContextBillingEnabledKey,
	"openai_device_id",
	codexFingerprintSeedExtraKey,
	"codex_fingerprint_mode",
	"openai_oauth_responses_websockets_v2_mode",
	"openai_oauth_responses_websockets_v2_enabled",
	"responses_websockets_v2_enabled",
	"openai_ws_enabled",
	"openai_ws_force_http",
	"openai_passthrough",
	"openai_oauth_passthrough",
	"openai_responses_flatten_namespaces",
	"codex_cli_only",
	"codex_cli_only_allowed_clients",
	"codex_cli_only_allow_app_server",
	"codex_image_generation_bridge",
	"codex_image_generation_bridge_enabled",
	"codex_image_generation_explicit_tool_policy",
	"openai_compact_mode",
	"openai_responses_mode",
}

var openAIShadowUpstreamProfileNestedExtraKeys = [...]string{
	"codex_image_generation_bridge",
	"codex_image_generation_bridge_enabled",
	"codex_image_generation_explicit_tool_policy",
}

// resolveCredentialAccount 解析影子账号到其母账号，用于凭据/Token 透传。
// - 普通账号（非影子）：直接返回自身。
// - 影子账号：通过 repo 取母账号，校验母账号存在且为 OpenAI OAuth 类型，否则返回错误。
// 设计为包级函数（非任何 service 的方法），以便 OpenAIGatewayService / OpenAIQuotaService /
// AccountUsageService 等不同接收者共享同一实现。
func resolveCredentialAccount(ctx context.Context, repo AccountRepository, account *Account) (*Account, error) {
	if account == nil || !account.IsShadow() {
		return account, nil
	}
	parent, err := repo.GetByID(ctx, *account.ParentAccountID)
	if err != nil {
		return nil, fmt.Errorf("resolve spark shadow parent %d: %w", *account.ParentAccountID, err)
	}
	if parent == nil {
		return nil, fmt.Errorf("spark shadow parent %d not found", *account.ParentAccountID)
	}
	// 防御:创建路径已禁二级影子(G6),此处再挡一层——畸形数据/手工 DB 写出的影子→影子链
	// 会让凭据解析停在无凭据的一级影子(只解一层),fail-closed 比静默返回坏母更安全(外审第6轮)。
	if parent.IsShadow() {
		return nil, fmt.Errorf("spark shadow parent %d is itself a shadow", parent.ID)
	}
	if !parent.IsOpenAIOAuth() {
		return nil, fmt.Errorf("spark shadow parent %d is not OpenAI OAuth", parent.ID)
	}
	return parent, nil
}

// InheritOpenAIShadowUpstreamProfile returns a request-scoped shadow copy with
// parent credentials and upstream settings while retaining shadow model mappings.
func InheritOpenAIShadowUpstreamProfile(shadow, parent *Account) *Account {
	projected := *shadow
	projected.Credentials = maps.Clone(parent.Credentials)
	if projected.Credentials == nil {
		projected.Credentials = make(map[string]any, 2)
	}
	for _, key := range []string{"model_mapping", "compact_model_mapping"} {
		delete(projected.Credentials, key)
		if value, ok := shadow.Credentials[key]; ok {
			projected.Credentials[key] = value
		}
	}

	projected.Extra = maps.Clone(shadow.Extra)
	if projected.Extra == nil {
		projected.Extra = make(map[string]any, len(openAIShadowUpstreamProfileExtraKeys))
	}
	for _, key := range openAIShadowUpstreamProfileExtraKeys {
		delete(projected.Extra, key)
		if key == openAILongContextBillingEnabledKey {
			projected.Extra[key] = parent.IsOpenAILongContextBillingEnabled()
			continue
		}
		if value, ok := parent.Extra[key]; ok {
			projected.Extra[key] = value
		}
	}

	shadowOpenAI, shadowHasOpenAI := shadow.Extra[PlatformOpenAI].(map[string]any)
	parentOpenAI, parentHasOpenAI := parent.Extra[PlatformOpenAI].(map[string]any)
	if shadowHasOpenAI || parentHasOpenAI {
		projectedOpenAI := maps.Clone(shadowOpenAI)
		if projectedOpenAI == nil {
			projectedOpenAI = make(map[string]any, len(openAIShadowUpstreamProfileNestedExtraKeys))
		}
		for _, key := range openAIShadowUpstreamProfileNestedExtraKeys {
			delete(projectedOpenAI, key)
			if value, ok := parentOpenAI[key]; ok {
				projectedOpenAI[key] = value
			}
		}
		if len(projectedOpenAI) == 0 {
			delete(projected.Extra, PlatformOpenAI)
		} else {
			projected.Extra[PlatformOpenAI] = projectedOpenAI
		}
	}

	return &projected
}

func effectiveOpenAIShadowUpstreamProfile(account *Account, lookup func(int64) *Account) *Account {
	if account == nil || !account.IsShadow() {
		return account
	}
	parent := lookup(*account.ParentAccountID)
	if parent == nil || parent.IsShadow() || !parent.IsOpenAIOAuth() {
		return nil
	}
	return InheritOpenAIShadowUpstreamProfile(account, parent)
}
