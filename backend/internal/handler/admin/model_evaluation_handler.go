package admin

import (
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ModelEvaluationHandler struct {
	service *service.ModelEvaluationHistoryService
}

func NewModelEvaluationHandler(svc *service.ModelEvaluationHistoryService) *ModelEvaluationHandler {
	return &ModelEvaluationHandler{service: svc}
}

func (h *ModelEvaluationHandler) Create(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req service.ModelEvaluationCreate
	if c.ShouldBindJSON(&req) != nil {
		response.BadRequest(c, "评测参数格式错误，请重新选择目标与模型")
		return
	}
	var createdBy int64
	if subject, ok := middleware.GetAuthSubjectFromContext(c); ok {
		createdBy = subject.UserID
	}
	result, err := h.service.Create(c.Request.Context(), req, createdBy)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ModelEvaluationHandler) Run(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req struct {
		Round int `json:"round"`
	}
	if c.ShouldBindJSON(&req) != nil {
		response.BadRequest(c, "轮次参数无效，请重新开始评测")
		return
	}
	result, err := h.service.Run(c.Request.Context(), c.Param("id"), req.Round)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ModelEvaluationHandler) List(c *gin.Context) {
	targetID, _ := strconv.ParseInt(c.Query("target_id"), 10, 64)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	result, err := h.service.List(c.Request.Context(), c.Query("target_type"), targetID, page)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ModelEvaluationHandler) Get(c *gin.Context) {
	result, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}
