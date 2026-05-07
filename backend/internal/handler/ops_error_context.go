package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const scheduledAccountDebugHeader = "X-Sub2api-Scheduled-Account"
const noScheduledAccountLabel = "No scheduled account"

func selectionUnavailableMessage(err error) string {
	const fallback = "Service temporarily unavailable"
	if err == nil {
		return fallback
	}

	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return fallback
	}
	msg = strings.Join(strings.Fields(msg), " ")
	return fallback + ": " + msg
}

func decorateScheduledAccountErrorMessage(c *gin.Context, status int, message string) string {
	message = strings.TrimSpace(message)
	if c == nil {
		return message
	}

	snapshot := service.GetOpsSelectedAccountSnapshot(c)
	accountName := strings.TrimSpace(snapshot.Name)
	if accountName == "" {
		return message
	}

	if c.Writer != nil {
		c.Header(scheduledAccountDebugHeader, accountName)
	}

	if status < http.StatusInternalServerError && status != http.StatusBadGateway && status != http.StatusServiceUnavailable {
		return message
	}
	if strings.Contains(strings.ToLower(message), "scheduled account:") {
		return message
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return fmt.Sprintf("%s [scheduled account: %s]", message, accountName)
}

func ensureOpsUpstreamErrorEvent(c *gin.Context, statusCode int, kind, message string) {
	if c == nil || statusCode <= 0 {
		return
	}

	if v, ok := c.Get(service.OpsUpstreamErrorsKey); ok {
		if events, ok := v.([]*service.OpsUpstreamErrorEvent); ok && len(events) > 0 {
			return
		}
	}

	service.AppendOpsUpstreamError(c, service.OpsUpstreamErrorEvent{
		UpstreamStatusCode: statusCode,
		Kind:               strings.TrimSpace(kind),
		Message:            strings.TrimSpace(message),
	})
}

func annotateOpenAISelectionFailure(
	c *gin.Context,
	gatewayService *service.OpenAIGatewayService,
	groupID *int64,
	sessionHash string,
	err error,
	message string,
) {
	if c == nil {
		return
	}

	snapshot := service.GetOpsSelectedAccountSnapshot(c)
	accountName := strings.TrimSpace(snapshot.Name)
	accountID := snapshot.ID
	if gatewayService != nil {
		diagCtx := context.Background()
		if c.Request != nil {
			diagCtx = context.WithoutCancel(c.Request.Context())
		}
		peekCtx, cancel := context.WithTimeout(diagCtx, 2*time.Second)
		defer cancel()

		if accountID <= 0 {
			if stickyAccount, err := gatewayService.PeekStickyOpenAIAccount(peekCtx, groupID, sessionHash); err == nil && stickyAccount != nil {
				accountID = stickyAccount.ID
				accountName = strings.TrimSpace(stickyAccount.Name)
				setOpsSelectedAccount(c, stickyAccount.ID, stickyAccount.Name, stickyAccount.Platform)
			}
		}
	}

	if accountName == "" {
		accountName = noScheduledAccountLabel
	}

	detail := ""
	var selectionErr *service.OpenAISelectionError
	if errors.As(err, &selectionErr) && selectionErr != nil {
		parts := make([]string, 0, 3)
		if phase := strings.TrimSpace(selectionErr.Phase); phase != "" {
			parts = append(parts, "phase="+phase)
		}
		if cause := strings.TrimSpace(selectionErr.Cause); cause != "" {
			parts = append(parts, "cause="+cause)
		}
		if diag := strings.TrimSpace(selectionErr.Detail); diag != "" {
			parts = append(parts, diag)
		}
		detail = strings.Join(parts, " | ")
	}

	service.AppendOpsUpstreamError(c, service.OpsUpstreamErrorEvent{
		AccountID:          accountID,
		AccountName:        accountName,
		Platform:           service.PlatformOpenAI,
		Kind:               "selection",
		Message:            strings.TrimSpace(message),
		Detail:             detail,
		UpstreamStatusCode: http.StatusServiceUnavailable,
	})
}
