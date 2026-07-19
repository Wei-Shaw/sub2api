package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
)

const (
	opsScheduledReportJobName = "ops_scheduled_reports"

	opsScheduledReportLeaderLockKeyDefault = "ops:scheduled_reports:leader"
	opsScheduledReportLeaderLockTTLDefault = 5 * time.Minute

	opsScheduledReportLastRunKeyPrefix = "ops:scheduled_reports:last_run:"

	opsScheduledReportTickInterval = 1 * time.Minute
)

var opsScheduledReportCronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

var opsScheduledReportReleaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

type OpsScheduledReportService struct {
	opsService   *OpsService
	userService  *UserService
	emailService *EmailService
	redisClient  *redis.Client
	cfg          *config.Config

	instanceID string
	loc        *time.Location

	distributedLockOn bool
	warnNoRedisOnce   sync.Once

	startOnce sync.Once
	stopOnce  sync.Once
	stopCtx   context.Context
	stop      context.CancelFunc
	wg        sync.WaitGroup
REDACTED

func NewOpsScheduledReportService(
	opsService *OpsService,
	userService *UserService,
	emailService *EmailService,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsScheduledReportService {
	lockOn := cfg == nil || strings.TrimSpace(cfg.RunMode) != config.RunModeSimple

	loc := time.Local
	if cfg != nil && strings.TrimSpace(cfg.Timezone) != "" {
		if parsed, err := time.LoadLocation(strings.TrimSpace(cfg.Timezone)); err == nil && parsed != nil {
			loc = parsed
	REDACTED
REDACTED
	return &OpsScheduledReportService{
		opsService:   opsService,
		userService:  userService,
		emailService: emailService,
		redisClient:  redisClient,
		cfg:          cfg,

		instanceID:        uuid.NewString(),
		loc:               loc,
		distributedLockOn: lockOn,
		warnNoRedisOnce:   sync.Once{REDACTED,
		startOnce:         sync.Once{REDACTED,
		stopOnce:          sync.Once{REDACTED,
		stopCtx:           nil,
		stop:              nil,
		wg:                sync.WaitGroup{REDACTED,
REDACTED
REDACTED

func (s *OpsScheduledReportService) Start() {
	s.StartWithContext(context.Background())
REDACTED

func (s *OpsScheduledReportService) StartWithContext(ctx context.Context) {
	if s == nil {
		return
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED
	if s.cfg != nil && !s.cfg.Ops.Enabled {
		return
REDACTED
	if s.opsService == nil || s.emailService == nil {
		return
REDACTED

	s.startOnce.Do(func() {
		s.stopCtx, s.stop = context.WithCancel(ctx)
		s.wg.Add(1)
		go s.run()
REDACTED)
REDACTED

func (s *OpsScheduledReportService) Stop() {
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

func (s *OpsScheduledReportService) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(opsScheduledReportTickInterval)
	defer ticker.Stop()

	s.runOnce()
	for {
		select {
		case <-ticker.C:
			s.runOnce()
		case <-s.stopCtx.Done():
			return
	REDACTED
REDACTED
REDACTED

func (s *OpsScheduledReportService) runOnce() {
	if s == nil || s.opsService == nil || s.emailService == nil {
		return
REDACTED

	startedAt := time.Now().UTC()
	runAt := startedAt

	ctx, cancel := context.WithTimeout(s.stopCtx, 60*time.Second)
	defer cancel()

	// Respect ops monitoring enabled switch.
	if !s.opsService.IsMonitoringEnabled(ctx) {
		return
REDACTED

	release, ok := s.tryAcquireLeaderLock(ctx)
	if !ok {
		return
REDACTED
	if release != nil {
		defer release()
REDACTED

	now := time.Now()
	if s.loc != nil {
		now = now.In(s.loc)
REDACTED

	reports := s.listScheduledReports(ctx, now)
	if len(reports) == 0 {
		return
REDACTED

	reportsTotal := len(reports)
	reportsDue := 0
	sentAttempts := 0

	for _, report := range reports {
		if report == nil || !report.Enabled {
			continue
	REDACTED
		if report.NextRunAt.After(now) {
			continue
	REDACTED
		reportsDue++

		attempts, err := s.runReport(ctx, report, now)
		if err != nil {
			s.recordHeartbeatError(runAt, time.Since(startedAt), err)
			return
	REDACTED
		sentAttempts += attempts
REDACTED

	result := truncateString(fmt.Sprintf("reports=%d due=%d send_attempts=%d", reportsTotal, reportsDue, sentAttempts), 2048)
	s.recordHeartbeatSuccess(runAt, time.Since(startedAt), result)
REDACTED

type opsScheduledReport struct {
	Name       string
	ReportType string
	Schedule   string
	Enabled    bool

	TimeRange time.Duration

	Recipients []string

	ErrorDigestMinCount             int
	AccountHealthErrorRateThreshold float64

	LastRunAt *time.Time
	NextRunAt time.Time
REDACTED

type opsScheduledReportContent struct {
	html     string
	overview *OpsDashboardOverview
REDACTED

func (s *OpsScheduledReportService) listScheduledReports(ctx context.Context, now time.Time) []*opsScheduledReport {
	if s == nil || s.opsService == nil {
		return nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	emailCfg, err := s.opsService.GetEmailNotificationConfig(ctx)
	if err != nil || emailCfg == nil {
		return nil
REDACTED
	if !emailCfg.Report.Enabled {
		return nil
REDACTED

	recipients := normalizeEmails(emailCfg.Report.Recipients)

	type reportDef struct {
		enabled   bool
		name      string
		kind      string
		timeRange time.Duration
		schedule  string
REDACTED

	defs := []reportDef{
		{enabled: emailCfg.Report.DailySummaryEnabled, name: "日报", kind: "daily_summary", timeRange: 24 * time.Hour, schedule: emailCfg.Report.DailySummaryScheduleREDACTED,
		{enabled: emailCfg.Report.WeeklySummaryEnabled, name: "周报", kind: "weekly_summary", timeRange: 7 * 24 * time.Hour, schedule: emailCfg.Report.WeeklySummaryScheduleREDACTED,
		{enabled: emailCfg.Report.ErrorDigestEnabled, name: "错误摘要", kind: "error_digest", timeRange: 24 * time.Hour, schedule: emailCfg.Report.ErrorDigestScheduleREDACTED,
		{enabled: emailCfg.Report.AccountHealthEnabled, name: "账号健康", kind: "account_health", timeRange: 24 * time.Hour, schedule: emailCfg.Report.AccountHealthScheduleREDACTED,
REDACTED

	out := make([]*opsScheduledReport, 0, len(defs))
	for _, d := range defs {
		if !d.enabled {
			continue
	REDACTED
		spec := strings.TrimSpace(d.schedule)
		if spec == "" {
			continue
	REDACTED
		sched, err := opsScheduledReportCronParser.Parse(spec)
		if err != nil {
			log.Printf("[OpsScheduledReport] invalid cron spec=%q for report=%s: %v", spec, d.kind, err)
			continue
	REDACTED

		lastRun := s.getLastRunAt(ctx, d.kind)
		base := lastRun
		if base.IsZero() {
			// Allow a schedule matching the current minute to trigger right after startup.
			base = now.Add(-1 * time.Minute)
	REDACTED
		next := sched.Next(base)
		if next.IsZero() {
			continue
	REDACTED

		var lastRunPtr *time.Time
		if !lastRun.IsZero() {
			lastCopy := lastRun
			lastRunPtr = &lastCopy
	REDACTED

		out = append(out, &opsScheduledReport{
			Name:       d.name,
			ReportType: d.kind,
			Schedule:   spec,
			Enabled:    true,

			TimeRange: d.timeRange,

			Recipients: recipients,

			ErrorDigestMinCount:             emailCfg.Report.ErrorDigestMinCount,
			AccountHealthErrorRateThreshold: emailCfg.Report.AccountHealthErrorRateThreshold,

			LastRunAt: lastRunPtr,
			NextRunAt: next,
	REDACTED)
REDACTED

	return out
REDACTED

func (s *OpsScheduledReportService) runReport(ctx context.Context, report *opsScheduledReport, now time.Time) (int, error) {
	if s == nil || s.opsService == nil || s.emailService == nil || report == nil {
		return 0, nil
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	// Mark as "run" up-front so a broken SMTP config doesn't spam retries every minute.
	s.setLastRunAt(ctx, report.ReportType, now)

	content, err := s.generateReportContent(ctx, report, now)
	if err != nil {
		return 0, err
REDACTED
	if strings.TrimSpace(content.html) == "" {
		// Skip sending when the report decides not to emit content (e.g., digest below min count).
		return 0, nil
REDACTED

	recipients := report.Recipients
	if len(recipients) == 0 && s.userService != nil {
		admin, err := s.userService.GetFirstAdmin(ctx)
		if err == nil && admin != nil && strings.TrimSpace(admin.Email) != "" {
			recipients = []string{strings.TrimSpace(admin.Email)REDACTED
	REDACTED
REDACTED
	if len(recipients) == 0 {
		return 0, nil
REDACTED

	attempts := 0
	for _, to := range recipients {
		addr := strings.TrimSpace(to)
		if addr == "" {
			continue
	REDACTED
		attempts++
		locale := ""
		if s.emailService.notificationEmailService != nil {
			locale = s.emailService.notificationEmailService.ResolveRecipientLocale(ctx, 0, addr)
			templateVariables := opsScheduledReportLocalizedEmailVariables(report, now, locale)
			rawHTMLVariables := map[string]string{"report_html": content.htmlREDACTED
			if isOpsSummaryReport(report) {
				templateVariables = opsSummaryReportEmailVariables(report, now, content.overview, locale)
		REDACTED
			if err := s.emailService.notificationEmailService.Send(ctx, NotificationEmailSendInput{
				Event:            NotificationEmailEventOpsScheduledReport,
				Locale:           locale,
				RecipientEmail:   addr,
				RecipientName:    emailRecipientName(addr),
				SourceType:       "ops_scheduled_report",
				SourceID:         opsScheduledReportDeliverySourceID(report),
				ReminderKey:      now.UTC().Format("2006-01-02T15:04"),
				Variables:        templateVariables,
				RawHTMLVariables: rawHTMLVariables,
		REDACTED); err == nil {
				continue
		REDACTED else if !shouldFallbackNotificationEmail(err) {
				continue
		REDACTED
	REDACTED
		subjectName := strings.TrimSpace(report.Name)
		if locale != "" {
			subjectName = opsScheduledReportLocalizedName(report, locale)
	REDACTED
		subjectPrefix := "[Ops Report]"
		if strings.HasPrefix(strings.ToLower(locale), "zh") {
			subjectPrefix = "[运维报表]"
	REDACTED
		subject := fmt.Sprintf("%s %s", subjectPrefix, subjectName)
		if err := s.emailService.SendEmail(ctx, addr, subject, content.html); err != nil {
			// Ignore per-recipient failures; continue best-effort.
			continue
	REDACTED
REDACTED
	return attempts, nil
REDACTED

func opsScheduledReportDeliverySourceID(report *opsScheduledReport) string {
	if report == nil {
		return "scheduled_report"
REDACTED
	parts := []string{
		strings.TrimSpace(report.ReportType),
		strings.TrimSpace(report.Name),
		strings.TrimSpace(report.Schedule),
REDACTED
	joined := strings.Trim(strings.Join(parts, ":"), ":")
	if joined == "" {
		return "scheduled_report"
REDACTED
	return joined
REDACTED

func opsScheduledReportEmailVariables(report *opsScheduledReport, now time.Time) map[string]string {
	end := now.UTC()
	start := end
	name := "Ops report"
	reportType := "scheduled_report"
	if report != nil {
		if strings.TrimSpace(report.Name) != "" {
			name = strings.TrimSpace(report.Name)
	REDACTED
		if strings.TrimSpace(report.ReportType) != "" {
			reportType = strings.TrimSpace(report.ReportType)
	REDACTED
		if report.TimeRange > 0 {
			start = end.Add(-report.TimeRange)
	REDACTED
REDACTED
	return map[string]string{
		"report_name":       name,
		"report_type":       reportType,
		"report_start_time": start.Format(time.RFC3339),
		"report_end_time":   end.Format(time.RFC3339),
REDACTED
REDACTED

func opsScheduledReportLocalizedEmailVariables(report *opsScheduledReport, now time.Time, locale string) map[string]string {
	variables := opsScheduledReportEmailVariables(report, now)
	variables["report_html"] = ""
	variables["report_detail_display"] = "block"
	for _, placeholder := range notificationEmailOpsSummaryPlaceholders {
		variables[placeholder] = "-"
REDACTED
	variables["report_summary_display"] = "none"
	if name := opsScheduledReportLocalizedName(report, locale); name != "" {
		variables["report_name"] = name
REDACTED
	return variables
REDACTED

func opsScheduledReportLocalizedName(report *opsScheduledReport, locale string) string {
	if report == nil {
		return "Ops report"
REDACTED
	chinese := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh")
	switch strings.TrimSpace(report.ReportType) {
	case "daily_summary":
		if chinese {
			return "日报"
	REDACTED
		return "Daily summary"
	case "weekly_summary":
		if chinese {
			return "周报"
	REDACTED
		return "Weekly summary"
	case "error_digest":
		if chinese {
			return "错误摘要"
	REDACTED
		return "Error digest"
	case "account_health":
		if chinese {
			return "账号健康"
	REDACTED
		return "Account health"
	default:
		return strings.TrimSpace(report.Name)
REDACTED
REDACTED

func isOpsSummaryReport(report *opsScheduledReport) bool {
	if report == nil {
		return false
REDACTED
	switch strings.TrimSpace(report.ReportType) {
	case "daily_summary", "weekly_summary":
		return true
	default:
		return false
REDACTED
REDACTED

func opsSummaryReportEmailVariables(report *opsScheduledReport, now time.Time, overview *OpsDashboardOverview, locale string) map[string]string {
	variables := opsScheduledReportLocalizedEmailVariables(report, now, locale)
	variables["report_detail_display"] = "none"
	if overview == nil {
		for _, placeholder := range notificationEmailOpsSummaryPlaceholders {
			if placeholder == "report_summary_display" {
				continue
		REDACTED
			variables[placeholder] = "-"
	REDACTED
		variables["report_summary_display"] = "block"
		return variables
REDACTED
	variables["report_summary_display"] = "block"

	variables["report_total_requests"] = formatOpsReportInteger(overview.RequestCountTotal)
	variables["report_success_count"] = formatOpsReportInteger(overview.SuccessCount)
	variables["report_sla_error_count"] = formatOpsReportInteger(overview.ErrorCountSLA)
	variables["report_business_limited_count"] = formatOpsReportInteger(overview.BusinessLimitedCount)
	variables["report_sla"] = fmt.Sprintf("%.2f%%", overview.SLA*100)
	variables["report_error_rate"] = fmt.Sprintf("%.2f%%", overview.ErrorRate*100)
	variables["report_upstream_error_rate"] = fmt.Sprintf("%.2f%%", overview.UpstreamErrorRate*100)
	variables["report_upstream_error_count_excl_429_529"] = formatOpsReportInteger(overview.UpstreamErrorCountExcl429529)
	variables["report_upstream_429_count"] = formatOpsReportInteger(overview.Upstream429Count)
	variables["report_upstream_529_count"] = formatOpsReportInteger(overview.Upstream529Count)
	variables["report_latency_p50"] = formatOpsReportMilliseconds(overview.Duration.P50)
	variables["report_latency_p99"] = formatOpsReportMilliseconds(overview.Duration.P99)
	variables["report_ttft_p50"] = formatOpsReportMilliseconds(overview.TTFT.P50)
	variables["report_ttft_p99"] = formatOpsReportMilliseconds(overview.TTFT.P99)
	variables["report_tokens"] = formatOpsReportInteger(overview.TokenConsumed)
	variables["report_qps_current"] = fmt.Sprintf("%.1f", overview.QPS.Current)
	variables["report_qps_peak"] = fmt.Sprintf("%.1f", overview.QPS.Peak)
	variables["report_qps_avg"] = fmt.Sprintf("%.1f", overview.QPS.Avg)
	variables["report_tps_current"] = fmt.Sprintf("%.1f", overview.TPS.Current)
	variables["report_tps_peak"] = fmt.Sprintf("%.1f", overview.TPS.Peak)
	variables["report_tps_avg"] = fmt.Sprintf("%.1f", overview.TPS.Avg)
	return variables
REDACTED

func formatOpsReportInteger(value int64) string {
	raw := strconv.FormatInt(value, 10)
	start := 0
	if strings.HasPrefix(raw, "-") {
		start = 1
REDACTED
	if len(raw)-start <= 3 {
		return raw
REDACTED

	var builder strings.Builder
	builder.Grow(len(raw) + (len(raw)-start-1)/3)
	_, _ = builder.WriteString(raw[:start])
	digitLen := len(raw) - start
	for offset := 0; offset < digitLen; offset++ {
		if offset > 0 && (digitLen-offset)%3 == 0 {
			_ = builder.WriteByte(',')
	REDACTED
		_ = builder.WriteByte(raw[start+offset])
REDACTED
	return builder.String()
REDACTED

func formatOpsReportMilliseconds(value *int) string {
	if value == nil {
		return "-"
REDACTED
	return fmt.Sprintf("%s ms", formatOpsReportInteger(int64(*value)))
REDACTED

func (s *OpsScheduledReportService) generateReportContent(ctx context.Context, report *opsScheduledReport, now time.Time) (opsScheduledReportContent, error) {
	if s == nil || s.opsService == nil || report == nil {
		return opsScheduledReportContent{REDACTED, fmt.Errorf("service not initialized")
REDACTED
	if report.TimeRange <= 0 {
		return opsScheduledReportContent{REDACTED, fmt.Errorf("invalid time range")
REDACTED

	end := now.UTC()
	start := end.Add(-report.TimeRange)

	switch strings.TrimSpace(report.ReportType) {
	case "daily_summary", "weekly_summary":
		overview, err := s.opsService.GetDashboardOverview(ctx, &OpsDashboardFilter{
			StartTime: start,
			EndTime:   end,
			Platform:  "",
			GroupID:   nil,
			QueryMode: OpsQueryModeAuto,
	REDACTED)
		if err != nil {
			// If pre-aggregation isn't ready but the report is requested, fall back to raw.
			if strings.TrimSpace(report.ReportType) == "daily_summary" || strings.TrimSpace(report.ReportType) == "weekly_summary" {
				overview, err = s.opsService.GetDashboardOverview(ctx, &OpsDashboardFilter{
					StartTime: start,
					EndTime:   end,
					Platform:  "",
					GroupID:   nil,
					QueryMode: OpsQueryModeRaw,
			REDACTED)
		REDACTED
			if err != nil {
				return opsScheduledReportContent{REDACTED, err
		REDACTED
	REDACTED
		return opsScheduledReportContent{
			html:     buildOpsSummaryEmailHTML(report.Name, start, end, overview),
			overview: overview,
	REDACTED, nil
	case "error_digest":
		// Lightweight digest: list recent errors (status>=400) and breakdown by type.
		startTime := start
		endTime := end
		filter := &OpsErrorLogFilter{
			StartTime: &startTime,
			EndTime:   &endTime,
			Page:      1,
			PageSize:  100,
	REDACTED
		out, err := s.opsService.GetErrorLogs(ctx, filter)
		if err != nil {
			return opsScheduledReportContent{REDACTED, err
	REDACTED
		if report.ErrorDigestMinCount > 0 && out != nil && out.Total < report.ErrorDigestMinCount {
			return opsScheduledReportContent{REDACTED, nil
	REDACTED
		return opsScheduledReportContent{html: buildOpsErrorDigestEmailHTML(report.Name, start, end, out)REDACTED, nil
	case "account_health":
		// Best-effort: use account availability (not error rate yet).
		avail, err := s.opsService.GetAccountAvailability(ctx, "", nil)
		if err != nil {
			return opsScheduledReportContent{REDACTED, err
	REDACTED
		_ = report.AccountHealthErrorRateThreshold // reserved for future per-account error rate report
		return opsScheduledReportContent{html: buildOpsAccountHealthEmailHTML(report.Name, start, end, avail)REDACTED, nil
	default:
		return opsScheduledReportContent{REDACTED, fmt.Errorf("unknown report type: %s", report.ReportType)
REDACTED
REDACTED

func buildOpsSummaryEmailHTML(title string, start, end time.Time, overview *OpsDashboardOverview) string {
	if overview == nil {
		return fmt.Sprintf("<h2>%s</h2><p>No data.</p>", htmlEscape(title))
REDACTED

	latP50 := "-"
	latP99 := "-"
	if overview.Duration.P50 != nil {
		latP50 = fmt.Sprintf("%dms", *overview.Duration.P50)
REDACTED
	if overview.Duration.P99 != nil {
		latP99 = fmt.Sprintf("%dms", *overview.Duration.P99)
REDACTED

	ttftP50 := "-"
	ttftP99 := "-"
	if overview.TTFT.P50 != nil {
		ttftP50 = fmt.Sprintf("%dms", *overview.TTFT.P50)
REDACTED
	if overview.TTFT.P99 != nil {
		ttftP99 = fmt.Sprintf("%dms", *overview.TTFT.P99)
REDACTED

	return fmt.Sprintf(`
<h2>%s</h2>
<p><b>Period</b>: %s ~ %s (UTC)</p>
<ul>
  <li><b>Total Requests</b>: %d</li>
  <li><b>Success</b>: %d</li>
  <li><b>Errors (SLA)</b>: %d</li>
  <li><b>Business Limited</b>: %d</li>
  <li><b>SLA</b>: %.2f%%</li>
  <li><b>Error Rate</b>: %.2f%%</li>
  <li><b>Upstream Error Rate (excl 429/529)</b>: %.2f%%</li>
  <li><b>Upstream Errors</b>: excl429/529=%d, 429=%d, 529=%d</li>
  <li><b>Latency</b>: p50=%s, p99=%s</li>
  <li><b>TTFT</b>: p50=%s, p99=%s</li>
  <li><b>Tokens</b>: %d</li>
  <li><b>QPS</b>: current=%.1f, peak=%.1f, avg=%.1f</li>
  <li><b>TPS</b>: current=%.1f, peak=%.1f, avg=%.1f</li>
</ul>
`,
		htmlEscape(strings.TrimSpace(title)),
		htmlEscape(start.UTC().Format(time.RFC3339)),
		htmlEscape(end.UTC().Format(time.RFC3339)),
		overview.RequestCountTotal,
		overview.SuccessCount,
		overview.ErrorCountSLA,
		overview.BusinessLimitedCount,
		overview.SLA*100,
		overview.ErrorRate*100,
		overview.UpstreamErrorRate*100,
		overview.UpstreamErrorCountExcl429529,
		overview.Upstream429Count,
		overview.Upstream529Count,
		htmlEscape(latP50),
		htmlEscape(latP99),
		htmlEscape(ttftP50),
		htmlEscape(ttftP99),
		overview.TokenConsumed,
		overview.QPS.Current,
		overview.QPS.Peak,
		overview.QPS.Avg,
		overview.TPS.Current,
		overview.TPS.Peak,
		overview.TPS.Avg,
	)
REDACTED

func buildOpsErrorDigestEmailHTML(title string, start, end time.Time, list *OpsErrorLogList) string {
	total := 0
	recent := []*OpsErrorLog{REDACTED
	if list != nil {
		total = list.Total
		recent = list.Errors
REDACTED
	if len(recent) > 10 {
		recent = recent[:10]
REDACTED

	rows := ""
	for _, item := range recent {
		if item == nil {
			continue
	REDACTED
		rows += fmt.Sprintf(
			"<tr><td>%s</td><td>%s</td><td>%d</td><td>%s</td></tr>",
			htmlEscape(item.CreatedAt.UTC().Format(time.RFC3339)),
			htmlEscape(item.Platform),
			item.StatusCode,
			htmlEscape(truncateString(item.Message, 180)),
		)
REDACTED
	if rows == "" {
		rows = "<tr><td colspan=\"4\">No recent errors.</td></tr>"
REDACTED

	return fmt.Sprintf(`
<h2>%s</h2>
<p><b>Period</b>: %s ~ %s (UTC)</p>
<p><b>Total Errors</b>: %d</p>
<h3>Recent</h3>
<table border="1" cellpadding="6" cellspacing="0" style="border-collapse:collapse;">
  <thead><tr><th>Time</th><th>Platform</th><th>Status</th><th>Message</th></tr></thead>
  <tbody>%s</tbody>
</table>
`,
		htmlEscape(strings.TrimSpace(title)),
		htmlEscape(start.UTC().Format(time.RFC3339)),
		htmlEscape(end.UTC().Format(time.RFC3339)),
		total,
		rows,
	)
REDACTED

func buildOpsAccountHealthEmailHTML(title string, start, end time.Time, avail *OpsAccountAvailability) string {
	total := 0
	available := 0
	rateLimited := 0
	hasError := 0

	if avail != nil && avail.Accounts != nil {
		for _, a := range avail.Accounts {
			if a == nil {
				continue
		REDACTED
			total++
			if a.IsAvailable {
				available++
		REDACTED
			if a.IsRateLimited {
				rateLimited++
		REDACTED
			if a.HasError {
				hasError++
		REDACTED
	REDACTED
REDACTED

	return fmt.Sprintf(`
<h2>%s</h2>
<p><b>Period</b>: %s ~ %s (UTC)</p>
<ul>
  <li><b>Total Accounts</b>: %d</li>
  <li><b>Available</b>: %d</li>
  <li><b>Rate Limited</b>: %d</li>
  <li><b>Error</b>: %d</li>
</ul>
<p>Note: This report currently reflects account availability status only.</p>
`,
		htmlEscape(strings.TrimSpace(title)),
		htmlEscape(start.UTC().Format(time.RFC3339)),
		htmlEscape(end.UTC().Format(time.RFC3339)),
		total,
		available,
		rateLimited,
		hasError,
	)
REDACTED

func (s *OpsScheduledReportService) tryAcquireLeaderLock(ctx context.Context) (func(), bool) {
	if s == nil || !s.distributedLockOn {
		return nil, true
REDACTED
	if s.redisClient == nil {
		s.warnNoRedisOnce.Do(func() {
			log.Printf("[OpsScheduledReport] redis not configured; running without distributed lock")
	REDACTED)
		return nil, true
REDACTED
	if ctx == nil {
		ctx = context.Background()
REDACTED

	key := opsScheduledReportLeaderLockKeyDefault
	ttl := opsScheduledReportLeaderLockTTLDefault
	if strings.TrimSpace(key) == "" {
		key = "ops:scheduled_reports:leader"
REDACTED
	if ttl <= 0 {
		ttl = 5 * time.Minute
REDACTED

	ok, err := s.redisClient.SetNX(ctx, key, s.instanceID, ttl).Result()
	if err != nil {
		// Prefer fail-closed to avoid duplicate report sends when Redis is flaky.
		log.Printf("[OpsScheduledReport] leader lock SetNX failed; skipping this cycle: %v", err)
		return nil, false
REDACTED
	if !ok {
		return nil, false
REDACTED
	return func() {
		_, _ = opsScheduledReportReleaseScript.Run(ctx, s.redisClient, []string{keyREDACTED, s.instanceID).Result()
REDACTED, true
REDACTED

func (s *OpsScheduledReportService) getLastRunAt(ctx context.Context, reportType string) time.Time {
	if s == nil || s.redisClient == nil {
		return time.Time{REDACTED
REDACTED
	kind := strings.TrimSpace(reportType)
	if kind == "" {
		return time.Time{REDACTED
REDACTED
	key := opsScheduledReportLastRunKeyPrefix + kind

	raw, err := s.redisClient.Get(ctx, key).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		return time.Time{REDACTED
REDACTED
	sec, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || sec <= 0 {
		return time.Time{REDACTED
REDACTED
	last := time.Unix(sec, 0)
	// Cron schedules are interpreted in the configured timezone (s.loc). Ensure the base time
	// passed into cron.Next() uses the same location; otherwise the job will drift by timezone
	// offset (e.g. Asia/Shanghai default would run 8h later after the first execution).
	if s.loc != nil {
		return last.In(s.loc)
REDACTED
	return last.UTC()
REDACTED

func (s *OpsScheduledReportService) setLastRunAt(ctx context.Context, reportType string, t time.Time) {
	if s == nil || s.redisClient == nil {
		return
REDACTED
	kind := strings.TrimSpace(reportType)
	if kind == "" {
		return
REDACTED
	if t.IsZero() {
		t = time.Now().UTC()
REDACTED
	key := opsScheduledReportLastRunKeyPrefix + kind
	_ = s.redisClient.Set(ctx, key, strconv.FormatInt(t.UTC().Unix(), 10), 14*24*time.Hour).Err()
REDACTED

func (s *OpsScheduledReportService) recordHeartbeatSuccess(runAt time.Time, duration time.Duration, result string) {
	if s == nil || s.opsService == nil || s.opsService.opsRepo == nil {
		return
REDACTED
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msg := strings.TrimSpace(result)
	if msg == "" {
		msg = "ok"
REDACTED
	msg = truncateString(msg, 2048)
	_ = s.opsService.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsScheduledReportJobName,
		LastRunAt:      &runAt,
		LastSuccessAt:  &now,
		LastDurationMs: &durMs,
		LastResult:     &msg,
REDACTED)
REDACTED

func (s *OpsScheduledReportService) recordHeartbeatError(runAt time.Time, duration time.Duration, err error) {
	if s == nil || s.opsService == nil || s.opsService.opsRepo == nil || err == nil {
		return
REDACTED
	now := time.Now().UTC()
	durMs := duration.Milliseconds()
	msg := truncateString(err.Error(), 2048)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.opsService.opsRepo.UpsertJobHeartbeat(ctx, &OpsUpsertJobHeartbeatInput{
		JobName:        opsScheduledReportJobName,
		LastRunAt:      &runAt,
		LastErrorAt:    &now,
		LastError:      &msg,
		LastDurationMs: &durMs,
REDACTED)
REDACTED

func normalizeEmails(in []string) []string {
	if len(in) == 0 {
		return nil
REDACTED
	seen := make(map[string]struct{REDACTED, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		addr := strings.ToLower(strings.TrimSpace(raw))
		if addr == "" {
			continue
	REDACTED
		if _, ok := seen[addr]; ok {
			continue
	REDACTED
		seen[addr] = struct{REDACTED{REDACTED
		out = append(out, addr)
REDACTED
	return out
REDACTED
