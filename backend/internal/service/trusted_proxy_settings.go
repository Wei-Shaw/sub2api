// Package service — trusted_proxy_settings.go 定义"可信代理动态拉取"
// (switch-trusted-proxies-dynamic) 的 domain 类型与 Parse/Normalize helper。
//
// 功能概述：admin 可配置总开关 + 一组拉取源（CDN IP 段 URL）+ 一组固定 CIDR。
// 后端启动一组后台 goroutine 定期抓取，把静态 config.yaml、admin 固定条目、动态
// 抓取结果去重合并后交给 Gin r.SetTrustedProxies。
//
// 与其它 setting 分组一致的编码约定：
//   - Parse* 用于 GET/加载路径：损坏/缺失回退到安全默认；GET 永远返回合法值
//   - Normalize* / Validate* 用于 PUT 严格校验（admin 输入错误 → 400）
package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// TrustedProxyDynamicSource 是"从远端 URL 拉取 IP 列表"的一个条目。
type TrustedProxyDynamicSource struct {
	ID              string `json:"id"`               // 唯一标识（前端 v-for key、后台 goroutine 标签）
	Name            string `json:"name"`             // 展示名
	URL             string `json:"url"`              // http(s):// 端点，返回一行一个 CIDR 的纯文本
	Enabled         bool   `json:"enabled"`          // 该 source 是否参与拉取
	IntervalSeconds int    `json:"interval_seconds"` // 拉取周期
	TimeoutSeconds  int    `json:"timeout_seconds"`  // 单次 HTTP 超时
}

// TrustedProxyDynamicSourceMaxItems 单实例支持的 source 上限（前端展示 & 防止 admin 误配爆炸）。
const TrustedProxyDynamicSourceMaxItems = 20

// TrustedProxy* 约束值。
const (
	TrustedProxyIntervalSecondsMin     = 60         // 1 分钟
	TrustedProxyIntervalSecondsMax     = 30 * 86400 // 30 天
	TrustedProxyIntervalSecondsDefault = 86400      // 24h
	TrustedProxyTimeoutSecondsMin      = 1
	TrustedProxyTimeoutSecondsMax      = 120
	TrustedProxyTimeoutSecondsDefault  = 30

	TrustedProxySourceNameMaxLen = 100
	TrustedProxySourceURLMaxLen  = 500
	TrustedProxySourceIDMaxLen   = 64

	// 单个 source 拉取到的最大 CIDR 数（防止畸形/巨大响应把 rebuild 打爆）。
	TrustedProxySourceMaxCIDRs = 10000

	// 单次响应体上限 1MiB。CF 全量列表 < 2KiB，1MiB 十分宽裕。
	TrustedProxySourceMaxResponseBytes = 1 << 20

	// admin 面板固定 CIDR 数量上限。
	TrustedProxyExtraCIDRMaxItems = 1000

	// 合并后（static + extra + all sources dedupe 后）总 CIDR 上限，
	// 避免 Gin trie 构造过大或拒绝超长切片。
	TrustedProxyMergedMaxCIDRs = 20000
)

// DefaultTrustedProxyDynamicSources 是内置的默认拉取源候选。
// 新实例首次读取 setting 时，若 setting 尚未存在则以这份作为回填，全部 enabled=false —
// admin 打开 admin 面板即可看到三个候选，只需一键 enable 就可用。
func DefaultTrustedProxyDynamicSources() []TrustedProxyDynamicSource {
	return []TrustedProxyDynamicSource{
		{
			ID:              "cloudflare-v4",
			Name:            "Cloudflare IPv4",
			URL:             "https://www.cloudflare.com/ips-v4",
			Enabled:         false,
			IntervalSeconds: TrustedProxyIntervalSecondsDefault,
			TimeoutSeconds:  TrustedProxyTimeoutSecondsDefault,
		},
		{
			ID:              "cloudflare-v6",
			Name:            "Cloudflare IPv6",
			URL:             "https://www.cloudflare.com/ips-v6",
			Enabled:         false,
			IntervalSeconds: TrustedProxyIntervalSecondsDefault,
			TimeoutSeconds:  TrustedProxyTimeoutSecondsDefault,
		},
		{
			ID:              "cnmcdn",
			Name:            "CNM CDN",
			URL:             "https://cnmcdn.net/ip1.txt",
			Enabled:         false,
			IntervalSeconds: TrustedProxyIntervalSecondsDefault,
			TimeoutSeconds:  TrustedProxyTimeoutSecondsDefault,
		},
	}
}

// ─── Parse (GET 路径：损坏/缺失回退到默认，永远返回合法值) ─────────────────────

// ParseTrustedProxyDynamicSources 从持久化字符串解析 sources 列表。
// - 空字符串 / 解析失败 → 返回默认 sources（三个内置候选，全部 enabled=false）
// - 非法字段（interval / timeout 越界 / URL 空）→ 用默认值兜底
func ParseTrustedProxyDynamicSources(raw string) []TrustedProxyDynamicSource {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultTrustedProxyDynamicSources()
	}
	var parsed []TrustedProxyDynamicSource
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return DefaultTrustedProxyDynamicSources()
	}
	// 逐条兜底
	out := make([]TrustedProxyDynamicSource, 0, len(parsed))
	seenIDs := make(map[string]struct{}, len(parsed))
	for _, s := range parsed {
		s.ID = strings.TrimSpace(s.ID)
		s.Name = strings.TrimSpace(s.Name)
		s.URL = strings.TrimSpace(s.URL)
		if s.ID == "" || s.URL == "" {
			continue
		}
		if _, dup := seenIDs[s.ID]; dup {
			continue
		}
		if s.IntervalSeconds < TrustedProxyIntervalSecondsMin || s.IntervalSeconds > TrustedProxyIntervalSecondsMax {
			s.IntervalSeconds = TrustedProxyIntervalSecondsDefault
		}
		if s.TimeoutSeconds < TrustedProxyTimeoutSecondsMin || s.TimeoutSeconds > TrustedProxyTimeoutSecondsMax {
			s.TimeoutSeconds = TrustedProxyTimeoutSecondsDefault
		}
		seenIDs[s.ID] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return DefaultTrustedProxyDynamicSources()
	}
	return out
}

// ParseTrustedProxyDynamicExtraCIDRs 从持久化 JSON 字符串解析 admin 固定 CIDR 列表。
// 非法条目静默丢弃；空字符串/解析失败 → 空切片。
func ParseTrustedProxyDynamicExtraCIDRs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return []string{}
	}
	out := make([]string, 0, len(parsed))
	seen := make(map[string]struct{}, len(parsed))
	for _, item := range parsed {
		normalized := normalizeCIDROrIP(item)
		if normalized == "" {
			continue
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

// ─── Normalize (PUT 路径：严格校验，非法即 400) ────────────────────────────────

// NormalizeTrustedProxyDynamicSources 严格校验并规范化 sources 列表。
func NormalizeTrustedProxyDynamicSources(items []TrustedProxyDynamicSource) ([]TrustedProxyDynamicSource, error) {
	if len(items) > TrustedProxyDynamicSourceMaxItems {
		return nil, infraerrors.BadRequest(
			"INVALID_TRUSTED_PROXIES_SOURCES",
			fmt.Sprintf("sources must contain at most %d items", TrustedProxyDynamicSourceMaxItems),
		)
	}
	out := make([]TrustedProxyDynamicSource, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i, s := range items {
		s.ID = strings.TrimSpace(s.ID)
		s.Name = strings.TrimSpace(s.Name)
		s.URL = strings.TrimSpace(s.URL)

		if s.ID == "" {
			return nil, invalidSource(i, "id must not be empty")
		}
		if len(s.ID) > TrustedProxySourceIDMaxLen {
			return nil, invalidSource(i, fmt.Sprintf("id must be at most %d chars", TrustedProxySourceIDMaxLen))
		}
		if _, dup := seen[s.ID]; dup {
			return nil, invalidSource(i, fmt.Sprintf("duplicate source id %q", s.ID))
		}
		if s.Name == "" {
			s.Name = s.ID // name 空时用 id 兜底展示
		}
		if len(s.Name) > TrustedProxySourceNameMaxLen {
			return nil, invalidSource(i, fmt.Sprintf("name must be at most %d chars", TrustedProxySourceNameMaxLen))
		}
		if s.URL == "" {
			return nil, invalidSource(i, "url must not be empty")
		}
		if len(s.URL) > TrustedProxySourceURLMaxLen {
			return nil, invalidSource(i, fmt.Sprintf("url must be at most %d chars", TrustedProxySourceURLMaxLen))
		}
		if err := validateHTTPURL(s.URL); err != nil {
			return nil, invalidSource(i, err.Error())
		}
		if s.IntervalSeconds < TrustedProxyIntervalSecondsMin || s.IntervalSeconds > TrustedProxyIntervalSecondsMax {
			return nil, invalidSource(i, fmt.Sprintf(
				"interval_seconds must be in [%d, %d]",
				TrustedProxyIntervalSecondsMin, TrustedProxyIntervalSecondsMax,
			))
		}
		if s.TimeoutSeconds < TrustedProxyTimeoutSecondsMin || s.TimeoutSeconds > TrustedProxyTimeoutSecondsMax {
			return nil, invalidSource(i, fmt.Sprintf(
				"timeout_seconds must be in [%d, %d]",
				TrustedProxyTimeoutSecondsMin, TrustedProxyTimeoutSecondsMax,
			))
		}
		seen[s.ID] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

// NormalizeTrustedProxyDynamicExtraCIDRs 严格校验固定 CIDR 列表。
// 每条走 net.ParseCIDR 或 net.ParseIP；纯 IP 会补上 /32 或 /128；非法条目即 400。
func NormalizeTrustedProxyDynamicExtraCIDRs(items []string) ([]string, error) {
	if len(items) > TrustedProxyExtraCIDRMaxItems {
		return nil, infraerrors.BadRequest(
			"INVALID_TRUSTED_PROXIES_EXTRA_CIDRS",
			fmt.Sprintf("extra_cidrs must contain at most %d items", TrustedProxyExtraCIDRMaxItems),
		)
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for i, item := range items {
		normalized := normalizeCIDROrIP(item)
		if normalized == "" {
			return nil, infraerrors.BadRequest(
				"INVALID_TRUSTED_PROXIES_EXTRA_CIDRS",
				fmt.Sprintf("extra_cidrs[%d]=%q is not a valid IP or CIDR", i, item),
			)
		}
		if _, dup := seen[normalized]; dup {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

// MarshalTrustedProxyDynamicSources 序列化 sources → 存 setting 的 JSON 字符串。
func MarshalTrustedProxyDynamicSources(items []TrustedProxyDynamicSource) (string, error) {
	if items == nil {
		items = []TrustedProxyDynamicSource{}
	}
	buf, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("marshal trusted proxy sources: %w", err)
	}
	return string(buf), nil
}

// MarshalTrustedProxyDynamicExtraCIDRs 序列化 extra_cidrs → JSON 字符串。
func MarshalTrustedProxyDynamicExtraCIDRs(items []string) (string, error) {
	if items == nil {
		items = []string{}
	}
	buf, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("marshal trusted proxy extra_cidrs: %w", err)
	}
	return string(buf), nil
}

// ─── helpers ────────────────────────────────────────────────────────────────

// normalizeCIDROrIP 把用户输入解析成规范的 CIDR 字符串；非法返回 ""。
// 纯 IP 会补上 /32 或 /128（与 net.ParseCIDR 的输出对齐）。
func normalizeCIDROrIP(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "/") {
		_, ipnet, err := net.ParseCIDR(v)
		if err != nil || ipnet == nil {
			return ""
		}
		return ipnet.String()
	}
	ip := net.ParseIP(v)
	if ip == nil {
		return ""
	}
	if ip.To4() != nil {
		return ip.String() + "/32"
	}
	return ip.String() + "/128"
}

// validateHTTPURL 确保 URL 是 http:// 或 https:// 且能被解析。
func validateHTTPURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url invalid: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("url must contain a host")
	}
	return nil
}

func invalidSource(index int, msg string) error {
	return infraerrors.BadRequest(
		"INVALID_TRUSTED_PROXIES_SOURCES",
		fmt.Sprintf("sources[%d]: %s", index, msg),
	)
}
