//go:build !embed

// Package web provides embedded web assets for the application.
package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func ServeEmbeddedFrontend() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.String(http.StatusNotFound, "Frontend not embedded. Build with -tags embed to include frontend.")
		c.Abort()
REDACTED
REDACTED

func HasEmbeddedFrontend() bool {
	return false
REDACTED
