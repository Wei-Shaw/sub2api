package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
)

func customDomainIDFromContext(ctx context.Context) *int64 {
	if ctx == nil {
		return nil
	}
	if id, ok := ctx.Value(ctxkey.CustomDomainID).(int64); ok && id > 0 {
		return &id
	}
	return nil
}

func customDomainFromContext(ctx context.Context) *string {
	if ctx == nil {
		return nil
	}
	if domain, ok := ctx.Value(ctxkey.CustomDomain).(string); ok {
		domain = strings.TrimSpace(domain)
		if domain != "" {
			return &domain
		}
	}
	return nil
}
