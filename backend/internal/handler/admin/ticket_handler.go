package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type TicketHandler struct{ tickets *service.TicketService }

func NewTicketHandler(tickets *service.TicketService) *TicketHandler {
	return &TicketHandler{tickets: tickets}
}

type adminTicketMessageRequest struct {
	Content string   `json:"content"`
	Images  []string `json:"images"`
}

func (h *TicketHandler) List(c *gin.Context) {
	items, err := h.tickets.List(c.Request.Context(), 0, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}
func (h *TicketHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}
	item, err := h.tickets.Get(c.Request.Context(), id, 0, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}
func (h *TicketHandler) Message(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}
	var req adminTicketMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	msg, err := h.tickets.AddMessage(c.Request.Context(), service.AddTicketMessageInput{TicketID: id, SenderID: subject.UserID, SenderType: service.TicketSenderAdmin, Content: req.Content, Images: req.Images}, subject.UserID, true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, msg)
}
func (h *TicketHandler) Close(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "Admin not authenticated")
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ticket ID")
		return
	}
	if err := h.tickets.Close(c.Request.Context(), id, subject.UserID, true); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"status": service.TicketStatusClosed})
}
