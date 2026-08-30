package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func NewProxyExitInfoProber(cfg *config.Config) service.ProxyExitInfoProber {
	insecure := false
	allowPrivate := false
	validateResolvedIP := true
	maxResponseBytes := defaultProxyProbeResponseMaxBytes
	if cfg != nil {
		insecure = cfg.Security.ProxyProbe.InsecureSkipVerify
		allowPrivate = cfg.Security.URLAllowlist.AllowPrivateHosts
		validateResolvedIP = cfg.Security.URLAllowlist.Enabled
		if cfg.Gateway.ProxyProbeResponseReadMaxBytes > 0 {
			maxResponseBytes = cfg.Gateway.ProxyProbeResponseReadMaxBytes
		}
	}
	if insecure {
		log.Printf("[ProxyProbe] Warning: insecure_skip_verify is not allowed and will cause probe failure.")
	}
	// 构建探测 URL 列表：配置存在时覆盖内置默认列表。
	var configuredTargets []configuredProbeTarget
	if cfg != nil && len(cfg.Security.ProxyProbe.URLs) > 0 {
		configuredTargets = make([]configuredProbeTarget, 0, len(cfg.Security.ProxyProbe.URLs))
		for _, u := range cfg.Security.ProxyProbe.URLs {
			configuredTargets = append(configuredTargets, configuredProbeTarget{
				url:    u.URL,
				parser: u.Parser,
			})
		}
	}

	return &proxyProbeService{
		insecureSkipVerify:  insecure,
		allowPrivateHosts:   allowPrivate,
		validateResolvedIP:  validateResolvedIP,
		maxResponseBytes:    maxResponseBytes,
		configuredProbeURLs: configuredTargets,
	}
}

const (
	defaultProxyProbeTimeout          = 10 * time.Second
	defaultProxyProbeResponseMaxBytes = int64(1024 * 1024)
)

// probeURLs 按优先级排列的内置探测 URL 列表。
// 某些 AI API 专用代理只允许访问特定域名，因此需要多个备选。
var probeURLs = []struct {
	url    string
	parser string
}{
	{"http://ip-api.com/json/?lang=zh-CN", "ip-api"},
	{"http://api64.ipify.org?format=json", "ipify"},
}

type configuredProbeTarget struct {
	url    string
	parser string
}

type proxyProbeService struct {
	insecureSkipVerify  bool
	allowPrivateHosts   bool
	validateResolvedIP  bool
	maxResponseBytes    int64
	configuredProbeURLs []configuredProbeTarget
}

func (s *proxyProbeService) ProbeProxy(ctx context.Context, proxyURL string) (*service.ProxyExitInfo, int64, error) {
	client, err := httpclient.GetClient(httpclient.Options{
		ProxyURL:           proxyURL,
		Timeout:            defaultProxyProbeTimeout,
		InsecureSkipVerify: s.insecureSkipVerify,
		ValidateResolvedIP: s.validateResolvedIP,
		AllowPrivateHosts:  s.allowPrivateHosts,
	})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create proxy client: %w", err)
	}

	var lastErr error
	if len(s.configuredProbeURLs) > 0 {
		for _, probe := range s.configuredProbeURLs {
			exitInfo, latencyMs, err := s.probeWithURL(ctx, client, probe.url, probe.parser)
			if err == nil {
				return exitInfo, latencyMs, nil
			}
			lastErr = err
		}
		return nil, 0, fmt.Errorf("all probe URLs failed, last error: %w", lastErr)
	}

	for _, probe := range probeURLs {
		exitInfo, latencyMs, err := s.probeWithURL(ctx, client, probe.url, probe.parser)
		if err == nil {
			return exitInfo, latencyMs, nil
		}
		lastErr = err
	}

	return nil, 0, fmt.Errorf("all probe URLs failed, last error: %w", lastErr)
}

func (s *proxyProbeService) probeWithURL(ctx context.Context, client *http.Client, url string, parser string) (*service.ProxyExitInfo, int64, error) {
	startTime := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("proxy connection failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	latencyMs := time.Since(startTime).Milliseconds()

	if resp.StatusCode != http.StatusOK {
		return nil, latencyMs, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	maxResponseBytes := s.maxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultProxyProbeResponseMaxBytes
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, latencyMs, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, latencyMs, fmt.Errorf("proxy probe response exceeds limit: %d", maxResponseBytes)
	}

	switch parser {
	case "ip-api":
		return s.parseIPAPI(body, latencyMs)
	case "ipify":
		return s.parseIPify(body, latencyMs)
	case "chatgpt-trace":
		return s.parseChatGPTTrace(body, latencyMs)
	default:
		return nil, latencyMs, fmt.Errorf("unknown parser: %s", parser)
	}
}

func (s *proxyProbeService) parseIPAPI(body []byte, latencyMs int64) (*service.ProxyExitInfo, int64, error) {
	var ipInfo struct {
		Status      string `json:"status"`
		Message     string `json:"message"`
		Query       string `json:"query"`
		City        string `json:"city"`
		Region      string `json:"region"`
		RegionName  string `json:"regionName"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
	}

	if err := json.Unmarshal(body, &ipInfo); err != nil {
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, latencyMs, fmt.Errorf("failed to parse response: %w (body: %s)", err, preview)
	}
	if strings.ToLower(ipInfo.Status) != "success" {
		if ipInfo.Message == "" {
			ipInfo.Message = "ip-api request failed"
		}
		return nil, latencyMs, fmt.Errorf("ip-api request failed: %s", ipInfo.Message)
	}

	region := ipInfo.RegionName
	if region == "" {
		region = ipInfo.Region
	}
	return &service.ProxyExitInfo{
		IP:          ipInfo.Query,
		City:        ipInfo.City,
		Region:      region,
		Country:     ipInfo.Country,
		CountryCode: ipInfo.CountryCode,
	}, latencyMs, nil
}

func (s *proxyProbeService) parseIPify(body []byte, latencyMs int64) (*service.ProxyExitInfo, int64, error) {
	var result struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, latencyMs, fmt.Errorf("failed to parse ipify response: %w", err)
	}
	if result.IP == "" {
		return nil, latencyMs, fmt.Errorf("ipify: no IP found in response")
	}
	return &service.ProxyExitInfo{
		IP: result.IP,
	}, latencyMs, nil
}

// parseChatGPTTrace 解析 Cloudflare trace 端点（如 chatgpt.com/cdn-cgi/trace）的纯文本响应。
// 响应按行给出键值对，其中 ip= 为出口 IP，loc= 为国家代码。
func (s *proxyProbeService) parseChatGPTTrace(body []byte, latencyMs int64) (*service.ProxyExitInfo, int64, error) {
	var ip, loc string
	for _, line := range strings.Split(string(body), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch key {
		case "ip":
			ip = strings.TrimSpace(value)
		case "loc":
			loc = strings.TrimSpace(value)
		}
	}
	if ip == "" {
		preview := string(body)
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		return nil, latencyMs, fmt.Errorf("chatgpt-trace: no ip= found in response (body: %s)", preview)
	}
	info := &service.ProxyExitInfo{
		IP: ip,
	}
	if loc != "" {
		info.CountryCode = loc
	}
	return info, latencyMs, nil
}
