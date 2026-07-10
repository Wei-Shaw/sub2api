package handler

import (
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// GenerationContentHandler exposes API-key adoption feedback for employee-owned video tasks.
type GenerationContentHandler struct {
	adoption *service.GenerationContentAdoptionService
}

func NewGenerationContentHandler(adoption *service.GenerationContentAdoptionService) *GenerationContentHandler {
	return &GenerationContentHandler{adoption: adoption}
}

type submitAdoptionRequest struct {
	AdoptionStatus string   `json:"adoption_status"`
	QualityScore   *float64 `json:"quality_score"`
	Notes          string   `json:"notes"`
}

// SubmitAdoption handles POST /v1/generation-content/:task_id/adoption.
func (h *GenerationContentHandler) SubmitAdoption(c *gin.Context) {
	if h == nil || h.adoption == nil {
		response.Error(c, http.StatusInternalServerError, "Adoption service unavailable")
		return
	}

	taskID, err := strconv.ParseInt(strings.TrimSpace(c.Param("task_id")), 10, 64)
	if err != nil || taskID <= 0 {
		response.BadRequest(c, "Invalid task_id")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "User not authenticated")
		return
	}

	var req submitAdoptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid adoption payload")
		return
	}

	status := strings.ToLower(strings.TrimSpace(req.AdoptionStatus))
	input := service.GenerationContentAdoptionInput{
		TaskID:         taskID,
		AdoptionStatus: status,
		QualityScore:   req.QualityScore,
		Notes:          truncateAdoptionNote(req.Notes, 2048),
	}

	result, err := h.adoption.SubmitVideoTaskAdoption(c.Request.Context(), subject.UserID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	if result == nil {
		response.Error(c, http.StatusInternalServerError, "Failed to save adoption feedback")
		return
	}

	if !h.adoption.AdoptionFeedbackEnabled() {
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

	saved := result.Saved
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

func truncateAdoptionNote(note string, max int) string {
	note = strings.TrimSpace(note)
	if max <= 0 || utf8.RuneCountInString(note) <= max {
		return note
	}
	return string([]rune(note)[:max])
}
