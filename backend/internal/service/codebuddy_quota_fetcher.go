package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/codebuddy"
)

// CodeBuddyQuotaFetcher 从 workbuddy.cn 计费接口获取 CodeBuddy 账号额度。
type CodeBuddyQuotaFetcher struct{}

// NewCodeBuddyQuotaFetcher 创建 CodeBuddyQuotaFetcher。
func NewCodeBuddyQuotaFetcher() *CodeBuddyQuotaFetcher {
	return &CodeBuddyQuotaFetcher{}
}

// CanFetch 检查是否可以获取此账户的额度。
func (f *CodeBuddyQuotaFetcher) CanFetch(account *Account) bool {
	if account.Platform != PlatformCodeBuddy {
		return false
	}
	// 使用 OAuth access_token（Bearer）即可拉取 workbuddy.cn 计费额度。
	return account.GetCodeBuddyAccessToken() != ""
}

// GetProxyURL 获取账户的代理 URL（CodeBuddy 计费接口可直连，无需代理时返回空字符串）。
func (f *CodeBuddyQuotaFetcher) GetProxyURL(_ context.Context, _ *Account) string {
	return ""
}

// FetchQuota 获取 CodeBuddy 账户额度信息。
func (f *CodeBuddyQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	accessToken := account.GetCodeBuddyAccessToken()
	userID := account.GetCredential("uid")
	if accessToken == "" {
		return nil, fmt.Errorf("codebuddy billing: missing access token (account %d needs to re-authorize)", account.ID)
	}
	if userID == "" {
		return nil, fmt.Errorf("codebuddy billing: empty user id (account %d is missing the 'uid' credential; please re-authorize the account)", account.ID)
	}

	raw, usage, err := codebuddy.GetUserResource(ctx, accessToken, userID, proxyURL)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	info := &UsageInfo{
		Source:    "active",
		UpdatedAt: &now,
		AICredits: []AICredit{
			{
				CreditType: "CodeBuddy Credits",
				Amount:     usage.Remaining,
			},
		},
		CodeBuddyUsage: &CodeBuddyUsageInfo{
			TotalCapacity: usage.TotalCapacity,
			Remaining:     usage.Remaining,
			Used:          usage.Used,
			AccountCount:  usage.AccountCount,
			TotalDosage:   raw.Data.Response.Data.TotalDosage,
		},
	}

	var rawMap map[string]any
	if raw != nil {
		// 保留原始响应以便排查。
		b, _ := json.Marshal(raw)
		_ = json.Unmarshal(b, &rawMap)
	}

	return &QuotaResult{
		UsageInfo: info,
		Raw:       rawMap,
	}, nil
}
