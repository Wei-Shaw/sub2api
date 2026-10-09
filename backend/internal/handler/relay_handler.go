package handler

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type RelayHandler struct {
	relay *service.RelayService
	keys  *service.APIKeyService
}

func NewRelayHandler(relay *service.RelayService, keys *service.APIKeyService) *RelayHandler {
	return &RelayHandler{relay: relay, keys: keys}
}

func relayUser(c *gin.Context) (int64, bool) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return 0, false
	}
	return subject.UserID, true
}

func relayID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid resource ID")
		return 0, false
	}
	return id, true
}

func relayPage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	if size < 1 || size > 100 {
		size = 20
	}
	return page, size
}

func (h *RelayHandler) Groups(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	groups, err := h.relay.Groups(c.Request.Context(), userID)
	if response.ErrorFrom(c, err) {
		return
	}
	private := make([]*dto.Group, 0, len(groups))
	for i := range groups {
		private = append(private, dto.GroupFromService(&groups[i]))
	}
	available, err := h.keys.GetAvailableGroups(c.Request.Context(), userID)
	if response.ErrorFrom(c, err) {
		return
	}
	public := []*dto.Group{}
	for i := range available {
		if available[i].IsPublicRelay() {
			public = append(public, dto.GroupFromService(&available[i]))
		}
	}
	response.Success(c, gin.H{"private_groups": private, "public_groups": public})
}

type relayGroupRequest struct {
	Name        string `json:"name" binding:"required,max=70"`
	Platform    string `json:"platform"`
	Description string `json:"description" binding:"max=1000"`
	Status      string `json:"status"`
}

func (h *RelayHandler) CreateGroup(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	var req relayGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid group settings")
		return
	}
	if req.Platform == "" {
		req.Platform = service.PlatformComposite
	}
	g, err := h.relay.CreateGroup(c.Request.Context(), userID, req.Name, req.Platform)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Created(c, dto.GroupFromService(g))
}

func (h *RelayHandler) UpdateGroup(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	id, ok := relayID(c)
	if !ok {
		return
	}
	var req relayGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid group settings")
		return
	}
	if req.Status == "" {
		req.Status = service.StatusActive
	}
	g, err := h.relay.UpdateGroup(c.Request.Context(), userID, id, req.Name, req.Description, req.Status)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, dto.GroupFromService(g))
}

func (h *RelayHandler) DeleteGroup(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	id, ok := relayID(c)
	if !ok {
		return
	}
	if response.ErrorFrom(c, h.relay.DeleteGroup(c.Request.Context(), userID, id)) {
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *RelayHandler) Accounts(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	page, size := relayPage(c)
	accounts, total, err := h.relay.Accounts(c.Request.Context(), userID, page, size)
	if response.ErrorFrom(c, err) {
		return
	}
	items := make([]*dto.Account, 0, len(accounts))
	for _, a := range accounts {
		items = append(items, dto.AccountFromService(a))
	}
	response.Success(c, gin.H{"items": items, "total": total, "page": page, "page_size": size})
}

type relayAccountRequest struct {
	Name        string         `json:"name" binding:"required,max=100"`
	Platform    string         `json:"platform"`
	Type        string         `json:"type"`
	Credentials map[string]any `json:"credentials"`
	Concurrency int            `json:"concurrency" binding:"gte=1,lte=100"`
	Status      string         `json:"status"`
	GroupIDs    []int64        `json:"group_ids" binding:"required,min=1,max=50"`
}

func (h *RelayHandler) CreateAccount(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	var req relayAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid account settings")
		return
	}
	a, err := h.relay.CreateAccount(c.Request.Context(), userID, &service.CreateAccountInput{Name: req.Name, Platform: req.Platform,
		Type: req.Type, Credentials: req.Credentials, Status: req.Status, Concurrency: req.Concurrency, GroupIDs: req.GroupIDs})
	if response.ErrorFrom(c, err) {
		return
	}
	response.Created(c, dto.AccountFromService(a))
}

func (h *RelayHandler) UpdateAccount(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	id, ok := relayID(c)
	if !ok {
		return
	}
	var req relayAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid account settings")
		return
	}
	a, err := h.relay.UpdateAccount(c.Request.Context(), userID, id, &service.UpdateAccountInput{Name: req.Name,
		Credentials: req.Credentials, Concurrency: &req.Concurrency, Status: req.Status, GroupIDs: &req.GroupIDs})
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, dto.AccountFromService(a))
}

func (h *RelayHandler) DeleteAccount(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	id, ok := relayID(c)
	if !ok {
		return
	}
	if response.ErrorFrom(c, h.relay.DeleteAccount(c.Request.Context(), userID, id)) {
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *RelayHandler) Earnings(c *gin.Context) {
	userID, ok := relayUser(c)
	if !ok {
		return
	}
	page, size := relayPage(c)
	out, err := h.relay.Earnings(c.Request.Context(), userID, page, size)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, out)
}
