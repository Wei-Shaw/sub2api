package typesafe

import (
	"strings"
	"testing"
)

func TestIsCommandCodeJevModel(t *testing.T) {
	for model, want := range map[string]bool{
		"typesafe/jev":        true,
		"typesafe/jev-1.14.0": true,
		"typesafe/jev-2":      true,
		"typesafe/jev-":       false,
		"typesafe/jev-latest": false,
		"typesafe/jev-1.x":    false,
		"jev-latest":          false,
		"jev":                 false,
		"typesafe/Jev":        false,
		"other/jev":           false,
	} {
		if got := IsCommandCodeJevModel(model); got != want {
			t.Errorf("IsCommandCodeJevModel(%q) = %v, want %v", model, got, want)
		}
	}
}

func TestValidateSystemOneRequestModelsPerUpstream(t *testing.T) {
	body := func(model string) []byte {
		return []byte(`{"model":"` + model + `","state":"s","questions":{"q":{"type":"noul"}}}`)
	}
	// TypeSafe's own API keeps accepting only jev-latest.
	if _, err := ValidateSystemOneRequest(body("typesafe/jev")); err == nil {
		t.Fatal("TypeSafe validation must reject Command Code model ids")
	}
	if m, err := ValidateSystemOneRequest(body("jev-latest")); err != nil || m != "jev-latest" {
		t.Fatalf("jev-latest: %q %v", m, err)
	}
	for _, model := range []string{"typesafe/jev", "typesafe/jev-1.14.0", "jev-latest"} {
		if m, err := ValidateSystemOneRequestAnyUpstream(body(model)); err != nil || m != model {
			t.Fatalf("%s: %q %v", model, m, err)
		}
	}
	_, err := ValidateSystemOneRequestAnyUpstream(body("deepseek/deepseek-v4-flash"))
	if err == nil || !strings.Contains(err.Error(), CommandCodeJevModel) {
		t.Fatalf("chat model must be rejected with the accepted ids named: %v", err)
	}
}

func TestNewCommandCodeSystemOneRequestPath(t *testing.T) {
	req, err := NewCommandCodeSystemOneRequest(t.Context(), "https://api.commandcode.ai/provider/v1/", "k", []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := req.URL.String(); got != "https://api.commandcode.ai/provider/v1/systemone" {
		t.Fatalf("url = %s", got)
	}
	if req.Header.Get("Authorization") != "Bearer k" {
		t.Fatal("missing bearer key")
	}
}
