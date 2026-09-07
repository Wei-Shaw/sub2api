package kiro

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Bonus 是一份赠送额度。只有 status 为 ACTIVE 的才计入可用总额。
type Bonus struct {
	CurrentUsage float64 `json:"currentUsage"`
	UsageLimit   float64 `json:"usageLimit"`
	Status       string  `json:"status"`
}

// FreeTrialInfo 是免费试用信息。
type FreeTrialInfo struct {
	Status     string
	ExpiryDate *time.Time
}

// rawFreeTrialInfo 镜像上游 freeTrialInfo 的原始 JSON 形状，只用作解析
// 中间态。RawExpiry 是 unix 秒的原始浮点值，转换成 ExpiryDate 后就不再
// 需要——之前 FreeTrialInfo 同时当公开类型和 JSON 解析目标用，导致这个
// 只在解析阶段有意义的原始字段一直留在对外的公开结构体上（Task 19 评审
// 记录的 deferred minor）。拆成两个类型后公开的 FreeTrialInfo 只暴露
// 调用方真正需要的 Status/ExpiryDate。
type rawFreeTrialInfo struct {
	Status    string  `json:"freeTrialStatus"`
	RawExpiry float64 `json:"freeTrialExpiry"`
}

// UsageBreakdown 是某一资源类型的用量明细。
// AGENTIC_REQUEST 口径下 CurrentUsage / UsageLimit 是**请求数**，不是 token。
type UsageBreakdown struct {
	ResourceType    string
	CurrentUsage    float64
	UsageLimit      float64
	OverageCap      float64
	OverageRate     float64
	CurrentOverages float64
	NextDateReset   *time.Time
	Bonuses         []Bonus
	FreeTrial       *FreeTrialInfo
}

// UsageLimits 是 getUsageLimits 的解析结果。
type UsageLimits struct {
	SubscriptionTitle string
	OverageStatus     string
	OverageCapability string
	NextDateReset     *time.Time
	Breakdowns        []UsageBreakdown
}

// rawUsageLimits 镜像上游响应，只保留需要的字段。
type rawUsageLimits struct {
	NextDateReset    *float64 `json:"nextDateReset"`
	SubscriptionInfo *struct {
		SubscriptionTitle string `json:"subscriptionTitle"`
		OverageCapability string `json:"overageCapability"`
	} `json:"subscriptionInfo"`
	OverageConfiguration *struct {
		OverageStatus string `json:"overageStatus"`
	} `json:"overageConfiguration"`
	UsageBreakdownList []struct {
		ResourceType              string            `json:"resourceType"`
		CurrentUsage              float64           `json:"currentUsage"`
		CurrentUsageWithPrecision *float64          `json:"currentUsageWithPrecision"`
		UsageLimit                float64           `json:"usageLimit"`
		UsageLimitWithPrecision   *float64          `json:"usageLimitWithPrecision"`
		OverageCap                float64           `json:"overageCap"`
		OverageRate               float64           `json:"overageRate"`
		CurrentOverages           float64           `json:"currentOverages"`
		NextDateReset             *float64          `json:"nextDateReset"`
		Bonuses                   []Bonus           `json:"bonuses"`
		FreeTrialInfo             *rawFreeTrialInfo `json:"freeTrialInfo"`
	} `json:"usageBreakdownList"`
}

func unixPtr(v *float64) *time.Time {
	if v == nil || *v <= 0 {
		return nil
	}
	t := time.Unix(int64(*v), 0).UTC()
	return &t
}

// ParseUsageLimits 解析 getUsageLimits 响应。
func ParseUsageLimits(raw []byte) (*UsageLimits, error) {
	var r rawUsageLimits
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("kiro: decode getUsageLimits: %w", err)
	}

	out := &UsageLimits{NextDateReset: unixPtr(r.NextDateReset)}
	if r.SubscriptionInfo != nil {
		out.SubscriptionTitle = r.SubscriptionInfo.SubscriptionTitle
		out.OverageCapability = r.SubscriptionInfo.OverageCapability
	}
	if r.OverageConfiguration != nil {
		out.OverageStatus = r.OverageConfiguration.OverageStatus
	}

	for _, b := range r.UsageBreakdownList {
		item := UsageBreakdown{
			ResourceType:    b.ResourceType,
			CurrentUsage:    b.CurrentUsage,
			UsageLimit:      b.UsageLimit,
			OverageCap:      b.OverageCap,
			OverageRate:     b.OverageRate,
			CurrentOverages: b.CurrentOverages,
			NextDateReset:   unixPtr(b.NextDateReset),
			Bonuses:         b.Bonuses,
		}
		// 有精确值时优先，避免整数截断造成的额度误判。
		if b.CurrentUsageWithPrecision != nil {
			item.CurrentUsage = *b.CurrentUsageWithPrecision
		}
		if b.UsageLimitWithPrecision != nil {
			item.UsageLimit = *b.UsageLimitWithPrecision
		}
		if b.FreeTrialInfo != nil {
			item.FreeTrial = &FreeTrialInfo{
				Status:     b.FreeTrialInfo.Status,
				ExpiryDate: unixPtr(&b.FreeTrialInfo.RawExpiry),
			}
		}
		out.Breakdowns = append(out.Breakdowns, item)
	}

	return out, nil
}

// AgenticRequest 返回 AGENTIC_REQUEST 明细，没有则返回 nil。
func (u *UsageLimits) AgenticRequest() *UsageBreakdown {
	if u == nil {
		return nil
	}
	for i := range u.Breakdowns {
		if strings.EqualFold(u.Breakdowns[i].ResourceType, "AGENTIC_REQUEST") {
			return &u.Breakdowns[i]
		}
	}
	return nil
}

// EffectiveLimit 返回基础额度加上全部 ACTIVE 赠送额度。
//
// 只看 UsageLimit 会把有赠送额度的账号误判为已耗尽，从而错误地冷却掉可用账号。
func (b *UsageBreakdown) EffectiveLimit() float64 {
	if b == nil {
		return 0
	}
	total := b.UsageLimit
	for _, bonus := range b.Bonuses {
		if strings.EqualFold(bonus.Status, "ACTIVE") {
			total += bonus.UsageLimit
		}
	}
	return total
}

// Exhausted 判断额度是否已用尽。
func (b *UsageBreakdown) Exhausted() bool {
	if b == nil {
		return false
	}
	limit := b.EffectiveLimit()
	return limit > 0 && b.CurrentUsage >= limit
}

// UtilizationPercent 返回使用率百分比（0-100+）。
func (b *UsageBreakdown) UtilizationPercent() float64 {
	if b == nil {
		return 0
	}
	limit := b.EffectiveLimit()
	if limit <= 0 {
		return 0
	}
	return b.CurrentUsage / limit * 100
}

// BuildUsageLimitsURL 拼出额度查询地址。profileArn 为空时省略该参数
// （API Key 账号不使用 profileArn）。
func BuildUsageLimitsURL(qHost, profileArn string) string {
	q := url.Values{}
	q.Set("origin", "AI_EDITOR")
	q.Set("resourceType", "AGENTIC_REQUEST")
	q.Set("isEmailRequired", "true")
	if arn := strings.TrimSpace(profileArn); arn != "" {
		q.Set("profileArn", arn)
	}
	return strings.TrimSuffix(qHost, "/") + "/getUsageLimits?" + q.Encode()
}
