package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const openAIAutoProvisionCallbackMaxBody = 32 << 10

// OpenAIAutoProvisionHandler receives terminal events and OAuth callbacks from
// the separately deployed turb-gpt-free-register worker.
type OpenAIAutoProvisionHandler struct {
	service *service.OpenAIAutoProvisionService
}

func NewOpenAIAutoProvisionHandler(svc *service.OpenAIAutoProvisionService) *OpenAIAutoProvisionHandler {
	return &OpenAIAutoProvisionHandler{service: svc}
}

// RuntimeStatus returns the credential-free coordinator status for the admin UI.
// The route is protected by the admin middleware, unlike machine callbacks below.
func (h *OpenAIAutoProvisionHandler) RuntimeStatus(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "automation service unavailable")
		return
	}
	status, err := h.service.GetRuntimeStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

// ResetRuntimeStatus retires a stale provisioning request for the admin UI.
func (h *OpenAIAutoProvisionHandler) ResetRuntimeStatus(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, "automation service unavailable")
		return
	}
	if err := h.service.ResetProvisioningStatus(c.Request.Context()); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	status, err := h.service.GetRuntimeStatus(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, status)
}

func (h *OpenAIAutoProvisionHandler) Callback(c *gin.Context) {
	if !h.authenticate(c) {
		return
	}
	var callback service.OpenAIAutoProvisionCallback
	if err := decodeAutomationJSON(c, &callback); err != nil {
		response.BadRequest(c, "invalid automation callback payload")
		return
	}
	replay, err := h.service.HandleProvisionCallback(c.Request.Context(), callback)
	if err != nil {
		slog.Error("openai auto-provision callback failed", "error_type", fmt.Sprintf("%T", err))
		response.BadRequest(c, "automation callback was rejected")
		return
	}
	response.Success(c, gin.H{"accepted": true, "replay": replay})
}

func (h *OpenAIAutoProvisionHandler) ReauthorizationCallback(c *gin.Context) {
	if !h.authenticate(c) {
		return
	}
	var callback service.OpenAIAutoReauthorizationCallback
	if err := decodeAutomationJSON(c, &callback); err != nil {
		response.BadRequest(c, "invalid reauthorization callback payload")
		return
	}
	replay, err := h.service.HandleReauthorizationCallback(c.Request.Context(), callback)
	if err != nil {
		slog.Error("openai auto-reauthorization callback failed", "error_type", fmt.Sprintf("%T", err))
		response.BadRequest(c, "reauthorization callback was rejected")
		return
	}
	response.Success(c, gin.H{"accepted": true, "replay": replay, "account_id": callback.AccountID})
}

func (h *OpenAIAutoProvisionHandler) ReauthorizationCompletion(c *gin.Context) {
	if !h.authenticate(c) {
		return
	}
	var callback service.OpenAIAutoProvisionCallback
	if err := decodeAutomationJSON(c, &callback); err != nil {
		response.BadRequest(c, "invalid reauthorization completion payload")
		return
	}
	replay, err := h.service.HandleReauthorizationCompletion(c.Request.Context(), callback)
	if err != nil {
		slog.Error("openai auto-reauthorization completion failed", "error_type", fmt.Sprintf("%T", err))
		response.BadRequest(c, "reauthorization completion was rejected")
		return
	}
	response.Success(c, gin.H{"accepted": true, "replay": replay})
}

func (h *OpenAIAutoProvisionHandler) authenticate(c *gin.Context) bool {
	if h == nil || h.service == nil {
		response.Error(c, http.StatusInternalServerError, "automation service unavailable")
		return false
	}
	secret := strings.TrimSpace(c.GetHeader("X-Sub2API-Automation-Secret"))
	if err := h.service.ValidateCallbackSecret(c.Request.Context(), secret); err != nil {
		response.Unauthorized(c, "invalid automation callback secret")
		return false
	}
	return true
}

func decodeAutomationJSON(c *gin.Context, target any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, openAIAutoProvisionCallbackMaxBody)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
