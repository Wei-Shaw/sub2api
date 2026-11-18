package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OpenDesign 的用量探测复用既有的 CN 配额框架。数据源是厂商的 funding-check
// 接口：一次请求返回 Go 套餐的 5h / 7d 两个滚动窗口（单位为 credits）。
// 解析失败或响应非 2xx 时保留上一份快照（fail-open），绝不因此停调账号。

const (
	// openDesignFundingCheckPath 追加在账号 base_url 之后。
	openDesignFundingCheckPath = "/api/v1/workspaces/%s/billing/funding-check"

	// DefaultOpenDesignAuthAPIBaseURL 是厂商控制面 API 主机（设备授权、钱包查询与
	// funding-check 探测共用）。
	DefaultOpenDesignAuthAPIBaseURL = "https://amr-api.open-design.ai"

	// OpenDesign 用量窗口的秒数，与 funding-check 返回的 durationSeconds 对齐。
	openDesignUsageWindow5HSeconds = 18000
	openDesignUsageWindow7DSeconds = 604800
)

// openDesignUsageSupported 报告账号是否带 funding-check 需要的控制面凭据。
// 周期探测用它在收集阶段跳过从未走设备授权的账号（探测必然 401）。
func (a *Account) openDesignUsageSupported() bool {
	if a == nil || a.Platform != PlatformOpenDesign || a.Type != AccountTypeAPIKey {
		return false
	}
	return strings.TrimSpace(a.GetCredential(OpenDesignUsageCredentialsKey)) != ""
}

// opendesignFundingCheckURL 用账号 base_url 与设备授权写入的 workspace id 拼出
// funding-check 地址。
func opendesignFundingCheckURL(baseURL, workspaceID string) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !isValidOpenDesignWorkspaceID(workspaceID) {
		return "", fmt.Errorf("invalid open_design workspace id")
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", fmt.Errorf("open_design account has no base url")
	}
	parsed, err := url.Parse(base + fmt.Sprintf(openDesignFundingCheckPath, url.PathEscape(workspaceID)))
	if err != nil || parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid open_design funding-check url")
	}
	return parsed.String(), nil
}

// opendesignParseCredits 解析 credits 字段：上游把同一字段既可能写成字符串，
// 也可能写成数字。
func opendesignParseCredits(raw any) (int64, bool) {
	switch v := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// opendesignWindowName 把 funding-check 的窗口时长映射成共享的 CN 窗口名。
// 未知时长返回 ok=false：以后上游新增窗口时忽略它，而不是让整份响应失败。
func opendesignWindowName(durationSeconds int) (string, bool) {
	switch durationSeconds {
	case openDesignUsageWindow5HSeconds:
		return "5h", true
	case openDesignUsageWindow7DSeconds:
		return "weekly", true
	default:
		return "", false
	}
}

// opendesignParseWindow 校验单个窗口并转换成共享的 CN 档位。
// 时长匹配但数值或 reset 时间不可用时视为报文改版：返回错误以保留上一份快照，
// 不能写出「新用量 + 旧重置时间」的混合快照。
func opendesignParseWindow(raw map[string]any, window string) (CNQuotaTier, error) {
	used, okUsed := opendesignParseCredits(raw["usedCredits"])
	limit, okLimit := opendesignParseCredits(raw["limitCredits"])
	remaining, okRemaining := opendesignParseCredits(raw["remainingCredits"])
	if !okUsed || !okLimit || !okRemaining {
		return CNQuotaTier{}, fmt.Errorf("%s window has non-numeric credits", window)
	}
	if used < 0 || limit <= 0 || remaining < 0 || remaining > limit {
		return CNQuotaTier{}, fmt.Errorf("%s window has out-of-range credits", window)
	}
	resetsAt, okReset := raw["resetsAt"].(string)
	if !okReset {
		return CNQuotaTier{}, fmt.Errorf("%s window has no resetsAt", window)
	}
	resetTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(resetsAt))
	if err != nil {
		return CNQuotaTier{}, fmt.Errorf("%s window has invalid resetsAt %q", window, resetsAt)
	}
	return CNQuotaTier{
		Window:      window,
		UsedPercent: float64(used) / float64(limit) * 100,
		ResetAt:     resetTime.UTC().Format(time.RFC3339),
	}, nil
}

// parseOpenDesignUsageTiers 把 funding-check 响应转换成 CN 配额档位。
//
// 明确「没有 coding plan」（codingPlan 为 null，或 eligible 字段存在且为 false）
// 返回 noPlan=true 且不报错：账号就是没套餐，调用方据此清空用量窗口，
// 而不是保留一份「已耗尽」的陈旧快照把它永久停调。
//
// 缺 codingPlan 字段、缺 eligible 字段、或任何窗口不可用，都视为上游报文改版
// 并返回错误，让调用方保留上一份快照，而不是写一份假的。
func parseOpenDesignUsageTiers(body []byte) (tiers []CNQuotaTier, planLevel string, noPlan bool, err error) {
	var payload struct {
		BalanceUsd string          `json:"balanceUsd"`
		CodingPlan json.RawMessage `json:"codingPlan"`
	}
	if decodeErr := json.Unmarshal(body, &payload); decodeErr != nil {
		return nil, "", false, fmt.Errorf("decode funding-check: %w", decodeErr)
	}
	if len(payload.CodingPlan) == 0 {
		return nil, "", false, fmt.Errorf("funding-check: response has no codingPlan field (upstream schema may have changed)")
	}
	if strings.TrimSpace(string(payload.CodingPlan)) == "null" {
		return nil, "", true, nil
	}

	var plan struct {
		// 用指针区分「字段缺失」与「明确为 false」：缺字段通常意味着报文改版，
		// 而不是「没有套餐」。
		Eligible *bool            `json:"eligible"`
		Tier     any              `json:"tier"`
		Windows  []map[string]any `json:"windows"`
	}
	if decodeErr := json.Unmarshal(payload.CodingPlan, &plan); decodeErr != nil {
		return nil, "", false, fmt.Errorf("decode funding-check codingPlan: %w", decodeErr)
	}
	if plan.Eligible == nil {
		return nil, "", false, fmt.Errorf("funding-check: codingPlan has no eligible field (upstream schema may have changed)")
	}
	if !*plan.Eligible {
		return nil, "", true, nil
	}

	tierName := ""
	if tier, ok := plan.Tier.(string); ok {
		tierName = strings.TrimSpace(tier)
	}

	byWindow := make(map[string]CNQuotaTier, 2)
	for _, raw := range plan.Windows {
		duration, ok := raw["durationSeconds"].(float64)
		if !ok {
			return nil, "", false, fmt.Errorf("funding-check: window has no numeric durationSeconds")
		}
		window, known := opendesignWindowName(int(duration))
		if !known {
			continue
		}
		tier, windowErr := opendesignParseWindow(raw, window)
		if windowErr != nil {
			return nil, "", false, windowErr
		}
		byWindow[window] = tier
	}
	for _, window := range []string{"5h", "weekly"} {
		if tier, ok := byWindow[window]; ok {
			tiers = append(tiers, tier)
		}
	}
	if len(tiers) == 0 {
		return nil, "", false, fmt.Errorf("funding-check: coding plan eligible but no usable usage window parsed")
	}
	return tiers, tierName, false, nil
}
