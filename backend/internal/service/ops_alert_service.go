package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type OpsAlertService struct {
	opsService   *OpsService
	userService  *UserService
	emailService *EmailService
	httpClient   *http.Client

	interval time.Duration

	startOnce sync.Once
	stopOnce  sync.Once
	stopCtx   context.Context
	stop      context.CancelFunc
	wg        sync.WaitGroup
REDACTED

// opsAlertEvalInterval defines how often OpsAlertService evaluates alert rules.
//
// Production uses opsMetricsInterval. Tests may override this variable to keep
// integration tests fast without changing production defaults.
var opsAlertEvalInterval = opsMetricsInterval

func NewOpsAlertService(opsService *OpsService, userService *UserService, emailService *EmailService) *OpsAlertService {
	return &OpsAlertService{
		opsService:   opsService,
		userService:  userService,
		emailService: emailService,
		httpClient:   &http.Client{Timeout: 10 * time.SecondREDACTED,
		interval:     opsAlertEvalInterval,
REDACTED
REDACTED

// Start launches the background alert evaluation loop.
//
// Stop must be called during shutdown to ensure the goroutine exits.
func (s *OpsAlertService) Start() {
	s.StartWithContext(context.Background())
REDACTED

// StartWithContext is like Start but allows the caller to provide a parent context.
// When the parent context is canceled, the service stops automatically.
func (s *OpsAlertService) StartWithContext(ctx context.Context) {
	if s == nil {
		return
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	s.startOnce.Do(func() {
		if s.interval <= 0 {
			s.interval = opsAlertEvalInterval
	REDACTED

		s.stopCtx, s.stop = context.WithCancel(ctx)
		s.wg.Add(1)
		go s.run()
REDACTED)
REDACTED

// Stop gracefully stops the background goroutine started by Start/StartWithContext.
// It is safe to call Stop multiple times.
func (s *OpsAlertService) Stop() {
	if s == nil {
		return
REDACTED

	s.stopOnce.Do(func() {
		if s.stop != nil {
			s.stop()
	REDACTED
REDACTED)
	s.wg.Wait()
REDACTED

func (s *OpsAlertService) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	s.evaluateOnce()
	for {
		select {
		case <-ticker.C:
			s.evaluateOnce()
		case <-s.stopCtx.Done():
			return
	REDACTED
REDACTED
REDACTED

func (s *OpsAlertService) evaluateOnce() {
	ctx, cancel := context.WithTimeout(s.stopCtx, opsAlertEvaluateTimeout)
	defer cancel()

	s.Evaluate(ctx, time.Now())
REDACTED

func (s *OpsAlertService) Evaluate(ctx context.Context, now time.Time) {
	if s == nil || s.opsService == nil {
		return
REDACTED

	rules, err := s.opsService.ListAlertRules(ctx)
	if err != nil {
		log.Printf("[OpsAlert] failed to list rules: %v", err)
		return
REDACTED
	if len(rules) == 0 {
		return
REDACTED

	maxSustainedByWindow := make(map[int]int)
	for _, rule := range rules {
		if !rule.Enabled {
			continue
	REDACTED
		window := rule.WindowMinutes
		if window <= 0 {
			window = 1
	REDACTED
		sustained := rule.SustainedMinutes
		if sustained <= 0 {
			sustained = 1
	REDACTED
		if sustained > maxSustainedByWindow[window] {
			maxSustainedByWindow[window] = sustained
	REDACTED
REDACTED

	metricsByWindow := make(map[int][]OpsMetrics)
	for window, limit := range maxSustainedByWindow {
		metrics, err := s.opsService.ListRecentSystemMetrics(ctx, window, limit)
		if err != nil {
			log.Printf("[OpsAlert] failed to load metrics window=%dm: %v", window, err)
			continue
	REDACTED
		metricsByWindow[window] = metrics
REDACTED

	for _, rule := range rules {
		if !rule.Enabled {
			continue
	REDACTED
		window := rule.WindowMinutes
		if window <= 0 {
			window = 1
	REDACTED
		sustained := rule.SustainedMinutes
		if sustained <= 0 {
			sustained = 1
	REDACTED

		metrics := metricsByWindow[window]
		selected, ok := selectContiguousMetrics(metrics, sustained, now)
		if !ok {
			continue
	REDACTED

		breached, latestValue, ok := evaluateRule(rule, selected)
		if !ok {
			continue
	REDACTED

		activeEvent, err := s.opsService.GetActiveAlertEvent(ctx, rule.ID)
		if err != nil {
			log.Printf("[OpsAlert] failed to get active event (rule=%d): %v", rule.ID, err)
			continue
	REDACTED

		if breached {
			if activeEvent != nil {
				continue
		REDACTED

			lastEvent, err := s.opsService.GetLatestAlertEvent(ctx, rule.ID)
			if err != nil {
				log.Printf("[OpsAlert] failed to get latest event (rule=%d): %v", rule.ID, err)
				continue
		REDACTED
			if lastEvent != nil && rule.CooldownMinutes > 0 {
				cooldown := time.Duration(rule.CooldownMinutes) * time.Minute
				if now.Sub(lastEvent.FiredAt) < cooldown {
					continue
			REDACTED
		REDACTED

			event := &OpsAlertEvent{
				RuleID:         rule.ID,
				Severity:       rule.Severity,
				Status:         OpsAlertStatusFiring,
				Title:          fmt.Sprintf("%s: %s", rule.Severity, rule.Name),
				Description:    buildAlertDescription(rule, latestValue),
				MetricValue:    latestValue,
				ThresholdValue: rule.Threshold,
				FiredAt:        now,
				CreatedAt:      now,
		REDACTED

			if err := s.opsService.CreateAlertEvent(ctx, event); err != nil {
				log.Printf("[OpsAlert] failed to create event (rule=%d): %v", rule.ID, err)
				continue
		REDACTED

			emailSent, webhookSent := s.dispatchNotifications(ctx, rule, event)
			if emailSent || webhookSent {
				if err := s.opsService.UpdateAlertEventNotifications(ctx, event.ID, emailSent, webhookSent); err != nil {
					log.Printf("[OpsAlert] failed to update notification flags (event=%d): %v", event.ID, err)
			REDACTED
		REDACTED
	REDACTED else if activeEvent != nil {
			resolvedAt := now
			if err := s.opsService.UpdateAlertEventStatus(ctx, activeEvent.ID, OpsAlertStatusResolved, &resolvedAt); err != nil {
				log.Printf("[OpsAlert] failed to resolve event (event=%d): %v", activeEvent.ID, err)
		REDACTED
	REDACTED
REDACTED
REDACTED

const opsMetricsContinuityTolerance = 20 * time.Second

// selectContiguousMetrics picks the newest N metrics and verifies they are continuous.
//
// This prevents a sustained rule from triggering when metrics sampling has gaps
// (e.g. collector downtime) and avoids evaluating "stale" data.
//
// Assumptions:
// - Metrics are ordered by UpdatedAt DESC (newest first).
// - Metrics are expected to be collected at opsMetricsInterval cadence.
func selectContiguousMetrics(metrics []OpsMetrics, needed int, now time.Time) ([]OpsMetrics, bool) {
	if needed <= 0 {
		return nil, false
REDACTED
	if len(metrics) < needed {
		return nil, false
REDACTED
	newest := metrics[0].UpdatedAt
	if newest.IsZero() {
		return nil, false
REDACTED
	if now.Sub(newest) > opsMetricsInterval+opsMetricsContinuityTolerance {
		return nil, false
REDACTED

	selected := metrics[:needed]
	for i := 0; i < len(selected)-1; i++ {
		a := selected[i].UpdatedAt
		b := selected[i+1].UpdatedAt
		if a.IsZero() || b.IsZero() {
			return nil, false
	REDACTED
		gap := a.Sub(b)
		if gap < opsMetricsInterval-opsMetricsContinuityTolerance || gap > opsMetricsInterval+opsMetricsContinuityTolerance {
			return nil, false
	REDACTED
REDACTED
	return selected, true
REDACTED

func evaluateRule(rule OpsAlertRule, metrics []OpsMetrics) (bool, float64, bool) {
	if len(metrics) == 0 {
		return false, 0, false
REDACTED

	latestValue, ok := metricValue(metrics[0], rule.MetricType)
	if !ok {
		return false, 0, false
REDACTED

	for _, metric := range metrics {
		value, ok := metricValue(metric, rule.MetricType)
		if !ok || !compareMetric(value, rule.Operator, rule.Threshold) {
			return false, latestValue, true
	REDACTED
REDACTED

	return true, latestValue, true
REDACTED

func metricValue(metric OpsMetrics, metricType string) (float64, bool) {
	switch metricType {
	case OpsMetricSuccessRate:
		if metric.RequestCount == 0 {
			return 0, false
	REDACTED
		return metric.SuccessRate, true
	case OpsMetricErrorRate:
		if metric.RequestCount == 0 {
			return 0, false
	REDACTED
		return metric.ErrorRate, true
	case OpsMetricP95LatencyMs:
		return float64(metric.P95LatencyMs), true
	case OpsMetricP99LatencyMs:
		return float64(metric.P99LatencyMs), true
	case OpsMetricHTTP2Errors:
		return float64(metric.HTTP2Errors), true
	case OpsMetricCPUUsagePercent:
		return metric.CPUUsagePercent, true
	case OpsMetricMemoryUsagePercent:
		return metric.MemoryUsagePercent, true
	case OpsMetricQueueDepth:
		return float64(metric.ConcurrencyQueueDepth), true
	default:
		return 0, false
REDACTED
REDACTED

func compareMetric(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return value == threshold
	default:
		return false
REDACTED
REDACTED

func buildAlertDescription(rule OpsAlertRule, value float64) string {
	window := rule.WindowMinutes
	if window <= 0 {
		window = 1
REDACTED
	return fmt.Sprintf("Rule %s triggered: %s %s %.2f (current %.2f) over last %dm",
		rule.Name,
		rule.MetricType,
		rule.Operator,
		rule.Threshold,
		value,
		window,
	)
REDACTED

func (s *OpsAlertService) dispatchNotifications(ctx context.Context, rule OpsAlertRule, event *OpsAlertEvent) (bool, bool) {
	emailSent := false
	webhookSent := false

	notifyCtx, cancel := s.notificationContext(ctx)
	defer cancel()

	if rule.NotifyEmail {
		emailSent = s.sendEmailNotification(notifyCtx, rule, event)
REDACTED
	if rule.NotifyWebhook && rule.WebhookURL != "" {
		webhookSent = s.sendWebhookNotification(notifyCtx, rule, event)
REDACTED
	// Fallback channel: if email is enabled but ultimately fails, try webhook even if the
	// webhook toggle is off (as long as a webhook URL is configured).
	if rule.NotifyEmail && !emailSent && !rule.NotifyWebhook && rule.WebhookURL != "" {
		log.Printf("[OpsAlert] email failed; attempting webhook fallback (rule=%d)", rule.ID)
		webhookSent = s.sendWebhookNotification(notifyCtx, rule, event)
REDACTED

	return emailSent, webhookSent
REDACTED

const (
	opsAlertEvaluateTimeout     = 45 * time.Second
	opsAlertNotificationTimeout = 30 * time.Second
	opsAlertEmailMaxRetries     = 3
)

var opsAlertEmailBackoff = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
REDACTED

func (s *OpsAlertService) notificationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	parent := ctx
	if s != nil && s.stopCtx != nil {
		parent = s.stopCtx
REDACTED
	if parent == nil {
		parent = context.Background()
REDACTED
	return context.WithTimeout(parent, opsAlertNotificationTimeout)
REDACTED

var opsAlertSleep = sleepWithContext

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
REDACTED
	if ctx == nil {
		time.Sleep(d)
		return nil
REDACTED
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
REDACTED
REDACTED

func retryWithBackoff(
	ctx context.Context,
	maxRetries int,
	backoff []time.Duration,
	fn func() error,
	onError func(attempt int, total int, nextDelay time.Duration, err error),
) error {
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if maxRetries < 0 {
		maxRetries = 0
REDACTED
	totalAttempts := maxRetries + 1

	var lastErr error
	for attempt := 1; attempt <= totalAttempts; attempt++ {
		if attempt > 1 {
			backoffIdx := attempt - 2
			if backoffIdx < len(backoff) {
				if err := opsAlertSleep(ctx, backoff[backoffIdx]); err != nil {
					return err
			REDACTED
		REDACTED
	REDACTED

		if err := ctx.Err(); err != nil {
			return err
	REDACTED

		if err := fn(); err != nil {
			lastErr = err
			nextDelay := time.Duration(0)
			if attempt < totalAttempts {
				nextIdx := attempt - 1
				if nextIdx < len(backoff) {
					nextDelay = backoff[nextIdx]
			REDACTED
		REDACTED
			if onError != nil {
				onError(attempt, totalAttempts, nextDelay, err)
		REDACTED
			continue
	REDACTED
		return nil
REDACTED

	return lastErr
REDACTED

func (s *OpsAlertService) sendEmailNotification(ctx context.Context, rule OpsAlertRule, event *OpsAlertEvent) bool {
	if s.emailService == nil || s.userService == nil {
		return false
REDACTED

	if ctx == nil {
		ctx = context.Background()
REDACTED

	admin, err := s.userService.GetFirstAdmin(ctx)
	if err != nil || admin == nil || admin.Email == "" {
		return false
REDACTED

	subject := fmt.Sprintf("[Ops Alert][%s] %s", rule.Severity, rule.Name)
	body := fmt.Sprintf(
		"Alert triggered: %s\n\nMetric: %s\nThreshold: %.2f\nCurrent: %.2f\nWindow: %dm\nStatus: %s\nTime: %s",
		rule.Name,
		rule.MetricType,
		rule.Threshold,
		event.MetricValue,
		rule.WindowMinutes,
		event.Status,
		event.FiredAt.Format(time.RFC3339),
	)

	config, err := s.emailService.GetSMTPConfig(ctx)
	if err != nil {
		log.Printf("[OpsAlert] email config load failed: %v", err)
		return false
REDACTED

	if err := retryWithBackoff(
		ctx,
		opsAlertEmailMaxRetries,
		opsAlertEmailBackoff,
		func() error {
			return s.emailService.SendEmailWithConfig(config, admin.Email, subject, body)
	REDACTED,
		func(attempt int, total int, nextDelay time.Duration, err error) {
			if attempt < total {
				log.Printf("[OpsAlert] email send failed (attempt=%d/%d), retrying in %s: %v", attempt, total, nextDelay, err)
				return
		REDACTED
			log.Printf("[OpsAlert] email send failed (attempt=%d/%d), giving up: %v", attempt, total, err)
	REDACTED,
	); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[OpsAlert] email send canceled: %v", err)
	REDACTED
		return false
REDACTED
	return true
REDACTED

func (s *OpsAlertService) sendWebhookNotification(ctx context.Context, rule OpsAlertRule, event *OpsAlertEvent) bool {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	webhookTarget, err := validateWebhookURL(ctx, rule.WebhookURL)
	if err != nil {
		log.Printf("[OpsAlert] invalid webhook url (rule=%d): %v", rule.ID, err)
		return false
REDACTED

	payload := map[string]any{
		"rule_id":         rule.ID,
		"rule_name":       rule.Name,
		"severity":        rule.Severity,
		"status":          event.Status,
		"metric_type":     rule.MetricType,
		"metric_value":    event.MetricValue,
		"threshold_value": rule.Threshold,
		"window_minutes":  rule.WindowMinutes,
		"fired_at":        event.FiredAt.Format(time.RFC3339),
REDACTED

	body, err := json.Marshal(payload)
	if err != nil {
		return false
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookTarget.URL.String(), bytes.NewReader(body))
	if err != nil {
		return false
REDACTED
	req.Header.Set("Content-Type", "application/json")

	resp, err := buildWebhookHTTPClient(s.httpClient, webhookTarget).Do(req)
	if err != nil {
		log.Printf("[OpsAlert] webhook send failed: %v", err)
		return false
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		log.Printf("[OpsAlert] webhook returned status %d", resp.StatusCode)
		return false
REDACTED
	return true
REDACTED

const webhookHTTPClientTimeout = 10 * time.Second

func buildWebhookHTTPClient(base *http.Client, webhookTarget *validatedWebhookTarget) *http.Client {
	var client http.Client
	if base != nil {
		client = *base
REDACTED
	if client.Timeout <= 0 {
		client.Timeout = webhookHTTPClientTimeout
REDACTED
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
REDACTED
	if webhookTarget != nil {
		client.Transport = buildWebhookTransport(client.Transport, webhookTarget)
REDACTED
	return &client
REDACTED

var disallowedWebhookIPNets = []net.IPNet{
	// "this host on this network" / unspecified.
	mustParseCIDR("0.0.0.0/8"),
	mustParseCIDR("127.0.0.0/8"),    // loopback (includes 127.0.0.1)
	mustParseCIDR("10.0.0.0/8"),     // RFC1918
	mustParseCIDR("192.168.0.0/16"), // RFC1918
	mustParseCIDR("172.16.0.0/12"),  // RFC1918 (172.16.0.0 - 172.31.255.255)
	mustParseCIDR("100.64.0.0/10"),  // RFC6598 (carrier-grade NAT)
	mustParseCIDR("169.254.0.0/16"), // IPv4 link-local (includes 169.254.169.254 metadata IP on many clouds)
	mustParseCIDR("198.18.0.0/15"),  // RFC2544 benchmark testing
	mustParseCIDR("224.0.0.0/4"),    // IPv4 multicast
	mustParseCIDR("240.0.0.0/4"),    // IPv4 reserved
	mustParseCIDR("::/128"),         // IPv6 unspecified
	mustParseCIDR("::1/128"),        // IPv6 loopback
	mustParseCIDR("fc00::/7"),       // IPv6 unique local
	mustParseCIDR("fe80::/10"),      // IPv6 link-local
	mustParseCIDR("ff00::/8"),       // IPv6 multicast
REDACTED

func mustParseCIDR(cidr string) net.IPNet {
	_, block, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
REDACTED
	return *block
REDACTED

var lookupIPAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
REDACTED

type validatedWebhookTarget struct {
	URL *url.URL

	host      string
	port      string
	pinnedIPs []net.IP
REDACTED

var webhookBaseDialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
REDACTED
	return dialer.DialContext(ctx, network, addr)
REDACTED

func buildWebhookTransport(base http.RoundTripper, webhookTarget *validatedWebhookTarget) http.RoundTripper {
	if webhookTarget == nil || webhookTarget.URL == nil {
		return base
REDACTED

	var transport *http.Transport
	switch typed := base.(type) {
	case *http.Transport:
		if typed != nil {
			transport = typed.Clone()
	REDACTED
REDACTED
	if transport == nil {
		if defaultTransport, ok := http.DefaultTransport.(*http.Transport); ok && defaultTransport != nil {
			transport = defaultTransport.Clone()
	REDACTED else {
			transport = (&http.Transport{REDACTED).Clone()
	REDACTED
REDACTED

	webhookHost := webhookTarget.host
	webhookPort := webhookTarget.port
	pinnedIPs := append([]net.IP(nil), webhookTarget.pinnedIPs...)

	transport.Proxy = nil
	transport.DialTLSContext = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil || host == "" || port == "" {
			return nil, fmt.Errorf("webhook dial target is invalid: %q", addr)
	REDACTED

		canonicalHost := strings.TrimSuffix(strings.ToLower(host), ".")
		if canonicalHost != webhookHost || port != webhookPort {
			return nil, fmt.Errorf("webhook dial target mismatch: %q", addr)
	REDACTED

		var lastErr error
		for _, ip := range pinnedIPs {
			if isDisallowedWebhookIP(ip) {
				lastErr = fmt.Errorf("webhook target resolves to a disallowed ip")
				continue
		REDACTED

			dialAddr := net.JoinHostPort(ip.String(), port)
			conn, err := webhookBaseDialContext(ctx, network, dialAddr)
			if err == nil {
				return conn, nil
		REDACTED
			lastErr = err
	REDACTED
		if lastErr == nil {
			lastErr = errors.New("webhook target has no resolved addresses")
	REDACTED
		return nil, lastErr
REDACTED

	return transport
REDACTED

func validateWebhookURL(ctx context.Context, raw string) (*validatedWebhookTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("webhook url is empty")
REDACTED
	// Avoid request smuggling / header injection vectors.
	if strings.ContainsAny(raw, "\r\n") {
		return nil, errors.New("webhook url contains invalid characters")
REDACTED

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("webhook url format is invalid")
REDACTED
	if !strings.EqualFold(parsed.Scheme, "https") {
		return nil, errors.New("webhook url scheme must be https")
REDACTED
	parsed.Scheme = "https"
	if parsed.Host == "" || parsed.Hostname() == "" {
		return nil, errors.New("webhook url must include host")
REDACTED
	if parsed.User != nil {
		return nil, errors.New("webhook url must not include userinfo")
REDACTED
	if parsed.Port() != "" {
		port, err := strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("webhook url port is invalid")
	REDACTED
REDACTED

	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" {
		return nil, errors.New("webhook url host must not be localhost")
REDACTED

	if ip := net.ParseIP(host); ip != nil {
		if isDisallowedWebhookIP(ip) {
			return nil, errors.New("webhook url host resolves to a disallowed ip")
	REDACTED
		return &validatedWebhookTarget{
			URL:       parsed,
			host:      host,
			port:      portForScheme(parsed),
			pinnedIPs: []net.IP{ipREDACTED,
	REDACTED, nil
REDACTED

	if ctx == nil {
		ctx = context.Background()
REDACTED
	ips, err := lookupIPAddrs(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("webhook url host cannot be resolved")
REDACTED
	pinned := make([]net.IP, 0, len(ips))
	for _, addr := range ips {
		if isDisallowedWebhookIP(addr.IP) {
			return nil, errors.New("webhook url host resolves to a disallowed ip")
	REDACTED
		if addr.IP != nil {
			pinned = append(pinned, addr.IP)
	REDACTED
REDACTED

	if len(pinned) == 0 {
		return nil, errors.New("webhook url host cannot be resolved")
REDACTED

	return &validatedWebhookTarget{
		URL:       parsed,
		host:      host,
		port:      portForScheme(parsed),
		pinnedIPs: uniqueResolvedIPs(pinned),
REDACTED, nil
REDACTED

func isDisallowedWebhookIP(ip net.IP) bool {
	if ip == nil {
		return false
REDACTED
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
REDACTED else if ip16 := ip.To16(); ip16 != nil {
		ip = ip16
REDACTED else {
		return false
REDACTED

	// Disallow non-public addresses even if they're not explicitly covered by the CIDR list.
	// This provides defense-in-depth against SSRF targets such as link-local, multicast, and
	// unspecified addresses, and ensures any "pinned" IP is still blocked at dial time.
	if ip.IsUnspecified() ||
		ip.IsLoopback() ||
		ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() {
		return true
REDACTED

	for _, block := range disallowedWebhookIPNets {
		if block.Contains(ip) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func portForScheme(u *url.URL) string {
	if u != nil && u.Port() != "" {
		return u.Port()
REDACTED
	return "443"
REDACTED

func uniqueResolvedIPs(ips []net.IP) []net.IP {
	seen := make(map[string]struct{REDACTED, len(ips))
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
	REDACTED
		key := ip.String()
		if _, ok := seen[key]; ok {
			continue
	REDACTED
		seen[key] = struct{REDACTED{REDACTED
		out = append(out, ip)
REDACTED
	return out
REDACTED
