package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (h *AccountHandler) platformClient(platform string) *plugin.AccountPlatformClient {
	if h.platformRegistry == nil {
		return nil
	}
	client, err := plugin.ClientForPlatform(h.platformRegistry, platform)
	if err != nil {
		return nil
	}
	return client
}

func (h *AccountHandler) tryPluginRefreshToken(ctx context.Context, account *service.Account) (*service.Account, bool, error) {
	client := h.platformClient(account.Platform)
	if client == nil {
		return nil, false, nil
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)

	resp, err := client.RefreshToken(
		ctx,
		account.ID, account.Platform, account.Type,
		creds, extra,
	)
	if err != nil {
		return nil, true, fmt.Errorf("plugin refresh token: %w", err)
	}
	if !resp.Success {
		return nil, true, fmt.Errorf("plugin refresh token failed: %s", resp.Error)
	}

	updates := &service.UpdateAccountInput{}
	if len(resp.UpdatedCredentialsJson) > 0 {
		var newCreds map[string]any
		if err := json.Unmarshal(resp.UpdatedCredentialsJson, &newCreds); err == nil {
			updates.Credentials = newCreds
		}
	}
	if len(resp.UpdatedExtraJson) > 0 {
		var newExtra map[string]any
		if err := json.Unmarshal(resp.UpdatedExtraJson, &newExtra); err == nil {
			updates.Extra = newExtra
		}
	}

	updated, err := h.adminService.UpdateAccount(ctx, account.ID, updates)
	if err != nil {
		return nil, true, fmt.Errorf("update account after plugin refresh: %w", err)
	}
	return updated, true, nil
}

func (h *AccountHandler) tryPluginGetModels(c *gin.Context, account *service.Account) bool {
	client := h.platformClient(account.Platform)
	if client == nil {
		return false
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)

	resp, err := client.GetAvailableModels(
		c.Request.Context(),
		account.ID, account.Platform, account.Type, creds, extra,
	)
	if err != nil {
		response.InternalError(c, "plugin get models: "+err.Error())
		return true
	}

	type modelItem struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		Available   bool   `json:"available"`
	}
	models := make([]modelItem, len(resp.Models))
	for i, m := range resp.Models {
		models[i] = modelItem{
			ID:          m.ModelId,
			DisplayName: m.DisplayName,
			Available:   m.Available,
		}
	}
	response.Success(c, models)
	return true
}

func (h *AccountHandler) tryPluginRefreshTier(c *gin.Context, account *service.Account) bool {
	client := h.platformClient(account.Platform)
	if client == nil {
		return false
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)

	resp, err := client.RefreshTier(
		c.Request.Context(),
		account.ID, account.Platform, account.Type,
		creds, extra,
	)
	if err != nil {
		response.InternalError(c, "plugin refresh tier: "+err.Error())
		return true
	}
	if !resp.Success {
		response.Error(c, http.StatusBadRequest, resp.Error)
		return true
	}

	if len(resp.UpdatedExtraJson) > 0 {
		var newExtra map[string]any
		if err := json.Unmarshal(resp.UpdatedExtraJson, &newExtra); err == nil {
			_, _ = h.adminService.UpdateAccount(c.Request.Context(), account.ID, &service.UpdateAccountInput{
				Extra: newExtra,
			})
		}
	}

	updated, _ := h.adminService.GetAccount(c.Request.Context(), account.ID)
	if updated != nil {
		response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), updated))
	} else {
		response.Success(c, gin.H{"success": true})
	}
	return true
}

func (h *AccountHandler) tryPluginTestConnection(c *gin.Context, account *service.Account, modelID string) bool {
	client := h.platformClient(account.Platform)
	if client == nil {
		return false
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)

	// Resolve account proxy URL for the plugin.
	var proxyURL string
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	stream, err := client.TestConnection(
		c.Request.Context(),
		account.ID, account.Platform, account.Type,
		creds, extra, modelID, proxyURL,
	)
	if err != nil {
		sendPluginSSEError(c, "plugin test connection: "+err.Error())
		return true
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)

	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			writePluginSSEEvent(c, gin.H{"type": "error", "error": err.Error()})
			break
		}

		evtType := event.Type
		if evtType == "test_end" {
			evtType = "test_complete"
		}
		evt := gin.H{"type": evtType}
		if event.Text != "" {
			evt["text"] = event.Text
		}
		if event.Model != "" {
			evt["model"] = event.Model
		}
		if event.Success {
			evt["success"] = event.Success
		}
		if event.Error != "" {
			evt["error"] = event.Error
		}
		writePluginSSEEvent(c, evt)

		if event.Type == "test_end" || event.Type == "error" {
			break
		}
	}
	return true
}

func writePluginSSEEvent(c *gin.Context, data any) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", jsonBytes)
	if f, ok := c.Writer.(http.Flusher); ok {
		f.Flush()
	}
}

func sendPluginSSEError(c *gin.Context, msg string) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.WriteHeader(http.StatusOK)
	writePluginSSEEvent(c, gin.H{"type": "error", "error": msg})
	writePluginSSEEvent(c, gin.H{"type": "test_complete", "success": false, "error": msg})
}

// validateAccountDataResult holds the outcome of plugin-side validation.
type validateAccountDataResult struct {
	FieldErrors          map[string]string
	ProcessedCredentials map[string]any
	ProcessedExtra       map[string]any
}

// tryPluginValidateAccountData delegates credential/extra validation to the
// plugin for plugin-owned platforms. Returns (result, handled):
//   - handled=true, result.FieldErrors non-empty -> validation failed
//   - handled=true, result.FieldErrors empty -> validation passed (use ProcessedCredentials/Extra if set)
//   - handled=false -> plugin unavailable or returned Unimplemented/error; skip validation
func (h *AccountHandler) tryPluginValidateAccountData(
	ctx context.Context,
	platform, accountType string,
	credentials, extra map[string]any,
	isUpdate bool,
	accountID int64,
) (*validateAccountDataResult, bool) {
	client := h.platformClient(platform)
	if client == nil {
		return nil, false
	}

	credsJSON, _ := json.Marshal(credentials)
	extraJSON, _ := json.Marshal(extra)

	resp, err := client.ValidateAccountData(
		ctx, platform, accountType,
		credsJSON, extraJSON,
		isUpdate, accountID,
	)
	if err != nil {
		// Unimplemented or any other error -> silently skip validation.
		return nil, false
	}

	result := &validateAccountDataResult{
		FieldErrors: resp.GetFieldErrors(),
	}
	if len(resp.GetProcessedCredentialsJson()) > 0 {
		var processed map[string]any
		if err := json.Unmarshal(resp.GetProcessedCredentialsJson(), &processed); err == nil {
			result.ProcessedCredentials = processed
		}
	}
	if len(resp.GetProcessedExtraJson()) > 0 {
		var processed map[string]any
		if err := json.Unmarshal(resp.GetProcessedExtraJson(), &processed); err == nil {
			result.ProcessedExtra = processed
		}
	}
	return result, true
}

// SetPlatformRegistry sets the platform registry after construction.
// Used by wire_gen.go when pluginManager is created after AccountHandler.
func (h *AccountHandler) SetPlatformRegistry(registry *plugin.PlatformRegistry) {
	h.platformRegistry = registry
}

// tryPluginSetPrivacy delegates privacy setting to the plugin.
// Returns true if the plugin handled the request (success or error).
// Returns false if the plugin is unavailable or returned Unimplemented.
func (h *AccountHandler) tryPluginSetPrivacy(c *gin.Context, account *service.Account, force bool) bool {
	client := h.platformClient(account.Platform)
	if client == nil {
		return false
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)

	resp, err := client.SetPrivacy(
		c.Request.Context(),
		account.ID, account.Platform,
		creds, extra, force,
	)
	if err != nil {
		if isUnimplemented(err) {
			return false
		}
		slog.WarnContext(c.Request.Context(), "plugin set privacy failed, falling back to core",
			"account_id", account.ID, "platform", account.Platform, "err", err)
		return false
	}
	if !resp.Success {
		response.Error(c, http.StatusBadRequest, resp.Error)
		return true
	}

	mode := resp.PrivacyMode
	if mode == "" {
		mode = "enabled"
	}
	if err := h.adminService.SetAccountPrivacyMode(c.Request.Context(), account.ID, mode); err != nil {
		slog.WarnContext(c.Request.Context(), "failed to persist plugin privacy mode",
			"account_id", account.ID, "mode", mode, "err", err)
	}

	updated, err := h.adminService.GetAccount(c.Request.Context(), account.ID)
	if err != nil {
		if account.Extra == nil {
			account.Extra = make(map[string]any)
		}
		account.Extra["privacy_mode"] = mode
		response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), account))
		return true
	}
	response.Success(c, h.buildAccountResponseWithRuntime(c.Request.Context(), updated))
	return true
}

// tryPluginPostAccountCreate fires a PostAccountCreate RPC to the plugin.
// This is fire-and-forget: runs in a goroutine, errors only logged.
func (h *AccountHandler) tryPluginPostAccountCreate(account *service.Account) {
	if account == nil {
		return
	}
	client := h.platformClient(account.Platform)
	if client == nil {
		return
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)
	accountID := account.ID
	platform := account.Platform
	accountType := account.Type

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("plugin_post_account_create_panic",
					"account_id", accountID, "recover", r)
			}
		}()
		resp, err := client.PostAccountCreate(
			context.Background(),
			accountID, platform, accountType,
			creds, extra,
		)
		if err != nil {
			if !isUnimplemented(err) {
				slog.Warn("plugin post account create failed",
					"account_id", accountID, "err", err)
			}
			return
		}
		updates := &service.UpdateAccountInput{}
		hasUpdates := false
		if len(resp.UpdatedCredentialsJson) > 0 {
			var newCreds map[string]any
			if err := json.Unmarshal(resp.UpdatedCredentialsJson, &newCreds); err == nil {
				updates.Credentials = newCreds
				hasUpdates = true
			}
		}
		if len(resp.UpdatedExtraJson) > 0 {
			var newExtra map[string]any
			if err := json.Unmarshal(resp.UpdatedExtraJson, &newExtra); err == nil {
				updates.Extra = newExtra
				hasUpdates = true
			}
		}
		if hasUpdates {
			if _, err := h.adminService.UpdateAccount(context.Background(), accountID, updates); err != nil {
				slog.Warn("failed to apply plugin post-create updates",
					"account_id", accountID, "err", err)
			}
		}
	}()
}

// isUnimplemented returns true when the gRPC error code is Unimplemented.
func isUnimplemented(err error) bool {
	s, ok := status.FromError(err)
	return ok && s.Code() == codes.Unimplemented
}
