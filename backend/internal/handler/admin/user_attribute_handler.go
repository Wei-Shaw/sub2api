package admin

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// UserAttributeHandler handles user attribute management
type UserAttributeHandler struct {
	attrService *service.UserAttributeService
}

// NewUserAttributeHandler creates a new handler
func NewUserAttributeHandler(attrService *service.UserAttributeService) *UserAttributeHandler {
	return &UserAttributeHandler{attrService: attrService}
}

// --- Request/Response DTOs ---

// CreateAttributeDefinitionRequest represents create attribute definition request
type CreateAttributeDefinitionRequest struct {
	Key         string                          `json:"key" binding:"required,min=1,max=100"`
	Name        string                          `json:"name" binding:"required,min=1,max=255"`
	Description string                          `json:"description"`
	Type        string                          `json:"type" binding:"required"`
	Options     []service.UserAttributeOption   `json:"options"`
	Required    bool                            `json:"required"`
	Validation  service.UserAttributeValidation `json:"validation"`
	Placeholder string                          `json:"placeholder"`
	Enabled     bool                            `json:"enabled"`
}

// UpdateAttributeDefinitionRequest represents update attribute definition request
type UpdateAttributeDefinitionRequest struct {
	Name        *string                          `json:"name"`
	Description *string                          `json:"description"`
	Type        *string                          `json:"type"`
	Options     *[]service.UserAttributeOption   `json:"options"`
	Required    *bool                            `json:"required"`
	Validation  *service.UserAttributeValidation `json:"validation"`
	Placeholder *string                          `json:"placeholder"`
	Enabled     *bool                            `json:"enabled"`
}

// ReorderRequest represents reorder attribute definitions request
type ReorderRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}

// UpdateUserAttributesRequest represents update user attributes request
type UpdateUserAttributesRequest struct {
	Values map[int64]string `json:"values" binding:"required"`
}

// BatchGetUserAttributesRequest represents batch get user attributes request
type BatchGetUserAttributesRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required"`
}

// BatchUserAttributesResponse represents batch user attributes response
type BatchUserAttributesResponse struct {
	// Map of userID -> map of attributeID -> value
	Attributes map[int64]map[int64]string `json:"attributes"`
}

var userAttributesBatchCache = newSnapshotCache(30 * time.Second)

// AttributeDefinitionResponse represents attribute definition response
type AttributeDefinitionResponse struct {
	ID           int64                           `json:"id"`
	Key          string                          `json:"key"`
	Name         string                          `json:"name"`
	Description  string                          `json:"description"`
	Type         string                          `json:"type"`
	Options      []service.UserAttributeOption   `json:"options"`
	Required     bool                            `json:"required"`
	Validation   service.UserAttributeValidation `json:"validation"`
	Placeholder  string                          `json:"placeholder"`
	DisplayOrder int                             `json:"display_order"`
	Enabled      bool                            `json:"enabled"`
	CreatedAt    string                          `json:"created_at"`
	UpdatedAt    string                          `json:"updated_at"`
}

// AttributeValueResponse represents attribute value response
type AttributeValueResponse struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	AttributeID int64  `json:"attribute_id"`
	Value       string `json:"value"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// --- Helpers ---

func defToResponse(def *service.UserAttributeDefinition) *AttributeDefinitionResponse {
	return &AttributeDefinitionResponse{
		ID:           def.ID,
		Key:          def.Key,
		Name:         def.Name,
		Description:  def.Description,
		Type:         string(def.Type),
		Options:      def.Options,
		Required:     def.Required,
		Validation:   def.Validation,
		Placeholder:  def.Placeholder,
		DisplayOrder: def.DisplayOrder,
		Enabled:      def.Enabled,
		CreatedAt:    def.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    def.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func valueToResponse(val *service.UserAttributeValue) *AttributeValueResponse {
	return &AttributeValueResponse{
		ID:          val.ID,
		UserID:      val.UserID,
		AttributeID: val.AttributeID,
		Value:       val.Value,
		CreatedAt:   val.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   val.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// --- Handlers ---

// ListDefinitions lists all attribute definitions
// GET /admin/user-attributes
func (h *UserAttributeHandler) ListDefinitions(c *gin.Context) {
	enabledOnly := c.Query("enabled") == "true"

	defs, err := h.attrService.ListDefinitions(c.Request.Context(), enabledOnly)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]*AttributeDefinitionResponse, 0, len(defs))
	for i := range defs {
		out = append(out, defToResponse(&defs[i]))
	}

	response.Success(c, out)
}

// CreateDefinition creates a new attribute definition
// POST /admin/user-attributes
func (h *UserAttributeHandler) CreateDefinition(c *gin.Context) {
	var req CreateAttributeDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	def, err := h.attrService.CreateDefinition(c.Request.Context(), service.CreateAttributeDefinitionInput{
		Key:         req.Key,
		Name:        req.Name,
		Description: req.Description,
		Type:        service.UserAttributeType(req.Type),
		Options:     req.Options,
		Required:    req.Required,
		Validation:  req.Validation,
		Placeholder: req.Placeholder,
		Enabled:     req.Enabled,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, defToResponse(def))
}

// UpdateDefinition updates an attribute definition
// PUT /admin/user-attributes/:id
func (h *UserAttributeHandler) UpdateDefinition(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid attribute ID")
		return
	}

	var req UpdateAttributeDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	input := service.UpdateAttributeDefinitionInput{
		Name:        req.Name,
		Description: req.Description,
		Options:     req.Options,
		Required:    req.Required,
		Validation:  req.Validation,
		Placeholder: req.Placeholder,
		Enabled:     req.Enabled,
	}
	if req.Type != nil {
		t := service.UserAttributeType(*req.Type)
		input.Type = &t
	}

	def, err := h.attrService.UpdateDefinition(c.Request.Context(), id, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, defToResponse(def))
}

// DeleteDefinition deletes an attribute definition
// DELETE /admin/user-attributes/:id
func (h *UserAttributeHandler) DeleteDefinition(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid attribute ID")
		return
	}

	if err := h.attrService.DeleteDefinition(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Attribute definition deleted successfully"})
}

// ReorderDefinitions reorders attribute definitions
// PUT /admin/user-attributes/reorder
func (h *UserAttributeHandler) ReorderDefinitions(c *gin.Context) {
	var req ReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Convert IDs array to orders map (position in array = display_order)
	orders := make(map[int64]int, len(req.IDs))
	for i, id := range req.IDs {
		orders[id] = i
	}

	if err := h.attrService.ReorderDefinitions(c.Request.Context(), orders); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Reorder successful"})
}

// GetUserAttributes gets a user's attribute values
// GET /admin/users/:id/attributes
func (h *UserAttributeHandler) GetUserAttributes(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	values, err := h.attrService.GetUserAttributes(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]*AttributeValueResponse, 0, len(values))
	for i := range values {
		out = append(out, valueToResponse(&values[i]))
	}

	response.Success(c, out)
}

// UpdateUserAttributes updates a user's attribute values
// PUT /admin/users/:id/attributes
func (h *UserAttributeHandler) UpdateUserAttributes(c *gin.Context) {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid user ID")
		return
	}

	var req UpdateUserAttributesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	inputs := make([]service.UpdateUserAttributeInput, 0, len(req.Values))
	for attrID, value := range req.Values {
		inputs = append(inputs, service.UpdateUserAttributeInput{
			AttributeID: attrID,
			Value:       value,
		})
	}

	if err := h.attrService.UpdateUserAttributes(c.Request.Context(), userID, inputs); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	// Return updated values
	values, err := h.attrService.GetUserAttributes(c.Request.Context(), userID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]*AttributeValueResponse, 0, len(values))
	for i := range values {
		out = append(out, valueToResponse(&values[i]))
	}

	response.Success(c, out)
}

// GetBatchUserAttributes gets attribute values for multiple users
// POST /admin/user-attributes/batch
func (h *UserAttributeHandler) GetBatchUserAttributes(c *gin.Context) {
	var req BatchGetUserAttributesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	userIDs := normalizeInt64IDList(req.UserIDs)
	if len(userIDs) == 0 {
		response.Success(c, BatchUserAttributesResponse{Attributes: map[int64]map[int64]string{}})
		return
	}

	keyRaw, _ := json.Marshal(struct {
		UserIDs []int64 `json:"user_ids"`
	}{
		UserIDs: userIDs,
	})
	cacheKey := string(keyRaw)
	if cached, ok := userAttributesBatchCache.Get(cacheKey); ok {
		c.Header("X-Snapshot-Cache", "hit")
		response.Success(c, cached.Payload)
		return
	}

	attrs, err := h.attrService.GetBatchUserAttributes(c.Request.Context(), userIDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	payload := BatchUserAttributesResponse{Attributes: attrs}
	userAttributesBatchCache.Set(cacheKey, payload)
	c.Header("X-Snapshot-Cache", "miss")
	response.Success(c, payload)
}
