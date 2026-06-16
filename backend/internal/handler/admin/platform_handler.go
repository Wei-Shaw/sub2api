package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/plugin"
	pluginsdk "github.com/Wei-Shaw/sub2api/plugin-sdk"

	"github.com/gin-gonic/gin"
)

// PlatformHandler serves the platforms API for the admin frontend.
type PlatformHandler struct {
	registry *plugin.PlatformRegistry
}

// NewPlatformHandler creates a new platform handler.
func NewPlatformHandler(registry *plugin.PlatformRegistry) *PlatformHandler {
	return &PlatformHandler{registry: registry}
}

// List returns all registered platforms.
// GET /api/v1/admin/platforms
func (h *PlatformHandler) List(c *gin.Context) {
	all := h.registry.All()
	out := make([]platformResponse, len(all))
	for i := range all {
		out[i] = toPlatformResponse(&all[i])
	}
	response.Success(c, out)
}

// Get returns a single platform by ID.
// GET /api/v1/admin/platforms/:platform
func (h *PlatformHandler) Get(c *gin.Context) {
	platform := c.Param("platform")
	rp, ok := h.registry.Get(platform)
	if !ok {
		response.Error(c, http.StatusNotFound, "platform not found")
		return
	}
	entry := plugin.RegisteredPlatform{
		Decl:       rp.Decl,
		PluginName: rp.PluginName,
	}
	response.Success(c, toPlatformResponse(&entry))
}

// GetModels returns the default model list for a platform by calling
// the plugin's GetAvailableModels RPC with empty credentials.
// GET /api/v1/admin/platforms/:platform/models
func (h *PlatformHandler) GetModels(c *gin.Context) {
	platform := c.Param("platform")
	rp, ok := h.registry.Get(platform)
	if !ok {
		response.Success(c, []platformModelResponse{})
		return
	}
	client := plugin.NewAccountPlatformClient(rp.Conn)
	resp, err := client.GetAvailableModels(
		c.Request.Context(),
		0, platform, "", nil, nil,
	)
	if err != nil {
		slog.WarnContext(c.Request.Context(),
			"plugin GetAvailableModels failed, returning empty list",
			"platform", platform, "err", err)
		response.Success(c, []platformModelResponse{})
		return
	}
	models := make([]platformModelResponse, len(resp.Models))
	for i, m := range resp.Models {
		models[i] = platformModelResponse{
			ModelID:     m.ModelId,
			DisplayName: m.DisplayName,
			Available:   m.Available,
		}
	}
	response.Success(c, models)
}

type platformModelResponse struct {
	ModelID     string `json:"model_id"`
	DisplayName string `json:"display_name"`
	Available   bool   `json:"available"`
}

// --- JSON response types ---

type platformResponse struct {
	Platform    string `json:"platform"`
	DisplayName string `json:"display_name"`
	IconSVG     string `json:"icon_svg"`
	ThemeColor  string `json:"theme_color"`
	PluginName  string `json:"plugin_name"`
	SortOrder   int    `json:"sort_order"`

	AccountTypes       []accountTypeResponse    `json:"account_types"`
	CapacityDisplay    *capacityDisplayResponse `json:"capacity_display,omitempty"`
	UsageDisplay       *usageDisplayResponse    `json:"usage_display,omitempty"`
	CustomActions      []customActionResponse   `json:"custom_actions,omitempty"`
	TestConfig         *testConfigResponse      `json:"test_config,omitempty"`
	PrivacyStates      []privacyStateResponse   `json:"privacy_states,omitempty"`
	GroupConfig        *groupConfigResponse     `json:"group_config,omitempty"`
	CompatibleGateways []string                 `json:"compatible_gateways,omitempty"`
	FrontendMeta       json.RawMessage          `json:"frontend_meta,omitempty"`
}

type accountTypeResponse struct {
	Type              string          `json:"type"`
	DisplayName       string          `json:"display_name"`
	Description       string          `json:"description,omitempty"`
	IconSVG           string          `json:"icon_svg,omitempty"`
	ThemeColor        string          `json:"theme_color,omitempty"`
	CredentialSchema  json.RawMessage `json:"credential_schema,omitempty"`
	ExtraSchema       json.RawMessage `json:"extra_schema,omitempty"`
	FormComponentPath string          `json:"form_component_path,omitempty"`
	SubTypes          []subTypeResp   `json:"sub_types,omitempty"`
	SortOrder         int             `json:"sort_order"`
	BadgeLabel        string          `json:"badge_label,omitempty"`
	FrontendMeta      json.RawMessage `json:"frontend_meta,omitempty"`
}

type subTypeResp struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type capacityDisplayResponse struct {
	ShowConcurrency bool             `json:"show_concurrency"`
	ExtraRows       []displayRowResp `json:"extra_rows,omitempty"`
}

type usageDisplayResponse struct {
	ComponentPath string           `json:"component_path,omitempty"`
	WindowLabel   string           `json:"window_label,omitempty"`
	ShowReqCount  bool             `json:"show_req_count"`
	ShowCost      bool             `json:"show_cost"`
	ExtraRows     []displayRowResp `json:"extra_rows,omitempty"`
}

type displayRowResp struct {
	Label  string `json:"label"`
	Source string `json:"source"`
	Format string `json:"format"`
}

type customActionResponse struct {
	ActionID      string            `json:"action_id"`
	IconSVG       string            `json:"icon_svg,omitempty"`
	Labels        map[string]string `json:"labels"`
	ActionType    string            `json:"action_type"`
	APIEndpoint   string            `json:"api_endpoint,omitempty"`
	ComponentPath string            `json:"component_path,omitempty"`
	SortOrder     int               `json:"sort_order"`
}

type testConfigResponse struct {
	ModelSelector      bool             `json:"model_selector"`
	TestComponentPath  string           `json:"test_component_path,omitempty"`
	DefaultTestModel   string           `json:"default_test_model,omitempty"`
	TestModes          []testModeOption `json:"test_modes,omitempty"`
	ImageModelPatterns []string         `json:"image_model_patterns,omitempty"`
	PrioritizedModels  []string         `json:"prioritized_models,omitempty"`
}

type testModeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type privacyStateResponse struct {
	Value       string `json:"value"`
	DisplayName string `json:"display_name"`
	BadgeColor  string `json:"badge_color,omitempty"`
	IsSet       bool   `json:"is_set"`
}

type groupConfigResponse struct {
	GroupExtraSchema  json.RawMessage `json:"group_extra_schema,omitempty"`
	FormComponentPath string          `json:"form_component_path,omitempty"`
	FrontendMeta      json.RawMessage `json:"frontend_meta,omitempty"`
}

// --- converters ---

func toPlatformResponse(e *plugin.RegisteredPlatform) platformResponse {
	d := &e.Decl
	r := platformResponse{
		Platform:    d.Platform,
		DisplayName: d.DisplayName,
		IconSVG:     d.IconSVG,
		ThemeColor:  d.ThemeColor,
		PluginName:  e.PluginName,
		SortOrder:   d.SortOrder,
	}
	for _, at := range d.AccountTypes {
		r.AccountTypes = append(r.AccountTypes, toAccountTypeResp(at))
	}
	if d.CapacityDisplay != nil {
		r.CapacityDisplay = &capacityDisplayResponse{
			ShowConcurrency: d.CapacityDisplay.ShowConcurrency,
			ExtraRows:       toDisplayRows(d.CapacityDisplay.ExtraRows),
		}
	}
	if d.UsageDisplay != nil {
		r.UsageDisplay = &usageDisplayResponse{
			ComponentPath: d.UsageDisplay.ComponentPath,
			WindowLabel:   d.UsageDisplay.WindowLabel,
			ShowReqCount:  d.UsageDisplay.ShowReqCount,
			ShowCost:      d.UsageDisplay.ShowCost,
			ExtraRows:     toDisplayRows(d.UsageDisplay.ExtraRows),
		}
	}
	for _, ca := range d.CustomActions {
		r.CustomActions = append(r.CustomActions, customActionResponse{
			ActionID:      ca.ActionID,
			IconSVG:       ca.IconSVG,
			Labels:        ca.Labels,
			ActionType:    ca.ActionType,
			APIEndpoint:   ca.APIEndpoint,
			ComponentPath: ca.ComponentPath,
			SortOrder:     ca.SortOrder,
		})
	}
	if d.TestConfig != nil {
		r.TestConfig = &testConfigResponse{
			ModelSelector:      d.TestConfig.ModelSelector,
			TestComponentPath:  d.TestConfig.TestComponentPath,
			DefaultTestModel:   d.TestConfig.DefaultTestModel,
			TestModes:          toTestModeOptions(d.TestConfig.TestModes),
			ImageModelPatterns: d.TestConfig.ImageModelPatterns,
			PrioritizedModels:  d.TestConfig.PrioritizedModels,
		}
	}
	for _, ps := range d.PrivacyStates {
		r.PrivacyStates = append(r.PrivacyStates, privacyStateResponse{
			Value:       ps.Value,
			DisplayName: ps.DisplayName,
			BadgeColor:  ps.BadgeColor,
			IsSet:       ps.IsSet,
		})
	}
	if d.GroupConfig != nil {
		r.GroupConfig = &groupConfigResponse{
			GroupExtraSchema:  d.GroupConfig.GroupExtraSchema,
			FormComponentPath: d.GroupConfig.FormComponentPath,
			FrontendMeta:      d.GroupConfig.FrontendMeta,
		}
	}
	r.CompatibleGateways = d.CompatibleGateways
	r.FrontendMeta = d.FrontendMeta
	return r
}

func toAccountTypeResp(at pluginsdk.AccountTypeDecl) accountTypeResponse {
	return accountTypeResponse{
		Type:              at.Type,
		DisplayName:       at.DisplayName,
		Description:       at.Description,
		IconSVG:           at.IconSVG,
		ThemeColor:        at.ThemeColor,
		CredentialSchema:  at.CredentialSchema,
		ExtraSchema:       at.ExtraSchema,
		FormComponentPath: at.FormComponentPath,
		SubTypes:          toSubTypes(at.SubTypes),
		SortOrder:         at.SortOrder,
		BadgeLabel:        at.BadgeLabel,
		FrontendMeta:      at.FrontendMeta,
	}
}

func toSubTypes(opts []pluginsdk.SubTypeOption) []subTypeResp {
	if len(opts) == 0 {
		return nil
	}
	out := make([]subTypeResp, len(opts))
	for i, o := range opts {
		out[i] = subTypeResp{Value: o.Value, Label: o.Label}
	}
	return out
}

func toDisplayRows(rows []pluginsdk.DisplayRow) []displayRowResp {
	if len(rows) == 0 {
		return nil
	}
	out := make([]displayRowResp, len(rows))
	for i, r := range rows {
		out[i] = displayRowResp{Label: r.Label, Source: r.Source, Format: r.Format}
	}
	return out
}

func toTestModeOptions(opts []pluginsdk.TestModeOption) []testModeOption {
	if len(opts) == 0 {
		return nil
	}
	out := make([]testModeOption, len(opts))
	for i, o := range opts {
		out[i] = testModeOption{Value: o.Value, Label: o.Label}
	}
	return out
}
