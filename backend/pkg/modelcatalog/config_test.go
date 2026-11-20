package modelcatalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogUIContractRoundTrip(t *testing.T) {
	raw := []byte(`{"library":[{"slug":"a","kind":"chat","input_modalities":[],"image_edit":false},{"slug":"b","kind":"image"}],
	"presets":[{"id":"one","name":"One","chat_models":["a"],"image_models":[],"default_chat_model":"a"}],
	"group_catalogs":[{"group_id":1,"preset_id":"one"},{"group_id":2,"chat_models":[]},{"group_id":3,"image_models":["b"]}]}`)
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(b)
	if err != nil || !reflect.DeepEqual(c, d) {
		t.Fatalf("normalization lost intent: %s, %v", b, err)
	}
	if d.SelectionForGroup(1) != &d.Presets[0].Selection || d.SelectionForGroup(2).ChatModels == nil ||
		d.SelectionForGroup(3).ChatModels != nil || d.ExplicitOnly(4) {
		t.Fatal("incorrect group binding")
	}
}

func TestCatalogUIContractRejectsAmbiguity(t *testing.T) {
	for _, raw := range []string{
		`{"group_catalogs":null}`, `{"group_catalogs":[{"group_id":1,"chat_models":null}]}`,
		`{"library":[{"kind":"chat","slug":"bad *"}]}`, `{"library":[{"kind":"video","slug":"a"}]}`,
		`{"library":[{"kind":"chat","slug":"a","unknown":false}]}`,
		`{"library":[{"kind":"chat","slug":"a"},{"kind":"image","slug":"a"}]}`,
		`{"presets":[{"id":"x","name":"X","chat_models":[]}]}`,
		`{"group_catalogs":[{"group_id":1,"preset_id":"missing"}]}`,
		`{"presets":[{"id":"x","name":"X","chat_models":[],"image_models":[]}],"group_catalogs":[{"group_id":1,"preset_id":"x","image_models":[]}]}`,
		`{"group_catalogs":[{"group_id":1},{"group_id":1}]}`,
		`{"group_catalogs":[{"group_id":0}]}`,
		`{"group_catalogs":[{"group_id":1,"chat_models":["a","a"]}]}`,
		`{"group_catalogs":[{"group_id":1,"chat_models":["a"],"image_models":["a"]}]}`,
		`{"group_catalogs":[{"group_id":1,"chat_models":["a"],"default_chat_model":"b"}]}`,
		`{"library":[{"kind":"image","slug":"a"}],"group_catalogs":[{"group_id":1,"chat_models":["a"]}]}`,
	} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}

func TestCatalogExactUpstreamAlias(t *testing.T) {
	const id = "GPT-6 Astra"
	raw := []byte(`{"library":[{"slug":"GPT-6 Astra","kind":"chat"}],"group_catalogs":[{"group_id":2,"chat_models":["GPT-6 Astra"],"default_chat_model":"GPT-6 Astra"}]}`)
	c, err := Parse(raw)
	if err != nil || c.Library[0].Slug != id || c.SelectionForGroup(2).ChatModels[0] != id {
		t.Fatalf("lost upstream ID: %v", err)
	}
	for _, bad := range []string{" " + id, id + " ", "GPT-6  Astra", "GPT-6\tAstra", "GPT-6\nAstra", "GPT-6 *", strings.Repeat("a", 161)} {
		if validCatalogID(bad) {
			t.Errorf("unexpectedly accepted %q", bad)
		}
	}
}

func TestOverrideIDUsesExactCatalogAlias(t *testing.T) {
	const id = "GPT-6 Astra"
	raw := strings.ReplaceAll(string(fixture(t)), "gpt-future-test", id)
	c, err := Parse([]byte(raw))
	if err != nil || c.Models[0].Slug() != id {
		t.Fatalf("visible override lost upstream alias: %v", err)
	}
	for _, bad := range []string{" GPT-6 Astra", "GPT-6  Astra", "GPT-6*", "GPT-6\tAstra", strings.Repeat("a", 161)} {
		descriptor, _ := json.Marshal(map[string]string{"slug": bad})
		entry := Model{Hidden: true, GroupIDs: []int64{7}, Descriptor: descriptor}
		var rules []Model
		for i := 1; i <= 16; i++ {
			rules = append(rules, Model{Hidden: true, GroupIDs: []int64{int64(i)}, Descriptor: json.RawMessage(`{"slug":"GPT-6 Astra"}`)})
		}
		input, _ := json.Marshal(Config{SchemaVersion: 1, Models: append(rules, entry)})
		_, err := Parse(input)
		if err == nil || !strings.Contains(err.Error(), "第 17 项") || !strings.Contains(err.Error(), "分组 [7]") {
			t.Errorf("wrong diagnostic for %q: %v", bad, err)
		}
	}
}

func fixture(t *testing.T) []byte {
	t.Helper()
	raw, err := json.Marshal(Config{SchemaVersion: 1, Models: []Model{{GroupIDs: []int64{2}, MinClientVersion: "0.160.0", MaxClientVersion: "0.164.9", Descriptor: json.RawMessage(`{"slug":"gpt-future-test","display_name":"Future","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"none"},{"effort":"medium"},{"effort":"max"}],"shell_type":"unified_exec","visibility":"list","base_instructions":"Test instructions","new_tool_field":{"enabled":true}}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func TestConfigPreservesFutureFieldsAndVersionSelection(t *testing.T) {
	c, err := Parse(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if !c.Models[0].Matches(2, "0.164.0-alpha.11.1") || c.Models[0].Matches(3, "0.164.0") || c.Models[0].Matches(2, "0.159.9") || c.Models[0].Matches(2, "") {
		t.Fatal("version/group boundary")
	}
	var d map[string]json.RawMessage
	_ = json.Unmarshal(c.Models[0].Descriptor, &d)
	if string(d["new_tool_field"]) != `{"enabled":true}` {
		t.Fatal("lost unknown fields")
	}
}
func TestConfigRejectsUnsafeOrAmbiguousPublication(t *testing.T) {
	var c Config
	_ = json.Unmarshal(fixture(t), &c)
	cases := map[string]func(*Config){
		"no group":                 func(c *Config) { c.Models[0].GroupIDs = nil },
		"invalid explicit group":   func(c *Config) { c.ExplicitOnlyGroupIDs = []int64{0} },
		"duplicate explicit group": func(c *Config) { c.ExplicitOnlyGroupIDs = []int64{2, 2} },
		"overlap":                  func(c *Config) { c.Models = append(c.Models, c.Models[0]) },
		"version":                  func(c *Config) { c.Models[0].MaxClientVersion = "0.159.0" },
		"missing instructions": func(c *Config) {
			var d map[string]any
			_ = json.Unmarshal(c.Models[0].Descriptor, &d)
			delete(d, "base_instructions")
			c.Models[0].Descriptor, _ = json.Marshal(d)
		},
		"default effort": func(c *Config) {
			var d map[string]any
			_ = json.Unmarshal(c.Models[0].Descriptor, &d)
			d["default_reasoning_level"] = "ultra"
			c.Models[0].Descriptor, _ = json.Marshal(d)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			var next Config
			_ = json.Unmarshal(fixture(t), &next)
			change(&next)
			raw, _ := json.Marshal(next)
			if _, err := Parse(raw); err == nil {
				t.Fatal("accepted invalid publication")
			}
		})
	}
	for _, raw := range []string{`null`, `{"models":[],"unexpected":true}`, `{} {}`} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestConfigHideOnlyRuleCannotBecomeInvalidVisibleDescriptor(t *testing.T) {
	raw := []byte(`{"schema_version":1,"models":[{"group_ids":[2],"hidden":true,"descriptor":{"slug":"gpt-6-sol"}}]}`)
	config, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	config.Models[0].Hidden = false
	raw, _ = json.Marshal(config)
	if _, err = Parse(raw); err == nil {
		t.Fatal("hide-only rule became an incomplete visible descriptor")
	}
}
