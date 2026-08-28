package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// WarpGatewayConfig is loaded from application config (Phase 2).
type WarpGatewayConfig struct {
	Enabled              bool
	BaseURL              string
	Token                string
	Timeout              time.Duration
	ReconcileInterval    time.Duration
	AutoDetachUnhealthy  bool // Phase 3: mark proxy inactive when unhealthy
	AlertDuplicateExitIP bool // Phase 3
	// Optional TLS / mTLS
	TLSCAFile             string
	TLSCertFile           string
	TLSKeyFile            string
	TLSInsecureSkipVerify bool
}

// WarpInstance is a subset of warp-gateway instance JSON.
type WarpInstance struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ListenHost    string `json:"listen_host"`
	ListenPort    int    `json:"listen_port"`
	Status        string `json:"status"`
	ExitIP        string `json:"exit_ip"`
	ExitColo      string `json:"exit_colo"`
	SocksAuthUser string `json:"socks_auth_user,omitempty"`
	SocksAuthPass string `json:"socks_auth_pass,omitempty"`
	LastError     string `json:"last_error"`
}

func (i WarpInstance) SocksURL() string {
	host := i.ListenHost
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("socks5h://%s:%d", host, i.ListenPort)
}

// WarpPoolSnapshot mirrors gateway /v1/pools/snapshot.
type WarpPoolSnapshot struct {
	Instances    []WarpInstance      `json:"instances"`
	SocksURLs    []string            `json:"socks_urls"`
	UnhealthyIDs []string            `json:"unhealthy_ids"`
	DuplicateIPs map[string][]string `json:"duplicate_exit_ips"`
	HealthyCount int                 `json:"healthy_count"`
	TotalCount   int                 `json:"total_count"`
}

// WarpGatewayClient talks to tools/warp-gateway control API.
type WarpGatewayClient struct {
	cfg     WarpGatewayConfig
	client  *http.Client
	initErr error
}

func NewWarpGatewayClient(cfg WarpGatewayConfig) *WarpGatewayClient {
	client, err := newWarpGatewayClient(cfg)
	if err != nil {
		client.initErr = err
	}
	return client
}

func newWarpGatewayClient(cfg WarpGatewayConfig) (*WarpGatewayClient, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if err := validateWarpGatewayConfig(cfg); err != nil {
		return &WarpGatewayClient{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, err
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		base = &http.Transport{}
	}
	transport := base.Clone()
	if cfg.Enabled && strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.BaseURL)), "https://") {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.TLSInsecureSkipVerify}
		if cfg.TLSCAFile != "" {
			pem, err := os.ReadFile(cfg.TLSCAFile)
			if err != nil {
				return &WarpGatewayClient{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, fmt.Errorf("read warp gateway CA file: %w", err)
			}
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pem) {
				return &WarpGatewayClient{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, fmt.Errorf("parse warp gateway CA file: no certificates found")
			}
			tlsCfg.RootCAs = pool
		}
		if cfg.TLSCertFile != "" {
			cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
			if err != nil {
				return &WarpGatewayClient{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, fmt.Errorf("load warp gateway client certificate: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		transport.TLSClientConfig = tlsCfg
	}
	return &WarpGatewayClient{
		cfg: cfg,
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func validateWarpGatewayConfig(cfg WarpGatewayConfig) error {
	if !cfg.Enabled {
		return nil
	}
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return fmt.Errorf("warp gateway base URL is required when enabled")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("invalid warp gateway base URL %q", baseURL)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("warp gateway base URL must not contain credentials, query, or fragment")
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("warp gateway base URL must use http or https")
	}
	if scheme == "http" && !isLoopbackWarpGatewayHost(parsed.Hostname()) {
		return fmt.Errorf("warp gateway requires HTTPS for non-loopback hosts")
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return fmt.Errorf("warp gateway TLS client certificate and key must be configured together")
	}
	if scheme != "https" && (cfg.TLSCAFile != "" || cfg.TLSCertFile != "" || cfg.TLSInsecureSkipVerify) {
		return fmt.Errorf("warp gateway TLS options require an HTTPS base URL")
	}
	return nil
}

func isLoopbackWarpGatewayHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *WarpGatewayClient) Enabled() bool {
	return c != nil && c.cfg.Enabled && strings.TrimSpace(c.cfg.BaseURL) != ""
}

func (c *WarpGatewayClient) do(ctx context.Context, method, path string, in any, out any) error {
	return c.doClient(ctx, c.client, method, path, in, out)
}

func (c *WarpGatewayClient) doClient(ctx context.Context, client *http.Client, method, path string, in any, out any) error {
	if c == nil {
		return fmt.Errorf("warp gateway client is nil")
	}
	if c.initErr != nil {
		return c.initErr
	}
	if !c.Enabled() {
		return fmt.Errorf("warp gateway disabled")
	}
	if client == nil {
		client = c.client
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		// Mutation endpoints may return structured partial results on failure.
		// Decode them before surfacing the HTTP error so callers can reconcile
		// resources that were already committed by the gateway.
		if out != nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, out)
		}
		return fmt.Errorf("warp gateway %s %s: HTTP %d: %s", method, path, resp.StatusCode, string(raw))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *WarpGatewayClient) ListInstances(ctx context.Context) ([]WarpInstance, error) {
	var resp struct {
		Instances []WarpInstance `json:"instances"`
	}
	if err := c.do(ctx, http.MethodGet, "/v1/instances", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Instances, nil
}

func (c *WarpGatewayClient) CreatePool(ctx context.Context, namePrefix string, count int) ([]WarpInstance, error) {
	return c.CreatePoolEx(ctx, namePrefix, count, false)
}

// CreatePoolEx creates a pool; register=true asks gateway to auto-register free WARP profiles.
func (c *WarpGatewayClient) CreatePoolEx(ctx context.Context, namePrefix string, count int, register bool) ([]WarpInstance, error) {
	var resp struct {
		Instances []WarpInstance `json:"instances"`
		Created   []WarpInstance `json:"created"`
	}
	// Real WARP registration can take a while (count * handshake).
	timeout := c.cfg.Timeout
	if register && timeout < 120*time.Second {
		timeout = time.Duration(30+count*25) * time.Second
	}
	err := c.doWithTimeout(ctx, http.MethodPost, "/v1/pools", map[string]any{
		"name_prefix": namePrefix,
		"count":       count,
		"register":    register,
	}, &resp, timeout)
	if len(resp.Instances) == 0 && len(resp.Created) > 0 {
		resp.Instances = resp.Created
	}
	return resp.Instances, err
}

// RegisterProfiles asks gateway to register free WARP accounts (smoke / readiness).
func (c *WarpGatewayClient) RegisterProfiles(ctx context.Context, count int) (int, error) {
	var resp struct {
		Count int `json:"count"`
	}
	timeout := time.Duration(20+count*15) * time.Second
	err := c.doWithTimeout(ctx, http.MethodPost, "/v1/profiles/register", map[string]any{
		"count": count,
	}, &resp, timeout)
	return resp.Count, err
}

func (c *WarpGatewayClient) doWithTimeout(ctx context.Context, method, path string, in any, out any, timeout time.Duration) error {
	if timeout <= 0 {
		return c.do(ctx, method, path, in, out)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cli := &http.Client{
		Timeout:       timeout,
		Transport:     c.client.Transport,
		CheckRedirect: c.client.CheckRedirect,
	}
	return c.doClient(ctx, cli, method, path, in, out)
}

func (c *WarpGatewayClient) PoolSnapshot(ctx context.Context) (*WarpPoolSnapshot, error) {
	var snap WarpPoolSnapshot
	if err := c.do(ctx, http.MethodGet, "/v1/pools/snapshot", nil, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (c *WarpGatewayClient) HealthAll(ctx context.Context) (*WarpPoolSnapshot, []string, map[string][]string, error) {
	var resp struct {
		UnhealthyIDs []string            `json:"unhealthy_ids"`
		DuplicateIPs map[string][]string `json:"duplicate_exit_ips"`
		Snapshot     WarpPoolSnapshot    `json:"snapshot"`
	}
	if err := c.do(ctx, http.MethodPost, "/v1/health/all", map[string]any{}, &resp); err != nil {
		return nil, nil, nil, err
	}
	return &resp.Snapshot, resp.UnhealthyIDs, resp.DuplicateIPs, nil
}

func (c *WarpGatewayClient) Rotate(ctx context.Context, id string) (*WarpInstance, error) {
	var inst WarpInstance
	if err := c.do(ctx, http.MethodPost, "/v1/instances/"+id+"/rotate", map[string]any{}, &inst); err != nil {
		return nil, err
	}
	return &inst, nil
}

// DeleteInstance removes a gateway instance; deregisterCloudflare defaults to true.
func (c *WarpGatewayClient) DeleteInstance(ctx context.Context, id string, deregisterCloudflare bool) error {
	path := "/v1/instances/" + id
	if !deregisterCloudflare {
		path += "?deregister_cloudflare=false"
	}
	return c.do(ctx, http.MethodDelete, path, map[string]any{
		"deregister_cloudflare": deregisterCloudflare,
	}, nil)
}

// WarpPoolAttachPlan is a Phase-3 plan to sync gateway instances into Proxy rows/group.
type WarpPoolAttachPlan struct {
	// ProxySpecs ready to create or update in proxies table.
	ProxySpecs []WarpProxySpec `json:"proxy_specs"`
	// DetachProxyNames should be marked inactive (unhealthy auto-detach).
	DetachProxyNames []string `json:"detach_proxy_names"`
	// DuplicateExitIPs for admin alerts.
	DuplicateExitIPs map[string][]string `json:"duplicate_exit_ips"`
	// SuggestedGroupName for ProxyGroup creation.
	SuggestedGroupName string `json:"suggested_group_name"`
}

// WarpProxySpec maps a WARP instance to a sub2api Proxy.
type WarpProxySpec struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"` // socks5h
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	WarpID     string `json:"warp_id"`
	ExitIP     string `json:"exit_ip"`
	Status     string `json:"status"` // active | error
	ExternalID string `json:"external_id"`
}

// BuildAttachPlan converts a gateway snapshot into proxy upsert/detach actions (Phase 3).
func BuildAttachPlan(snap *WarpPoolSnapshot, groupName string) WarpPoolAttachPlan {
	if groupName == "" {
		groupName = "warp-pool"
	}
	plan := WarpPoolAttachPlan{
		SuggestedGroupName: groupName,
		DuplicateExitIPs:   map[string][]string{},
	}
	if snap == nil {
		return plan
	}
	plan.DuplicateExitIPs = snap.DuplicateIPs
	unhealthy := map[string]struct{}{}
	for _, id := range snap.UnhealthyIDs {
		unhealthy[id] = struct{}{}
	}
	usedNames := map[string]struct{}{}
	for _, inst := range snap.Instances {
		status := StatusActive
		name := "warp-" + inst.Name
		// Within one snapshot, gateway used to emit duplicate instance names on
		// multi-batch create; disambiguate so each SOCKS endpoint gets its own proxy.
		if _, taken := usedNames[name]; taken {
			if inst.ListenPort > 0 {
				name = fmt.Sprintf("%s-%d", name, inst.ListenPort)
			} else if inst.ID != "" {
				short := inst.ID
				if len(short) > 8 {
					short = short[:8]
				}
				name = fmt.Sprintf("%s-%s", name, short)
			}
		}
		// Still colliding (rare): keep appending short id.
		if _, taken := usedNames[name]; taken && inst.ID != "" {
			short := inst.ID
			if len(short) > 8 {
				short = short[:8]
			}
			name = fmt.Sprintf("%s-%s", name, short)
		}
		usedNames[name] = struct{}{}
		_, unhealthyByID := unhealthy[inst.ID]
		if !strings.EqualFold(strings.TrimSpace(inst.Status), "running") || unhealthyByID {
			status = StatusError
			plan.DetachProxyNames = append(plan.DetachProxyNames, name)
		}
		host := inst.ListenHost
		if host == "" {
			host = "127.0.0.1"
		}
		plan.ProxySpecs = append(plan.ProxySpecs, WarpProxySpec{
			Name:       name,
			Protocol:   "socks5h",
			Host:       host,
			Port:       inst.ListenPort,
			WarpID:     inst.ID,
			ExitIP:     inst.ExitIP,
			Status:     status,
			ExternalID: inst.ID,
			Username:   inst.SocksAuthUser,
			Password:   inst.SocksAuthPass,
		})
	}
	return plan
}
