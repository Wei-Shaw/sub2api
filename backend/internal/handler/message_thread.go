package handler

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// messagesTargetPlatform resolves where /v1/messages is going: a forced
// platform (the /antigravity routes), then the platform a composite group
// resolved for the model, then the group's own platform.
func messagesTargetPlatform(c *gin.Context, key *service.APIKey) string {
	if p, ok := middleware.GetForcePlatformFromContext(c); ok {
		return p
	}
	if p, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context()); ok {
		return p
	}
	if key != nil && key.Group != nil {
		return key.Group.Platform
	}
	return ""
}

// messageThreadRawModel is the model name exactly as the client sent it.
func messageThreadRawModel(body []byte) string {
	return gjson.GetBytes(body, "model").String()
}

// Only the Anthropic gateway forwards a Messages request without converting
// it. Whether the selected account can serve a thread is decided later, on the
// account itself.
func messageThreadNativePlatform(platform string) bool {
	return platform == service.PlatformAnthropic || platform == ""
}

// rejectUnsupportedMessageThread refuses a thread request on a dispatcher that
// has no thread binding. Its routes either convert the request, which drops
// the thread, or reach a third-party Messages-compatible endpoint that is not
// known to implement threads; both could answer 200 without the conversation.
func rejectUnsupportedMessageThread(c *gin.Context, body []byte) bool {
	_, err := (*service.GatewayService)(nil).PrepareMessageThread(c.Request.Context(), body, c.Request.Header, "", 0, 0, nil, false)
	if err == nil {
		return false
	}
	writeMessageThreadError(c, err)
	return true
}

// Token counting does not support threads upstream either: the caller counts
// the full conversation, with system and tools.
func rejectCountTokensThread(c *gin.Context, body []byte) bool {
	if !gjson.GetBytes(body, "thread").Exists() {
		return false
	}
	c.JSON(http.StatusBadRequest, gin.H{"type": "error", "error": gin.H{
		"type":    "invalid_request_error",
		"message": "thread: token counting does not support message threads. Count the full conversation, with system and tools.",
	}})
	return true
}

func writeMessageThreadError(c *gin.Context, err error) {
	var threadErr *service.MessageThreadError
	if !errors.As(err, &threadErr) {
		c.JSON(http.StatusBadGateway, gin.H{"type": "error", "error": gin.H{"type": "api_error", "message": "Upstream message thread request failed"}})
		return
	}
	if threadErr.Written {
		return
	}
	// A thread error is decided by the request; it says nothing about an
	// account. Unreadable thread state stays an ordinary service failure.
	if threadErr.Status < http.StatusInternalServerError {
		service.MarkOpsRequestScopedError(c, threadErr.Type)
	}
	c.JSON(threadErr.Status, threadErr.Body())
}

// messageThreadSelectionError turns an account selection failure of a thread
// request into its answer, or returns nil to leave it to normal handling.
//
// A continue request has exactly one usable account. When that account is not
// available the new messages are not offered to another one: the client is
// told to replay. A failure that says nothing about the account is reported as
// unavailable instead of being passed off as a missing thread.
func messageThreadSelectionError(thread *service.MessageThread, err error, skippedIncapable, upstreamFailed bool) error {
	if thread == nil || err == nil {
		return nil
	}
	var threadErr *service.MessageThreadError
	if errors.As(err, &threadErr) {
		return threadErr
	}
	if thread.AdmissionFault() {
		// An account was passed over because a limit of it could not be
		// evaluated. That is not a group without room.
		return service.NewMessageThreadUnavailableError()
	}
	if thread.IsContinue() {
		if errors.Is(err, service.ErrNoAvailableAccounts) || errors.Is(err, service.ErrClaudeCodeOnly) {
			return service.NewMessageThreadNotFoundError()
		}
		return service.NewMessageThreadUnavailableError()
	}
	if skippedIncapable && errors.Is(err, service.ErrNoAvailableAccounts) {
		return service.ErrMessageThreadNoCapableAccount
	}
	if thread.LimitDenied() && !upstreamFailed && errors.Is(err, service.ErrNoAvailableAccounts) {
		// Every account that could serve the request is at a limit.
		return service.NewMessageThreadNoRoomError()
	}
	return nil
}
