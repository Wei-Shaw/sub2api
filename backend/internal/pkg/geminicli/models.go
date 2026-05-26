package geminicli

// Model represents a selectable Gemini model for UI/testing purposes.
// Keep JSON fields consistent with existing frontend expectations.
type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

// DefaultModels is the curated Gemini model list used by the admin UI "test account" flow.
var DefaultModels = []Model{
	{ID: "gemini-2.0-flash", Type: "model", DisplayName: "Gemini 2.0 Flash", CreatedAt: ""},
	{ID: "gemini-2.5-flash", Type: "model", DisplayName: "Gemini 2.5 Flash", CreatedAt: ""},
	{ID: "gemini-2.5-flash-image", Type: "model", DisplayName: "Gemini 2.5 Flash Image", CreatedAt: ""},
	{ID: "gemini-2.5-pro", Type: "model", DisplayName: "Gemini 2.5 Pro", CreatedAt: ""},
	{ID: "gemini-3-flash-preview", Type: "model", DisplayName: "Gemini 3 Flash Preview", CreatedAt: ""},
	{ID: "gemini-3-pro-preview", Type: "model", DisplayName: "Gemini 3 Pro Preview", CreatedAt: ""},
	{ID: "gemini-3.1-pro-preview", Type: "model", DisplayName: "Gemini 3.1 Pro Preview", CreatedAt: ""},
	{ID: "gemini-3.1-flash-image", Type: "model", DisplayName: "Gemini 3.1 Flash Image", CreatedAt: ""},
}

// DefaultTestModel is the default model to preselect in test flows.
const DefaultTestModel = "gemini-2.0-flash"
