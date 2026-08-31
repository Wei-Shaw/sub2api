package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

var miniMaxM3CodexModel = map[string]any{
	"slug":                    "MiniMax-M3",
	"display_name":            "MiniMax-M3",
	"description":             "MiniMax",
	"default_reasoning_level": "high",
	"supported_reasoning_levels": []map[string]string{
		{"effort": "none", "description": "Think-Off"},
		{"effort": "high", "description": "Deep"},
	},
	"shell_type":                   "shell_command",
	"visibility":                   "list",
	"supported_in_api":             true,
	"priority":                     0,
	"base_instructions":            "You are Codex, a coding agent based on MiniMax-M3. You and the user share the same workspace and collaborate to achieve the user's goals.",
	"supports_reasoning_summaries": true,
	"default_reasoning_summary":    "none",
	"support_verbosity":            false,
	"truncation_policy":            map[string]any{"mode": "bytes", "limit": 10000},
	"supports_parallel_tool_calls": true,
	"experimental_supported_tools": []string{},
	"input_modalities":             []string{"text", "image"},
}

func miniMaxCodexModelsManifest() ([]byte, error) {
	return json.Marshal(map[string]any{"models": []any{miniMaxM3CodexModel}})
}

func appendMiniMaxCodexModel(body []byte) ([]byte, bool, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, false, fmt.Errorf("decode Codex models manifest: %w", err)
	}
	var models []json.RawMessage
	if err := json.Unmarshal(envelope["models"], &models); err != nil {
		return nil, false, fmt.Errorf("decode Codex models: %w", err)
	}
	for _, rawModel := range models {
		var descriptor struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(rawModel, &descriptor) == nil && strings.EqualFold(strings.TrimSpace(descriptor.Slug), "MiniMax-M3") {
			return body, false, nil
		}
	}
	rawModel, err := json.Marshal(miniMaxM3CodexModel)
	if err != nil {
		return nil, false, fmt.Errorf("encode MiniMax Codex model: %w", err)
	}
	models = append(models, rawModel)
	envelope["models"], err = json.Marshal(models)
	if err != nil {
		return nil, false, fmt.Errorf("encode Codex models: %w", err)
	}
	merged, err := json.Marshal(envelope)
	if err != nil {
		return nil, false, fmt.Errorf("encode Codex models manifest: %w", err)
	}
	return merged, true, nil
}

// CodexModels serves the Codex models manifest for Codex clients.
//
// Codex CLI and the Codex desktop app refresh their model picker from
// GET {base_url}/models?client_version=... (custom provider mode) or
// GET /backend-api/codex/models (chatgpt_base_url mode). Both routes land
// here. ChatGPT manifests are proxied verbatim; custom API key manifests receive
// provider-compatibility normalization and use a short-lived, asynchronously
// revalidated cache to tolerate canceled client requests.
func (h *OpenAIGatewayHandler) CodexModels(c *gin.Context) {
	if c.Request.Context().Err() != nil {
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "invalid_request_error", "API key group is required")
		return
	}
	if apiKey.Group.Platform != service.PlatformOpenAI && apiKey.Group.Platform != service.PlatformMiniMax && apiKey.Group.Platform != service.PlatformComposite {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Codex models manifest is not available for this group")
		return
	}
	if apiKey.Group.Platform == service.PlatformMiniMax {
		body, err := miniMaxCodexModelsManifest()
		if err != nil {
			h.errorResponse(c, http.StatusInternalServerError, "internal_error", "Failed to build MiniMax models manifest")
			return
		}
		c.Data(http.StatusOK, "application/json", body)
		return
	}
	compositeHasMiniMax := apiKey.Group.Platform == service.PlatformComposite &&
		h.gatewayService.HasSchedulableAccountForPlatform(c.Request.Context(), apiKey.GroupID, service.PlatformMiniMax)
	if compositeHasMiniMax && !h.gatewayService.HasSchedulableAccountForPlatform(c.Request.Context(), apiKey.GroupID, service.PlatformOpenAI) {
		body, err := miniMaxCodexModelsManifest()
		if err != nil {
			h.errorResponse(c, http.StatusInternalServerError, "internal_error", "Failed to build MiniMax models manifest")
			return
		}
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	failedAccountIDs := make(map[int64]struct{})
	switchCount := 0
	var lastUpstreamErr error

	for {
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(c.Request.Context(), apiKey.GroupID, "", "", failedAccountIDs)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if lastUpstreamErr != nil {
				h.errorResponse(c, infraerrors.Code(lastUpstreamErr), "upstream_error", infraerrors.Message(lastUpstreamErr))
				return
			}
			h.errorResponse(c, http.StatusServiceUnavailable, "upstream_error", "No available OpenAI accounts")
			return
		}
		// 让 ops 错误日志携带实际选中的上游账号，便于定位失效账号（#4544）。
		setOpsSelectedAccount(c, account.ID, account.Platform)

		ifNoneMatch := c.GetHeader("If-None-Match")
		if compositeHasMiniMax {
			// The combined manifest has local MiniMax capability metadata, so an
			// upstream-only ETag cannot validate the client representation.
			ifNoneMatch = ""
		}
		manifest, err := h.gatewayService.FetchCodexModelsManifest(c.Request.Context(), account, c.Query("client_version"), ifNoneMatch)
		if err != nil {
			if c.Request.Context().Err() != nil {
				return
			}
			if service.IsRetryableCodexModelsManifestError(err) && switchCount < maxAccountSwitches {
				failedAccountIDs[account.ID] = struct{}{}
				switchCount++
				lastUpstreamErr = err
				continue
			}
			h.errorResponse(c, infraerrors.Code(err), "upstream_error", infraerrors.Message(err))
			return
		}
		if c.Request.Context().Err() != nil {
			return
		}

		if manifest.NotModified {
			c.Status(http.StatusNotModified)
			return
		}
		if compositeHasMiniMax {
			mergedBody, changed, mergeErr := appendMiniMaxCodexModel(manifest.Body)
			if mergeErr != nil {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Invalid Codex models manifest")
				return
			}
			if changed {
				manifest.Body = mergedBody
				manifest.ETag = ""
			}
		}
		if manifest.ETag != "" {
			c.Header("ETag", manifest.ETag)
		}
		c.Data(http.StatusOK, "application/json", manifest.Body)
		return
	}
}
