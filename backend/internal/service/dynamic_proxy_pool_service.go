package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clashsub"
	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	DynamicPoolSourceExtractAPI   = "extract_api"
	DynamicPoolSourceSubscription = "subscription"
	DynamicPoolNamePrefixBase     = "dpool-"
)

// DynamicProxyPoolService manages CRUD and extraction for dynamic proxy pools.
type DynamicProxyPoolService struct {
	repo       DynamicProxyPoolRepository
	proxyRepo  ProxyRepository
	subRepo    ProxySubscriptionRepository
	httpClient *http.Client
	entryMu    sync.Mutex
	entries    map[int64]*poolEntryServer // pool ID -> entry server
	healthMu   sync.Mutex
	healthCheckLast map[int64]time.Time // pool ID -> last health check time
}

// NewDynamicProxyPoolService constructs the service.
// Uses a Transport that bypasses the system HTTP_PROXY/HTTPS_PROXY env vars
// so that extract requests go directly to the vendor API, not through the
// local proxy chain (e.g. 127.0.0.1:7887).
func NewDynamicProxyPoolService(
	repo DynamicProxyPoolRepository,
	proxyRepo ProxyRepository,
	subRepo ProxySubscriptionRepository,
) *DynamicProxyPoolService {
	return &DynamicProxyPoolService{
		repo:      repo,
		proxyRepo: proxyRepo,
		subRepo:   subRepo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: nil, // nil = no proxy, bypasses HTTP_PROXY env var
			},
		},
		entries:         make(map[int64]*poolEntryServer),
		healthCheckLast: make(map[int64]time.Time),
	}
}

// Create validates and persists a new dynamic proxy pool.
func (s *DynamicProxyPoolService) Create(ctx context.Context, p DynamicProxyPoolCreateParams) (*DynamicProxyPool, error) {
	m := &DynamicProxyPool{
		Name:               p.Name,
		Enabled:            true,
		SourceType:         coalesce(p.SourceType, DynamicPoolSourceExtractAPI),
		SubscriptionID:     p.SubscriptionID,
		ExtractURL:         p.ExtractURL,
		Protocol:           coalesce(p.Protocol, "http"),
		AuthMode:           coalesce(p.AuthMode, "none"),
		Username:           p.Username,
		Password:           p.Password,
		ResponseFormat:     coalesce(p.ResponseFormat, "txt"),
		LineSeparator:      coalesce(p.LineSeparator, "\r\n"),
		IPFieldPath:        p.IPFieldPath,
		PortFieldPath:      p.PortFieldPath,
		RefreshIntervalSec: max(p.RefreshIntervalSec, 60),
		IPDurationSec:      max(p.IPDurationSec, 30),
		ExtractCount:       max(p.ExtractCount, 1),
		MinAlive:           max(p.MinAlive, 1),
		HealthCheckIntervalSec: max(p.HealthCheckIntervalSec, 0),
	}

	// Generate name prefix
	m.NamePrefix = fmt.Sprintf("dpool-%s-", sanitizePrefix(m.Name))

	if err := s.validate(ctx, m, 0); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// Get returns a pool by ID.
func (s *DynamicProxyPoolService) Get(ctx context.Context, id int64) (*DynamicProxyPool, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns paginated pools.
func (s *DynamicProxyPoolService) List(ctx context.Context, params DynamicProxyPoolListParams) ([]*DynamicProxyPool, int64, error) {
	return s.repo.List(ctx, params)
}

// Update modifies an existing pool.
func (s *DynamicProxyPoolService) Update(ctx context.Context, id int64, p DynamicProxyPoolUpdateParams) (*DynamicProxyPool, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.Name != nil {
		m.Name = *p.Name
	}
	if p.Enabled != nil {
		m.Enabled = *p.Enabled
	}
	if p.SourceType != nil {
		m.SourceType = *p.SourceType
	}
	if p.SubscriptionID != nil {
		m.SubscriptionID = p.SubscriptionID
	}
	if p.ExtractURL != nil {
		m.ExtractURL = *p.ExtractURL
	}
	if p.Protocol != nil {
		m.Protocol = *p.Protocol
	}
	if p.AuthMode != nil {
		m.AuthMode = *p.AuthMode
	}
	if p.Username != nil {
		m.Username = *p.Username
	}
	if p.Password != nil {
		m.Password = *p.Password
	}
	if p.ResponseFormat != nil {
		m.ResponseFormat = *p.ResponseFormat
	}
	if p.LineSeparator != nil {
		m.LineSeparator = *p.LineSeparator
	}
	if p.IPFieldPath != nil {
		m.IPFieldPath = *p.IPFieldPath
	}
	if p.PortFieldPath != nil {
		m.PortFieldPath = *p.PortFieldPath
	}
	if p.RefreshIntervalSec != nil {
		m.RefreshIntervalSec = max(*p.RefreshIntervalSec, 60)
	}
	if p.IPDurationSec != nil {
		m.IPDurationSec = max(*p.IPDurationSec, 30)
	}
	if p.ExtractCount != nil {
		m.ExtractCount = max(*p.ExtractCount, 1)
	}
	if p.MinAlive != nil {
		m.MinAlive = max(*p.MinAlive, 1)
	}
	if p.HealthCheckIntervalSec != nil {
		m.HealthCheckIntervalSec = max(*p.HealthCheckIntervalSec, 0)
	}

	if err := s.validate(ctx, m, id); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

// Delete removes a pool, its owned proxy records, and any entry proxy record.
func (s *DynamicProxyPoolService) Delete(ctx context.Context, id int64) error {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	// Stop entry proxy if running
	s.StopEntryProxy(id)

	// Clean up entry proxy record
	entryPrefix := "pool-entry-"
	entryName := fmt.Sprintf("%s%s", entryPrefix, sanitizePrefix(m.Name))
	existing, _ := s.proxyRepo.ListOwnedByPrefix(ctx, entryPrefix)
	for _, p := range existing {
		if p.Name == entryName && p.AccountCount == 0 {
			_ = s.proxyRepo.Delete(ctx, p.ID)
		}
	}

	// Clean up owned proxies
	owned, err := s.proxyRepo.ListOwnedByPrefix(ctx, m.NamePrefix)
	if err != nil {
		return fmt.Errorf("list owned proxies: %w", err)
	}
	for _, p := range owned {
		if p.AccountCount == 0 {
			_ = s.proxyRepo.Delete(ctx, p.ID)
		}
	}
	return s.repo.Delete(ctx, id)
}

// Extract triggers an immediate IP extraction for the pool.
func (s *DynamicProxyPoolService) Extract(ctx context.Context, id int64) (*DynamicProxyPoolExtractResult, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	switch m.SourceType {
	case DynamicPoolSourceExtractAPI:
		if m.ExtractURL == "" {
			return nil, errors.BadRequest("POOL_NO_URL", "extract_url is empty")
		}
		return s.doExtract(ctx, m)
	case DynamicPoolSourceSubscription:
		return s.doRefreshFromSubscription(ctx, m)
	default:
		return nil, errors.BadRequest("POOL_SOURCE_INVALID", "unsupported source_type")
	}
}

// RefreshDue extracts new IPs for enabled pools. For extract_api pools it calls
// the extraction URL; for subscription pools it parses the linked subscription source.
// Expired proxy rows owned by a pool are pruned first so alive counts reflect reality.
func (s *DynamicProxyPoolService) RefreshDue(ctx context.Context, now time.Time) error {
	pools, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("list enabled pools: %w", err)
	}
	for _, m := range pools {
		alive, err := s.pruneAndCount(ctx, m.NamePrefix, now)
		if err != nil {
			continue
		}
		if alive != m.AliveCount {
			_ = s.repo.UpdateAliveCount(ctx, m.ID, alive)
		}

		// Run periodic health checks if enabled; mark dead proxies inactive so
		// the entry proxy won't pick them, and refresh when too few remain.
		if m.HealthCheckIntervalSec > 0 {
			s.healthCheckPool(ctx, m, now)
			alive, _ = s.pruneAndCount(ctx, m.NamePrefix, now)
			_ = s.repo.UpdateAliveCount(ctx, m.ID, alive)
		}

		due := alive < m.MinAlive
		if !due && m.LastExtractAt != nil {
			due = now.Sub(*m.LastExtractAt) >= time.Duration(m.RefreshIntervalSec)*time.Second
		}
		if !due && m.LastExtractAt == nil {
			due = true
		}
		if !due {
			continue
		}
		if err := s.refreshOne(ctx, m); err != nil {
			log.Printf("[DynamicProxyPool] refresh failed pool=%d name=%s: %v", m.ID, m.Name, err)
		}
	}
	return nil
}

// healthCheckPool tests each active pool-owned proxy and marks failures inactive.
// The last check time is tracked in-memory per pool to respect the interval.
func (s *DynamicProxyPoolService) healthCheckPool(ctx context.Context, m *DynamicProxyPool, now time.Time) {
	if m == nil || m.HealthCheckIntervalSec <= 0 {
		return
	}
	// Respect interval: only check if enough time has passed since last check.
	lastCheck, ok := s.healthCheckLast[m.ID]
	if ok && now.Sub(lastCheck) < time.Duration(m.HealthCheckIntervalSec)*time.Second {
		return
	}
	s.healthCheckLast[m.ID] = now

	owned, err := s.proxyRepo.ListOwnedByPrefix(ctx, m.NamePrefix)
	if err != nil {
		return
	}
	for _, p := range owned {
		if p.IsExpired(now) {
			continue
		}
		// Only check active or previously-unhealthy proxies; skip already-expired/inactive.
		if p.Status != StatusActive && p.Status != StatusError {
			continue
		}
		ok := s.testProxyLive(p.Proxy)
		target := StatusActive
		if !ok {
			target = StatusError
		}
		if p.Status != target {
			// Fetch fresh proxy to preserve all fields, then update status.
			proxy, err := s.proxyRepo.GetByID(ctx, p.ID)
			if err == nil && proxy != nil {
				proxy.Status = target
				_ = s.proxyRepo.Update(ctx, proxy)
			}
		}
	}
}

// testProxyLive performs a lightweight HTTP GET through the proxy to verify it works.
func (s *DynamicProxyPoolService) testProxyLive(p Proxy) bool {
	proxyURL := p.URL()
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse(proxyURL)
			},
		},
	}
	resp, err := client.Get("http://api64.ipify.org?format=json")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

func (s *DynamicProxyPoolService) refreshOne(ctx context.Context, m *DynamicProxyPool) error {
	switch m.SourceType {
	case DynamicPoolSourceExtractAPI:
		if strings.TrimSpace(m.ExtractURL) == "" {
			return nil
		}
		_, err := s.doExtract(ctx, m)
		return err
	case DynamicPoolSourceSubscription:
		_, err := s.doRefreshFromSubscription(ctx, m)
		return err
	}
	return nil
}

// doRefreshFromSubscription fetches the linked subscription's sidecar proxies
// (local HTTP endpoints spawned by mihomo) and creates short-lived alias proxy
// records for the pool. Raw VLESS/Trojan/Shadowsocks nodes are not directly
// usable as HTTP proxies, so we use the sidecar endpoints instead.
func (s *DynamicProxyPoolService) doRefreshFromSubscription(ctx context.Context, m *DynamicProxyPool) (*DynamicProxyPoolExtractResult, error) {
	if m.SubscriptionID == nil {
		return nil, errors.BadRequest("POOL_NO_SUBSCRIPTION", "subscription_id is required for subscription source")
	}
	sub, err := s.subRepo.GetByID(ctx, *m.SubscriptionID)
	if err != nil {
		s.updateExtractState(ctx, m.ID, "error", fmt.Sprintf("get subscription: %v", err))
		return nil, fmt.Errorf("get subscription %d: %w", *m.SubscriptionID, err)
	}
	if !sub.Enabled {
		return nil, errors.BadRequest("POOL_SUBSCRIPTION_DISABLED", "linked subscription source is disabled")
	}

	// Get sidecar proxies from the linked subscription's name_prefix
	sidecars, err := s.proxyRepo.ListOwnedByPrefix(ctx, sub.NamePrefix)
	if err != nil {
		msg := fmt.Sprintf("list sidecar proxies: %v", err)
		s.updateExtractState(ctx, m.ID, "error", msg)
		return nil, fmt.Errorf("%s", msg)
	}

	// Filter only active sidecar proxies (local HTTP listeners)
	now := time.Now()
	var activeSidecars []ProxyWithAccountCount
	for _, p := range sidecars {
		if p.Status == StatusActive && !p.IsExpired(now) {
			activeSidecars = append(activeSidecars, p)
		}
	}
	if len(activeSidecars) == 0 {
		msg := "no active sidecar proxies found for linked subscription"
		s.updateExtractState(ctx, m.ID, "error", msg)
		return nil, fmt.Errorf("%s", msg)
	}

	// Create pool proxy records referencing the sidecar endpoints
	result := &DynamicProxyPoolExtractResult{}
	expiresAt := now.Add(time.Duration(m.IPDurationSec) * time.Second)

	for _, sc := range activeSidecars {
		proxy := &Proxy{
			Name:           fmt.Sprintf("%s%s", m.NamePrefix, sc.Name),
			Protocol:       sc.Protocol,
			Host:           sc.Host,
			Port:           sc.Port,
			Username:       sc.Username,
			Password:       sc.Password,
			Status:         StatusActive,
			ExpiresAt:      &expiresAt,
			FallbackMode:   FallbackModeNone,
			ExpiryWarnDays: 0,
		}
		// Apply fixed auth if configured
		if m.AuthMode == "fixed" {
			proxy.Username = m.Username
			proxy.Password = m.Password
		}

		if err := s.proxyRepo.Create(ctx, proxy); err != nil {
			result.Failed++
			continue
		}
		result.Created++
	}

	s.updateExtractState(ctx, m.ID, "success", "")
	alive, _ := s.pruneAndCount(ctx, m.NamePrefix, time.Now())
	_ = s.repo.UpdateAliveCount(ctx, m.ID, alive)
	result.AliveCount = alive

	return result, nil
}


// ---------------------------------------------------------------------------
// Entry proxy server — a local HTTP proxy that randomly selects a live pool IP
// each request. Bind a single proxy entry (e.g. 127.0.0.1:9900) and point
// accounts at it; every request gets a random pool exit IP.
// ---------------------------------------------------------------------------

// poolEntryServer is a local HTTP proxy that picks a random alive proxy from
// the pool for each CONNECT / GET request.
type poolEntryServer struct {
	poolID  int64
	svc     *DynamicProxyPoolService
	server  *http.Server
	addr    string
	running bool
}

// ListPoolProxies returns pool-owned proxies by name_prefix.
func (s *DynamicProxyPoolService) ListPoolProxies(ctx context.Context, prefix string) ([]ProxyWithAccountCount, error) {
	return s.proxyRepo.ListOwnedByPrefix(ctx, prefix)
}

// AddSubscriptionNodesToProxies fetches the linked subscription, parses its nodes,
// and writes selected nodes (by identity) as regular Proxy records.
// If identities is empty, adds all nodes. Returns the created proxy IDs.
func (s *DynamicProxyPoolService) AddSubscriptionNodesToProxies(ctx context.Context, poolID int64, identities []string) ([]int64, error) {
	m, err := s.repo.GetByID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	if m.SourceType != DynamicPoolSourceSubscription || m.SubscriptionID == nil {
		return nil, errors.BadRequest("POOL_NOT_SUBSCRIPTION", "pool must be subscription type")
	}
	sub, err := s.subRepo.GetByID(ctx, *m.SubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}

	// Fetch and parse nodes
	body, err := s.fetchSubBody(ctx, sub)
	if err != nil {
		return nil, err
	}
	nodes, err := clashsub.ParseSubscription(body)
	if err != nil {
		return nil, fmt.Errorf("parse subscription: %w", err)
	}

	identitySet := make(map[string]bool, len(identities))
	for _, id := range identities {
		identitySet[id] = true
	}

	var createdIDs []int64
	for _, node := range nodes {
		if len(identitySet) > 0 && !identitySet[node.Identity] {
			continue
		}
		server, portStr := extractNodeServerPort(node)
		if server == "" || portStr == "" {
			continue
		}
		port, _ := strconv.Atoi(portStr)
		if port <= 0 || port > 65535 {
			continue
		}
		proxy := &Proxy{
			Name:           fmt.Sprintf("%s%s", m.NamePrefix, node.Name),
			Protocol:       m.Protocol,
			Host:           server,
			Port:           port,
			Status:         StatusActive,
			FallbackMode:   FallbackModeNone,
			ExpiryWarnDays: 0,
		}
		if m.AuthMode == "fixed" {
			proxy.Username = m.Username
			proxy.Password = m.Password
		}
		if err := s.proxyRepo.Create(ctx, proxy); err != nil {
			continue
		}
		createdIDs = append(createdIDs, proxy.ID)
	}
	return createdIDs, nil
}

// LookupSubscription returns a subscription by ID.
func (s *DynamicProxyPoolService) LookupSubscription(ctx context.Context, id int64) (*ProxySubscription, error) {
	return s.subRepo.GetByID(ctx, id)
}

// PreviewSubscriptionNodes fetches and parses the subscription nodes.
func (s *DynamicProxyPoolService) PreviewSubscriptionNodes(ctx context.Context, sub *ProxySubscription) ([]ProxySubscriptionPreviewNode, error) {
	body, err := s.fetchSubBody(ctx, sub)
	if err != nil {
		return nil, err
	}
	nodes, err := clashsub.ParseSubscription(body)
	if err != nil {
		return nil, err
	}
	out := make([]ProxySubscriptionPreviewNode, 0, len(nodes))
	for _, n := range nodes {
		server, port := extractNodeServerPort(n)
		out = append(out, ProxySubscriptionPreviewNode{
			Identity: n.Identity,
			Name:     n.Name,
			Type:     n.Type,
			Server:   server,
			Port:     port,
		})
	}
	return out, nil
}

func (s *DynamicProxyPoolService) fetchSubBody(ctx context.Context, sub *ProxySubscription) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(sub.SourceType)) {
	case "inline", "body", "text":
		body := strings.TrimSpace(sub.InlineBody)
		if body == "" {
			return nil, fmt.Errorf("inline body is empty")
		}
		return []byte(body), nil
	default:
		rawURL := strings.TrimSpace(sub.SubscriptionURL)
		if rawURL == "" {
			return nil, fmt.Errorf("subscription_url is empty")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "sub2api-dynamic-pool/0.1")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch subscription: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
		}
		return io.ReadAll(io.LimitReader(resp.Body, 1<<24))
	}
}

func extractNodeServerPort(node clashsub.Node) (server, port string) {
	if node.Raw != nil {
		if v, ok := node.Raw["server"]; ok {
			server = fmt.Sprintf("%v", v)
		}
		if server == "" {
			if v, ok := node.Raw["servername"]; ok {
				server = fmt.Sprintf("%v", v)
			}
		}
		if v, ok := node.Raw["port"]; ok {
			port = fmt.Sprintf("%v", v)
		}
	}
	return
}

// TestPoolProxy tests a pool-owned proxy connection.
func (s *DynamicProxyPoolService) TestPoolProxy(ctx context.Context, proxyID int64) (*ProxyTestResult, error) {
	proxy, err := s.proxyRepo.GetByID(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	proxyURL := proxy.URL()
	// Simple connectivity test via HTTP GET
	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return url.Parse(proxyURL)
			},
		},
	}
	t0 := time.Now()
	resp, err := client.Get("http://api64.ipify.org?format=json")
	if err != nil {
		return &ProxyTestResult{Success: false, Message: err.Error()}, nil
	}
	defer resp.Body.Close()
	latency := time.Since(t0).Milliseconds()
	return &ProxyTestResult{Success: true, LatencyMs: latency, Message: "proxy is accessible"}, nil
}

// QualityCheckPoolProxy checks a pool-owned proxy against common AI targets.
func (s *DynamicProxyPoolService) QualityCheckPoolProxy(ctx context.Context, proxyID int64) (*ProxyQualityCheckResult, error) {
	// Proxy quality check reuses the existing adminService.CheckProxyQuality
	// via the proxyProber from the injected service.
	proxy, err := s.proxyRepo.GetByID(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	_ = proxy
	return &ProxyQualityCheckResult{
		Score:          0,
		Grade:          "skip",
		Summary:        "quality check not implemented for dynamic pool proxies",
		PassedCount:    0,
		WarnCount:      0,
		FailedCount:    0,
		ChallengeCount: 0,
		CheckedAt:      time.Now().Unix(),
		Items:          []ProxyQualityCheckItem{},
	}, nil
}

// ListPoolEntryProxies returns all pool entry proxy addresses for group binding.
func (s *DynamicProxyPoolService) ListPoolEntryProxies(ctx context.Context) ([]Proxy, error) {
	pools, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	var result []Proxy
	for _, pool := range pools {
		owned, err := s.proxyRepo.ListOwnedByPrefix(ctx, pool.NamePrefix)
		if err != nil {
			continue
		}
		now := time.Now()
		for _, p := range owned {
			if p.Status == StatusActive && !p.IsExpired(now) {
				result = append(result, p.Proxy)
				break // one representative per pool is enough
			}
		}
	}
	return result, nil
}

// ListPoolEntryProxiesForGroup returns pool entry proxies formatted for group API.
func (s *DynamicProxyPoolService) ListPoolEntryProxiesForGroup(ctx context.Context) ([]proxyOption, error) {
	pools, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	var opts []proxyOption
	for _, pool := range pools {
		owned, err := s.proxyRepo.ListOwnedByPrefix(ctx, pool.NamePrefix)
		if err != nil {
			continue
		}
		now := time.Now()
		for _, p := range owned {
			if p.Status == StatusActive && !p.IsExpired(now) {
				opts = append(opts, proxyOption{
					ID:   p.ID,
					Name: fmt.Sprintf("[池] %s - %s", pool.Name, p.Name),
					URL:  p.URL(),
				})
				break
			}
		}
	}
	return opts, nil
}

type proxyOption struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// AssociateProxies creates pool-owned copies of existing proxies so the pool
// entry proxy can randomly select from them. The original proxies are left intact.
func (s *DynamicProxyPoolService) AssociateProxies(ctx context.Context, poolID int64, proxyIDs []int64) (*DynamicProxyPoolExtractResult, error) {
	m, err := s.repo.GetByID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	result := &DynamicProxyPoolExtractResult{}
	now := time.Now()
	expiresAt := now.Add(time.Duration(m.IPDurationSec) * time.Second)

	for _, pid := range proxyIDs {
		proxy, err := s.proxyRepo.GetByID(ctx, pid)
		if err != nil {
			result.Failed++
			continue
		}
		// Copy with pool prefix
		cp := &Proxy{
			Name:           fmt.Sprintf("%s%s", m.NamePrefix, proxy.Name),
			Protocol:       proxy.Protocol,
			Host:           proxy.Host,
			Port:           proxy.Port,
			Username:       proxy.Username,
			Password:       proxy.Password,
			Status:         StatusActive,
			ExpiresAt:      &expiresAt,
			FallbackMode:   FallbackModeNone,
			ExpiryWarnDays: 0,
		}
		if err := s.proxyRepo.Create(ctx, cp); err != nil {
			result.Failed++
			continue
		}
		result.Created++
	}

	s.updateExtractState(ctx, m.ID, "success", "")
	alive, _ := s.pruneAndCount(ctx, m.NamePrefix, time.Now())
	_ = s.repo.UpdateAliveCount(ctx, m.ID, alive)
	result.AliveCount = alive
	return result, nil
}

// DisassociateProxies removes pool-owned proxy copies by their IDs.
func (s *DynamicProxyPoolService) DisassociateProxies(ctx context.Context, poolID int64, proxyIDs []int64) *DynamicProxyPoolExtractResult {
	result := &DynamicProxyPoolExtractResult{}
	// Get pool to find prefix
	pool, err := s.repo.GetByID(ctx, poolID)
	if err != nil {
		return result
	}
	// Get all owned proxies to check account counts
	owned, err := s.proxyRepo.ListOwnedByPrefix(ctx, pool.NamePrefix)
	if err != nil {
		return result
	}
	ownedByID := make(map[int64]int64)
	for _, p := range owned {
		ownedByID[p.ID] = p.AccountCount
	}
	for _, pid := range proxyIDs {
		if acctCount, ok := ownedByID[pid]; ok {
			if acctCount > 0 {
				result.Failed++
				continue
			}
		}
		if err := s.proxyRepo.Delete(ctx, pid); err != nil {
			result.Failed++
			continue
		}
		result.AliveCount++
	}
	return result
}

// EnsureEntryProxy starts or updates the entry proxy for a pool on the given
// bind address. The proxy handles HTTP CONNECT (tunnels) and direct HTTP GET.
func (s *DynamicProxyPoolService) EnsureEntryProxy(ctx context.Context, poolID int64, bindAddr string) error {
	s.entryMu.Lock()
	defer s.entryMu.Unlock()

	// Stop existing if running
	if existing, ok := s.entries[poolID]; ok && existing.running {
		if existing.addr == bindAddr {
			return nil // already running on same address
		}
		_ = existing.server.Close()
		delete(s.entries, poolID)
	}

	entry := &poolEntryServer{
		poolID: poolID,
		svc:    s,
		addr:   bindAddr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", entry.serveHTTP)

	entry.server = &http.Server{
		Addr:    bindAddr,
		Handler: mux,
	}

	go func() {
		log.Printf("[DynamicProxyPool] entry proxy starting pool=%d addr=%s", poolID, bindAddr)
		if err := entry.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[DynamicProxyPool] entry proxy error pool=%d: %v", poolID, err)
		}
	}()

	entry.running = true
	s.entries[poolID] = entry

	// Create a persistent Proxy record so group/account/batch-edit selectors
	// automatically see this pool entry proxy as a bindable proxy.
	s.ensureEntryProxyRecord(ctx, poolID, bindAddr)

	return nil
}

// ensureEntryProxyRecord upserts a durable Proxy row pointing at the local entry
// proxy listener so the standard proxy selectors (group default proxy, account
// proxy, batch test proxy) can bind to it by proxy ID.
func (s *DynamicProxyPoolService) ensureEntryProxyRecord(ctx context.Context, poolID int64, bindAddr string) {
	if poolID <= 0 || bindAddr == "" {
		return
	}
	host, portStr, err := net.SplitHostPort(bindAddr)
	if err != nil || portStr == "" {
		host = "127.0.0.1"
		portStr = bindAddr
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return
	}
	pool, err := s.repo.GetByID(ctx, poolID)
	if err != nil {
		return
	}

	const entryPrefix = "pool-entry-"
	entryName := fmt.Sprintf("%s%s", entryPrefix, sanitizePrefix(pool.Name))

	existing, err := s.proxyRepo.ListOwnedByPrefix(ctx, entryPrefix)
	if err == nil {
		for _, p := range existing {
			if strings.Contains(p.Name, sanitizePrefix(pool.Name)) {
				return // already exists
			}
		}
	}

	proxy := &Proxy{
		Name:           entryName,
		Protocol:       "http",
		Host:           host,
		Port:           port,
		Status:         StatusActive,
		FallbackMode:   FallbackModeNone,
		ExpiryWarnDays: 0,
	}
	_ = s.proxyRepo.Create(ctx, proxy)
}

// StopEntryProxy stops the entry proxy for a pool.
func (s *DynamicProxyPoolService) StopEntryProxy(poolID int64) {
	s.entryMu.Lock()
	defer s.entryMu.Unlock()
	if entry, ok := s.entries[poolID]; ok && entry.running {
		_ = entry.server.Close()
		entry.running = false
		delete(s.entries, poolID)
	}
}

// StopAllEntryProxies stops all entry proxies (used on server shutdown).
func (s *DynamicProxyPoolService) StopAllEntryProxies() {
	s.entryMu.Lock()
	defer s.entryMu.Unlock()
	for id, entry := range s.entries {
		if entry.running {
			_ = entry.server.Close()
		}
		delete(s.entries, id)
	}
}

// pickRandomAlive picks a random alive proxy from the pool. Returns nil if none.
func (s *DynamicProxyPoolService) pickRandomAlive(ctx context.Context, poolID int64) (*Proxy, error) {
	m, err := s.repo.GetByID(ctx, poolID)
	if err != nil {
		return nil, err
	}
	owned, err := s.proxyRepo.ListOwnedByPrefix(ctx, m.NamePrefix)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var alive []*ProxyWithAccountCount
	for _, p := range owned {
		if p.Status == StatusActive && !p.IsExpired(now) {
			alive = append(alive, &p)
		}
	}
	if len(alive) == 0 {
		return nil, nil
	}
	return &alive[rand.Intn(len(alive))].Proxy, nil
}

func (e *poolEntryServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodConnect {
		e.serveConnect(w, r)
		return
	}
	e.serveHTTPProxy(w, r)
}

func (e *poolEntryServer) serveConnect(w http.ResponseWriter, r *http.Request) {
	proxy, err := e.svc.pickRandomAlive(context.Background(), e.poolID)
	if err != nil || proxy == nil {
		http.Error(w, "no alive proxy in pool", http.StatusServiceUnavailable)
		return
	}

	dest := r.Host

	// Connect to the proxy and send CONNECT
	proxyHost := net.JoinHostPort(proxy.Host, strconv.Itoa(proxy.Port))
	targetConn, err := net.DialTimeout("tcp", proxyHost, 10*time.Second)
	if err != nil {
		http.Error(w, "proxy connect failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", dest, dest)
	if proxy.Username != "" && proxy.Password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(proxy.Username + ":" + proxy.Password))
		req += "Proxy-Authorization: Basic " + auth + "\r\n"
	}
	req += "\r\n"

	if _, err := targetConn.Write([]byte(req)); err != nil {
		targetConn.Close()
		http.Error(w, "proxy write failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	resp, err := http.ReadResponse(bufio.NewReader(targetConn), nil)
	if err != nil {
		targetConn.Close()
		http.Error(w, "proxy read response failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		targetConn.Close()
		http.Error(w, "proxy CONNECT failed: "+resp.Status, http.StatusBadGateway)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		targetConn.Close()
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		targetConn.Close()
		http.Error(w, "hijack failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	clientConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(targetConn, clientConn)
		targetConn.Close()
	}()
	go func() {
		defer wg.Done()
		io.Copy(clientConn, targetConn)
		clientConn.Close()
	}()
	wg.Wait()
}

func (e *poolEntryServer) serveHTTPProxy(w http.ResponseWriter, r *http.Request) {
	proxy, err := e.svc.pickRandomAlive(context.Background(), e.poolID)
	if err != nil || proxy == nil {
		http.Error(w, "no alive proxy in pool", http.StatusServiceUnavailable)
		return
	}

	proxyURL := proxy.URL()
	transport := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			return url.Parse(proxyURL)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}

	outReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for key, values := range r.Header {
		for _, v := range values {
			outReq.Header.Add(key, v)
		}
	}

	resp, err := client.Do(outReq)
	if err != nil {
		http.Error(w, "proxy request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// pruneAndCount deletes expired unused proxy rows owned by prefix and returns
// the remaining alive count.
func (s *DynamicProxyPoolService) pruneAndCount(ctx context.Context, prefix string, now time.Time) (int, error) {
	owned, err := s.proxyRepo.ListOwnedByPrefix(ctx, prefix)
	if err != nil {
		return 0, err
	}
	alive := 0
	for _, p := range owned {
		if p.IsExpired(now) {
			if p.AccountCount == 0 {
				_ = s.proxyRepo.Delete(ctx, p.ID)
			}
			continue
		}
		if p.Status == StatusActive {
			alive++
		}
	}
	return alive, nil
}

func (s *DynamicProxyPoolService) doExtract(ctx context.Context, m *DynamicProxyPool) (*DynamicProxyPoolExtractResult, error) {
	extractURL := sanitizeExtractURL(m.ExtractURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, extractURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.updateExtractState(ctx, m.ID, "error", err.Error())
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		s.updateExtractState(ctx, m.ID, "error", err.Error())
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncateStr(string(body), 200))
		s.updateExtractState(ctx, m.ID, "error", msg)
		return nil, fmt.Errorf("extract failed: %s", msg)
	}

	// Parse IPs
	endpoints, err := s.parseResponse(m, body)
	if err != nil {
		s.updateExtractState(ctx, m.ID, "error", err.Error())
		return nil, err
	}

	// Create proxy records
	result := &DynamicProxyPoolExtractResult{}
	now := time.Now()
	expiresAt := now.Add(time.Duration(m.IPDurationSec) * time.Second)

	for _, ep := range endpoints {
		proxy := &Proxy{
			Name:           fmt.Sprintf("%s%s:%d", m.NamePrefix, ep.IP, ep.Port),
			Protocol:       m.Protocol,
			Host:           ep.IP,
			Port:           ep.Port,
			Username:       ep.Username,
			Password:       ep.Password,
			Status:         StatusActive,
			ExpiresAt:      &expiresAt,
			FallbackMode:   FallbackModeNone,
			ExpiryWarnDays: 0,
		}
		// Apply fixed auth if configured
		if m.AuthMode == "fixed" {
			proxy.Username = m.Username
			proxy.Password = m.Password
		}

		if err := s.proxyRepo.Create(ctx, proxy); err != nil {
			result.Failed++
			continue
		}
		result.Created++
	}

	s.updateExtractState(ctx, m.ID, "success", "")
	alive, _ := s.pruneAndCount(ctx, m.NamePrefix, time.Now())
	_ = s.repo.UpdateAliveCount(ctx, m.ID, alive)
	result.AliveCount = alive

	return result, nil
}

func (s *DynamicProxyPoolService) parseResponse(m *DynamicProxyPool, body []byte) ([]extractedEndpoint, error) {
	switch strings.ToLower(m.ResponseFormat) {
	case "json":
		return s.parseJSON(m, body)
	default:
		return s.parseTxt(m, body)
	}
}

type extractedEndpoint struct {
	IP       string
	Port     int
	Username string
	Password string
}

// proxyLineRegex matches formats like:
// ip:port
// ip:port:user:pass
// protocol://user:pass@ip:port
var proxyLineRegex = regexp.MustCompile(`^(?:(?:[\w]+)://)?(?:([^:@]+):([^@]+)@)?([^:]+):(\d+)(?::([^:]+):(.+))?$`)

func (s *DynamicProxyPoolService) parseTxt(m *DynamicProxyPool, body []byte) ([]extractedEndpoint, error) {
	sep := m.LineSeparator
	switch sep {
	case "", `\r\n`:
		sep = "\r\n"
	case `\n`:
		sep = "\n"
	}
	content := strings.TrimSpace(string(body))
	lines := strings.Split(content, sep)
	if len(lines) == 1 && sep == "\r\n" {
		// Fallback to \n if \r\n didn't split
		lines = strings.Split(content, "\n")
	}

	var endpoints []extractedEndpoint
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ep, err := parseProxyLine(line)
		if err != nil {
			continue
		}
		endpoints = append(endpoints, ep)
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no valid IPs found in response")
	}
	return endpoints, nil
}

func parseProxyLine(line string) (extractedEndpoint, error) {
	matches := proxyLineRegex.FindStringSubmatch(line)
	if matches == nil {
		// Try simple ip:port
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			port, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err == nil && port > 0 && port <= 65535 {
				return extractedEndpoint{IP: strings.TrimSpace(parts[0]), Port: port}, nil
			}
		}
		return extractedEndpoint{}, fmt.Errorf("unrecognized format: %s", line)
	}

	ip := matches[3]
	port, _ := strconv.Atoi(matches[4])
	user := matches[1]
	pass := matches[2]
	// Check for ip:port:user:pass format
	if matches[5] != "" && matches[6] != "" {
		user = matches[5]
		pass = matches[6]
	}
	return extractedEndpoint{IP: ip, Port: port, Username: user, Password: pass}, nil
}

func (s *DynamicProxyPoolService) parseJSON(m *DynamicProxyPool, body []byte) ([]extractedEndpoint, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	ipPath := m.IPFieldPath
	portPath := m.PortFieldPath
	if ipPath == "" {
		ipPath = "ip"
	}
	if portPath == "" {
		portPath = "port"
	}

	// Try to find array in response
	items := findJSONArray(raw, ipPath)
	if items == nil {
		return nil, fmt.Errorf("no array found at path for IP extraction")
	}

	var endpoints []extractedEndpoint
	for _, item := range items {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ip := jsonString(obj, ipPath)
		port := jsonInt(obj, portPath)
		if ip == "" || port == 0 {
			continue
		}
		endpoints = append(endpoints, extractedEndpoint{
			IP:       ip,
			Port:     port,
			Username: jsonString(obj, "username"),
			Password: jsonString(obj, "password"),
		})
	}
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no valid IPs in JSON response")
	}
	return endpoints, nil
}

func (s *DynamicProxyPoolService) updateExtractState(ctx context.Context, id int64, status, errMsg string) {
	now := time.Now()
	_ = s.repo.UpdateExtractState(ctx, id, status, errMsg, &now)
}

func (s *DynamicProxyPoolService) validate(ctx context.Context, m *DynamicProxyPool, excludeID int64) error {
	if m.Name == "" {
		return errors.BadRequest("POOL_NAME_REQUIRED", "name is required")
	}
	if m.SourceType == DynamicPoolSourceExtractAPI && m.ExtractURL == "" {
		return errors.BadRequest("POOL_URL_REQUIRED", "extract_url is required for extract_api source")
	}
	// Check name_prefix uniqueness
	exists, err := s.repo.ExistsNamePrefix(ctx, m.NamePrefix, excludeID)
	if err != nil {
		return err
	}
	if exists {
		return errors.Conflict("POOL_PREFIX_CONFLICT", "name_prefix already in use")
	}
	return nil
}

// --- Helpers ---

func coalesce(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}

// sanitizeExtractURL removes/encodes control characters that break net/url parsing.
// Vendors sometimes put literal \r\n in query values (e.g. dl=\r\n); those must be
// percent-encoded or treated as the two-char sequence "\r\n".
func sanitizeExtractURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		switch c {
		case '\r':
			b.WriteString("%0D")
		case '\n':
			b.WriteString("%0A")
		case '\t':
			b.WriteString("%09")
		default:
			if c < 0x20 || c == 0x7f {
				b.WriteString(fmt.Sprintf("%%%02X", c))
				continue
			}
			b.WriteByte(c)
		}
	}
	return b.String()
}

func sanitizePrefix(name string) string {
	// Keep alphanumeric, dash, and non-ASCII (Chinese etc.), lowercase, max 20 chars
	var b strings.Builder
	lastDash := false
	count := 0
	for _, r := range name {
		if count >= 20 {
			break
		}
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
			count++
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
			lastDash = false
			count++
		case r > 0x7f:
			// Non-ASCII: keep as-is (Chinese, etc.)
			b.WriteRune(r)
			lastDash = false
			count++
		case r == '-':
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
				count++
			}
		case r == ' ':
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
				count++
			}
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		result = "pool"
	}
	return result
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func findJSONArray(v any, _ string) []any {
	// Simple: if top-level is array, use it; if map, look for "data" key
	switch val := v.(type) {
	case []any:
		return val
	case map[string]any:
		if d, ok := val["data"]; ok {
			if arr, ok2 := d.([]any); ok2 {
				return arr
			}
		}
		// Try first array field
		for _, field := range val {
			if arr, ok := field.([]any); ok {
				return arr
			}
		}
	}
	return nil
}

func jsonString(obj map[string]any, key string) string {
	v, ok := obj[key]
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return fmt.Sprintf("%v", s)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func jsonInt(obj map[string]any, key string) int {
	v, ok := obj[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(n)
		return i
	default:
		return 0
	}
}
