package admin

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type OpsWSProxyConfig struct {
	TrustProxy     bool
	TrustedProxies []netip.Prefix
	OriginPolicy   string
REDACTED

const (
	envOpsWSTrustProxy     = "OPS_WS_TRUST_PROXY"
	envOpsWSTrustedProxies = "OPS_WS_TRUSTED_PROXIES"
	envOpsWSOriginPolicy   = "OPS_WS_ORIGIN_POLICY"
)

const (
	OriginPolicyStrict     = "strict"
	OriginPolicyPermissive = "permissive"
)

var opsWSProxyConfig = loadOpsWSProxyConfigFromEnv()

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedOpsWSOrigin(r)
REDACTED,
REDACTED

// QPSWSHandler handles realtime QPS push via WebSocket.
// GET /api/v1/admin/ops/ws/qps
func (h *OpsHandler) QPSWSHandler(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[OpsWS] upgrade failed: %v", err)
		return
REDACTED
	defer func() { _ = conn.Close() REDACTED()

	// Set pong handler
	if err := conn.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
		log.Printf("[OpsWS] set read deadline failed: %v", err)
		return
REDACTED
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
REDACTED)

	// Push QPS data every 2 seconds
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Heartbeat ping every 30 seconds
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	for {
		select {
		case <-ticker.C:
			// Fetch 1m window stats for current QPS
			data, err := h.opsService.GetDashboardOverview(ctx, "5m")
			if err != nil {
				log.Printf("[OpsWS] get overview failed: %v", err)
				continue
		REDACTED

			payload := gin.H{
				"type":      "qps_update",
				"timestamp": time.Now().Format(time.RFC3339),
				"data": gin.H{
					"qps":           data.QPS.Current,
					"tps":           data.TPS.Current,
					"request_count": data.Errors.TotalCount + int64(data.QPS.Avg1h*60), // Rough estimate
			REDACTED,
		REDACTED

			msg, _ := json.Marshal(payload)
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.Printf("[OpsWS] write failed: %v", err)
				return
		REDACTED
		case <-pingTicker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("[OpsWS] ping failed: %v", err)
				return
		REDACTED
		case <-ctx.Done():
			return
	REDACTED
REDACTED
REDACTED

func isAllowedOpsWSOrigin(r *http.Request) bool {
	if r == nil {
		return false
REDACTED
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		switch strings.ToLower(strings.TrimSpace(opsWSProxyConfig.OriginPolicy)) {
		case OriginPolicyStrict:
			return false
		case OriginPolicyPermissive, "":
			return true
		default:
			return true
	REDACTED
REDACTED
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Hostname() == "" {
		return false
REDACTED
	originHost := strings.ToLower(parsed.Hostname())

	trustProxyHeaders := shouldTrustOpsWSProxyHeaders(r)
	reqHost := hostWithoutPort(r.Host)
	if trustProxyHeaders {
		xfHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
		if xfHost != "" {
			xfHost = strings.TrimSpace(strings.Split(xfHost, ",")[0])
			if xfHost != "" {
				reqHost = hostWithoutPort(xfHost)
		REDACTED
	REDACTED
REDACTED
	reqHost = strings.ToLower(reqHost)
	if reqHost == "" {
		return false
REDACTED
	return originHost == reqHost
REDACTED

func shouldTrustOpsWSProxyHeaders(r *http.Request) bool {
	if r == nil {
		return false
REDACTED
	if !opsWSProxyConfig.TrustProxy {
		return false
REDACTED
	peerIP, ok := requestPeerIP(r)
	if !ok {
		return false
REDACTED
	return isAddrInTrustedProxies(peerIP, opsWSProxyConfig.TrustedProxies)
REDACTED

func requestPeerIP(r *http.Request) (netip.Addr, bool) {
	if r == nil {
		return netip.Addr{REDACTED, false
REDACTED
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
REDACTED
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if host == "" {
		return netip.Addr{REDACTED, false
REDACTED
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{REDACTED, false
REDACTED
	return addr.Unmap(), true
REDACTED

func isAddrInTrustedProxies(addr netip.Addr, trusted []netip.Prefix) bool {
	if !addr.IsValid() {
		return false
REDACTED
	for _, p := range trusted {
		if p.Contains(addr) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func loadOpsWSProxyConfigFromEnv() OpsWSProxyConfig {
	cfg := OpsWSProxyConfig{
		TrustProxy:     true,
		TrustedProxies: defaultTrustedProxies(),
		OriginPolicy:   OriginPolicyPermissive,
REDACTED

	if v := strings.TrimSpace(os.Getenv(envOpsWSTrustProxy)); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.TrustProxy = parsed
	REDACTED else {
			log.Printf("[OpsWS] invalid %s=%q (expected bool); using default=%v", envOpsWSTrustProxy, v, cfg.TrustProxy)
	REDACTED
REDACTED

	if raw := strings.TrimSpace(os.Getenv(envOpsWSTrustedProxies)); raw != "" {
		prefixes, invalid := parseTrustedProxyList(raw)
		if len(invalid) > 0 {
			log.Printf("[OpsWS] invalid %s entries ignored: %s", envOpsWSTrustedProxies, strings.Join(invalid, ", "))
	REDACTED
		cfg.TrustedProxies = prefixes
REDACTED

	if v := strings.TrimSpace(os.Getenv(envOpsWSOriginPolicy)); v != "" {
		normalized := strings.ToLower(v)
		switch normalized {
		case OriginPolicyStrict, OriginPolicyPermissive:
			cfg.OriginPolicy = normalized
		default:
			log.Printf("[OpsWS] invalid %s=%q (expected %q or %q); using default=%q", envOpsWSOriginPolicy, v, OriginPolicyStrict, OriginPolicyPermissive, cfg.OriginPolicy)
	REDACTED
REDACTED

	return cfg
REDACTED

func defaultTrustedProxies() []netip.Prefix {
	prefixes, _ := parseTrustedProxyList("127.0.0.0/8,::1/128")
	return prefixes
REDACTED

func parseTrustedProxyList(raw string) (prefixes []netip.Prefix, invalid []string) {
	for _, token := range strings.Split(raw, ",") {
		item := strings.TrimSpace(token)
		if item == "" {
			continue
	REDACTED

		var (
			p   netip.Prefix
			err error
		)
		if strings.Contains(item, "/") {
			p, err = netip.ParsePrefix(item)
	REDACTED else {
			var addr netip.Addr
			addr, err = netip.ParseAddr(item)
			if err == nil {
				addr = addr.Unmap()
				bits := 128
				if addr.Is4() {
					bits = 32
			REDACTED
				p = netip.PrefixFrom(addr, bits)
		REDACTED
	REDACTED

		if err != nil || !p.IsValid() {
			invalid = append(invalid, item)
			continue
	REDACTED

		prefixes = append(prefixes, p.Masked())
REDACTED
	return prefixes, invalid
REDACTED

func hostWithoutPort(hostport string) string {
	hostport = strings.TrimSpace(hostport)
	if hostport == "" {
		return ""
REDACTED
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
REDACTED
	if strings.HasPrefix(hostport, "[") && strings.HasSuffix(hostport, "]") {
		return strings.Trim(hostport, "[]")
REDACTED
	parts := strings.Split(hostport, ":")
	return parts[0]
REDACTED
