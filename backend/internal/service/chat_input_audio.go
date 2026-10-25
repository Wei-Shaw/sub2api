package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

func writeChatInputAudioValidationError(ctx *gin.Context, err error) bool {
	var validationErr *apicompat.InputAudioValidationError
	if !errors.As(err, &validationErr) {
		return false
	}
	if ctx.Writer.Written() && strings.HasPrefix(ctx.Writer.Header().Get("Content-Type"), "text/event-stream") {
		MarkResponseCommitted(ctx)
		payload, _ := json.Marshal(gin.H{"error": gin.H{
			"type":    "invalid_request_error",
			"message": err.Error(),
		}})
		_, _ = fmt.Fprintf(ctx.Writer, "data: %s\n\n", payload)
		ctx.Writer.Flush()
	} else {
		writeChatCompletionsError(ctx, http.StatusBadRequest, "invalid_request_error", err.Error())
	}
	return true
}
