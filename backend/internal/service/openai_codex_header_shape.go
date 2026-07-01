package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	openAIHeaderSessionID           = "session_id"
	openAIHeaderConversationID      = "conversation_id"
	openAIHeaderCodexWindowID       = "x-codex-window-id"
	openAIHeaderCodexInstallationID = "x-codex-installation-id"
)

var openAICodexManagedHeaders = []string{
	"x-client-request-id",
	"x-openai-subagent",
	"x-codex-beta-features",
	"x-codex-turn-metadata",
	"x-codex-parent-thread-id",
	"x-responsesapi-include-timing-metrics",
	"x-codex-inference-call-id",
	"x-oai-attestation",
}

func applyOpenAICodexHeaderShape(req *http.Request, c *gin.Context, account *Account, isCompact bool) {
	if req == nil || account == nil || account.Type != AccountTypeOAuth {
		return
	}

	for _, name := range openAICodexManagedHeaders {
		if value := getOpenAIIncomingHeader(c, name); value != "" && req.Header.Get(name) == "" {
			req.Header.Set(name, value)
		}
	}

	req.Header.Del(openAIHeaderConversationID)
	req.Header.Del("OpenAI-Beta")
	req.Header.Del("version")
	// Codex-Manager 仅在 /compact 头部携带 x-codex-installation-id；
	// 标准 /responses 通过 body.client_metadata 携带，避免 header/body 双写。
	if isCompact {
		if installationID := resolveOpenAICodexInstallationID(c, account); installationID != "" {
			req.Header.Set(openAIHeaderCodexInstallationID, installationID)
		}
	} else {
		req.Header.Del(openAIHeaderCodexInstallationID)
	}

	sessionID := strings.TrimSpace(req.Header.Get(openAIHeaderSessionID))
	incomingWindowID := getOpenAIIncomingHeader(c, openAIHeaderCodexWindowID)
	if windowID := resolveOpenAICodexWindowID(incomingWindowID, sessionID); windowID != "" {
		req.Header.Set(openAIHeaderCodexWindowID, windowID)
	}

	// Codex /compact 会发送 thread_id；没有客户端值时用 conversation/session
	// 的已隔离值兜底，保证 window/session/thread 形态稳定。
	if isCompact && req.Header.Get("thread_id") == "" {
		if threadID := strings.TrimSpace(req.Header.Get(openAIHeaderConversationID)); threadID != "" {
			req.Header.Set("thread_id", threadID)
		} else if sessionID != "" {
			req.Header.Set("thread_id", sessionID)
		}
	}
}

func applyCodexClientMetadataFromRequest(reqBody map[string]any, c *gin.Context, account *Account) bool {
	if len(reqBody) == 0 {
		return false
	}
	return applyCodexClientMetadataWithInstallationID(reqBody, resolveOpenAICodexInstallationID(c, account))
}

func applyCodexClientMetadataToBodyJSON(body []byte, c *gin.Context, account *Account) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return body, false, err
	}
	if !applyCodexClientMetadataFromRequest(decoded, c, account) {
		return body, false, nil
	}
	next, err := json.Marshal(decoded)
	if err != nil {
		return body, false, err
	}
	return next, true, nil
}

func resolveOpenAICodexInstallationID(c *gin.Context, account *Account) string {
	if fromClient := getOpenAIIncomingHeader(c, openAIHeaderCodexInstallationID); fromClient != "" {
		return fromClient
	}
	if fromAccount := openAICodexInstallationIDFromAccount(account); fromAccount != "" {
		return fromAccount
	}
	return resolveOpenAICodexPersistedInstallationID()
}

func openAICodexInstallationIDFromAccount(account *Account) string {
	if account == nil {
		return ""
	}
	return strings.TrimSpace(account.GetOpenAIDeviceID())
}

func getOpenAIIncomingHeader(c *gin.Context, name string) string {
	if c == nil || c.Request == nil {
		return ""
	}
	return strings.TrimSpace(c.Request.Header.Get(name))
}

func resolveOpenAICodexWindowID(incomingWindowID, sessionID string) string {
	windowID := strings.TrimSpace(incomingWindowID)
	sessionID = strings.TrimSpace(sessionID)
	if windowID != "" {
		if sessionID == "" || windowID == sessionID || strings.HasPrefix(windowID, sessionID+":") {
			return windowID
		}
	}
	if sessionID == "" {
		return ""
	}
	return fmt.Sprintf("%s:0", sessionID)
}
