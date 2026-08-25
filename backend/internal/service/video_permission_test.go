package service

import "testing"

func TestGroupAllowsVideoGeneration(t *testing.T) {
	if !GroupAllowsVideoGeneration(nil) {
		t.Fatal("ungrouped keys should retain media compatibility")
	}
	if !GroupAllowsVideoGeneration(&Group{Platform: PlatformOpenAI}) {
		t.Fatal("OpenAI video groups should not depend on image permission")
	}
	if GroupAllowsVideoGeneration(&Group{Platform: PlatformGrok, AllowImageGeneration: false}) {
		t.Fatal("Grok video groups should retain the existing media permission")
	}
}
