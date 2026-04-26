package service

import (
	"context"
	"net/url"
	"strings"
)

// 渠道监控参数校验与归一化辅助函数。
// 校验失败一律返回 channel_monitor_const.go 中预定义的 Err* 错误，错误信息不含具体 IP/hostname，避免泄露内网拓扑。

// validateProvider 校验 provider 字符串。
// 唯一来源于 providerAdapters：新增 provider 只需要在 channel_monitor_checker.go 注册 adapter。
func validateProvider(p string) error {
	if !isSupportedProvider(p) {
		return ErrChannelMonitorInvalidProvider
	}
	return nil
}

// validateInterval 校验 interval_seconds 范围。
func validateInterval(sec int) error {
	if sec < monitorMinIntervalSeconds || sec > monitorMaxIntervalSeconds {
		return ErrChannelMonitorInvalidInterval
	}
	return nil
}

// validateEndpoint 校验 endpoint：
//   - scheme 强制 https（拒绝 http，避免明文凭证 + 部分 SSRF 利用面）
//   - 必须为 origin（无 path/query/fragment），防止用户填 https://api.openai.com/v1
//     导致 joinURL 拼出 /v1/v1/chat/completions
//   - hostname 不能是 localhost/metadata 等已知元数据 hostname
//   - 解析所有 IP，任一落在 loopback/RFC1918/link-local/ULA 段即拒绝（防 SSRF）
//
// 错误信息不暴露具体 IP / hostname，避免泄露内网拓扑。
func validateEndpoint(ep string) error {
	ep = strings.TrimSpace(ep)
	if ep == "" {
		return ErrChannelMonitorInvalidEndpoint
	}
	u, err := url.Parse(ep)
	if err != nil {
		return ErrChannelMonitorInvalidEndpoint
	}
	if u.Scheme != "https" {
		return ErrChannelMonitorEndpointScheme
	}
	if u.Host == "" {
		return ErrChannelMonitorInvalidEndpoint
	}
	if u.Path != "" && u.Path != "/" {
		return ErrChannelMonitorEndpointPath
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return ErrChannelMonitorEndpointPath
	}

	hostname := u.Hostname()
	ctx, cancel := context.WithTimeout(context.Background(), monitorEndpointResolveTimeout)
	defer cancel()
	blocked, err := isPrivateOrLoopbackHost(ctx, hostname)
	if err != nil {
		return ErrChannelMonitorEndpointUnreachable
	}
	if blocked {
		return ErrChannelMonitorEndpointPrivate
	}
	return nil
}

// normalizeEndpoint 去除前后空白与末尾 `/`，保证存储统一为 origin。
// validateEndpoint 已确保格式合法（仅 origin），这里只做最终归一化。
func normalizeEndpoint(ep string) string {
	ep = strings.TrimSpace(ep)
	ep = strings.TrimRight(ep, "/")
	return ep
}

// normalizeModels 去除空白、重复模型名。保留输入顺序（map 的迭代顺序无关）。
func normalizeModels(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}
