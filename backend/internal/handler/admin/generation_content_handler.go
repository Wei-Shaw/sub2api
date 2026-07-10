package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// 采集内容看板（只读）的展示常量。
const (
	genContentSampleLimit   = 20  // 取最近 N 条供样本墙挑选展示
	genContentPromptPreview = 120 // prompt 预览最大 rune 数
	genContentRespPreview   = 80  // response 预览最大 rune 数
)

// GenerationContentHandler 提供 ai_generation_content 表的只读统计 + 样本端点（护城河看板）。
// 纯聚合无业务逻辑，故直接依赖 repo 接口，不引入额外 service 层。
type GenerationContentHandler struct {
	repo           service.GenerationContentRepository
	cfg            *config.Config
	settingService *service.SettingService
}

func NewGenerationContentHandler(repo service.GenerationContentRepository, cfg *config.Config, settingServices ...*service.SettingService) *GenerationContentHandler {
	var settingService *service.SettingService
	if len(settingServices) > 0 {
		settingService = settingServices[0]
	}
	return &GenerationContentHandler{repo: repo, cfg: cfg, settingService: settingService}
}

// GetStats handles GET /api/v1/admin/generation-content/stats
func (h *GenerationContentHandler) GetStats(c *gin.Context) {
	stats, err := h.repo.GetCaptureStats(c.Request.Context())
	if err != nil {
		response.Error(c, 500, "Failed to get generation content stats")
		return
	}

	response.Success(c, gin.H{
		"captured_today":     stats.CapturedToday,
		"captured_week":      stats.CapturedWeek,
		"distinct_employees": stats.DistinctEmployees,
		"distinct_teams":     stats.DistinctTeams,
		"distinct_models":    stats.DistinctModels,
		"total_bytes":        stats.TotalBytes,
		"daily_rate":         float64(stats.CapturedWeek) / 7.0,
		"daily_series":       buildDailySeries(stats.DailySeries),
		// 有任何数据即为实时态；空表 → 看板显"示例(未开启采集)"，绝不伪造聚合。
		"is_live": stats.Total > 0,
	})
}

// GetSamples handles GET /api/v1/admin/generation-content/samples
func (h *GenerationContentHandler) GetSamples(c *gin.Context) {
	rows, err := h.repo.GetRecent(c.Request.Context(), genContentSampleLimit)
	if err != nil {
		response.Error(c, 500, "Failed to get generation content samples")
		return
	}

	samples := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		promptPreview, pTrunc := truncateRunes(r.PromptRedacted, genContentPromptPreview)
		respPreview, rTrunc := truncateRunes(r.ResponseRedacted, genContentRespPreview)
		samples = append(samples, gin.H{
			"employee_name":    employeeName(r.Username, r.Email),
			"team_name":        teamName(r.GroupName),
			"task_id":          r.TaskID,
			"model":            r.Model,
			"video_status":     r.VideoStatus,
			"cost_estimate":    r.CostEstimate,
			"currency":         r.Currency,
			"created_at":       r.CreatedAt,
			"prompt_preview":   promptPreview,
			"response_preview": respPreview,
			"total_bytes":      r.PromptBytes + r.ResponseBytes,
			"adoption_status":  r.AdoptionStatus,
			"quality_score":    r.QualityScore,
			"adoption_notes":   r.AdoptionNotes,
			// 截断标:预览被裁 或 上游响应被限容截断,任一即标记。
			"truncated": pTrunc || rTrunc || r.ResponseTruncated,
		})
	}

	response.Success(c, gin.H{
		"samples":      samples,
		"is_live":      len(rows) > 0,
		"usd_cny_rate": resolveUSDCNYRate(c.Request.Context(), h.settingService),
	})
}

// buildDailySeries 把仓库返回的稀疏日序列零填充为最近 7 个 UTC 日点(老→新)，前端永远拿到 7 个点。
type updateAdoptionRequest struct {
	AdoptionStatus string   `json:"adoption_status"`
	QualityScore   *float64 `json:"quality_score"`
	Notes          string   `json:"notes"`
}

// UpdateAdoption handles POST /api/v1/admin/generation-content/:task_id/adoption.
func (h *GenerationContentHandler) UpdateAdoption(c *gin.Context) {
	taskID, err := strconv.ParseInt(strings.TrimSpace(c.Param("task_id")), 10, 64)
	if err != nil || taskID <= 0 {
		response.BadRequest(c, "Invalid task_id")
		return
	}

	var req updateAdoptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid adoption payload")
		return
	}

	status := normalizeAdoptionStatus(req.AdoptionStatus)
	if !isValidAdoptionStatus(status) {
		response.BadRequest(c, "Invalid adoption_status")
		return
	}
	if req.QualityScore != nil && (*req.QualityScore < 0 || *req.QualityScore > 1) {
		response.BadRequest(c, "quality_score must be between 0 and 1")
		return
	}

	input := service.GenerationContentAdoptionInput{
		TaskID:         taskID,
		AdoptionStatus: status,
		QualityScore:   req.QualityScore,
		Notes:          truncateNote(req.Notes, 2048),
	}
	if !h.adoptionFeedbackEnabled() {
		response.Success(c, gin.H{
			"enabled":         false,
			"saved":           false,
			"reason":          "content_capture_disabled",
			"task_id":         taskID,
			"adoption_status": status,
			"quality_score":   req.QualityScore,
			"notes":           input.Notes,
		})
		return
	}

	result, err := h.repo.UpdateVideoTaskAdoption(c.Request.Context(), input)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to save adoption feedback")
		return
	}
	saved := result != nil && result.Saved
	payload := gin.H{
		"enabled":         true,
		"saved":           saved,
		"task_id":         taskID,
		"adoption_status": status,
		"quality_score":   req.QualityScore,
		"notes":           input.Notes,
	}
	if !saved {
		payload["reason"] = "task_not_found"
	}
	response.Success(c, payload)
}

// GetWeeklyReport handles GET /api/v1/admin/generation-content/weekly-report.
func (h *GenerationContentHandler) GetWeeklyReport(c *gin.Context) {
	start, end, err := weeklyWindow(c.Query("start"), c.Query("end"), time.Now().UTC())
	if err != nil {
		response.BadRequest(c, "Invalid weekly report window")
		return
	}
	report, err := h.repo.GetWeeklyReport(c.Request.Context(), start, end)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to get generation content weekly report")
		return
	}
	if report == nil {
		report = &service.GenerationContentWeeklyReport{PeriodStart: start, PeriodEnd: end}
	}
	response.Success(c, gin.H{
		"period_start":        report.PeriodStart,
		"period_end":          report.PeriodEnd,
		"entries":             report.Entries,
		"video_tasks":         report.VideoTasks,
		"total_cost_estimate": report.TotalCostEstimate,
		"adopted_count":       report.AdoptedCount,
		"rejected_count":      report.RejectedCount,
		"pending_count":       report.PendingCount,
		"unreviewed_count":    report.UnreviewedCount,
		"adoption_rate":       report.AdoptionRate,
		"usd_cny_rate":        resolveUSDCNYRate(c.Request.Context(), h.settingService),
		"anomalies": gin.H{
			"failed_tasks":       report.Anomalies.FailedTasks,
			"missing_task_joins": report.Anomalies.MissingTaskJoins,
			"truncated_rows":     report.Anomalies.TruncatedRows,
		},
		"markdown": buildWeeklyReportMarkdown(report),
	})
}

func (h *GenerationContentHandler) adoptionFeedbackEnabled() bool {
	return h != nil && h.cfg != nil && h.cfg.Gateway.ContentCapture.Enabled
}

func normalizeAdoptionStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func isValidAdoptionStatus(status string) bool {
	switch status {
	case "adopted", "rejected", "pending":
		return true
	default:
		return false
	}
}

func truncateNote(note string, max int) string {
	note = strings.TrimSpace(note)
	if max <= 0 || utf8.RuneCountInString(note) <= max {
		return note
	}
	return string([]rune(note)[:max])
}

func weeklyWindow(startRaw, endRaw string, now time.Time) (time.Time, time.Time, error) {
	end := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	start := end.AddDate(0, 0, -7)
	if strings.TrimSpace(startRaw) != "" {
		parsed, err := parseReportDate(startRaw)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		start = parsed
	}
	if strings.TrimSpace(endRaw) != "" {
		parsed, err := parseReportDate(endRaw)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		end = parsed
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, fmt.Errorf("start must be before end")
	}
	return start, end, nil
}

func parseReportDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse("2006-01-02", raw); err == nil {
		return t.UTC(), nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

func buildWeeklyReportMarkdown(report *service.GenerationContentWeeklyReport) string {
	if report == nil {
		return "# Weekly Production Ledger\n\nNo data.\n"
	}
	return fmt.Sprintf(`# Weekly Production Ledger

- Period: %s to %s
- Entries: %d
- Video tasks: %d
- Cost estimate: %.6f
- Adoption: adopted=%d rejected=%d pending=%d unreviewed=%d rate=%.2f
- Anomalies: failed_tasks=%d missing_task_joins=%d truncated_rows=%d
`,
		report.PeriodStart.Format("2006-01-02"),
		report.PeriodEnd.Format("2006-01-02"),
		report.Entries,
		report.VideoTasks,
		report.TotalCostEstimate,
		report.AdoptedCount,
		report.RejectedCount,
		report.PendingCount,
		report.UnreviewedCount,
		report.AdoptionRate,
		report.Anomalies.FailedTasks,
		report.Anomalies.MissingTaskJoins,
		report.Anomalies.TruncatedRows,
	)
}

func buildDailySeries(points []service.GenerationContentDailyPoint) []gin.H {
	counts := make(map[string]int64, len(points))
	for _, p := range points {
		counts[p.Date] = p.Count
	}
	now := time.Now().UTC()
	out := make([]gin.H, 0, 7)
	for i := 6; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		out = append(out, gin.H{"date": d, "count": counts[d]})
	}
	return out
}

// truncateRunes 返回最多 max 个 rune(UTF-8 安全,中文不截半)及是否发生截断。
func truncateRunes(s string, max int) (string, bool) {
	if utf8.RuneCountInString(s) <= max {
		return s, false
	}
	return string([]rune(s)[:max]), true
}

// employeeName 优先用户名,退化为邮箱,再退化为占位符。
func employeeName(username, email string) string {
	if username != "" {
		return username
	}
	if email != "" {
		return email
	}
	return "—"
}

// teamName 团队名退化为占位符。
func teamName(name string) string {
	if name != "" {
		return name
	}
	return "—"
}
