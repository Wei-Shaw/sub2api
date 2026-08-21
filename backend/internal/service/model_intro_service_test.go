package service

import "testing"

func TestNormalizeOutputFieldsMaxChars(t *testing.T) {
	limit := 12
	fields := []OutputFieldSpec{{Key: "message", Type: "string", MaxChars: &limit}}
	if err := normalizeOutputFields(fields); err != nil {
		t.Fatalf("expected string max_chars to be accepted: %v", err)
	}
	if fields[0].MaxChars == nil || *fields[0].MaxChars != limit {
		t.Fatalf("max_chars was not preserved")
	}

	withoutLimit := []OutputFieldSpec{{Key: "message", Type: "string"}}
	if err := normalizeOutputFields(withoutLimit); err != nil {
		t.Fatalf("omitted max_chars should mean unlimited: %v", err)
	}

	zero := 0
	if err := normalizeOutputFields([]OutputFieldSpec{{Key: "message", Type: "string", MaxChars: &zero}}); err == nil {
		t.Fatal("expected zero max_chars to be rejected")
	}

	if err := normalizeOutputFields([]OutputFieldSpec{{Key: "count", Type: "number", MaxChars: &limit}}); err == nil {
		t.Fatal("expected max_chars on non-string field to be rejected")
	}
}

func TestNormalizeNestedOutputSchemaMaxChars(t *testing.T) {
	limit := 8
	fields := []OutputFieldSpec{
		{
			Key:  "result",
			Type: "object",
			Properties: map[string]any{
				"message": map[string]any{"value": "example", "max_chars": float64(limit)},
			},
		},
	}
	if err := normalizeOutputFields(fields); err != nil {
		t.Fatalf("expected nested string max_chars to be accepted: %v", err)
	}
	nested, ok := fields[0].Properties["message"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested message schema, got %T", fields[0].Properties["message"])
	}
	if got := nested["max_chars"]; got != limit {
		t.Fatalf("expected nested max_chars normalized to int, got %T %v", got, got)
	}
}
