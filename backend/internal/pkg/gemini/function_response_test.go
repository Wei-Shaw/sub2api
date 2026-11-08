package gemini

import "testing"

func TestSupportsMultimodalFunctionResponses(t *testing.T) {
	for model, want := range map[string]bool{
		"gemini-3-pro": true, "gemini-3.8-flash": true, "gemini-4-argon": true,
		"models/gemini-3-flash-preview": true, "gemini-2.5-flash": false,
		"claude-sonnet-4-6": false, "client-alias": false, "": false,
	} {
		t.Run(model, func(t *testing.T) {
			if got := SupportsMultimodalFunctionResponses(model); got != want {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}
