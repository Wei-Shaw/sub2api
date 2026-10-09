package testutil

import (
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// NewRealHTTPUpstream preserves production transport and decompression behavior
// in higher-layer tests without exposing repository types to those layers.
func NewRealHTTPUpstream(cfg *config.Config) service.HTTPUpstream {
	return repository.NewHTTPUpstream(cfg)
}
