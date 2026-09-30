package service

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
)

// A canceled upstream child context is not proof that the client disconnected.
// Require cancellation on the incoming request as well as the returned cause.
func isExcelBPSClientCancellation(c *gin.Context, err error) bool {
	return c != nil && c.Request != nil && errors.Is(err, context.Canceled) &&
		errors.Is(c.Request.Context().Err(), context.Canceled)
}
