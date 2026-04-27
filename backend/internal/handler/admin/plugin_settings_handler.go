// Package admin — plugin_settings_handler.go
//
// V5 W3 — REST endpoints the admin UI uses to discover and edit
// plugin-scoped settings. Three endpoints suffice:
//
//   GET  /api/v1/admin/plugin-settings                     — list namespaces (one per plugin)
//   GET  /api/v1/admin/plugin-settings/:plugin             — schema + current values
//   PUT  /api/v1/admin/plugin-settings/:plugin/:key        — update a single key (validated)
//
// We deliberately update one key at a time because the UI form widgets
// (vue-json-schema-form) commit per-property and partial saves are easier
// to reason about than the all-or-nothing alternative.
package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// PluginSettingsHandler exposes the admin REST surface for plugin
// settings. service can be nil when the host runs without the plugin
// system, in which case all endpoints return 503.
type PluginSettingsHandler struct {
	service *service.PluginSettingsService
}

// NewPluginSettingsHandler wires the handler. Pass nil when the plugin
// subsystem is disabled; the handler degrades gracefully.
func NewPluginSettingsHandler(svc *service.PluginSettingsService) *PluginSettingsHandler {
	return &PluginSettingsHandler{service: svc}
}

// UpdatePluginSettingValueRequest is the body for PUT.
//
// Value is the raw JSON the schema will validate; we deliberately do not
// pre-decode because vue-json-schema-form already produces the correct
// JSON shape for primitives, arrays, and objects.
type UpdatePluginSettingValueRequest struct {
	Value json.RawMessage `json:"value" binding:"required"`
}

// List returns every plugin that has registered a schema. The UI uses
// this to render one tab per plugin namespace.
func (h *PluginSettingsHandler) List(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	names, err := h.service.ListPlugins(c.Request.Context())
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if names == nil {
		names = []string{}
	}
	response.Success(c, gin.H{"items": names})
}

// Get returns the schema + current values for one plugin. 404 when the
// plugin has not registered anything.
func (h *PluginSettingsHandler) Get(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	pluginName, ok := h.parsePlugin(c)
	if !ok {
		return
	}
	info, err := h.service.SchemaInfo(c.Request.Context(), pluginName)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	if info == nil {
		response.NotFound(c, "plugin settings schema not registered")
		return
	}
	response.Success(c, info)
}

// Update writes a single key after schema validation. Validation failures
// return 422 so the UI can map them to a per-field error message; missing
// schemas return 409 because the plugin is expected to register a schema
// before the admin can edit anything.
func (h *PluginSettingsHandler) Update(c *gin.Context) {
	if !h.requireService(c) {
		return
	}
	pluginName, ok := h.parsePlugin(c)
	if !ok {
		return
	}
	key, ok := h.parseKey(c)
	if !ok {
		return
	}
	var req UpdatePluginSettingValueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}
	rev, err := h.service.SetByKey(c.Request.Context(), pluginName, key, req.Value)
	if err != nil {
		var ve *service.ErrPluginSettingsValidation
		if errors.As(err, &ve) {
			response.ErrorWithDetails(c, http.StatusUnprocessableEntity,
				"plugin settings validation failed", ve.Reason, map[string]string{
					"plugin": pluginName,
					"key":    key,
				})
			return
		}
		if errors.Is(err, service.ErrPluginSettingsSchemaMissing) {
			response.Error(c, http.StatusConflict, "plugin schema not registered; restart the plugin first")
			return
		}
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{
		"plugin":   pluginName,
		"key":      key,
		"value":    req.Value,
		"revision": rev,
	})
}

func (h *PluginSettingsHandler) requireService(c *gin.Context) bool {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "plugin settings service not enabled")
		return false
	}
	return true
}

func (h *PluginSettingsHandler) parsePlugin(c *gin.Context) (string, bool) {
	name := strings.TrimSpace(c.Param("plugin"))
	if name == "" {
		response.BadRequest(c, "plugin name is required")
		return "", false
	}
	if len(name) > 64 {
		response.BadRequest(c, "plugin name too long (max 64)")
		return "", false
	}
	return name, true
}

func (h *PluginSettingsHandler) parseKey(c *gin.Context) (string, bool) {
	key := strings.TrimSpace(c.Param("key"))
	if key == "" {
		response.BadRequest(c, "settings key is required")
		return "", false
	}
	if len(key) > 128 {
		response.BadRequest(c, "settings key too long (max 128)")
		return "", false
	}
	return key, true
}
