package ticketproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const onesProxyOrigin = "https://thirdapi.onesproxy.com"
const onesProxyGeneratePath = "/v1/openApi/openApiDynamicGenerateProxyRule"
const onesProxyExtractPath = "/api/proxy/getProxyList"
const autoRefillInterval = time.Minute

var OnesProxy = NewOnesProxyProvider("/app/data/ticket-proxy-provider.json", Default)

type ProviderConfig struct {
	Type        string `json:"type"`
	UserID      string `json:"user_id"`
	Token       string `json:"token,omitempty"`
	ProxyID     string `json:"proxy_id"`
	ExtractURL  string `json:"extract_url,omitempty"`
	Country     string `json:"country"`
	Mode        int    `json:"mode"`
	SessionTime int    `json:"session_time"`
	Route       string `json:"route"`
	AutoRefill  bool   `json:"auto_refill"`
}

type ProviderStatus struct {
	ProviderConfig
	Configured        bool `json:"configured"`
	TokenConfigured   bool `json:"token_configured"`
	ExtractConfigured bool `json:"extract_configured"`
}

type FetchResult struct {
	ImportResult
	Requested int `json:"requested"`
	Received  int `json:"received"`
}

type OnesProxyProvider struct {
	mu      sync.Mutex
	fetchMu sync.Mutex
	path    string
	pool    *Store
	client  *http.Client
	// Protected by fetchMu, shared with manual fetches to prevent duplicate requests.
	nextAutoRefill time.Time
}

func NewOnesProxyProvider(path string, pool *Store) *OnesProxyProvider {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &OnesProxyProvider{path: path, pool: pool, client: &http.Client{
		Transport: transport, Timeout: 45 * time.Second,
		// Never forward the API token or extraction key to a redirected destination.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func defaultProviderConfig() ProviderConfig {
	return ProviderConfig{Type: "generate", Country: "US", Mode: 1, SessionTime: 30, Route: "NA"}
}
func (p *OnesProxyProvider) read() (ProviderConfig, error) {
	data, err := os.ReadFile(p.path)
	if os.IsNotExist(err) {
		return defaultProviderConfig(), nil
	}
	if err != nil {
		return ProviderConfig{}, errors.New("无法读取 OnesProxy 配置")
	}
	var cfg ProviderConfig
	if json.Unmarshal(data, &cfg) != nil {
		return cfg, errors.New("OnesProxy 配置文件格式无效")
	}
	return cfg, nil
}
func providerStatus(cfg ProviderConfig) ProviderStatus {
	result := ProviderStatus{ProviderConfig: cfg, TokenConfigured: cfg.Token != "", ExtractConfigured: cfg.ExtractURL != ""}
	result.Configured = cfg.Type == "generate" && cfg.UserID != "" && cfg.Token != "" && cfg.ProxyID != "" || cfg.Type == "extract" && cfg.ExtractURL != ""
	result.Token = ""
	result.ExtractURL = ""
	return result
}
func (p *OnesProxyProvider) Status() (ProviderStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cfg, err := p.read()
	return providerStatus(cfg), err
}

func validateExtractionURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "thirdapi.onesproxy.com" || u.Path != onesProxyExtractPath || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("请填写 OnesProxy 官方 HTTPS 提取链接（thirdapi.onesproxy.com/api/proxy/getProxyList）")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || q.Get("key") == "" || len(q.Get("key")) > 512 {
		return nil, errors.New("提取链接缺少有效的 key 参数")
	}
	if index := q.Get("index"); index != "" {
		n, err := strconv.Atoi(index)
		if err != nil || n < 1 {
			return nil, errors.New("提取链接的 index 必须是正整数")
		}
	}
	return u, nil
}

func (p *OnesProxyProvider) Save(cfg ProviderConfig) (ProviderStatus, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	old, err := p.read()
	if err != nil {
		return ProviderStatus{}, err
	}
	cfg.UserID = strings.TrimSpace(cfg.UserID)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.ProxyID = strings.TrimSpace(cfg.ProxyID)
	cfg.ExtractURL = strings.TrimSpace(cfg.ExtractURL)
	cfg.Country = strings.ToUpper(strings.TrimSpace(cfg.Country))
	cfg.Route = strings.ToUpper(strings.TrimSpace(cfg.Route))
	// Empty password-style inputs preserve the stored secret.
	if cfg.Token == "" {
		cfg.Token = old.Token
	}
	if cfg.ExtractURL == "" {
		cfg.ExtractURL = old.ExtractURL
	}
	if len(cfg.UserID) > 256 || len(cfg.Token) > 4096 || len(cfg.ProxyID) > 256 || len(cfg.ExtractURL) > 8192 || strings.ContainsAny(cfg.UserID+cfg.Token+cfg.ProxyID, "\r\n") {
		return ProviderStatus{}, errors.New("API 配置内容无效或过长")
	}
	switch cfg.Type {
	case "generate":
		if cfg.UserID == "" || cfg.Token == "" || cfg.ProxyID == "" {
			return ProviderStatus{}, errors.New("请填写 Userid、Token 和代理编号 proxy_id")
		}
		if cfg.Country != "" && (len(cfg.Country) != 2 || cfg.Country[0] < 'A' || cfg.Country[0] > 'Z' || cfg.Country[1] < 'A' || cfg.Country[1] > 'Z') {
			return ProviderStatus{}, errors.New("国家代码须为两位英文字母，例如 US；留空表示随机")
		}
		if cfg.Mode != 1 && cfg.Mode != 2 {
			return ProviderStatus{}, errors.New("请选择粘性或轮转模式")
		}
		if cfg.Mode == 1 && (cfg.SessionTime < 1 || cfg.SessionTime > 120) {
			return ProviderStatus{}, errors.New("粘性时间须为 1–120 分钟，且不超过套餐允许值")
		}
		if len(cfg.Route) > 24 || strings.ContainsAny(cfg.Route, " \t\r\n") {
			return ProviderStatus{}, errors.New("代理线路无效，请使用套餐对应的线路代码")
		}
	case "extract":
		if _, err = validateExtractionURL(cfg.ExtractURL); err != nil {
			return ProviderStatus{}, err
		}
	default:
		return ProviderStatus{}, errors.New("请选择有效的 API 接口类型")
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return ProviderStatus{}, errors.New("无法保存 OnesProxy 配置")
	}
	if err = os.MkdirAll(filepath.Dir(p.path), 0700); err != nil {
		return ProviderStatus{}, errors.New("无法保存 OnesProxy 配置")
	}
	f, err := os.CreateTemp(filepath.Dir(p.path), ".ticket-provider-*")
	if err != nil {
		return ProviderStatus{}, errors.New("无法保存 OnesProxy 配置")
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ProviderStatus{}, errors.New("无法保存 OnesProxy 配置")
	}
	if err = os.Rename(f.Name(), p.path); err != nil {
		return ProviderStatus{}, errors.New("无法保存 OnesProxy 配置")
	}
	return providerStatus(cfg), nil
}

func (p *OnesProxyProvider) Fetch(ctx context.Context, count int) (FetchResult, error) {
	if count < 1 || count > MaxProxies {
		return FetchResult{}, fmt.Errorf("单次提取数量须为 1–%d", MaxProxies)
	}
	if !p.fetchMu.TryLock() {
		return FetchResult{}, errors.New("正在提取代理，请等待本次完成")
	}
	defer p.fetchMu.Unlock()
	p.mu.Lock()
	cfg, err := p.read()
	p.mu.Unlock()
	if err != nil {
		return FetchResult{}, err
	}
	return p.fetch(ctx, count, cfg)
}

// RefillIfEmpty runs from the existing harvester loop. It makes at most one
// supplier request per minute and shares the manual fetch lock.
func (p *OnesProxyProvider) RefillIfEmpty(ctx context.Context) (*FetchResult, error) {
	if ctx.Err() != nil || !p.fetchMu.TryLock() {
		return nil, nil
	}
	defer p.fetchMu.Unlock()
	if time.Now().Before(p.nextAutoRefill) {
		return nil, nil
	}
	p.mu.Lock()
	cfg, err := p.read()
	p.mu.Unlock()
	if err != nil {
		p.nextAutoRefill = time.Now().Add(autoRefillInterval)
		return nil, err
	}
	if !cfg.AutoRefill || !providerStatus(cfg).Configured {
		return nil, nil
	}
	status, err := p.pool.Status()
	if err != nil {
		p.nextAutoRefill = time.Now().Add(autoRefillInterval)
		return nil, err
	}
	if !status.Managed || status.Count != 0 || ctx.Err() != nil {
		return nil, nil
	}
	defer func() { p.nextAutoRefill = time.Now().Add(autoRefillInterval) }()
	result, err := p.fetch(ctx, MaxProxies, cfg)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// fetch requires fetchMu. Both manual and automatic imports use the same checks.
func (p *OnesProxyProvider) fetch(ctx context.Context, count int, cfg ProviderConfig) (FetchResult, error) {
	if !providerStatus(cfg).Configured {
		return FetchResult{}, errors.New("请先保存 OnesProxy API 配置")
	}
	status, err := p.pool.Status()
	if err != nil {
		return FetchResult{}, errors.New("无法读取代理池")
	}
	if status.Count+count > MaxPoolProxies {
		return FetchResult{}, fmt.Errorf("代理池剩余容量 %d 条，请减少提取数量", MaxPoolProxies-status.Count)
	}
	var req *http.Request
	if cfg.Type == "generate" {
		body := map[string]any{"proxy_id": cfg.ProxyID, "proxy_mode": cfg.Mode, "protocol_type": 2, "proxy_conn_type": 1001, "proxy_num": count}
		if cfg.Country != "" {
			body["country_code"] = cfg.Country
		}
		if cfg.Route != "" {
			body["conn_route_code"] = cfg.Route
		}
		if cfg.Mode == 1 {
			body["session_time"] = cfg.SessionTime
		}
		data, _ := json.Marshal(body)
		req, err = http.NewRequestWithContext(ctx, http.MethodPost, onesProxyOrigin+onesProxyGeneratePath, bytes.NewReader(data))
		if err == nil {
			req.Header.Set("Userid", cfg.UserID)
			req.Header.Set("Token", cfg.Token)
			req.Header.Set("Content-Type", "application/json")
		}
	} else {
		var u *url.URL
		u, err = validateExtractionURL(cfg.ExtractURL)
		if err == nil {
			old := u.Query()
			q := url.Values{"key": {old.Get("key")}, "index": {old.Get("index")}, "num": {strconv.Itoa(count)}, "return_type": {"json"}}
			if q.Get("index") == "" {
				q.Set("index", "1")
			}
			u.RawQuery = q.Encode()
			req, err = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		}
	}
	if err != nil {
		return FetchResult{}, errors.New("无法创建 OnesProxy 提取请求，请检查配置")
	}
	response, err := p.client.Do(req)
	if err != nil {
		return FetchResult{}, errors.New("连接 OnesProxy 失败或超时，请稍后重试")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("OnesProxy 返回 HTTP %d，请检查 API 凭证、访问权限和提取数量", response.StatusCode)
	}
	const maxResponse = 8 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(data) > maxResponse {
		return FetchResult{}, errors.New("OnesProxy 响应读取失败或内容过大")
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return FetchResult{}, errors.New("OnesProxy 未返回有效 JSON 数据")
	}
	var lines []string
	if cfg.Type == "generate" {
		if envelope.Code != 200 {
			return FetchResult{}, fmt.Errorf("OnesProxy 生成失败（代码 %d），请检查凭证、套餐和提取数量", envelope.Code)
		}
		var body struct {
			Results []string `json:"results"`
		}
		err = json.Unmarshal(envelope.Data, &body)
		lines = body.Results
	} else {
		if envelope.Code != 0 && envelope.Code != 200 {
			return FetchResult{}, fmt.Errorf("OnesProxy 提取失败（代码 %d），请检查链接和白名单", envelope.Code)
		}
		err = json.Unmarshal(envelope.Data, &lines)
	}
	if err != nil || len(lines) == 0 {
		return FetchResult{}, errors.New("OnesProxy 未返回代理，原有代理保持不变")
	}
	if len(lines) > count {
		return FetchResult{}, errors.New("OnesProxy 返回数量超过请求数量，未导入，请检查接口配置")
	}
	proxies, duplicates, err := Parse(lines)
	if err != nil {
		return FetchResult{}, errors.New("OnesProxy 返回的代理格式无效，原有代理保持不变")
	}
	if ctx.Err() != nil {
		return FetchResult{}, errors.New("提取请求已取消，未导入")
	}
	result, err := p.pool.Import(proxies, duplicates)
	if err != nil {
		return FetchResult{}, errors.New("无法保存提取结果，请检查代理池容量和服务器存储状态")
	}
	return FetchResult{ImportResult: result, Requested: count, Received: len(lines)}, nil
}
