package geminicli

import "testing"

func TestDefaultModels_ContainsImageModels(t *testing.T) {
	t.Parallel()

	byID := make(map[string]Model, len(DefaultModels))
	for _, model := range DefaultModels {
		byID[model.ID] = model
REDACTED

	required := []string{
		"gemini-2.5-flash-image",
		"gemini-3.1-flash-image",
REDACTED

	for _, id := range required {
		if _, ok := byID[id]; !ok {
			t.Fatalf("expected curated Gemini model %q to exist", id)
	REDACTED
REDACTED
REDACTED
