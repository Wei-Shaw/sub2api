package kiro

type Model struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
}

var DefaultModels = []Model{
	{ID: "claude-opus-4-8", Type: "model", DisplayName: "Claude Opus 4.8"},
	{ID: "claude-opus-4-8-thinking", Type: "model", DisplayName: "Claude Opus 4.8 (Thinking)"},
	{ID: "claude-opus-4-7", Type: "model", DisplayName: "Claude Opus 4.7"},
	{ID: "claude-opus-4-7-thinking", Type: "model", DisplayName: "Claude Opus 4.7 (Thinking)"},
	{ID: "claude-opus-4-6", Type: "model", DisplayName: "Claude Opus 4.6"},
	{ID: "claude-opus-4-6-thinking", Type: "model", DisplayName: "Claude Opus 4.6 (Thinking)"},
	{ID: "claude-sonnet-4-6", Type: "model", DisplayName: "Claude Sonnet 4.6"},
	{ID: "claude-sonnet-4-6-thinking", Type: "model", DisplayName: "Claude Sonnet 4.6 (Thinking)"},
	{ID: "claude-opus-4-5-20251101", Type: "model", DisplayName: "Claude Opus 4.5"},
	{ID: "claude-opus-4-5-20251101-thinking", Type: "model", DisplayName: "Claude Opus 4.5 (Thinking)"},
	{ID: "claude-sonnet-4-5-20250929", Type: "model", DisplayName: "Claude Sonnet 4.5"},
	{ID: "claude-sonnet-4-5-20250929-thinking", Type: "model", DisplayName: "Claude Sonnet 4.5 (Thinking)"},
	{ID: "claude-haiku-4-5-20251001", Type: "model", DisplayName: "Claude Haiku 4.5"},
	{ID: "claude-haiku-4-5-20251001-thinking", Type: "model", DisplayName: "Claude Haiku 4.5 (Thinking)"},
}
