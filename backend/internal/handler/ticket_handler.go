package handler

import (
	"errors"
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

type createTicketRequest struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Images      []string `json:"images"`
}
type ticketMessageRequest struct {
	Content string   `json:"content"`
	Images  []string `json:"images"`
}

func ticketID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid ticket ID")
		return 0, false
	}
	return id, true
}
func ticketError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrTicketSystemDisabled):
		response.Forbidden(c, "Ticket system is disabled")
	case errors.Is(err, service.ErrTicketNotFound):
		response.NotFound(c, "Ticket not found")
	case errors.Is(err, service.ErrTicketClosed):
		response.BadRequest(c, "Ticket is closed")
	default:
		response.ErrorFrom(c, err)
	}
}

func (h *TicketHandler) List(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	items, err := h.tickets.List(c.Request.Context(), subject.UserID, false)
	if err != nil {
		ticketError(c, err)
		return
	}
	response.Success(c, items)
}
func (h *TicketHandler) Create(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req createTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	item, err := h.tickets.Create(c.Request.Context(), service.CreateTicketInput{UserID: subject.UserID, Title: req.Title, Description: req.Description, Images: req.Images})
	if err != nil {
		ticketError(c, err)
		return
	}
	response.Success(c, item)
}
func (h *TicketHandler) Get(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, ok := ticketID(c)
	if !ok {
		return
	}
	item, err := h.tickets.Get(c.Request.Context(), id, subject.UserID, false)
	if err != nil {
		ticketError(c, err)
		return
	}
	response.Success(c, item)
}
func (h *TicketHandler) Message(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, ok := ticketID(c)
	if !ok {
		return
	}
	var req ticketMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	msg, err := h.tickets.AddMessage(c.Request.Context(), service.AddTicketMessageInput{TicketID: id, SenderID: subject.UserID, SenderType: service.TicketSenderUser, Content: req.Content, Images: req.Images}, subject.UserID, false)
	if err != nil {
		ticketError(c, err)
		return
	}
	response.Success(c, msg)
}
func (h *TicketHandler) Close(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	id, ok := ticketID(c)
	if !ok {
		return
	}
	if err := h.tickets.Close(c.Request.Context(), id, subject.UserID, false); err != nil {
		ticketError(c, err)
		return
	}
	response.Success(c, gin.H{"status": service.TicketStatusClosed})
}
