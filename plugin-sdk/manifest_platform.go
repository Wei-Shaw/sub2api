package pluginsdk

import "encoding/json"

// PlatformDecl describes an account platform a gateway plugin provides.
// The host registers these into PlatformRegistry at plugin load time and
// renders them on the admin account management pages.
type PlatformDecl struct {
	Platform    string
	DisplayName string
	IconSVG     string
	ThemeColor  string

	AccountTypes       []AccountTypeDecl
	CapacityDisplay    *CapacityDisplayConfig
	UsageDisplay       *UsageDisplayConfig
	CustomActions      []CustomActionDecl
	TestConfig         *TestConnectionConfig
	SortOrder          int
	PrivacyStates      []PrivacyStateDecl
	CompatibleGateways []string
	GroupConfig        *GroupConfigDecl
	FrontendMeta       json.RawMessage `json:"frontend_meta,omitempty"` // Opaque JSON passed to frontend; backend does not interpret
}

// GroupConfigDecl declares the group-level configuration schema for a platform.
// The host renders either a JSON-schema-driven form or mounts a custom Vue
// component on the group management page.
type GroupConfigDecl struct {
	GroupExtraSchema  json.RawMessage // JSON Schema (Draft-07)
	FormComponentPath string          // custom Vue component path; empty = schema renderer
	FrontendMeta      json.RawMessage `json:"frontend_meta,omitempty"` // Opaque JSON passed to frontend; backend does not interpret
}

// AccountTypeDecl describes an account type within a platform.
type AccountTypeDecl struct {
	Type        string
	DisplayName string
	Description string
	IconSVG     string
	ThemeColor  string

	CredentialSchema  json.RawMessage
	ExtraSchema       json.RawMessage
	FormComponentPath string
	SubTypes          []SubTypeOption
	SortOrder         int
	BadgeLabel        string
	FrontendMeta      json.RawMessage `json:"frontend_meta,omitempty"` // Opaque JSON passed to frontend; backend does not interpret
}

// SubTypeOption represents a sub-type choice within an account type.
type SubTypeOption struct {
	Value string
	Label string
}

// CapacityDisplayConfig customizes the capacity column rendering.
type CapacityDisplayConfig struct {
	ShowConcurrency bool
	ExtraRows       []DisplayRow
}

// UsageDisplayConfig customizes the usage window column rendering.
type UsageDisplayConfig struct {
	ComponentPath string
	WindowLabel   string
	ShowReqCount  bool
	ShowCost      bool
	ExtraRows     []DisplayRow
}

// DisplayRow describes a single row in capacity/usage display.
type DisplayRow struct {
	Label  string
	Source string
	Format string
}

// CustomActionDecl declares a custom menu item in the account actions menu.
type CustomActionDecl struct {
	ActionID      string
	IconSVG       string
	Labels        map[string]string
	ActionType    string
	APIEndpoint   string
	ComponentPath string
	SortOrder     int
}

// TestModeOption represents a selectable test mode in the test dialog.
type TestModeOption struct {
	Value string
	Label string
}

// TestConnectionConfig customizes the test connection dialog.
type TestConnectionConfig struct {
	ModelSelector      bool
	TestComponentPath  string
	DefaultTestModel   string
	TestModes          []TestModeOption
	ImageModelPatterns []string
	PrioritizedModels  []string
}

// PrivacyStateDecl declares a privacy state for the platform.
type PrivacyStateDecl struct {
	Value       string
	DisplayName string
	BadgeColor  string
	IsSet       bool
}
