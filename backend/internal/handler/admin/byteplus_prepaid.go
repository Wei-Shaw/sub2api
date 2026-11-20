package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/byteplusbilling"
	"github.com/Wei-Shaw/sub2api/internal/pkg/packguard"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) SetBytePlusPrepaidService(s service.BytePlusPrepaidController) {
	h.bytePlusPrepaid = s
}

func (h *AccountHandler) prepaidAccess(c *gin.Context) (int64, bool) {
	role, ok := middleware.GetUserRoleFromContext(c)
	// A shared pool can affect accounts outside a delegated key's resource rule.
	if !ok || role != "admin" || adminaccess.RestrictedFromContext(c.Request.Context()) != nil {
		response.Error(c, http.StatusForbidden, "Full administrator access is required for shared funding pools")
		return 0, false
	}
	if h == nil || h.bytePlusPrepaid == nil {
		response.Error(c, http.StatusServiceUnavailable, "Prepaid funding service is unavailable")
		return 0, false
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return 0, false
	}
	return id, true
}

func prepaidError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, packguard.ErrInvalid):
		response.BadRequest(c, "Invalid funding input: use a qualified model, exact decimal token balance, verified expiry and the required acknowledgement")
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, service.ErrAccountNotFound):
		response.NotFound(c, "Funding resource not found")
	case errors.Is(err, packguard.ErrRegion):
		response.BadRequest(c, err.Error())
	case errors.Is(err, packguard.ErrConflict), errors.Is(err, packguard.ErrPauseRequired), errors.Is(err, packguard.ErrPendingJobs), errors.Is(err, packguard.ErrRecheck), errors.Is(err, packguard.ErrSafetyLock), errors.Is(err, packguard.ErrScope):
		response.Error(c, http.StatusConflict, err.Error())
	case errors.Is(err, packguard.ErrExhausted):
		response.Error(c, http.StatusPaymentRequired, err.Error())
	case errors.Is(err, service.ErrBytePlusProviderStatus):
		response.Error(c, http.StatusBadGateway, err.Error())
	default:
		response.Error(c, http.StatusServiceUnavailable, "Funding state could not be verified; no funding was released")
	}
}

func (h *AccountHandler) BytePlusPrepaidStatus(c *gin.Context) {
	id, ok := h.prepaidAccess(c)
	if !ok {
		return
	}
	st, err := h.bytePlusPrepaid.Status(c.Request.Context(), id)
	if err != nil {
		prepaidError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, st)
}
func (h *AccountHandler) BytePlusPrepaidPools(c *gin.Context) {
	if _, ok := h.prepaidAccess(c); !ok {
		return
	}
	pools, err := h.bytePlusPrepaid.Pools(c.Request.Context())
	if err != nil {
		prepaidError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, pools)
}
func (h *AccountHandler) BytePlusPrepaidUpdate(c *gin.Context) {
	id, ok := h.prepaidAccess(c)
	if !ok {
		return
	}
	var in service.BytePlusPrepaidInput
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		response.BadRequest(c, "Invalid funding request")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		response.BadRequest(c, "Invalid funding request")
		return
	}
	st, err := h.bytePlusPrepaid.Update(c.Request.Context(), id, c.Param("action"), in)
	if err != nil {
		prepaidError(c, err)
		return
	}
	response.Success(c, st)
}
func (h *AccountHandler) BytePlusPrepaidReconcile(c *gin.Context) {
	id, ok := h.prepaidAccess(c)
	if !ok {
		return
	}
	st, err := h.bytePlusPrepaid.Reconcile(c.Request.Context(), id, c.Param("reservation_id"))
	if err != nil {
		prepaidError(c, err)
		return
	}
	response.Success(c, st)
}

func (h *AccountHandler) BytePlusProviderStatus(c *gin.Context) { h.bytePlusProvider(c, false) }
func (h *AccountHandler) BytePlusProviderQuery(c *gin.Context)  { h.bytePlusProvider(c, true) }
func (h *AccountHandler) bytePlusProvider(c *gin.Context, query bool) {
	id, ok := h.prepaidAccess(c)
	if !ok {
		return
	}
	provider, ok := h.bytePlusPrepaid.(interface {
		ProviderBilling(context.Context, int64, bool) (*byteplusbilling.Status, error)
	})
	if !ok {
		response.Error(c, http.StatusServiceUnavailable, "Billing query service is unavailable")
		return
	}
	st, err := provider.ProviderBilling(c.Request.Context(), id, query)
	if err != nil {
		prepaidError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	response.Success(c, st)
}
