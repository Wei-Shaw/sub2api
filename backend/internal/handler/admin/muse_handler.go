package admin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/muse"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type MuseHandler struct{ core *service.MuseCoreService }

func NewMuseHandler(core *service.MuseCoreService) *MuseHandler { return &MuseHandler{core: core} }

func (h *MuseHandler) Status(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid account ID")
		return
	}
	data, err := h.core.Status(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Muse account state unavailable")
		return
	}
	response.Success(c, data)
}
func (h *MuseHandler) Verify(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid account ID")
		return
	}
	_, err = h.core.Verify(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Muse protocol verification is not available or the session needs reconnecting")
		return
	}
	data, err := h.core.Status(c.Request.Context(), id)
	if err != nil {
		response.Error(c, http.StatusConflict, "account changed during verification")
		return
	}
	response.Success(c, data)
}
func (h *MuseHandler) Renew(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "invalid account ID")
		return
	}
	if err = h.core.RenewSession(c.Request.Context(), id); err != nil {
		response.Error(c, http.StatusServiceUnavailable, "Muse session renewal could not be verified")
		return
	}
	response.Success(c, gin.H{"renewed": true})
}
func (h *MuseHandler) Turn(c *gin.Context) {
	data, err := h.core.AdminTurn(c.Request.Context(), c.Param("turn"))
	if err != nil {
		response.Error(c, http.StatusNotFound, "turn not found")
		return
	}
	response.Success(c, data)
}
func (h *MuseHandler) Resolve(c *gin.Context) {
	var body struct {
		Outcome muse.State `json:"outcome"`
		Confirm bool       `json:"confirm_remote_terminal"`
	}
	if c.ShouldBindJSON(&body) != nil || !body.Confirm {
		response.Error(c, http.StatusBadRequest, "confirm the remote task is terminal before releasing its workspace")
		return
	}
	if err := h.core.Resolve(c.Request.Context(), c.Param("turn"), body.Outcome); err != nil {
		response.Error(c, http.StatusConflict, "turn cannot be resolved from its current state")
		return
	}
	response.Success(c, gin.H{"resolved": true})
}

func (h *MuseHandler) Settle(c *gin.Context) {
	if err := h.core.RetrySettlement(c.Request.Context(), c.Param("turn")); err != nil {
		response.Error(c, http.StatusConflict, "local settlement is pending; no remote task was replayed")
		return
	}
	response.Success(c, gin.H{"settled": true})
}

func (h *MuseHandler) Authenticate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Error(c, http.StatusBadRequest, "invalid account ID")
		return
	}
	check, err := h.core.CheckSession(c.Request.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, muse.ErrSessionCredentials):
			response.Error(c, http.StatusBadRequest, "import the four Muse app HttpOnly cookies")
		case errors.Is(err, muse.ErrBusy):
			response.Error(c, http.StatusConflict, "finish or resolve workspace work before renewing its app cookies")
		case errors.Is(err, muse.ErrSessionExpired):
			response.Error(c, http.StatusBadRequest, "Muse app session expired; reconnect in Muse and import fresh cookies")
		default:
			response.Error(c, http.StatusServiceUnavailable, "Muse app session check could not be verified")
		}
		return
	}
	response.Success(c, check)
}
