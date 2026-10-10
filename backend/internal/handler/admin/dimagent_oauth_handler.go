package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// DimAgentOAuthHandler owns the administrator-only authorization-code handoff.
// DimAgent's registered OAuth redirect is localhost, so this handler accepts the
// complete callback URL pasted by the authorizing administrator; it never asks
// a person to view or paste an access/refresh token.
type DimAgentOAuthHandler struct {
	oauth *service.DimAgentOAuthService
	admin service.AdminService
}

func NewDimAgentOAuthHandler(oauth *service.DimAgentOAuthService, admin service.AdminService) *DimAgentOAuthHandler {
	return &DimAgentOAuthHandler{oauth: oauth, admin: admin}
}

type dimAgentGenerateAuthURLRequest struct {
	ProxyID *int64 `json:"proxy_id"`
}

func (h *DimAgentOAuthHandler) GenerateAuthURL(c *gin.Context) {
	if h == nil || h.oauth == nil {
		response.ErrorFrom(c, infraerrors.New(http.StatusServiceUnavailable, "DIMAGENT_OAUTH_UNAVAILABLE", "DimAgent OAuth service is unavailable"))
		return
	}
	var req dimAgentGenerateAuthURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = dimAgentGenerateAuthURLRequest{}
	}
	result, err := h.oauth.GenerateAuthURL(c.Request.Context(), req.ProxyID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

type dimAgentCreateFromCallbackRequest struct {
	SessionID        string         `json:"session_id" binding:"required"`
	CallbackURL      string         `json:"callback_url" binding:"required"`
	ProxyID          *int64         `json:"proxy_id"`
	Name             string         `json:"name"`
	Concurrency      int            `json:"concurrency"`
	Priority         int            `json:"priority"`
	GroupIDs         []int64        `json:"group_ids"`
	CredentialExtras map[string]any `json:"credential_extras"`
}

func (h *DimAgentOAuthHandler) CreateAccountFromCallback(c *gin.Context) {
	if h == nil || h.oauth == nil {
		response.ErrorFrom(c, infraerrors.New(http.StatusServiceUnavailable, "DIMAGENT_OAUTH_UNAVAILABLE", "DimAgent OAuth service is unavailable"))
		return
	}
	var req dimAgentCreateFromCallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	info, err := h.oauth.ExchangeCallbackURL(c.Request.Context(), &service.DimAgentExchangeCodeInput{
		SessionID:   req.SessionID,
		CallbackURL: req.CallbackURL,
		ProxyID:     req.ProxyID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "DimAgent OAuth Account"
	}
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	priority := req.Priority
	if priority < 0 {
		priority = 0
	}
	credentials := h.oauth.BuildAccountCredentials(info)
	if mapping := dimAgentModelMappingCredential(req.CredentialExtras); mapping != nil {
		credentials["model_mapping"] = mapping
	}
	account, err := h.admin.CreateAccount(c.Request.Context(), &service.CreateAccountInput{
		Name:        name,
		Platform:    service.PlatformDimAgent,
		Type:        service.AccountTypeOAuth,
		Credentials: credentials,
		ProxyID:     req.ProxyID,
		Concurrency: concurrency,
		Priority:    priority,
		GroupIDs:    req.GroupIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(account))
}

// dimAgentModelMappingCredential is deliberately a narrow allowlist. OAuth
// credential material is built exclusively by the server from the exchange
// response; the client may only carry an optional public-model mapping chosen
// in the account form.
func dimAgentModelMappingCredential(extras map[string]any) map[string]any {
	if extras == nil {
		return nil
	}
	mapping, ok := extras["model_mapping"].(map[string]any)
	if !ok || len(mapping) == 0 {
		return nil
	}
	out := make(map[string]any, len(mapping))
	for key, value := range mapping {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if target, ok := value.(string); ok && strings.TrimSpace(target) != "" {
			out[key] = strings.TrimSpace(target)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *DimAgentOAuthHandler) RefreshAccountToken(c *gin.Context) {
	if h == nil || h.oauth == nil {
		response.ErrorFrom(c, infraerrors.New(http.StatusServiceUnavailable, "DIMAGENT_OAUTH_UNAVAILABLE", "DimAgent OAuth service is unavailable"))
		return
	}
	accountID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || accountID <= 0 {
		response.BadRequest(c, "Invalid account ID")
		return
	}
	account, err := h.admin.GetAccount(c.Request.Context(), accountID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	info, err := h.oauth.RefreshAccountToken(c.Request.Context(), account)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	credentials := service.MergeCredentials(account.Credentials, h.oauth.BuildAccountCredentials(info))
	updated, err := h.admin.UpdateAccount(c.Request.Context(), account.ID, &service.UpdateAccountInput{Credentials: credentials})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.AccountFromService(updated))
}
