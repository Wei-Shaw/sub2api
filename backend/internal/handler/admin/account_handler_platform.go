package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plugin"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
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

	stream, err := client.TestConnection(
		c.Request.Context(),
		account.ID, account.Platform, account.Type,
		creds, extra, modelID,
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

		evt := gin.H{"type": event.Type}
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

func (h *AccountHandler) tryPluginCustomAction(c *gin.Context, account *service.Account, actionID string, payload json.RawMessage) bool {
	client := h.platformClient(account.Platform)
	if client == nil {
		return false
	}
	creds, _ := json.Marshal(account.Credentials)
	extra, _ := json.Marshal(account.Extra)

	resp, err := client.ExecuteCustomAction(
		c.Request.Context(),
		actionID, account.ID, account.Platform,
		creds, extra, payload,
	)
	if err != nil {
		response.InternalError(c, "plugin custom action: "+err.Error())
		return true
	}
	if !resp.Success {
		response.Error(c, http.StatusBadRequest, resp.Error)
		return true
	}

	if len(resp.UpdatedCredentialsJson) > 0 || len(resp.UpdatedExtraJson) > 0 {
		updates := &service.UpdateAccountInput{}
		if len(resp.UpdatedCredentialsJson) > 0 {
			var c2 map[string]any
			if err := json.Unmarshal(resp.UpdatedCredentialsJson, &c2); err == nil {
				updates.Credentials = c2
			}
		}
		if len(resp.UpdatedExtraJson) > 0 {
			var e2 map[string]any
			if err := json.Unmarshal(resp.UpdatedExtraJson, &e2); err == nil {
				updates.Extra = e2
			}
		}
		_, _ = h.adminService.UpdateAccount(c.Request.Context(), account.ID, updates)
	}

	var result any
	if len(resp.ResultJson) > 0 {
		_ = json.Unmarshal(resp.ResultJson, &result)
	}
	response.Success(c, result)
	return true
}

func writePluginSSEEvent(c *gin.Context, data any) {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(c.Writer, "data: %s\n\n", jsonBytes)
	c.Writer.(http.Flusher).Flush()
}

func sendPluginSSEError(c *gin.Context, msg string) {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.WriteHeader(http.StatusOK)
	writePluginSSEEvent(c, gin.H{"type": "error", "error": msg})
	writePluginSSEEvent(c, gin.H{"type": "test_end", "success": false, "error": msg})
}

// SetPlatformRegistry sets the platform registry after construction.
// Used by wire_gen.go when pluginManager is created after AccountHandler.
func (h *AccountHandler) SetPlatformRegistry(registry *plugin.PlatformRegistry) {
	h.platformRegistry = registry
}
