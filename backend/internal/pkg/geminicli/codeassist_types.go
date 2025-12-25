package geminicli

// LoadCodeAssistRequest matches done-hub's internal Code Assist call.
type LoadCodeAssistRequest struct {
	Metadata LoadCodeAssistMetadata `json:"metadata"`
REDACTED

type LoadCodeAssistMetadata struct {
	IDEType    string `json:"ideType"`
	Platform   string `json:"platform"`
	PluginType string `json:"pluginType"`
REDACTED

type LoadCodeAssistResponse struct {
	CurrentTier             string        `json:"currentTier,omitempty"`
	CloudAICompanionProject string        `json:"cloudaicompanionProject,omitempty"`
	AllowedTiers            []AllowedTier `json:"allowedTiers,omitempty"`
REDACTED

type AllowedTier struct {
	ID        string `json:"id"`
	IsDefault bool   `json:"isDefault,omitempty"`
REDACTED

type OnboardUserRequest struct {
	TierID   string                 `json:"tierId"`
	Metadata LoadCodeAssistMetadata `json:"metadata"`
REDACTED

type OnboardUserResponse struct {
	Done     bool                   `json:"done"`
	Response *OnboardUserResultData `json:"response,omitempty"`
	Name     string                 `json:"name,omitempty"`
REDACTED

type OnboardUserResultData struct {
	CloudAICompanionProject any `json:"cloudaicompanionProject,omitempty"`
REDACTED
