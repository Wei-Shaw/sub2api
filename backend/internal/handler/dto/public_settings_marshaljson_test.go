//go:build unit

package dto

import (
	"encoding/json"
	"testing"
)

func TestPublicSettings_MarshalJSON_FlattensPluginFlags(t *testing.T) {
	p := PublicSettings{
		SiteName: "Sub2API",
		PluginFlags: map[string]any{
			"payment_enabled": true,
			"site_name":       "should-be-shadowed-by-host", // collides
			"plugin_only":     "hello",
			"some_number":     42.5,
		},
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := got["site_name"].(string); !ok || v != "Sub2API" {
		t.Fatalf("site_name should be host value, got %v", got["site_name"])
	}
	if v, ok := got["payment_enabled"].(bool); !ok || v != true {
		t.Fatalf("payment_enabled missing/wrong, got %v", got["payment_enabled"])
	}
	if v, ok := got["plugin_only"].(string); !ok || v != "hello" {
		t.Fatalf("plugin_only missing/wrong, got %v", got["plugin_only"])
	}
	if v, ok := got["some_number"].(float64); !ok || v != 42.5 {
		t.Fatalf("some_number missing/wrong, got %v", got["some_number"])
	}
	if _, ok := got["plugin_flags"]; ok {
		t.Fatalf("plugin_flags should be flattened away, got %v", got["plugin_flags"])
	}
}
