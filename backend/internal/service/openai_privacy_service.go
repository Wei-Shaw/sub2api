package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/imroc/req/v3"
)

// PrivacyClientFactory creates an HTTP client for privacy API calls.
// Injected from repository layer to avoid import cycles.
type PrivacyClientFactory func(proxyURL string) (*req.Client, error)

const (
	openAISettingsURL = "https://chatgpt.com/backend-api/settings/account_user_setting"

	PrivacyModeTrainingOff = "training_off"
	PrivacyModeFailed      = "training_set_failed"
	PrivacyModeCFBlocked   = "training_set_cf_blocked"
)

func shouldSkipOpenAIPrivacyEnsure(extra map[string]any) bool {
	if extra == nil {
		return false
	}
	raw, ok := extra["privacy_mode"]
	if !ok {
		return false
	}
	mode, _ := raw.(string)
	mode = strings.TrimSpace(mode)
	return mode != PrivacyModeFailed && mode != PrivacyModeCFBlocked
}

// disableOpenAITraining calls ChatGPT settings API to turn off "Improve the model for everyone".
// Returns privacy_mode value: "training_off" on success, "cf_blocked" / "failed" on failure.
func disableOpenAITraining(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL string) string {
	if accessToken == "" || clientFactory == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := clientFactory(proxyURL)
	if err != nil {
		slog.Warn("openai_privacy_client_error", "error", err.Error())
		return PrivacyModeFailed
	}

	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/").
		SetHeader("Accept", "application/json").
		SetHeader("sec-fetch-mode", "cors").
		SetHeader("sec-fetch-site", "same-origin").
		SetHeader("sec-fetch-dest", "empty").
		SetQueryParam("feature", "training_allowed").
		SetQueryParam("value", "false").
		Patch(openAISettingsURL)

	if err != nil {
		slog.Warn("openai_privacy_request_error", "error", err.Error())
		return PrivacyModeFailed
	}

	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		body := resp.String()
		if strings.Contains(body, "cloudflare") || strings.Contains(body, "cf-") || strings.Contains(body, "Just a moment") {
			slog.Warn("openai_privacy_cf_blocked", "status", resp.StatusCode)
			return PrivacyModeCFBlocked
		}
	}

	if !resp.IsSuccessState() {
		slog.Warn("openai_privacy_failed", "status", resp.StatusCode, "body", truncate(resp.String(), 200))
		return PrivacyModeFailed
	}

	slog.Info("openai_privacy_training_disabled")
	return PrivacyModeTrainingOff
}

// ChatGPTAccountInfo 从 chatgpt.com/backend-api/accounts/check 获取的账号信息
type ChatGPTAccountInfo struct {
	PlanType              string
	Email                 string
	SubscriptionExpiresAt string // entitlement.expires_at (RFC3339)
}

const chatGPTAccountsCheckURL = "https://chatgpt.com/backend-api/accounts/check/v4-2023-04-27"

// fetchChatGPTAccountInfo calls ChatGPT backend-api to get account info (plan_type, etc.).
// Used as fallback when id_token doesn't contain these fields (e.g., Mobile RT).
// orgID is used to match the correct account when multiple accounts exist (e.g., personal + team).
// Returns nil on any failure (best-effort, non-blocking).
func fetchChatGPTAccountInfo(ctx context.Context, clientFactory PrivacyClientFactory, accessToken, proxyURL, orgID string) *ChatGPTAccountInfo {
	if accessToken == "" || clientFactory == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client, err := clientFactory(proxyURL)
	if err != nil {
		slog.Debug("chatgpt_account_check_client_error", "error", err.Error())
		return nil
	}

	var result map[string]any
	resp, err := client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+accessToken).
		SetHeader("Origin", "https://chatgpt.com").
		SetHeader("Referer", "https://chatgpt.com/").
		SetHeader("Accept", "application/json").
		SetSuccessResult(&result).
		Get(chatGPTAccountsCheckURL)

	if err != nil {
		slog.Debug("chatgpt_account_check_request_error", "error", err.Error())
		return nil
	}

	if !resp.IsSuccessState() {
		slog.Debug("chatgpt_account_check_failed", "status", resp.StatusCode, "body", truncate(resp.String(), 200))
		return nil
	}

	info := &ChatGPTAccountInfo{}

	accounts, ok := result["accounts"].(map[string]any)
	if !ok {
		slog.Debug("chatgpt_account_check_no_accounts", "body", truncate(resp.String(), 300))
		return nil
	}

	// 优先匹配 orgID 对应的账号（access_token JWT 中的 poid）
	if orgID != "" {
		if acctRaw, ok := accounts[orgID]; ok {
			if acct, ok := acctRaw.(map[string]any); ok {
				fillAccountInfo(info, acct)
			}
		}
	}

	// 未匹配到时，遍历所有账号：优先 is_default，次选非 free
	if info.PlanType == "" {
		type candidate struct {
			planType  string
			expiresAt string
		}
		var defaultC, paidC, anyC candidate
		for _, acctRaw := range accounts {
			acct, ok := acctRaw.(map[string]any)
			if !ok {
				continue
			}
			planType := extractPlanType(acct)
			if planType == "" {
				continue
			}
			ea := extractEntitlementExpiresAt(acct)
			if anyC.planType == "" {
				anyC = candidate{planType, ea}
			}
			if account, ok := acct["account"].(map[string]any); ok {
				if isDefault, _ := account["is_default"].(bool); isDefault {
					defaultC = candidate{planType, ea}
				}
			}
			if !strings.EqualFold(planType, "free") && paidC.planType == "" {
				paidC = candidate{planType, ea}
			}
		}
		// 优先级：default > 非 free > 任意
		switch {
		case defaultC.planType != "":
			info.PlanType, info.SubscriptionExpiresAt = defaultC.planType, defaultC.expiresAt
		case paidC.planType != "":
			info.PlanType, info.SubscriptionExpiresAt = paidC.planType, paidC.expiresAt
		default:
			info.PlanType, info.SubscriptionExpiresAt = anyC.planType, anyC.expiresAt
		}
	}

	if info.PlanType == "" {
		slog.Debug("chatgpt_account_check_no_plan_type", "body", truncate(resp.String(), 300))
		return nil
	}

	slog.Info("chatgpt_account_check_success", "plan_type", info.PlanType, "subscription_expires_at", info.SubscriptionExpiresAt, "org_id", orgID)
	return info
}

// fillAccountInfo 从单个 account 对象中提取 plan_type 和 subscription_expires_at
func fillAccountInfo(info *ChatGPTAccountInfo, acct map[string]any) {
	info.PlanType = extractPlanType(acct)
	info.SubscriptionExpiresAt = extractEntitlementExpiresAt(acct)
}

// extractPlanType 从单个 account 对象中提取 plan_type
func extractPlanType(acct map[string]any) string {
	if account, ok := acct["account"].(map[string]any); ok {
		if planType, ok := account["plan_type"].(string); ok && planType != "" {
			return planType
		}
	}
	if entitlement, ok := acct["entitlement"].(map[string]any); ok {
		if subPlan, ok := entitlement["subscription_plan"].(string); ok && subPlan != "" {
			return subPlan
		}
	}
	return ""
}

// extractEntitlementExpiresAt 从 entitlement 中提取 expires_at。
// 预期为 RFC3339 字符串格式，如 "2026-05-02T20:32:12+00:00"。
func extractEntitlementExpiresAt(acct map[string]any) string {
	entitlement, ok := acct["entitlement"].(map[string]any)
	if !ok {
		return ""
	}
	ea, _ := entitlement["expires_at"].(string)
	return ea
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("...(%d more)", len(s)-n)
}
