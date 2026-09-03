package pluginv1

import (
	"encoding/json"
	"os"
	"testing"
)

func TestManifestSchemaIsValidJSON(t *testing.T) {
	raw, err := os.ReadFile("manifest.schema.json")
	if err != nil {
		t.Fatalf("读取插件清单 Schema 失败: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("插件清单 Schema 不是有效 JSON: %v", err)
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("插件清单 Schema 版本不符合预期: %v", schema["$schema"])
	}
}
