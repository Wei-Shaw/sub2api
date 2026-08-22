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

func TestGoogleOneModels_ExcludeUnsupportedNewAndImageModels(t *testing.T) {
	t.Parallel()

	mapping := GoogleOneModelMapping()
	for _, id := range []string{"gemini-2.0-flash", "gemini-2.5-flash", "gemini-2.5-pro"REDACTED {
		if mapping[id] != id {
			t.Fatalf("expected Google One model %q to map to itself", id)
	REDACTED
REDACTED
	for _, id := range []string{"gemini-2.5-flash-image", "gemini-3.1-flash-image", "gemini-3.5-flash"REDACTED {
		if _, ok := mapping[id]; ok {
			t.Fatalf("did not expect unsupported Google One model %q", id)
	REDACTED
REDACTED

	mapping["gemini-2.5-flash"] = "mutated"
	if GoogleOneModelMapping()["gemini-2.5-flash"] != "gemini-2.5-flash" {
		t.Fatal("GoogleOneModelMapping must return a defensive copy")
REDACTED
REDACTED
