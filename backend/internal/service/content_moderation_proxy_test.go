package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

// contentModerationTestProxyRepo 仅实现审计代理路径用到的 GetByID，其余方法不应被调用。
type contentModerationTestProxyRepo struct {
	proxies    map[int64]*Proxy
	getByIDErr error
	getCalls   atomic.Int64
REDACTED

func (r *contentModerationTestProxyRepo) GetByID(ctx context.Context, id int64) (*Proxy, error) {
	r.getCalls.Add(1)
	if r.getByIDErr != nil {
		return nil, r.getByIDErr
REDACTED
	if px, ok := r.proxies[id]; ok {
		return px, nil
REDACTED
	return nil, errors.New("proxy not found")
REDACTED

func (r *contentModerationTestProxyRepo) Create(ctx context.Context, proxy *Proxy) error {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) ListByIDs(ctx context.Context, ids []int64) ([]Proxy, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) Update(ctx context.Context, proxy *Proxy) error {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) Delete(ctx context.Context, id int64) error {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) List(ctx context.Context, params pagination.PaginationParams) ([]Proxy, *pagination.PaginationResult, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]Proxy, *pagination.PaginationResult, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) ListWithFiltersAndAccountCount(ctx context.Context, params pagination.PaginationParams, protocol, status, search string) ([]ProxyWithAccountCount, *pagination.PaginationResult, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) ListActive(ctx context.Context) ([]Proxy, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) ListActiveWithAccountCount(ctx context.Context) ([]ProxyWithAccountCount, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) ExistsByHostPortAuth(ctx context.Context, host string, port int, username, password string) (bool, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) CountAccountsByProxyID(ctx context.Context, proxyID int64) (int64, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) ListAccountSummariesByProxyID(ctx context.Context, proxyID int64) ([]ProxyAccountSummary, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) SweepExpiredProxies(ctx context.Context, now time.Time) (int64, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) ListAllForFallback(ctx context.Context) ([]Proxy, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) CountExpired(ctx context.Context) (int64, error) {
	panic("not implemented")
REDACTED

func (r *contentModerationTestProxyRepo) CountExpiringSoon(ctx context.Context, now time.Time) (int64, error) {
	panic("not implemented")
REDACTED

func moderationProxyIDPtr(v int64) *int64 { return &v REDACTED

// 审计请求必须真正经过配置的代理发出（#2646 核心行为）。
// 通过一个本地 HTTP 正向代理验证：BaseURL 指向不可直连的假域名，
// 请求只有走代理才能得到响应。
func TestContentModerationCallRoutesThroughProxy(t *testing.T) {
	var proxied atomic.Int64
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HTTP 目标经正向代理时，代理收到的是绝对 URI 请求。
		if !strings.HasPrefix(r.RequestURI, "http://moderation-proxy-test.invalid") {
			t.Errorf("expected absolute-URI proxy request, got %q", r.RequestURI)
	REDACTED
		proxied.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{Flagged: falseREDACTEDREDACTEDREDACTED)
REDACTED))
	defer proxySrv.Close()

	proxyAddr := strings.TrimPrefix(proxySrv.URL, "http://")
	host, portStr, ok := strings.Cut(proxyAddr, ":")
	if !ok {
		t.Fatalf("unexpected proxy addr: %s", proxyAddr)
REDACTED
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse proxy port: %v", err)
REDACTED

	proxyRepo := &contentModerationTestProxyRepo{proxies: map[int64]*Proxy{
		7: {ID: 7, Name: "audit-proxy", Protocol: "http", Host: host, Port: port, Status: StatusActiveREDACTED,
REDACTEDREDACTED
	svc := NewContentModerationService(nil, nil, nil, nil, nil, proxyRepo, nil, nil)

	cfg := defaultContentModerationConfig()
	cfg.BaseURL = "http://moderation-proxy-test.invalid"
	cfg.ProxyID = moderationProxyIDPtr(7)
	cfg.normalize()

	httpStatus := 0
	if _, err := svc.callModerationOnceWithInput(context.Background(), cfg, "sk-test", "hello", &httpStatus); err != nil {
		t.Fatalf("expected moderation call via proxy to succeed, got: %v", err)
REDACTED
	if proxied.Load() == 0 {
		t.Fatal("expected request to be routed through the proxy server")
REDACTED
REDACTED

// 代理解析失败必须报错，而不是静默回退直连。
func TestContentModerationProxyResolveFailureDoesNotFallBackToDirect(t *testing.T) {
	var direct atomic.Int64
	directSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{Flagged: falseREDACTEDREDACTEDREDACTED)
REDACTED))
	defer directSrv.Close()

	proxyRepo := &contentModerationTestProxyRepo{getByIDErr: errors.New("proxy deleted")REDACTED
	svc := NewContentModerationService(nil, nil, nil, nil, nil, proxyRepo, nil, nil)

	cfg := defaultContentModerationConfig()
	cfg.BaseURL = directSrv.URL
	cfg.ProxyID = moderationProxyIDPtr(9)
	cfg.normalize()

	httpStatus := 0
	_, err := svc.callModerationOnceWithInput(context.Background(), cfg, "sk-test", "hello", &httpStatus)
	if err == nil || !strings.Contains(err.Error(), "resolve moderation proxy") {
		t.Fatalf("expected proxy resolve error, got: %v", err)
REDACTED
	if direct.Load() != 0 {
		t.Fatal("must not fall back to direct connection when proxy resolution fails")
REDACTED
REDACTED

// 代理 URL 解析结果按 TTL 缓存，热路径不应每次调用都查库。
func TestContentModerationProxyURLResolutionCached(t *testing.T) {
	proxyRepo := &contentModerationTestProxyRepo{proxies: map[int64]*Proxy{
		3: {ID: 3, Name: "p", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: StatusActiveREDACTED,
REDACTEDREDACTED
	svc := NewContentModerationService(nil, nil, nil, nil, nil, proxyRepo, nil, nil)

	for i := 0; i < 5; i++ {
		if _, err := svc.resolveModerationProxyURL(context.Background(), 3); err != nil {
			t.Fatalf("resolve attempt %d failed: %v", i, err)
	REDACTED
REDACTED
	if got := proxyRepo.getCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 repository lookup thanks to caching, got %d", got)
REDACTED
REDACTED

// UpdateConfig 的 proxy_id 语义：>0 设置、nil 保持、<=0 清除；配置视图回显。
func TestContentModerationUpdateConfigProxyIDSemantics(t *testing.T) {
	settingRepo := &contentModerationTestSettingRepo{values: map[string]string{REDACTEDREDACTED
	proxyRepo := &contentModerationTestProxyRepo{proxies: map[int64]*Proxy{
		5: {ID: 5, Name: "p", Protocol: "http", Host: "127.0.0.1", Port: 8080, Status: StatusActiveREDACTED,
REDACTEDREDACTED
	svc := NewContentModerationService(settingRepo, nil, nil, nil, nil, proxyRepo, nil, nil)
	ctx := context.Background()

	view, err := svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{ProxyID: moderationProxyIDPtr(5)REDACTED)
	if err != nil {
		t.Fatalf("set proxy_id=5: %v", err)
REDACTED
	if view.ProxyID == nil || *view.ProxyID != 5 {
		t.Fatalf("expected proxy_id=5 in view, got %v", view.ProxyID)
REDACTED

	// nil 表示不修改，代理保持不变。
	enabled := true
	view, err = svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{Enabled: &enabledREDACTED)
	if err != nil {
		t.Fatalf("update unrelated field: %v", err)
REDACTED
	if view.ProxyID == nil || *view.ProxyID != 5 {
		t.Fatalf("expected proxy_id to stay 5 when omitted, got %v", view.ProxyID)
REDACTED

	// 0 表示清除，恢复直连。
	view, err = svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{ProxyID: moderationProxyIDPtr(0)REDACTED)
	if err != nil {
		t.Fatalf("clear proxy_id: %v", err)
REDACTED
	if view.ProxyID != nil {
		t.Fatalf("expected proxy_id cleared, got %v", *view.ProxyID)
REDACTED

	// 不存在的代理必须被校验拒绝。
	if _, err := svc.UpdateConfig(ctx, UpdateContentModerationConfigInput{ProxyID: moderationProxyIDPtr(404)REDACTED); err == nil {
		t.Fatal("expected validation error for nonexistent proxy")
REDACTED
REDACTED

// TestAPIKeys 的 proxy_id 语义：nil 沿用已保存配置的代理；0 强制直连；>0 指定代理。
func TestContentModerationTestAPIKeysProxySemantics(t *testing.T) {
	var proxied atomic.Int64
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{Flagged: falseREDACTEDREDACTEDREDACTED)
REDACTED))
	defer proxySrv.Close()
	proxyAddr := strings.TrimPrefix(proxySrv.URL, "http://")
	host, portStr, _ := strings.Cut(proxyAddr, ":")
	port, _ := strconv.Atoi(portStr)

	savedCfg := defaultContentModerationConfig()
	savedCfg.BaseURL = "http://moderation-proxy-test.invalid"
	savedCfg.ProxyID = moderationProxyIDPtr(7)
	savedCfg.APIKeys = []string{"sk-saved"REDACTED
	rawCfg, err := json.Marshal(savedCfg)
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
REDACTED

	settingRepo := &contentModerationTestSettingRepo{values: map[string]string{
		SettingKeyContentModerationConfig: string(rawCfg),
REDACTEDREDACTED
	proxyRepo := &contentModerationTestProxyRepo{proxies: map[int64]*Proxy{
		7: {ID: 7, Name: "audit-proxy", Protocol: "http", Host: host, Port: port, Status: StatusActiveREDACTED,
REDACTEDREDACTED
	svc := NewContentModerationService(settingRepo, nil, nil, nil, nil, proxyRepo, nil, nil)

	// nil：沿用已保存配置的代理，测试请求应经过代理成功。
	result, err := svc.TestAPIKeys(context.Background(), TestContentModerationAPIKeysInput{APIKeys: []string{"sk-input"REDACTEDREDACTED)
	if err != nil {
		t.Fatalf("test with saved proxy: %v", err)
REDACTED
	if len(result.Items) != 1 || result.Items[0].Status == "error" {
		t.Fatalf("expected key test via proxy to succeed, got %+v", result.Items)
REDACTED
	if proxied.Load() == 0 {
		t.Fatal("expected test request to route through the saved proxy")
REDACTED

	// 0：强制直连；BaseURL 指向本地可直连服务器，应成功且不再经过代理。
	var direct atomic.Int64
	directSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{Flagged: falseREDACTEDREDACTEDREDACTED)
REDACTED))
	defer directSrv.Close()

	before := proxied.Load()
	result, err = svc.TestAPIKeys(context.Background(), TestContentModerationAPIKeysInput{
		APIKeys: []string{"sk-input"REDACTED,
		BaseURL: directSrv.URL,
		ProxyID: moderationProxyIDPtr(0),
REDACTED)
	if err != nil {
		t.Fatalf("test with forced direct: %v", err)
REDACTED
	if len(result.Items) != 1 || result.Items[0].Status == "error" {
		t.Fatalf("expected forced-direct test to succeed, got %+v", result.Items)
REDACTED
	if direct.Load() == 0 {
		t.Fatal("expected forced-direct test to reach the base URL directly")
REDACTED
	if proxied.Load() != before {
		t.Fatal("forced-direct test must not route through the proxy")
REDACTED
REDACTED
