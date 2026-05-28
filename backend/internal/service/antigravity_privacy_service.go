package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
)

const (
	AntigravityPrivacySet    = "privacy_set"
	AntigravityPrivacyFailed = "privacy_set_failed"
)

// setAntigravityPrivacy 调用 Antigravity API 设置隐私并验证结果。
// 流程：
//  1. setUserSettings 清空设置 → 检查返回值 {"userSettings":{}}
//  2. fetchUserInfo 二次验证隐私是否已生效（需要 project_id）
//
// 返回 privacy_mode 值："privacy_set" 成功，"privacy_set_failed" 失败，空串表示无法执行。
func setAntigravityPrivacy(ctx context.Context, accessToken, projectID, proxyURL string) string {
	return setAntigravityPrivacyDetailed(ctx, accessToken, projectID, proxyURL).Mode
}

func setAntigravityPrivacyDetailed(ctx context.Context, accessToken, projectID, proxyURL string) PrivacySetResult {
	if accessToken == "" {
		return PrivacySetResult{
			Reason:  "PRIVACY_SET_NOT_EXECUTABLE",
			Message: "Cannot set privacy: missing access token",
			Stage:   "precheck",
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	client, err := antigravity.NewClient(proxyURL)
	if err != nil {
		slog.Warn("antigravity_privacy_client_error", "error", err.Error())
		return PrivacySetResult{
			Mode:    AntigravityPrivacyFailed,
			Reason:  "PRIVACY_CLIENT_ERROR",
			Message: "Failed to create privacy API client",
			Stage:   "client",
			Detail:  truncate(err.Error(), 300),
		}
	}

	// 第 1 步：调用 setUserSettings，检查返回值
	setResp, err := client.SetUserSettings(ctx, accessToken)
	if err != nil {
		slog.Warn("antigravity_privacy_set_failed", "error", err.Error())
		return PrivacySetResult{
			Mode:    AntigravityPrivacyFailed,
			Reason:  "PRIVACY_REQUEST_ERROR",
			Message: "setUserSettings request failed",
			Stage:   "set_user_settings",
			Detail:  truncate(err.Error(), 300),
		}
	}
	if !setResp.IsSuccess() {
		slog.Warn("antigravity_privacy_set_response_not_empty",
			"user_settings", setResp.UserSettings,
		)
		return PrivacySetResult{
			Mode:    AntigravityPrivacyFailed,
			Reason:  "PRIVACY_SET_RESPONSE_INVALID",
			Message: "setUserSettings did not clear privacy settings",
			Stage:   "set_user_settings",
			Detail:  truncate(fmt.Sprintf("userSettings=%v", setResp.UserSettings), 300),
		}
	}

	// 第 2 步：调用 fetchUserInfo 二次验证隐私是否已生效
	if strings.TrimSpace(projectID) == "" {
		slog.Warn("antigravity_privacy_missing_project_id")
		return PrivacySetResult{
			Mode:    AntigravityPrivacyFailed,
			Reason:  "PRIVACY_MISSING_PROJECT_ID",
			Message: "Cannot verify privacy setting: missing project_id",
			Stage:   "verify",
		}
	}
	userInfo, err := client.FetchUserInfo(ctx, accessToken, projectID)
	if err != nil {
		slog.Warn("antigravity_privacy_verify_failed", "error", err.Error())
		return PrivacySetResult{
			Mode:    AntigravityPrivacyFailed,
			Reason:  "PRIVACY_VERIFY_REQUEST_ERROR",
			Message: "fetchUserInfo verification request failed",
			Stage:   "verify",
			Detail:  truncate(err.Error(), 300),
		}
	}
	if !userInfo.IsPrivate() {
		slog.Warn("antigravity_privacy_verify_not_private",
			"user_settings", userInfo.UserSettings,
		)
		return PrivacySetResult{
			Mode:    AntigravityPrivacyFailed,
			Reason:  "PRIVACY_VERIFY_NOT_PRIVATE",
			Message: "Privacy verification still shows telemetry settings enabled",
			Stage:   "verify",
			Detail:  truncate(fmt.Sprintf("userSettings=%v", userInfo.UserSettings), 300),
		}
	}

	slog.Info("antigravity_privacy_set_success")
	return PrivacySetResult{
		Mode:    AntigravityPrivacySet,
		Success: true,
		Message: "Telemetry and marketing email settings disabled",
		Stage:   "verify",
	}
}

func applyAntigravityPrivacyMode(account *Account, mode string) {
	if account == nil || strings.TrimSpace(mode) == "" {
		return
	}
	extra := make(map[string]any, len(account.Extra)+1)
	for k, v := range account.Extra {
		extra[k] = v
	}
	extra["privacy_mode"] = mode
	account.Extra = extra
}
