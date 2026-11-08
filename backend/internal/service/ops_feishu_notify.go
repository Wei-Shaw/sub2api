package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 飞书群机器人告警：把「请求错误」及时发到运营群。
//
// 为什么不直接每条错误发一条消息：线上一次上游抖动就能刷出几十条一模一样的
// 503（见使用记录 → 错误请求）。所以这里做两件事：
//  1. 按「签名」把窗口内相同的错误合并成一条，消息里带累计次数；
//  2. 再叠一层每小时条数上限，防止异常放大时把群刷爆。
//
// 另外：Webhook 地址是凭据，只存在 settings 表里，不写进代码仓库。
const (
	opsFeishuWebhookTimeout       = 5 * time.Second
	opsFeishuConfigCacheTTL       = 30 * time.Second
	opsFeishuFlushInterval        = time.Second
	opsFeishuPendingMaxEntries    = 200
	opsFeishuPendingMaxAge        = 15 * time.Minute
	opsFeishuMessageMaxRunes      = 1500
	opsFeishuSignatureMsgMaxRunes = 120
	opsFeishuErrorDetailMaxRunes  = 300
)

var opsFeishuHTTPClient = &http.Client{Timeout: opsFeishuWebhookTimeout}

// =========================
// 配置读写
// =========================

func (s *OpsService) GetFeishuNotificationConfig(ctx context.Context) (*OpsFeishuNotificationConfig, error) {
	defaultCfg := defaultOpsFeishuNotificationConfig()
	if s == nil || s.settingRepo == nil {
		return defaultCfg, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsFeishuNotificationConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			if b, mErr := json.Marshal(defaultCfg); mErr == nil {
				_ = s.settingRepo.Set(ctx, SettingKeyOpsFeishuNotificationConfig, string(b))
			}
			return defaultCfg, nil
		}
		return nil, err
	}

	cfg := &OpsFeishuNotificationConfig{}
	if err := json.Unmarshal([]byte(raw), cfg); err != nil {
		// 配置坏了不能让运维页打不开，退回默认值（默认是关闭的）。
		return defaultCfg, nil
	}
	normalizeOpsFeishuNotificationConfig(cfg)
	return cfg, nil
}

func (s *OpsService) UpdateFeishuNotificationConfig(ctx context.Context, req *OpsFeishuNotificationConfigUpdateRequest) (*OpsFeishuNotificationConfig, error) {
	if s == nil || s.settingRepo == nil {
		return nil, errors.New("setting repository not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if req == nil {
		return nil, errors.New("invalid request")
	}

	cfg, err := s.GetFeishuNotificationConfig(ctx)
	if err != nil {
		return nil, err
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.WebhookURL != nil {
		cfg.WebhookURL = *req.WebhookURL
	}
	if req.MinSeverity != nil {
		cfg.MinSeverity = *req.MinSeverity
	}
	if req.RateLimitPerHour != nil {
		cfg.RateLimitPerHour = *req.RateLimitPerHour
	}
	if req.BatchingWindowSeconds != nil {
		cfg.BatchingWindowSeconds = *req.BatchingWindowSeconds
	}

	if err := validateOpsFeishuNotificationConfig(cfg); err != nil {
		return nil, err
	}
	normalizeOpsFeishuNotificationConfig(cfg)

	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := s.settingRepo.Set(ctx, SettingKeyOpsFeishuNotificationConfig, string(raw)); err != nil {
		return nil, err
	}
	// 立即生效，不用等 30 秒缓存过期。
	s.applyFeishuNotificationConfig(cfg)
	return cfg, nil
}

// SendFeishuTestNotification 用当前保存的 Webhook 发一条测试消息。
// 注意：即使开关是关的也允许测试，否则「先填地址、点测试、再打开」这个顺序走不通。
func (s *OpsService) SendFeishuTestNotification(ctx context.Context) error {
	cfg, err := s.GetFeishuNotificationConfig(ctx)
	if err != nil {
		return err
	}
	url := strings.TrimSpace(cfg.WebhookURL)
	if url == "" {
		return errors.New("feishu webhook url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return errors.New("feishu webhook url must start with http:// or https://")
	}
	text := fmt.Sprintf("【辰数告警】飞书通知测试\n时间：%s\n说明：能看到这条消息，说明 Webhook 已经配好；网站出现请求错误时会自动发到这里。",
		time.Now().Format("2006-01-02 15:04:05"))
	return sendFeishuWebhookText(ctx, url, text)
}

func defaultOpsFeishuNotificationConfig() *OpsFeishuNotificationConfig {
	return &OpsFeishuNotificationConfig{
		Enabled:    false,
		WebhookURL: "",
		// P1 = 5xx / 429，也就是「平台自己出问题」；4xx 多是客户自己传错，默认不打扰。
		MinSeverity:           "P1",
		RateLimitPerHour:      60,
		BatchingWindowSeconds: 60,
	}
}

func normalizeOpsFeishuNotificationConfig(cfg *OpsFeishuNotificationConfig) {
	if cfg == nil {
		return
	}
	cfg.WebhookURL = strings.TrimSpace(cfg.WebhookURL)
	cfg.MinSeverity = normalizeOpsFeishuSeverity(cfg.MinSeverity)
	if cfg.RateLimitPerHour < 0 {
		cfg.RateLimitPerHour = 0
	}
	if cfg.BatchingWindowSeconds < 0 {
		cfg.BatchingWindowSeconds = 0
	}
}

func normalizeOpsFeishuSeverity(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "P1":
		return "P1"
	case "P2":
		return "P2"
	case "P3":
		return "P3"
	default:
		return ""
	}
}

func validateOpsFeishuNotificationConfig(cfg *OpsFeishuNotificationConfig) error {
	if cfg == nil {
		return errors.New("invalid config")
	}
	url := strings.TrimSpace(cfg.WebhookURL)
	if cfg.Enabled && url == "" {
		return errors.New("feishu.webhook_url is required when enabled")
	}
	if url != "" && !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return errors.New("feishu.webhook_url must start with http:// or https://")
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.MinSeverity)) {
	case "", "P1", "P2", "P3":
	default:
		return errors.New("feishu.min_severity must be one of: P1, P2, P3, or empty")
	}
	if cfg.RateLimitPerHour < 0 || cfg.RateLimitPerHour > 100000 {
		return errors.New("feishu.rate_limit_per_hour must be between 0 and 100000")
	}
	if cfg.BatchingWindowSeconds < 0 || cfg.BatchingWindowSeconds > 86400 {
		return errors.New("feishu.batching_window_seconds must be between 0 and 86400")
	}
	return nil
}

// =========================
// 通知器
// =========================

// opsFeishuSeverityRank 数字越小越严重：P1（5xx/429）> P2（4xx）> P3（其余）。
func opsFeishuSeverityRank(severity string) int {
	switch strings.ToUpper(strings.TrimSpace(severity)) {
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	default:
		// 没有级别的按最低处理：只有「全部级别」时才发。
		return 4
	}
}

func opsFeishuShouldNotify(minSeverity, severity string) bool {
	min := strings.ToUpper(strings.TrimSpace(minSeverity))
	if min == "" {
		return true
	}
	return opsFeishuSeverityRank(severity) <= opsFeishuSeverityRank(min)
}

type opsFeishuPendingError struct {
	signature string
	sample    OpsInsertErrorLogInput
	count     int
	firstSeen time.Time
	dueAt     time.Time
}

type opsFeishuNotifier struct {
	loadConfig func(ctx context.Context) (*OpsFeishuNotificationConfig, error)

	mu       sync.Mutex
	cfg      *OpsFeishuNotificationConfig
	loadedAt time.Time
	pending  map[string]*opsFeishuPendingError

	limiter   *slidingWindowLimiter
	flushOnce sync.Once
}

func newOpsFeishuNotifier(loadConfig func(ctx context.Context) (*OpsFeishuNotificationConfig, error)) *opsFeishuNotifier {
	return &opsFeishuNotifier{
		loadConfig: loadConfig,
		pending:    make(map[string]*opsFeishuPendingError),
		limiter:    newSlidingWindowLimiter(0, time.Hour),
	}
}

func (s *OpsService) applyFeishuNotificationConfig(cfg *OpsFeishuNotificationConfig) {
	if s == nil || s.feishuNotifier == nil || cfg == nil {
		return
	}
	s.feishuNotifier.setConfig(cfg)
}

func (n *opsFeishuNotifier) setConfig(cfg *OpsFeishuNotificationConfig) {
	if n == nil || cfg == nil {
		return
	}
	copied := *cfg
	n.mu.Lock()
	n.cfg = &copied
	n.loadedAt = time.Now()
	n.mu.Unlock()
	n.limiter.SetLimit(copied.RateLimitPerHour)
}

func (n *opsFeishuNotifier) currentConfig(ctx context.Context, now time.Time) *OpsFeishuNotificationConfig {
	n.mu.Lock()
	cfg := n.cfg
	fresh := cfg != nil && now.Sub(n.loadedAt) <= opsFeishuConfigCacheTTL
	n.mu.Unlock()

	if !fresh && n.loadConfig != nil {
		if loaded, err := n.loadConfig(ctx); err == nil && loaded != nil {
			n.setConfig(loaded)
			n.mu.Lock()
			cfg = n.cfg
			n.mu.Unlock()
		}
	}
	if cfg == nil {
		return nil
	}
	copied := *cfg
	return &copied
}

// enqueue 把一批刚落库的错误交给通知器。这里只做内存操作，不做网络请求，
// 保证不影响错误日志写入本身。
func (n *opsFeishuNotifier) enqueue(ctx context.Context, entries []*OpsInsertErrorLogInput, now time.Time) {
	if n == nil || len(entries) == 0 {
		return
	}
	cfg := n.currentConfig(ctx, now)
	if cfg == nil || !cfg.Enabled {
		return
	}
	url := strings.TrimSpace(cfg.WebhookURL)
	if url == "" {
		return
	}
	window := time.Duration(cfg.BatchingWindowSeconds) * time.Second

	immediate := make([]string, 0, len(entries))
	n.mu.Lock()
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		if !opsFeishuShouldNotify(cfg.MinSeverity, entry.Severity) {
			continue
		}
		signature := opsFeishuErrorSignature(entry)
		if signature == "" {
			continue
		}
		firstSeen := entry.CreatedAt
		if firstSeen.IsZero() {
			firstSeen = now
		}
		if pending, ok := n.pending[signature]; ok {
			pending.count++
			continue
		}
		if window <= 0 {
			immediate = append(immediate, buildFeishuErrorText(entry, 1, firstSeen))
			continue
		}
		if len(n.pending) >= opsFeishuPendingMaxEntries {
			// 兜底：签名太多（例如被扫）时不再累积，避免内存被刷爆。
			continue
		}
		sample := *entry
		// 只留拼消息要用的字段：ErrorBody 最多 20KB，留在内存里等窗口没有意义。
		sample.ErrorBody = ""
		sample.UpstreamErrors = nil
		sample.UpstreamErrorsJSON = nil
		sample.UserAgent = ""
		n.pending[signature] = &opsFeishuPendingError{
			signature: signature,
			sample:    sample,
			count:     1,
			firstSeen: firstSeen,
			dueAt:     now.Add(window),
		}
	}
	n.mu.Unlock()

	for _, text := range immediate {
		go n.deliver(url, text, time.Now())
	}
	if window > 0 {
		n.ensureFlusher()
	}
}

func (n *opsFeishuNotifier) ensureFlusher() {
	n.flushOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(opsFeishuFlushInterval)
			defer ticker.Stop()
			for range ticker.C {
				n.flush(time.Now())
			}
		}()
	})
}

// flush 把到期（或等太久）的合并项发出去，返回实际发出去的文案。
// now 可注入，方便单测不依赖真实时间。
func (n *opsFeishuNotifier) flush(now time.Time) []string {
	n.mu.Lock()
	due := make([]*opsFeishuPendingError, 0, len(n.pending))
	for signature, pending := range n.pending {
		if !pending.dueAt.After(now) || now.Sub(pending.firstSeen) > opsFeishuPendingMaxAge {
			due = append(due, pending)
			delete(n.pending, signature)
		}
	}
	cfg := n.cfg
	n.mu.Unlock()

	if len(due) == 0 || cfg == nil || !cfg.Enabled {
		return nil
	}
	url := strings.TrimSpace(cfg.WebhookURL)
	if url == "" {
		return nil
	}

	sent := make([]string, 0, len(due))
	for _, pending := range due {
		text := buildFeishuErrorText(&pending.sample, pending.count, pending.firstSeen)
		if n.send(url, text, now) {
			sent = append(sent, text)
		}
	}
	return sent
}

func (n *opsFeishuNotifier) deliver(url, text string, now time.Time) bool {
	if n == nil {
		return false
	}
	return n.send(url, text, now)
}

func (n *opsFeishuNotifier) send(url, text string, now time.Time) bool {
	if !n.limiter.Allow(now) {
		log.Printf("[OpsFeishu] hourly rate limit reached, dropping alert")
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), opsFeishuWebhookTimeout)
	defer cancel()
	if err := sendFeishuWebhookText(ctx, url, text); err != nil {
		log.Printf("[OpsFeishu] send failed: %v", err)
		return false
	}
	return true
}

// notifyFeishuErrors 是错误日志落库之后的旁路动作：只把数据交给通知器，
// 真正的 HTTP 推送由通知器自己的 goroutine 完成。任何异常都不允许影响落库。
func (s *OpsService) notifyFeishuErrors(ctx context.Context, entries []*OpsInsertErrorLogInput) {
	if s == nil || s.feishuNotifier == nil || len(entries) == 0 {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[OpsFeishu] notify panic: %v", r)
		}
	}()
	s.feishuNotifier.enqueue(ctx, entries, time.Now())
}

// =========================
// 消息拼装 / 发送
// =========================

func opsFeishuErrorSignature(entry *OpsInsertErrorLogInput) string {
	if entry == nil {
		return ""
	}
	message := strings.TrimSpace(entry.ErrorMessage)
	if message == "" && entry.UpstreamErrorMessage != nil {
		message = strings.TrimSpace(*entry.UpstreamErrorMessage)
	}
	message = strings.ToLower(opsFeishuTruncate(opsFeishuOneLine(message), opsFeishuSignatureMsgMaxRunes))
	return fmt.Sprintf("%s|%s|%d|%s|%s|%s",
		strings.TrimSpace(entry.ErrorPhase),
		strings.TrimSpace(entry.ErrorType),
		entry.StatusCode,
		strings.TrimSpace(entry.Platform),
		strings.TrimSpace(entry.Model),
		message,
	)
}

// buildFeishuErrorText 拼一条给人看的告警：够定位（哪个模型、哪个端点、什么错误），
// 但不含客户的 Key / 密码 / 请求体。
func buildFeishuErrorText(entry *OpsInsertErrorLogInput, count int, firstSeen time.Time) string {
	if entry == nil {
		return ""
	}
	ts := entry.CreatedAt
	if ts.IsZero() {
		ts = firstSeen
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	lines := []string{"【辰数告警】请求错误", "时间：" + ts.Format("2006-01-02 15:04:05")}

	head := fmt.Sprintf("状态码：%d", entry.StatusCode)
	if severity := strings.TrimSpace(entry.Severity); severity != "" {
		head += "　级别：" + severity
	}
	lines = append(lines, head)

	if phase := strings.TrimSpace(entry.ErrorPhase); phase != "" {
		line := "阶段：" + phase
		if errType := strings.TrimSpace(entry.ErrorType); errType != "" {
			line += "　类型：" + errType
		}
		lines = append(lines, line)
	}

	platformModel := make([]string, 0, 2)
	if platform := strings.TrimSpace(entry.Platform); platform != "" {
		platformModel = append(platformModel, "平台："+platform)
	}
	if model := strings.TrimSpace(opsFeishuFirstNonEmpty(entry.Model, entry.RequestedModel)); model != "" {
		platformModel = append(platformModel, "模型："+model)
	}
	if len(platformModel) > 0 {
		lines = append(lines, strings.Join(platformModel, "　"))
	}

	if endpoint := opsFeishuEndpointLine(entry); endpoint != "" {
		lines = append(lines, endpoint)
	}

	if entry.UpstreamStatusCode != nil {
		line := fmt.Sprintf("上游状态码：%d", *entry.UpstreamStatusCode)
		if entry.UpstreamErrorMessage != nil {
			if msg := opsFeishuTruncate(opsFeishuOneLine(*entry.UpstreamErrorMessage), opsFeishuErrorDetailMaxRunes); msg != "" {
				line += "　上游信息：" + msg
			}
		}
		lines = append(lines, line)
	}

	if message := opsFeishuTruncate(opsFeishuOneLine(entry.ErrorMessage), opsFeishuErrorDetailMaxRunes); message != "" {
		lines = append(lines, "错误："+message)
	}

	ids := make([]string, 0, 3)
	if entry.UserID != nil {
		ids = append(ids, fmt.Sprintf("用户ID：%d", *entry.UserID))
	}
	if entry.AccountID != nil {
		ids = append(ids, fmt.Sprintf("账号ID：%d", *entry.AccountID))
	}
	if requestID := strings.TrimSpace(opsFeishuFirstNonEmpty(entry.RequestID, entry.ClientRequestID)); requestID != "" {
		ids = append(ids, "请求ID："+requestID)
	}
	if len(ids) > 0 {
		lines = append(lines, strings.Join(ids, "　"))
	}

	if count > 1 {
		lines = append(lines, fmt.Sprintf("次数：%d（窗口内相同错误已合并，首次 %s）", count, firstSeen.Format("15:04:05")))
	}
	return opsFeishuTruncate(strings.Join(lines, "\n"), opsFeishuMessageMaxRunes)
}

func opsFeishuEndpointLine(entry *OpsInsertErrorLogInput) string {
	inbound := strings.TrimSpace(opsFeishuFirstNonEmpty(entry.InboundEndpoint, entry.RequestPath))
	upstream := strings.TrimSpace(entry.UpstreamEndpoint)
	switch {
	case inbound != "" && upstream != "" && inbound != upstream:
		return "端点：入站 " + inbound + " → 上游 " + upstream
	case inbound != "":
		return "端点：入站 " + inbound
	case upstream != "":
		return "端点：上游 " + upstream
	default:
		return ""
	}
}

// sendFeishuWebhookText 发一条 text 类型的群消息。
// 判定成功的依据不止 HTTP 200：飞书失败时也回 200，靠 body 里的 code/StatusCode 区分。
func sendFeishuWebhookText(ctx context.Context, webhookURL, text string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	payload := map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": opsFeishuTruncate(text, opsFeishuMessageMaxRunes)},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := opsFeishuHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feishu webhook http %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed struct {
		Code          *int   `json:"code"`
		Msg           string `json:"msg"`
		StatusCode    *int   `json:"StatusCode"`
		StatusMessage string `json:"StatusMessage"`
	}
	if err := json.Unmarshal(raw, &parsed); err == nil {
		if parsed.Code != nil && *parsed.Code != 0 {
			return fmt.Errorf("feishu webhook error code=%d msg=%s", *parsed.Code, parsed.Msg)
		}
		if parsed.StatusCode != nil && *parsed.StatusCode != 0 {
			return fmt.Errorf("feishu webhook error status=%d msg=%s", *parsed.StatusCode, parsed.StatusMessage)
		}
	}
	return nil
}

// opsFeishuOneLine 把多行文本压成一行：告警里每个字段占一行，字段内部不该再换行。
func opsFeishuOneLine(value string) string {
	replacer := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ")
	return strings.TrimSpace(replacer.Replace(value))
}

func opsFeishuTruncate(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "…"
}

func opsFeishuFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
