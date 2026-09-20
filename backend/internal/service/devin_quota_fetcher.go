package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/devin"
)

// DevinQuota 是 Devin 账号额度快照（由 SeatManagementService/GetUserStatus
// 解码而来）。credits 字段 -1 为 unlimited 哨兵。
type DevinQuota struct {
	PlanName           string `json:"plan_name,omitempty"`
	AccountDisplayName string `json:"account_display_name,omitempty"`
	OrgID              string `json:"org_id,omitempty"`
	TeamID             string `json:"team_id,omitempty"`
	IsEnterprise       bool   `json:"is_enterprise,omitempty"`
	CanUseCLI          bool   `json:"can_use_cli,omitempty"`

	MonthlyPromptCredits   int64 `json:"monthly_prompt_credits,omitempty"`
	MonthlyFlowCredits     int64 `json:"monthly_flow_credits,omitempty"`
	AvailablePromptCredits int64 `json:"available_prompt_credits,omitempty"`
	AvailableFlowCredits   int64 `json:"available_flow_credits,omitempty"`
	AvailableFlexCredits   int64 `json:"available_flex_credits,omitempty"`
	UsedPromptCredits      int64 `json:"used_prompt_credits,omitempty"`
	UsedFlowCredits        int64 `json:"used_flow_credits,omitempty"`
	UsedFlexCredits        int64 `json:"used_flex_credits,omitempty"`

	DailyQuotaRemainingPercent  int64      `json:"daily_quota_remaining_percent,omitempty"`
	WeeklyQuotaRemainingPercent int64      `json:"weekly_quota_remaining_percent,omitempty"`
	DailyQuotaResetAt           *time.Time `json:"daily_quota_reset_at,omitempty"`
	WeeklyQuotaResetAt          *time.Time `json:"weekly_quota_reset_at,omitempty"`

	ACUConsumed float64 `json:"acu_consumed,omitempty"`
	ACULimit    float64 `json:"acu_limit,omitempty"`
}

// DevinQuotaFetcher 从 Devin Connect API 获取账号额度。
type DevinQuotaFetcher struct {
	proxyRepo ProxyRepository
	upstream  HTTPUpstream
}

// NewDevinQuotaFetcher 创建 DevinQuotaFetcher。
func NewDevinQuotaFetcher(proxyRepo ProxyRepository, upstream HTTPUpstream) *DevinQuotaFetcher {
	return &DevinQuotaFetcher{proxyRepo: proxyRepo, upstream: upstream}
}

// CanFetch 检查是否可以获取此账户的额度。
func (f *DevinQuotaFetcher) CanFetch(account *Account) bool {
	if account == nil || account.Platform != PlatformDevin {
		return false
	}
	return account.GetDevinToken() != ""
}

// GetProxyURL 解析账号绑定的代理地址。
func (f *DevinQuotaFetcher) GetProxyURL(ctx context.Context, account *Account) string {
	if account.ProxyID == nil || f.proxyRepo == nil {
		return ""
	}
	proxy, err := f.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.URL()
}

// FetchQuota 调用 GetUserStatus 取回额度快照。
func (f *DevinQuotaFetcher) FetchQuota(ctx context.Context, account *Account, proxyURL string) (*QuotaResult, error) {
	token := account.GetDevinToken()
	if token == "" {
		return nil, errors.New("devin account has no access_token credential")
	}
	baseURL := account.GetDevinBaseURL()
	if baseURL == "" {
		baseURL = devin.DefaultBaseURL
	}
	version := account.GetDevinClientVersion()
	if version == "" {
		version = devin.DefaultClientVersion
	}

	body := devin.MarshalGetUserStatus(token, version, devin.ClientOS())
	req, err := devin.NewUnaryRequest(baseURL, devin.PathGetUserStatus, token, body)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)
	resp, err := f.upstream.Do(req, proxyURL, account.ID, 0)
	if err != nil {
		return nil, fmt.Errorf("devin GetUserStatus: %w", err)
	}
	raw, err := devin.ReadUnaryResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("devin GetUserStatus: %w", err)
	}
	status := devin.DecodeUserStatus(raw)
	if status == nil {
		return nil, errors.New("devin GetUserStatus: empty response")
	}

	now := time.Now()
	usage := &UsageInfo{
		UpdatedAt: &now,
		DevinQuota: &DevinQuota{
			PlanName:           status.PlanName,
			AccountDisplayName: status.AccountDisplayName,
			OrgID:              status.OrgID,
			TeamID:             status.TeamID,
			IsEnterprise:       status.IsEnterprise,
			CanUseCLI:          status.CanUseCLI,

			MonthlyPromptCredits:   status.MonthlyPromptCredits,
			MonthlyFlowCredits:     status.MonthlyFlowCredits,
			AvailablePromptCredits: status.AvailablePromptCredits,
			AvailableFlowCredits:   status.AvailableFlowCredits,
			AvailableFlexCredits:   status.AvailableFlexCredits,
			UsedPromptCredits:      status.UsedPromptCredits,
			UsedFlowCredits:        status.UsedFlowCredits,
			UsedFlexCredits:        status.UsedFlexCredits,

			DailyQuotaRemainingPercent:  status.DailyQuotaRemainingPercent,
			WeeklyQuotaRemainingPercent: status.WeeklyQuotaRemainingPercent,
			DailyQuotaResetAt:           unixToTimePtr(status.DailyQuotaResetAtUnix),
			WeeklyQuotaResetAt:          unixToTimePtr(status.WeeklyQuotaResetAtUnix),

			ACUConsumed: status.ACUConsumed,
			ACULimit:    status.ACULimit,
		},
	}

	rawMap := map[string]any{}
	if json.Unmarshal(raw, &rawMap) != nil {
		// protobuf 不能直接 json.Unmarshal——Raw 放结构化摘要即可。
		rawMap = map[string]any{}
	}
	rawMap["plan_name"] = status.PlanName
	rawMap["org_id"] = status.OrgID
	rawMap["can_use_cli"] = status.CanUseCLI

	return &QuotaResult{UsageInfo: usage, Raw: rawMap}, nil
}

// unixToTimePtr 把 unix 秒转 *time.Time；<=0 视为缺省。
func unixToTimePtr(sec int64) *time.Time {
	if sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}
