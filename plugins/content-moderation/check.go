package main

import (
	"context"
	"embed"

	cmService "github.com/Wei-Shaw/sub2api/plugins/content-moderation/service"
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

// Check evaluates request content against the moderation policy by delegating
// to the migrated ModerationService. The host calls it during gateway
// preflight (wired via GRPCServiceRegistrar once the SDK exposes the
// ContentInterceptExtension proto bindings). When the service has not been
// initialised yet the request is allowed through so a misconfigured plugin
// never blocks traffic.
func (p *ContentModerationPlugin) Check(ctx context.Context, req *ContentCheckRequest) (*ContentCheckResponse, error) {
	if p.service == nil || req == nil {
		return &ContentCheckResponse{Action: ContentActionAllow}, nil
	}
	decision, err := p.service.Check(ctx, contentCheckRequestToInput(req))
	if err != nil {
		return &ContentCheckResponse{Action: ContentActionAllow}, nil
	}
	return moderationDecisionToResponse(decision), nil
}

func contentCheckRequestToInput(req *ContentCheckRequest) cmService.ContentModerationCheckInput {
	var groupID *int64
	if req.GroupID > 0 {
		gid := req.GroupID
		groupID = &gid
	}
	return cmService.ContentModerationCheckInput{
		RequestID:  req.RequestID,
		UserID:     req.UserID,
		UserEmail:  req.UserEmail,
		APIKeyID:   req.APIKeyID,
		APIKeyName: req.APIKeyName,
		GroupID:    groupID,
		GroupName:  req.GroupName,
		Endpoint:   req.Endpoint,
		Provider:   req.Provider,
		Model:      req.Model,
		Protocol:   req.Protocol,
		Body:       req.Body,
	}
}

func moderationDecisionToResponse(decision *cmService.ContentModerationDecision) *ContentCheckResponse {
	if decision == nil {
		return &ContentCheckResponse{Action: ContentActionAllow}
	}
	action := ContentActionAllow
	if decision.Blocked {
		action = ContentActionBlock
	} else if decision.Flagged {
		action = ContentActionObserve
	}
	return &ContentCheckResponse{
		Action:          action,
		Blocked:         decision.Blocked,
		Flagged:         decision.Flagged,
		StatusCode:      int32(decision.StatusCode),
		Message:         decision.Message,
		HighestCategory: decision.HighestCategory,
		HighestScore:    decision.HighestScore,
	}
}
