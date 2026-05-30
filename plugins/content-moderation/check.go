package main

import (
	"context"
	"embed"
)

// migrationAssets embeds the SQL migrations the plugin ships. The host fetches
// each declared body via PluginLifecycle.GetMigration (served by OpenMigration
// in plugin.go) and re-verifies it against the manifest's pinned SHA-256.
//
//go:embed migrations/*.sql
var migrationAssets embed.FS

// ContentAction mirrors the proto ContentAction enum. It is defined locally as
// plain Go until the ContentInterceptExtension proto bindings are wired into
// the SDK (CM 插件: Proto 类型和 gRPC 接口定义); the values match the proto.
type ContentAction int

const (
	ContentActionAllow   ContentAction = 0
	ContentActionBlock   ContentAction = 1
	ContentActionObserve ContentAction = 2
)

// ContentCheckRequest mirrors the proto ContentCheckRequest message: the
// request context the host passes to the moderation policy during gateway
// preflight.
type ContentCheckRequest struct {
	RequestID  string
	UserID     int64
	UserEmail  string
	APIKeyID   int64
	APIKeyName string
	GroupID    int64
	GroupName  string
	Endpoint   string
	Protocol   string
	Provider   string
	Model      string
	Body       []byte
}

// ContentCheckResponse mirrors the proto ContentCheckResponse message: the
// moderation decision the host enforces.
type ContentCheckResponse struct {
	Action          ContentAction
	Blocked         bool
	Flagged         bool
	StatusCode      int32
	Message         string
	HighestCategory string
	HighestScore    float64
}

// Check evaluates request content against the moderation policy. This is the
// stub: it allows everything until the moderation service is migrated. Once
// the SDK exposes the ContentInterceptExtension gRPC service, this method is
// registered via GRPCServiceRegistrar and called by the host in preflight.
func (p *ContentModerationPlugin) Check(_ context.Context, _ *ContentCheckRequest) (*ContentCheckResponse, error) {
	return &ContentCheckResponse{Action: ContentActionAllow}, nil
}
