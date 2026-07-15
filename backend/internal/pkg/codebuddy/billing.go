package codebuddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BillingBaseURL 是 CodeBuddy 计费（额度）接口所在域名（与 copilot.tencent.com 不同源）。
const BillingBaseURL = "https://www.workbuddy.cn"

// BillingUserResourcePath 获取用户资源（额度）的接口路径。
const BillingUserResourcePath = "/billing/meter/get-user-resource"

// BillingProductCode CodeBuddy 计费产品码。
const BillingProductCode = "p_tcaca"

// BillingUserResourceRequest 是 get-user-resource 的请求体。
type BillingUserResourceRequest struct {
	PageNumber                 int    `json:"PageNumber"`
	PageSize                   int    `json:"PageSize"`
	ProductCode                string `json:"ProductCode"`
	Status                     []int  `json:"Status"`
	PackageStartTimeRangeBegin string `json:"PackageStartTimeRangeBegin"`
	PackageStartTimeRangeEnd   string `json:"PackageStartTimeRangeEnd"`
}

// BillingAccount 表示 get-user-resource 返回中的单个资源包。
type BillingAccount struct {
	AccountID                  json.Number `json:"AccountId"`
	PackageName                string      `json:"PackageName"`
	CapacityType               int         `json:"CapacityType"`
	CapacityUnit               string      `json:"CapacityUnit"`
	CycleCapacitySizePrecise   string      `json:"CycleCapacitySizePrecise"`
	CycleCapacityRemainPrecise string      `json:"CycleCapacityRemainPrecise"`
	CycleCapacityUsedPrecise   string      `json:"CycleCapacityUsedPrecise"`
}

// BillingUserResourceData 是 get-user-resource 的核心数据。
type BillingUserResourceData struct {
	TotalCount  int              `json:"TotalCount"`
	TotalDosage float64          `json:"TotalDosage"`
	Accounts    []BillingAccount `json:"Accounts"`
}

// BillingUserResourceResponse 是 get-user-resource 的完整响应。
type BillingUserResourceResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Response struct {
			Data BillingUserResourceData `json:"Data"`
		} `json:"Response"`
	} `json:"data"`
}

// BillingUsage 汇总后的额度用量（各资源包的 CycleCapacity*Precise 之和）。
type BillingUsage struct {
	TotalCapacity float64 `json:"total_capacity"`
	Remaining     float64 `json:"remaining"`
	Used          float64 `json:"used"`
	AccountCount  int     `json:"account_count"`
}

// BuildUserResourceURL 返回 get-user-resource 的完整地址。
func BuildUserResourceURL() string {
	return strings.TrimRight(BillingBaseURL, "/") + BillingUserResourcePath
}

// NewBillingRequest 构造默认的分页请求，时间范围覆盖到当前时刻。
func NewBillingRequest() BillingUserResourceRequest {
	return BillingUserResourceRequest{
		PageNumber:                 1,
		PageSize:                   100,
		ProductCode:                BillingProductCode,
		Status:                     []int{0, 3},
		PackageStartTimeRangeBegin: "2024-12-01 21:25:00",
		PackageStartTimeRangeEnd:   time.Now().Format("2006-01-02 15:04:05"),
	}
}

func parsePrecise(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// SumBillingUsage 根据响应汇总总容量、剩余、已用（按用户说明使用 *Precise 字段求和）。
func SumBillingUsage(data BillingUserResourceData) BillingUsage {
	var total, remain, used float64
	for _, acc := range data.Accounts {
		total += parsePrecise(acc.CycleCapacitySizePrecise)
		remain += parsePrecise(acc.CycleCapacityRemainPrecise)
		used += parsePrecise(acc.CycleCapacityUsedPrecise)
	}
	return BillingUsage{
		TotalCapacity: total,
		Remaining:     remain,
		Used:          used,
		AccountCount:  len(data.Accounts),
	}
}

func newProxyTransport(proxyURL string) (*http.Transport, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}
	return &http.Transport{Proxy: http.ProxyURL(u)}, nil
}

// GetUserResource 调用 get-user-resource 并返回原始响应与汇总用量。
// accessToken 为 CodeBuddy OAuth access_token（作为 Bearer 鉴权，实测对 workbuddy.cn 计费接口有效）；
// userID 对应账号的 uid（x-user-id 头）。proxyURL 可选（计费接口通常直连）。
func GetUserResource(ctx context.Context, accessToken, userID, proxyURL string) (*BillingUserResourceResponse, *BillingUsage, error) {
	if strings.TrimSpace(accessToken) == "" {
		return nil, nil, fmt.Errorf("codebuddy billing: empty access token")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, nil, fmt.Errorf("codebuddy billing: empty user id")
	}

	reqBody, err := json.Marshal(NewBillingRequest())
	if err != nil {
		return nil, nil, fmt.Errorf("marshal billing request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, BuildUserResourceURL(), bytes.NewReader(reqBody))
	if err != nil {
		return nil, nil, fmt.Errorf("build billing request: %w", err)
	}
	req.Header.Set("accept", "application/json")
	req.Header.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("origin", BillingBaseURL)
	req.Header.Set("referer", BillingBaseURL+"/app")
	req.Header.Set("authorization", "Bearer "+accessToken)
	req.Header.Set("x-user-id", userID)
	req.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 30 * time.Second}
	if proxyURL != "" {
		if transport, terr := newProxyTransport(proxyURL); terr == nil {
			client.Transport = transport
		} else {
			slog.Warn("codebuddy billing: invalid proxy url, falling back to direct connection",
				"proxy_url", proxyURL, "error", terr)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("codebuddy billing request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("read billing response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("codebuddy billing returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed BillingUserResourceResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, nil, fmt.Errorf("parse billing response: %w", err)
	}
	if parsed.Code != 0 {
		return &parsed, nil, fmt.Errorf("codebuddy billing error code=%d msg=%s", parsed.Code, parsed.Msg)
	}

	usage := SumBillingUsage(parsed.Data.Response.Data)
	return &parsed, &usage, nil
}
