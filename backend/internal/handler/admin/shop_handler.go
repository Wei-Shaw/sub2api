package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// ShopHandler handles administrator product and inventory management.
type ShopHandler struct{ service *service.ShopService }

func NewShopHandler(shopService *service.ShopService) *ShopHandler {
	return &ShopHandler{service: shopService}
}

func (h *ShopHandler) ListProducts(c *gin.Context) {
	products, err := h.service.ListProducts(c.Request.Context(), true)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, products)
}

func (h *ShopHandler) GetOverview(c *gin.Context) {
	overview, err := h.service.GetOverview(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, overview)
}

func (h *ShopHandler) ListInventory(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid product ID")
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListInventory(c.Request.Context(), id, strings.TrimSpace(c.Query("status")), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *ShopHandler) ListOrders(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid product ID")
		return
	}
	page, pageSize := response.ParsePagination(c)
	items, total, err := h.service.ListAdminOrders(c.Request.Context(), id, c.Query("search"), page, pageSize)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, total, page, pageSize)
}

func (h *ShopHandler) CreateProduct(c *gin.Context) {
	var req struct {
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		Price        float64 `json:"price"`
		LimitPerUser *int    `json:"limit_per_user"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	p, err := h.service.CreateProduct(c.Request.Context(), req.Name, req.Description, req.Price, req.LimitPerUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, p)
}

func (h *ShopHandler) UpdateProduct(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid product ID")
		return
	}
	var req struct {
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		Price        float64 `json:"price"`
		Status       string  `json:"status"`
		LimitPerUser *int    `json:"limit_per_user"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	p, err := h.service.UpdateProduct(c.Request.Context(), id, req.Name, req.Description, req.Price, req.Status, req.LimitPerUser)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, p)
}

func (h *ShopHandler) DeleteProduct(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid product ID")
		return
	}
	if err := h.service.DeleteProduct(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *ShopHandler) UpdateInventoryStatus(c *gin.Context) {
	productID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid product ID")
		return
	}
	inventoryID, err := strconv.ParseInt(c.Param("inventoryID"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid inventory ID")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if err := h.service.UpdateInventoryStatus(c.Request.Context(), productID, inventoryID, req.Status); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"updated": true})
}

func (h *ShopHandler) DeleteInventory(c *gin.Context) {
	productID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid product ID")
		return
	}
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	deleted, err := h.service.DeleteInventory(c.Request.Context(), productID, req.IDs)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"deleted": deleted})
}

func (h *ShopHandler) AddCodes(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid product ID")
		return
	}
	var req struct {
		Codes string `json:"codes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	lines := strings.FieldsFunc(req.Codes, func(r rune) bool { return r == '\n' || r == '\r' })
	added, err := h.service.AddCodes(c.Request.Context(), id, lines)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, gin.H{"added": added})
}
