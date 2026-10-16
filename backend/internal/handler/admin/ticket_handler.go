package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type TicketHandler struct {
	ticketService *service.TicketService
}

func NewTicketHandler(ticketService *service.TicketService) *TicketHandler {
	return &TicketHandler{ticketService: ticketService}
}

type replyTicketRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *TicketHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "updated_at"),
		SortOrder: c.DefaultQuery("sort_order", "desc"),
	}
	items, result, err := h.ticketService.ListForAdmin(
		c.Request.Context(),
		params,
		strings.TrimSpace(c.Query("status")),
		strings.TrimSpace(c.Query("search")),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, dto.TicketsFromService(items), result.Total, page, pageSize)
}

func (h *TicketHandler) GetByID(c *gin.Context) {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || ticketID <= 0 {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}
	ticket, err := h.ticketService.GetForAdmin(c.Request.Context(), ticketID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.TicketFromService(ticket))
}

func (h *TicketHandler) Reply(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || ticketID <= 0 {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}
	var req replyTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	ticket, err := h.ticketService.ReplyAsAdmin(c.Request.Context(), subject.UserID, ticketID, &service.ReplyTicketInput{
		Content: req.Content,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.TicketFromService(ticket))
}

func (h *TicketHandler) Close(c *gin.Context) {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || ticketID <= 0 {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}
	ticket, err := h.ticketService.CloseAsAdmin(c.Request.Context(), ticketID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.TicketFromService(ticket))
}

func (h *TicketHandler) Reopen(c *gin.Context) {
	ticketID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || ticketID <= 0 {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}
	ticket, err := h.ticketService.ReopenAsAdmin(c.Request.Context(), ticketID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.TicketFromService(ticket))
}
