package managementmcp

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestToolCatalogRouteAndSchemaBoundaries(t *testing.T) {
	names := map[string]bool{}
	for _, spec := range ToolCatalog() {
		t.Run(spec.Name, func(t *testing.T) {
			if names[spec.Name] || !regexp.MustCompile(`^(user|admin)_[a-z0-9_]+$`).MatchString(spec.Name) {
				t.Fatalf("duplicate or invalid tool name: %q", spec.Name)
			}
			names[spec.Name] = true
			if spec.Description == "" || !slices.Contains([]string{"GET", "POST", "PUT", "DELETE"}, spec.Method) {
				t.Fatal("tool must have a description and supported method")
			}
			if !strings.HasPrefix(spec.Path, "/api/v1/") || strings.ContainsAny(spec.Path, "?*#") || strings.Contains(spec.Path, "..") {
				t.Fatalf("route must be a fixed local API path: %q", spec.Path)
			}
			if spec.Admin != strings.HasPrefix(spec.Path, "/api/v1/admin/") || spec.Admin != strings.HasPrefix(spec.Name, "admin_") {
				t.Fatal("tool role does not match its route")
			}
			if spec.ReadOnly != (spec.Method == "GET") || (spec.ReadOnly && spec.Destructive) {
				t.Fatal("tool mutability metadata does not match its route")
			}
			checkCatalogSchema(t, spec.InputSchema)
			properties, ok := spec.InputSchema["properties"].(map[string]any)
			if !ok {
				t.Fatal("input schema properties must be an object")
			}
			required, _ := spec.InputSchema["required"].([]string)
			for name := range properties {
				if !slices.Contains([]string{"id", "query", "body", "idempotency_key"}, name) {
					t.Fatalf("unexpected top-level field %q", name)
				}
			}
			pathParameters := regexp.MustCompile(`:([a-z_]+)`).FindAllStringSubmatch(spec.Path, -1)
			for _, match := range pathParameters {
				if match[1] != "id" || !slices.Contains(required, match[1]) {
					t.Fatalf("path parameter %q must be a required ID", match[1])
				}
			}
			if _, exists := properties["id"]; exists != (len(pathParameters) > 0) {
				t.Fatal("ID input and route placeholder disagree")
			}
			if body, exists := properties["body"]; exists {
				bodySchema, ok := body.(map[string]any)
				if !ok || spec.ReadOnly || !slices.Contains(required, "body") || bodySchema["type"] != "object" {
					t.Fatal("write bodies must be required objects")
				}
			}
			if query, exists := properties["query"].(map[string]any); exists {
				queryFields, ok := query["properties"].(map[string]any)
				if !ok {
					t.Fatal("query schema properties must be an object")
				}
				if size, exists := queryFields["page_size"].(map[string]any); exists && size["maximum"] != int64(100) {
					t.Fatal("list page size must be bounded to 100")
				}
				if !spec.Admin {
					for _, name := range []string{"user_id", "account_id"} {
						if _, exists := queryFields[name]; exists {
							t.Fatalf("user tool accepts cross-user filter %q", name)
						}
					}
				}
			}
			if _, err := json.Marshal(spec.InputSchema); err != nil {
				t.Fatalf("schema is not JSON: %v", err)
			}
		})
	}
}

func checkCatalogSchema(t *testing.T, schema map[string]any) {
	t.Helper()
	switch schema["type"] {
	case "object":
		if schema["additionalProperties"] != false {
			t.Fatal("all objects must reject unknown fields")
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			t.Fatal("object lacks explicit properties")
		}
		for name, field := range properties {
			child, ok := field.(map[string]any)
			if !ok {
				t.Fatalf("field %q lacks a typed schema", name)
			}
			checkCatalogSchema(t, child)
		}
		if required, ok := schema["required"].([]string); ok {
			for _, name := range required {
				if _, exists := properties[name]; !exists {
					t.Fatalf("required field %q is not declared", name)
				}
			}
		}
	case "array":
		child, ok := schema["items"].(map[string]any)
		if !ok || schema["maxItems"] == nil {
			t.Fatal("arrays must have typed items and size limits")
		}
		checkCatalogSchema(t, child)
	case "integer", "number", "string", "boolean":
	default:
		t.Fatalf("unsupported or missing schema type: %v", schema["type"])
	}
}

func TestToolCatalogDoesNotExposeCredentialOrIdentityManagement(t *testing.T) {
	for _, spec := range ToolCatalog() {
		if spec.RevealCreatedKey != (spec.Name == "user_create_api_key") {
			t.Fatalf("unexpected secret disclosure policy for %s", spec.Name)
		}
		for _, forbidden := range []string{"admin-api-key", "/auth", "/password", "/totp", "/passkeys", "/payment", "/backup", "/data"} {
			if strings.Contains(spec.Path, forbidden) {
				t.Fatalf("%s exposes sensitive route %s", spec.Name, spec.Path)
			}
		}
		properties := catalogTestObject(t, spec.InputSchema["properties"])
		if body, ok := properties["body"].(map[string]any); ok {
			fields := catalogTestObject(t, body["properties"])
			for _, forbidden := range []string{"credentials", "extra", "password", "role", "email", "custom_key", "proxy_id", "url"} {
				if _, exists := fields[forbidden]; exists {
					t.Fatalf("%s accepts sensitive field %s", spec.Name, forbidden)
				}
			}
		}
	}
}

func TestToolCatalogSchemasAreIndependent(t *testing.T) {
	first := ToolCatalog()
	first[0].InputSchema["additionalProperties"] = true
	for _, spec := range first {
		if spec.Name == "admin_create_group" {
			body := catalogTestObject(t, catalogTestObject(t, spec.InputSchema["properties"])["body"])
			catalogTestObject(t, body["properties"])["password"] = catalogText("Unexpected", 100)
		}
	}
	for _, spec := range ToolCatalog() {
		if spec.InputSchema["additionalProperties"] != false {
			t.Fatal("catalog calls share schema state")
		}
		if spec.Name == "admin_update_group" || spec.Name == "admin_create_group" {
			body := catalogTestObject(t, catalogTestObject(t, spec.InputSchema["properties"])["body"])
			if _, exists := catalogTestObject(t, body["properties"])["password"]; exists {
				t.Fatal("group schemas share mutable fields")
			}
		}
	}
}

func catalogTestObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected schema object, got %T", value)
	}
	return object
}
