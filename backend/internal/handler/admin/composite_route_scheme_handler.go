package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type CompositeRouteSchemeRequest struct {
	Name             string `json:"name" binding:"required"`
	Description      string `json:"description"`
	CopyFromSchemeID int64  `json:"copy_from_scheme_id"`
}

type DuplicateCompositeRouteSchemeRequest struct {
	Name string `json:"name"`
}

// ListCompositeRouteSchemes GET /api/v1/admin/composite-route-schemes
func (h *GroupHandler) ListCompositeRouteSchemes(c *gin.Context) {
	schemes, err := h.adminService.ListCompositeRouteSchemes(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, schemes)
}

// GetCompositeRouteScheme GET /api/v1/admin/composite-route-schemes/:id
func (h *GroupHandler) GetCompositeRouteScheme(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	scheme, err := h.adminService.GetCompositeRouteScheme(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, scheme)
}

// CreateCompositeRouteScheme POST /api/v1/admin/composite-route-schemes
func (h *GroupHandler) CreateCompositeRouteScheme(c *gin.Context) {
	var req CompositeRouteSchemeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	scheme, err := h.adminService.CreateCompositeRouteScheme(c.Request.Context(), service.CompositeRouteSchemeInput{
		Name:             req.Name,
		Description:      req.Description,
		CopyFromSchemeID: req.CopyFromSchemeID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, scheme)
}

// UpdateCompositeRouteScheme PUT /api/v1/admin/composite-route-schemes/:id
func (h *GroupHandler) UpdateCompositeRouteScheme(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	var req CompositeRouteSchemeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	scheme, err := h.adminService.UpdateCompositeRouteScheme(c.Request.Context(), id, service.CompositeRouteSchemeInput{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, scheme)
}

// DeleteCompositeRouteScheme DELETE /api/v1/admin/composite-route-schemes/:id
func (h *GroupHandler) DeleteCompositeRouteScheme(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.adminService.DeleteCompositeRouteScheme(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Composite route scheme deleted"})
}

// DuplicateCompositeRouteScheme POST /api/v1/admin/composite-route-schemes/:id/duplicate
func (h *GroupHandler) DuplicateCompositeRouteScheme(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	var req DuplicateCompositeRouteSchemeRequest
	_ = c.ShouldBindJSON(&req)
	scheme, err := h.adminService.DuplicateCompositeRouteScheme(c.Request.Context(), id, strings.TrimSpace(req.Name))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, scheme)
}

// ListCompositeRouteSchemeRoutes GET /api/v1/admin/composite-route-schemes/:id/routes
func (h *GroupHandler) ListCompositeRouteSchemeRoutes(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	routes, err := h.adminService.ListCompositeRouteSchemeRoutes(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, routes)
}

// CreateCompositeRouteSchemeRoute POST /api/v1/admin/composite-route-schemes/:id/routes
func (h *GroupHandler) CreateCompositeRouteSchemeRoute(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	var req CompositeRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	route, err := h.adminService.CreateCompositeRouteSchemeRoute(c.Request.Context(), id, compositeRouteRequestToInput(req, true))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, route)
}

// UpdateCompositeRouteSchemeRoute PUT /api/v1/admin/composite-route-schemes/:id/routes/:route_id
func (h *GroupHandler) UpdateCompositeRouteSchemeRoute(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	routeID, ok := parsePositiveIDParam(c, "route_id")
	if !ok {
		return
	}
	var req CompositeRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	route, err := h.adminService.UpdateCompositeRouteSchemeRoute(c.Request.Context(), id, routeID, compositeRouteRequestToInput(req, true))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, route)
}

// DeleteCompositeRouteSchemeRoute DELETE /api/v1/admin/composite-route-schemes/:id/routes/:route_id
func (h *GroupHandler) DeleteCompositeRouteSchemeRoute(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	routeID, ok := parsePositiveIDParam(c, "route_id")
	if !ok {
		return
	}
	if err := h.adminService.DeleteCompositeRouteSchemeRoute(c.Request.Context(), id, routeID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "Composite route deleted"})
}

// PreviewCompositeRouteScheme POST /api/v1/admin/composite-route-schemes/:id/preview
func (h *GroupHandler) PreviewCompositeRouteScheme(c *gin.Context) {
	id, ok := parsePositiveIDParam(c, "id")
	if !ok {
		return
	}
	var req CompositeRoutePreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: "+err.Error())
		return
	}
	decision, err := h.adminService.PreviewCompositeRouteScheme(c.Request.Context(), id, service.CompositeRoutePreviewRequest{
		Model:    req.Model,
		Endpoint: req.Endpoint,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, decision)
}
