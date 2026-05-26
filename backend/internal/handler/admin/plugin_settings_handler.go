// Package admin — plugin_settings_handler.go
//
// V5 W3 — REST endpoints the admin UI uses to discover and edit
// plugin-scoped settings. Three endpoints suffice:
//
//	GET  /api/v1/admin/plugin-settings                     — list namespaces (one per plugin)
//	GET  /api/v1/admin/plugin-settings/:plugin             — schema + current values
//	PUT  /api/v1/admin/plugin-settings/:plugin/:key        — update a single key (validated)
//
// We deliberately update one key at a time because the UI form widgets
// (vue-json-schema-form) commit per-property and partial saves are easier
// to reason about than the all-or-nothing alternative.
//
// V5/W6 SETTINGS-V2 (W3-A):
//   - GET response carries `schema_version` and `properties_meta` (per-property
//     marker triple visibility/deprecated/requires_reload) so the UI widget
//     map can pick the correct widget per property. The fields are populated
//     by service.SchemaInfo (DESIGN §4.6).
//   - PUT explicitly forwards SetSourceAdmin and translates the new
//     ErrPluginSettingsBackendOnly into HTTP 403 with errcode
//     PLUGIN_SETTINGS_BACKEND_ONLY. Other errcodes follow DESIGN §7.2 / §F.
package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plugin"
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
//
// V5/W6 SETTINGS-V2: the JSON body inherits the new fields from
// service.PluginSettingsSchemaInfo:
//
//	schema_version  string                      — schema_version stamp ("0" when undeclared)
//	properties_meta map[string]{visibility,deprecated,requires_reload}
//	secret_keys     []string                    — sorted keys whose values were masked to null
//
// Field shape and JSON tags are owned by the service struct so the wire
// contract stays in lockstep with persistence. See DESIGN §4.6 / 附录 D.
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

// errcode constants — wire-format string codes returned in the response
// metadata.code field. See SETTINGS-V2-DESIGN §7.2 / 附录 F.
const (
	errcodePluginSettingsSchemaMissing    = "PLUGIN_SETTINGS_SCHEMA_MISSING"
	errcodePluginSettingsValidationFailed = "PLUGIN_SETTINGS_VALIDATION_FAILED"
	errcodePluginSettingsBackendOnly      = "PLUGIN_SETTINGS_BACKEND_ONLY"
)

// Update writes a single key after schema validation.
//
// Status codes (DESIGN §7.2 / 附录 F):
//
//	422 — PLUGIN_SETTINGS_VALIDATION_FAILED (schema rejection; UI maps to per-field error)
//	409 — PLUGIN_SETTINGS_SCHEMA_MISSING (plugin not yet registered a schema; admin must wait for restart)
//	403 — PLUGIN_SETTINGS_BACKEND_ONLY (visibility=backend; UI hides the input but
//	      a stale tab may still POST — server is the source of truth)
//
// V5/W6 SETTINGS-V2: writes go through SetByKeyWithSource(SetSourceAdmin)
// so the service can reject visibility=backend keys against admin
// callers. SetByKey (the backward-compat shim) already forwards to
// SetSourceAdmin but this handler invokes the explicit form per
// DESIGN §4.7 to make the source visible at the call site and avoid
// silent regressions if the shim's default ever changes.
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
	rev, err := h.service.SetByKeyWithSource(
		c.Request.Context(), pluginName, key, req.Value, service.SetSourceAdmin,
	)
	if err != nil {
		h.writeUpdateError(c, pluginName, key, err)
		return
	}
	response.Success(c, gin.H{
		"plugin":   pluginName,
		"key":      key,
		"value":    req.Value,
		"revision": rev,
	})
}

// writeUpdateError translates service-layer errors into the SETTINGS-V2
// errcode + HTTP status table (DESIGN §7.2 / 附录 F). The errcode string
// rides in the metadata.code field so the front-end can map to an i18n
// key without parsing the English message.
func (h *PluginSettingsHandler) writeUpdateError(c *gin.Context, pluginName, key string, err error) {
	var ve *service.ErrPluginSettingsValidation
	if errors.As(err, &ve) {
		response.ErrorWithDetails(c, http.StatusUnprocessableEntity,
			"plugin settings validation failed", ve.Reason, map[string]string{
				"code":   errcodePluginSettingsValidationFailed,
				"plugin": pluginName,
				"key":    key,
			})
		return
	}
	var be *service.ErrPluginSettingsBackendOnly
	if errors.As(err, &be) {
		response.ErrorWithDetails(c, http.StatusForbidden,
			"plugin settings key is backend-only", "", map[string]string{
				"code":   errcodePluginSettingsBackendOnly,
				"plugin": pluginName,
				"key":    key,
			})
		return
	}
	if errors.Is(err, service.ErrPluginSettingsSchemaMissing) {
		response.ErrorWithDetails(c, http.StatusConflict,
			"plugin schema not registered", "", map[string]string{
				"code":   errcodePluginSettingsSchemaMissing,
				"plugin": pluginName,
			})
		return
	}
	response.InternalError(c, err.Error())
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
	if !plugin.IsValidPluginName(name) {
		response.BadRequest(c, "plugin name contains invalid characters or is too long")
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
