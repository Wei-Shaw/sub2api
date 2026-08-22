package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type notificationChannelEndpointView struct {
	URL          string `json:"url"`
	BodyTemplate string `json:"body_template,omitempty"`
}

type notificationChannelWebhookView struct {
	Enabled  bool                            `json:"enabled"`
	Endpoint notificationChannelEndpointView `json:"endpoint"`
	// Secret is returned in the clear, unlike the SMTP password: it is a shared
	// secret the operator must copy into their receiver to verify signatures, so
	// a write-only field would make it unusable.
	Secret         string `json:"secret"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	MaxRetries     int    `json:"max_retries"`
}

type notificationChannelEventView struct {
	Event       string `json:"event"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Audience    string `json:"audience"`
	Optional    bool   `json:"optional"`

	Email   bool `json:"email"`
	Webhook bool `json:"webhook"`

	Endpoint     *notificationChannelEndpointView `json:"endpoint,omitempty"`
	Placeholders []string                         `json:"placeholders"`
}

type notificationChannelConfigResponse struct {
	Webhook             notificationChannelWebhookView `json:"webhook"`
	Events              []notificationChannelEventView `json:"events"`
	WebhookPlaceholders []string                       `json:"webhook_placeholders"`
}

// GetNotificationChannels returns the per-event channel matrix plus webhook
// transport configuration.
// GET /api/v1/admin/settings/notification-channels
func (h *SettingHandler) GetNotificationChannels(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification service is not configured")
		return
	}
	// The response carries the webhook signing secret in the clear, so it must
	// not sit in a proxy or browser cache.
	c.Header("Cache-Control", "no-store")

	ctx := c.Request.Context()
	cfg, err := h.notificationEmailService.GetChannelConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	resp := notificationChannelConfigResponse{
		Webhook: notificationChannelWebhookView{
			Enabled:        cfg.Webhook.Enabled,
			Endpoint:       notificationChannelEndpointToView(cfg.Webhook.Endpoint),
			Secret:         cfg.Webhook.Secret,
			TimeoutSeconds: cfg.Webhook.TimeoutSeconds,
			MaxRetries:     notificationChannelMaxRetriesView(cfg),
		},
		WebhookPlaceholders: service.NotificationWebhookExtraPlaceholders(),
	}

	for _, info := range h.notificationEmailService.ListEventInfos() {
		resolved := h.notificationEmailService.ResolveChannels(ctx, info.Event)
		view := notificationChannelEventView{
			Event:        info.Event,
			Label:        info.Label,
			Description:  info.Description,
			Category:     info.Category,
			Audience:     service.NotificationAudienceForEvent(info),
			Optional:     info.Optional,
			Email:        resolved.Email,
			Webhook:      resolved.Webhook,
			Placeholders: info.Placeholders,
		}
		if eventCfg, ok := cfg.Events[info.Event]; ok && eventCfg.Endpoint != nil {
			endpointView := notificationChannelEndpointToView(*eventCfg.Endpoint)
			view.Endpoint = &endpointView
		}
		resp.Events = append(resp.Events, view)
	}

	response.Success(c, resp)
}

// UpdateNotificationChannels replaces the channel configuration.
// PUT /api/v1/admin/settings/notification-channels
func (h *SettingHandler) UpdateNotificationChannels(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification service is not configured")
		return
	}
	ctx := c.Request.Context()

	var req service.NotificationChannelConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	stored, err := h.notificationEmailService.GetChannelConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	carryOverNotificationSecret(&req, stored)

	if err := h.notificationEmailService.SaveChannelConfig(ctx, req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	h.GetNotificationChannels(c)
}

type testNotificationWebhookRequest struct {
	Event string `json:"event"`
}

// TestNotificationWebhook posts a sample payload to the configured endpoint so
// an operator can verify the endpoint before enabling delivery.
// POST /api/v1/admin/settings/notification-channels/test
func (h *SettingHandler) TestNotificationWebhook(c *gin.Context) {
	if h.notificationEmailService == nil {
		response.InternalError(c, "notification service is not configured")
		return
	}
	ctx := c.Request.Context()

	var req testNotificationWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	event := strings.TrimSpace(req.Event)
	if event == "" {
		event = service.NotificationEmailEventOpsAlert
	}

	cfg, err := h.notificationEmailService.GetChannelConfig(ctx)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if err := h.notificationEmailService.SendTestWebhook(ctx, cfg, event); err != nil {
		response.BadRequest(c, "Test webhook failed: "+err.Error())
		return
	}
	response.Success(c, gin.H{"message": "Test webhook delivered"})
}

// notificationChannelMaxRetriesView resolves the effective retry count so the
// UI shows what actually applies, including an explicitly configured 0.
func notificationChannelMaxRetriesView(cfg service.NotificationChannelConfig) int {
	if cfg.Webhook.MaxRetries != nil {
		return *cfg.Webhook.MaxRetries
	}
	return service.NotificationWebhookDefaultMaxRetries()
}

func notificationChannelEndpointToView(endpoint service.NotificationWebhookEndpoint) notificationChannelEndpointView {
	return notificationChannelEndpointView{
		URL:          endpoint.URL,
		BodyTemplate: endpoint.BodyTemplate,
	}
}

// carryOverNotificationSecret keeps the stored signing secret when the client
// submits an empty one, so saving unrelated settings never rotates it.
func carryOverNotificationSecret(incoming *service.NotificationChannelConfig, stored service.NotificationChannelConfig) {
	if strings.TrimSpace(incoming.Webhook.Secret) == "" {
		incoming.Webhook.Secret = stored.Webhook.Secret
	}
}
