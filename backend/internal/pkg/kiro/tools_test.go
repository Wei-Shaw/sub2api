package kiro

import (
	"strings"
	"testing"
)

func TestSanitizeToolName_BasicCases(t *testing.T) {
	cases := map[string]string{
		"list_files":             "listFiles",
		"get-weather":            "getWeather",
		"already_camelCase":      "alreadyCamelCase",
		"mcp__server__do_thing":  "mcpServerDoThing",
		"":                       "tool",
		"__":                     "tool",
		"weather":                "weather",
		"OneTwoThree":            "oneTwoThree",
	}
	for in, want := range cases {
		got := SanitizeToolName(in)
		if got != want {
			t.Errorf("SanitizeToolName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortenToolName_NoOpUnderLimit(t *testing.T) {
	short := strings.Repeat("a", MaxToolNameLen)
	if ShortenToolName(short) != short {
		t.Fatal("should not modify name <= 64")
	}
}

func TestShortenToolName_TrimsHardWhenNoNamespace(t *testing.T) {
	long := strings.Repeat("a", MaxToolNameLen+10)
	got := ShortenToolName(long)
	if len(got) != MaxToolNameLen {
		t.Fatalf("len = %d", len(got))
	}
}

func TestShortenToolName_DropsMCPServerSegment(t *testing.T) {
	// mcp__longServerName__tool — when long enough, the server segment drops.
	long := "mcp__" + strings.Repeat("s", 100) + "__doIt"
	got := ShortenToolName(long)
	if got != "mcp__doIt" {
		t.Fatalf("got %q", got)
	}
}

func TestEnsureObjectSchema_AddsType(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{}}
	out := EnsureObjectSchema(schema).(map[string]any)
	if out["type"] != "object" {
		t.Fatal("expected type=object")
	}
}

func TestEnsureObjectSchema_NonMapBecomesObject(t *testing.T) {
	out := EnsureObjectSchema("not a schema").(map[string]any)
	if out["type"] != "object" {
		t.Fatal("expected fallback object schema")
	}
}

func TestEnsureObjectSchema_DropsNullRequired(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": nil,
	}
	out := EnsureObjectSchema(schema).(map[string]any)
	if _, ok := out["required"]; ok {
		t.Fatal("required: nil should be dropped")
	}
}

func TestEnsureObjectSchema_DropsEmptyRequired(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{},
	}
	out := EnsureObjectSchema(schema).(map[string]any)
	if _, ok := out["required"]; ok {
		t.Fatal("required: [] should be dropped")
	}
}

func TestEnsureObjectSchema_CleansNestedProperties(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"foo": map[string]any{
				"type":     "object",
				"required": nil,
				"properties": map[string]any{
					"bar": map[string]any{"type": "string"},
				},
			},
		},
	}
	out := EnsureObjectSchema(schema).(map[string]any)
	foo := out["properties"].(map[string]any)["foo"].(map[string]any)
	if _, ok := foo["required"]; ok {
		t.Fatal("nested required: nil should be dropped")
	}
}

func TestTruncateToolDescription_NoOpUnderLimit(t *testing.T) {
	desc := strings.Repeat("a", 100)
	if TruncateToolDescription(desc) != desc {
		t.Fatal("short desc should not be modified")
	}
}

func TestTruncateToolDescription_Truncates(t *testing.T) {
	desc := strings.Repeat("a", MaxToolDescLen+50)
	got := TruncateToolDescription(desc)
	if !strings.HasSuffix(got, "...") {
		t.Fatal("truncated desc should end with ...")
	}
	if len(got) != MaxToolDescLen+3 {
		t.Fatalf("len = %d, want %d", len(got), MaxToolDescLen+3)
	}
}
