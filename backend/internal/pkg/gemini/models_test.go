package gemini

import "testing"

func TestDefaultModels_ContainsImageModels(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	byName := make(map[string]Model, len(models))
	for _, model := range models {
		byName[model.Name] = model
REDACTED

	required := []string{
		"models/gemini-2.5-flash-image",
		"models/gemini-3.1-flash-image",
REDACTED

	for _, name := range required {
		model, ok := byName[name]
		if !ok {
			t.Fatalf("expected fallback model %q to exist", name)
	REDACTED
		if len(model.SupportedGenerationMethods) == 0 {
			t.Fatalf("expected fallback model %q to advertise generation methods", name)
	REDACTED
REDACTED
REDACTED
