// Package gemini provides minimal fallback model metadata for Gemini native endpoints.
// It is used when upstream model listing is unavailable (e.g. OAuth token missing AI Studio scopes).
package gemini

type Model struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName,omitempty"`
	Description                string   `json:"description,omitempty"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods,omitempty"`
REDACTED

type ModelsListResponse struct {
	Models []Model `json:"models"`
REDACTED

func DefaultModels() []Model {
	methods := []string{"generateContent", "streamGenerateContent"REDACTED
	return []Model{
		{Name: "models/gemini-2.0-flash", SupportedGenerationMethods: methodsREDACTED,
		{Name: "models/gemini-2.5-flash", SupportedGenerationMethods: methodsREDACTED,
		{Name: "models/gemini-2.5-pro", SupportedGenerationMethods: methodsREDACTED,
		{Name: "models/gemini-3-flash-preview", SupportedGenerationMethods: methodsREDACTED,
		{Name: "models/gemini-3-pro-preview", SupportedGenerationMethods: methodsREDACTED,
		{Name: "models/gemini-3.1-pro-preview", SupportedGenerationMethods: methodsREDACTED,
REDACTED
REDACTED

func FallbackModelsList() ModelsListResponse {
	return ModelsListResponse{Models: DefaultModels()REDACTED
REDACTED

func FallbackModel(model string) Model {
	methods := []string{"generateContent", "streamGenerateContent"REDACTED
	if model == "" {
		return Model{Name: "models/unknown", SupportedGenerationMethods: methodsREDACTED
REDACTED
	if len(model) >= 7 && model[:7] == "models/" {
		return Model{Name: model, SupportedGenerationMethods: methodsREDACTED
REDACTED
	return Model{Name: "models/" + model, SupportedGenerationMethods: methodsREDACTED
REDACTED
