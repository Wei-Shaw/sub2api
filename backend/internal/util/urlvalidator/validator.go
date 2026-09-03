package urlvalidator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// AllowPrivateHostsSettingKey / AllowPrivateHostsEnvKey 是「允许私网上游」这一策略
// 对应的配置键与环境变量名。所有因私网阻断而产生的错误都必须点名它，
// 否则运营者只会看到一句 "host is not allowed" 而不知道去哪里放行。
const (
	AllowPrivateHostsSettingKey = "security.url_allowlist.allow_private_hosts"
	AllowPrivateHostsEnvKey     = "SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS"
)

type ValidationOptions struct {
	AllowedHosts     []string
	RequireAllowlist bool
	AllowPrivate     bool
}

// blockPrivateUpstreams 保存全局「私网目标」策略在本包的投影。
//
// 这里刻意与 security.url_allowlist.enabled 解耦：两者是两个不同的关注点。
//   - 主机白名单（enabled + upstream_hosts）是「只准访问这几个域名」的收敛策略。
//     调用方普遍使用 ValidateHTTPSURL + RequireAllowlist=true，即开启后强制 HTTPS
//     且要求逐一列名主机，只适合封闭部署，必须由运营者显式开启，默认必须保持关闭，
//     否则所有自建/第三方中转上游会被一刀切断。
//   - 私网阻断是 SSRF 基线防护：任何部署都不应允许把上游 base_url 指向
//     127.0.0.1 / 10.0.0.0/8 / 169.254.169.254 等地址。它与白名单是否开启无关，
//     因此由 security.url_allowlist.allow_private_hosts 单独控制（默认 false = 阻断）。
//
// 之所以要用包级变量：白名单关闭时的降级路径 ValidateURLFormat 只拿得到
// (raw, allowInsecureHTTP)，没有 *config.Config 可读，而它恰恰是默认部署真正走到
// 的路径。config.Load 会在启动时调用 SetBlockPrivateUpstreams 注入运营者策略。
//
// 零值（未注入）沿用历史宽松行为，原因是这个变量只是「配置的投影」而不是独立策略：
// 未经 config.Load 构造 Config 的调用方（单测、库使用者）如果在这里被强制阻断，
// 反而会与它自己手上的 Config 不一致。真正 fail-closed 的兜底放在能直接读到
// Config 的出网口 repository.httpUpstreamService.shouldValidateResolvedIP()：
// 那里只有显式 AllowPrivateHosts=true 才会关闭解析后 IP 校验。
var blockPrivateUpstreams atomic.Bool

// SetBlockPrivateUpstreams 由配置加载流程注入运营者策略。
// 仅应由 config 包在 Load 时调用。
func SetBlockPrivateUpstreams(block bool) {
	blockPrivateUpstreams.Store(block)
}

// BlockPrivateUpstreams 返回当前是否阻断私网/环回/链路本地上游目标。
func BlockPrivateUpstreams() bool {
	return blockPrivateUpstreams.Load()
}

// ErrPrivateHostBlocked 供调用方判定「因私网策略被拒」。
var ErrPrivateHostBlocked = errors.New("private upstream host blocked")

func blockedPrivateHostError(host string) error {
	return fmt.Errorf(
		"%w: %s is a loopback/private/link-local address; set %s=true (env %s=true) to allow private upstreams",
		ErrPrivateHostBlocked, host, AllowPrivateHostsSettingKey, AllowPrivateHostsEnvKey,
	)
}

// ValidateHTTPURL validates an outbound HTTP/HTTPS URL.
//
// It provides a single validation entry point that supports:
// - scheme 校验（https 或可选允许 http）
// - 可选 allowlist（支持 *.example.com 通配）
// - allow_private_hosts 策略（阻断 localhost/私网字面量 IP）
//
// 注意：DNS Rebinding 防护（解析后 IP 校验）应在实际发起请求时执行，避免 TOCTOU。
func ValidateHTTPURL(raw string, allowInsecureHTTP bool, opts ValidationOptions) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("url is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid url: %s", trimmed)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && (!allowInsecureHTTP || scheme != "http") {
		return "", fmt.Errorf("invalid url scheme: %s", parsed.Scheme)
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "", errors.New("invalid host")
	}
	if !opts.AllowPrivate && isBlockedHost(host) {
		return "", blockedPrivateHostError(host)
	}

	if port := parsed.Port(); port != "" {
		num, err := strconv.Atoi(port)
		if err != nil || num <= 0 || num > 65535 {
			return "", fmt.Errorf("invalid port: %s", port)
		}
	}

	allowlist := normalizeAllowlist(opts.AllowedHosts)
	if opts.RequireAllowlist && len(allowlist) == 0 {
		return "", errors.New("allowlist is not configured")
	}
	if len(allowlist) > 0 && !isAllowedHost(host, allowlist) {
		return "", fmt.Errorf("host is not allowed: %s", host)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// ValidateURLFormat 是关闭主机白名单（security.url_allowlist.enabled=false）时的
// 降级校验路径：不做白名单收敛，但仍然执行 SSRF 基线检查。
//
// 历史行为是「纯格式校验」，导致 base_url 可以直接指向 http://169.254.169.254 等
// 云元数据服务；由于白名单默认关闭，这条降级路径才是绝大多数部署实际走到的路径
// （安全审计 M2）。现在它同样受 security.url_allowlist.allow_private_hosts 约束，
// 运营者要指向 LAN/localhost 上游时显式放行即可。
func ValidateURLFormat(raw string, allowInsecureHTTP bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("url is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid url: %s", trimmed)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && (!allowInsecureHTTP || scheme != "http") {
		return "", fmt.Errorf("invalid url scheme: %s", parsed.Scheme)
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", errors.New("invalid host")
	}
	if BlockPrivateUpstreams() && isBlockedHost(strings.ToLower(host)) {
		return "", blockedPrivateHostError(host)
	}

	if port := parsed.Port(); port != "" {
		num, err := strconv.Atoi(port)
		if err != nil || num <= 0 || num > 65535 {
			return "", fmt.Errorf("invalid port: %s", port)
		}
	}

	return strings.TrimRight(trimmed, "/"), nil
}

func ValidateHTTPSURL(raw string, opts ValidationOptions) (string, error) {
	return ValidateHTTPURL(raw, false, opts)
}

// ValidateResolvedIP 验证 DNS 解析后的 IP 地址是否安全
// 用于防止 DNS Rebinding 攻击：在实际 HTTP 请求时调用此函数验证解析后的 IP
func ValidateResolvedIP(host string) error {
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		if isBlockedIP(ip) {
			return blockedPrivateHostError(ip.String())
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("dns resolution failed: %w", err)
	}

	for _, ip := range ips {
		if isBlockedIP(ip) {
			return blockedPrivateHostError(ip.String())
		}
	}
	return nil
}

func normalizeAllowlist(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, v := range values {
		entry := strings.ToLower(strings.TrimSpace(v))
		if entry == "" {
			continue
		}
		if host, _, err := net.SplitHostPort(entry); err == nil {
			entry = host
		}
		normalized = append(normalized, entry)
	}
	return normalized
}

func isAllowedHost(host string, allowlist []string) bool {
	for _, entry := range allowlist {
		if entry == "" {
			continue
		}
		if strings.HasPrefix(entry, "*.") {
			suffix := strings.TrimPrefix(entry, "*.")
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == entry {
			return true
		}
	}
	return false
}

func isBlockedHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return isBlockedIP(ip)
	}
	return false
}

// isBlockedIP 判定一个 IP 是否属于 SSRF 相关地址段。
// 与 isBlockedHost / ValidateResolvedIP 共用，保证「字面量校验」与
// 「解析后校验（DNS Rebinding / 重定向）」使用同一套判定标准。
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsUnspecified()
}
