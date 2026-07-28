package service

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestValidateAgentBodyContractEnforcesSchemaConstraints(t *testing.T) {
	tests := []struct {
		name    string
		schema  map[string]any
		value   any
		wantErr bool
	}{
		{name: "numeric minimum", schema: map[string]any{"type": "number", "minimum": 1}, value: float64(0), wantErr: true},
		{name: "numeric maximum", schema: map[string]any{"type": "number", "maximum": 10}, value: float64(11), wantErr: true},
		{name: "fractional integer", schema: map[string]any{"type": "integer"}, value: 1.5, wantErr: true},
		{name: "integer", schema: map[string]any{"type": "integer", "minimum": 1}, value: float64(2)},
		{name: "array minimum", schema: map[string]any{"type": "array", "minimum": 2, "items": map[string]any{"type": "string"}}, value: []any{"one"}, wantErr: true},
		{name: "array maximum", schema: map[string]any{"type": "array", "maximum": 1, "items": map[string]any{"type": "string"}}, value: []any{"one", "two"}, wantErr: true},
		{name: "string minimum", schema: map[string]any{"type": "string", "minimum": 2}, value: "a", wantErr: true},
		{name: "string maximum", schema: map[string]any{"type": "string", "maximum": 2}, value: "abc", wantErr: true},
		{name: "invalid date time", schema: map[string]any{"type": "string", "format": "date-time"}, value: "2026-99-99", wantErr: true},
		{name: "valid date time", schema: map[string]any{"type": "string", "format": "date-time"}, value: "2026-07-23T11:00:00Z"},
		{name: "unknown object field", schema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}}, value: map[string]any{"unknown": true}, wantErr: true},
		{name: "typed additional property", schema: map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}, value: map[string]any{"key": float64(1)}, wantErr: true},
		{name: "valid typed additional property", schema: map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}}, value: map[string]any{"key": "value"}},
		{name: "untyped additional property", schema: map[string]any{"type": "object", "additionalProperties": map[string]any{}}, value: map[string]any{"key": []any{float64(1)}}},
		{name: "nullable field", schema: map[string]any{"type": "number", "minimum": 1}, value: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAgentBodyContract(test.schema, test.value, "body")
			if (err != nil) != test.wantErr {
				t.Fatalf("validateAgentBodyContract() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateAgentOperationSemanticsCoversAuditedConditionalRules(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   map[string]any
	}{
		{name: "batch users need targets", method: "POST", path: "/admin/users/batch-concurrency", body: map[string]any{"all": false}},
		{name: "affiliate rate required", method: "POST", path: "/admin/affiliates/users/batch-rate", body: map[string]any{"clear": false}},
		{name: "subscription reset needs true window", method: "POST", path: "/admin/subscriptions/12/reset-quota", body: map[string]any{"daily": false, "weekly": false, "monthly": false}},
		{name: "redeem expiry conflict", method: "POST", path: "/admin/redeem-codes/generate", body: map[string]any{"expires_at": "2030-01-01T00:00:00Z", "expires_in_days": float64(30)}},
		{name: "subscription redeem validity", method: "POST", path: "/admin/redeem-codes/create-and-redeem", body: map[string]any{"type": "subscription", "group_id": float64(1), "validity_days": float64(0)}},
		{name: "channel pricing target", method: "PUT", path: "/admin/channels/3", body: map[string]any{"account_stats_pricing_rules": []any{map[string]any{"pricing": []any{map[string]any{"billing_mode": "token"}}}}}},
		{name: "group hold below discount", method: "PUT", path: "/admin/groups/4", body: map[string]any{"batch_image_discount_multiplier": 0.8, "batch_image_hold_multiplier": 0.7}},
		{name: "credential discriminator", method: "POST", path: "/admin/accounts/batch-update-credentials", body: map[string]any{"field": "intercept_warmup_requests", "value": "true"}},
		{name: "cleanup reversed range", method: "POST", path: "/admin/usage/cleanup-tasks", body: map[string]any{"start_date": "2026-02-02", "end_date": "2026-02-01"}},
		{name: "invalid plan currency", method: "POST", path: "/admin/payment/plans", body: map[string]any{"currency": "US"}},
		{name: "recharge precision", method: "PUT", path: "/admin/payment/config", body: map[string]any{"recharge_fee_rate": 1.234}},
		{name: "rate threshold range", method: "POST", path: "/admin/ops/alert-rules", body: map[string]any{"metric_type": "error_rate", "threshold": 101.0}},
		{name: "ollama refresh interval gap", method: "PUT", path: "/admin/accounts/ollama-cloud-usage/settings", body: map[string]any{"interval_minutes": float64(10)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateAgentOperationSemantics(test.method, test.path, test.body); err == nil {
				t.Fatal("validateAgentOperationSemantics() unexpectedly accepted invalid payload")
			}
		})
	}
}

func TestValidateAgentOperationQuerySemanticsRejectsInvalidRanges(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		query map[string]any
	}{
		{name: "reversed dates", query: map[string]any{"start_date": "2030-01-03", "end_date": "2030-01-02"}},
		{name: "invalid timestamps", query: map[string]any{"start_at": "not-a-date", "end_at": "2030-01-02"}},
		{name: "reversed duration", query: map[string]any{"min_duration_ms": float64(200), "max_duration_ms": float64(100)}},
		{name: "conflicting token pagination", path: "/admin/ops/dashboard/openai-token-stats", query: map[string]any{"top_n": float64(10), "page": float64(1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateAgentOperationQuerySemantics(test.path, test.query); err == nil {
				t.Fatal("validateAgentOperationQuerySemantics() unexpectedly accepted invalid query")
			}
		})
	}
}

func TestMergeAgentSingletonPutBodyPreservesCurrentNonSensitiveFields(t *testing.T) {
	schema := map[string]any{"type": "object", "properties": map[string]any{
		"level":   map[string]any{"type": "string"},
		"caller":  map[string]any{"type": "boolean"},
		"api_key": map[string]any{"type": "string"},
		"count":   map[string]any{"type": "integer"},
	}}
	before := map[string]any{"level": "warn", "caller": true, "api_key": "secret", "count": "invalid", "server_only": "ignored"}
	merged := mergeAgentSingletonPutBody(schema, before, map[string]any{"level": "error"})
	if merged["level"] != "error" || merged["caller"] != true {
		t.Fatalf("singleton merge lost requested or current values: %#v", merged)
	}
	for _, forbidden := range []string{"api_key", "count", "server_only"} {
		if _, exists := merged[forbidden]; exists {
			t.Fatalf("singleton merge retained forbidden current field %s: %#v", forbidden, merged)
		}
	}
	withUnknown := mergeAgentSingletonPutBody(schema, before, map[string]any{"unknown": true})
	if err := validateAgentBodyContract(schema, withUnknown, "body"); err == nil {
		t.Fatalf("singleton merge silently dropped an explicit unknown field: %#v", withUnknown)
	}
}

func TestAIAgentCatalogMatchesRegisteredAdminRoutes(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	registered := registeredAdminRouteKeys(t)
	catalog := make(map[string]struct{}, len(service.catalog))
	for _, operation := range service.catalog {
		catalog[operation.Key] = struct{}{}
	}
	var missing, stale []string
	for key := range registered {
		if _, exists := catalog[key]; !exists {
			missing = append(missing, key)
		}
	}
	for key := range catalog {
		if _, exists := registered[key]; !exists {
			stale = append(stale, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("Agent catalog drifted from registered admin routes\nmissing=%v\nstale=%v", missing, stale)
	}
}

func registeredAdminRouteKeys(t *testing.T) map[string]struct{} {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve contract audit test path")
	}
	routeDirectory := filepath.Join(filepath.Dir(currentFile), "..", "server", "routes")
	methods := map[string]bool{"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true}
	routes := make(map[string]struct{})
	routeFiles, err := filepath.Glob(filepath.Join(routeDirectory, "*.go"))
	if err != nil {
		t.Fatalf("list route source files: %v", err)
	}
	for _, routeFile := range routeFiles {
		if strings.HasSuffix(routeFile, "_test.go") {
			continue
		}
		filename := filepath.Base(routeFile)
		parsed, err := parser.ParseFile(token.NewFileSet(), routeFile, nil, 0)
		if err != nil {
			t.Fatalf("parse %s routes: %v", filename, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || !strings.HasPrefix(strings.ToLower(function.Name.Name), "register") || function.Name.Name == "registerAIAgentRoutes" {
				continue
			}
			prefixes := map[string]string{"admin": "/admin", "v1": ""}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				switch typed := node.(type) {
				case *ast.AssignStmt:
					for index, right := range typed.Rhs {
						if index >= len(typed.Lhs) {
							continue
						}
						left, leftOK := typed.Lhs[index].(*ast.Ident)
						call, callOK := right.(*ast.CallExpr)
						if !leftOK || !callOK || len(call.Args) == 0 {
							continue
						}
						selector, selectorOK := call.Fun.(*ast.SelectorExpr)
						if !selectorOK || selector.Sel.Name != "Group" {
							continue
						}
						parent, parentOK := selector.X.(*ast.Ident)
						if !parentOK {
							continue
						}
						segment, segmentOK := routeStringLiteral(call.Args[0])
						parentPrefix, prefixOK := prefixes[parent.Name]
						if segmentOK && prefixOK {
							prefixes[left.Name] = strings.TrimSuffix(parentPrefix, "/") + segment
						}
					}
				case *ast.CallExpr:
					selector, selectorOK := typed.Fun.(*ast.SelectorExpr)
					if !selectorOK || !methods[selector.Sel.Name] || len(typed.Args) == 0 {
						return true
					}
					receiver, receiverOK := selector.X.(*ast.Ident)
					if !receiverOK {
						return true
					}
					path, pathOK := routeStringLiteral(typed.Args[0])
					prefix, prefixOK := prefixes[receiver.Name]
					if pathOK && prefixOK {
						fullPath := strings.TrimSuffix(prefix, "/") + path
						if strings.HasPrefix(fullPath, "/admin/") {
							routes[selector.Sel.Name+":"+fullPath] = struct{}{}
						}
					}
				}
				return true
			})
		}
	}
	return routes
}

func routeStringLiteral(expression ast.Expr) (string, bool) {
	literal, ok := expression.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	return value, err == nil
}

func TestAIAgentWriteCatalogHasCompleteBodyClassification(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	writes, contracted, verifiedBodyless := 0, 0, 0
	for _, operation := range service.catalog {
		if operation.Method == "GET" {
			continue
		}
		writes++
		if len(operation.BodySchema) > 0 {
			contracted++
			continue
		}
		verifiedBodyless++
		if len(operation.BodyExample) > 0 {
			t.Errorf("bodyless operation %s still exposes a body example", operation.Key)
		}
	}
	if writes != 229 || contracted != 163 || verifiedBodyless != 66 {
		t.Fatalf("write classification = writes:%d contracts:%d bodyless:%d, want 229/163/66", writes, contracted, verifiedBodyless)
	}
}

func TestAIAgentRequestContractsCoverEveryCatalogOperation(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	var stored map[string]agentOperationContract
	if err := json.Unmarshal(agentContractsJSON, &stored); err != nil {
		t.Fatalf("decode Agent contracts: %v", err)
	}
	bodies, queries, paths := 0, 0, 0
	var violations []string
	for _, operation := range service.catalog {
		contract, exists := stored[operation.Key]
		if !exists {
			violations = append(violations, operation.Key+": missing request contract entry")
			continue
		}
		if len(contract.BodySchema) > 0 {
			bodies++
		}
		if len(contract.QuerySchema) > 0 {
			queries++
		}
		if len(contract.PathSchema) > 0 {
			paths++
		}
		pathProperties, _ := contract.PathSchema["properties"].(map[string]any)
		if len(pathProperties) != len(operation.PathParams) {
			violations = append(violations, fmt.Sprintf("%s: path schema has %d fields, catalog has %d", operation.Key, len(pathProperties), len(operation.PathParams)))
		}
		for _, name := range operation.PathParams {
			if _, exists := pathProperties[name]; !exists {
				violations = append(violations, operation.Key+": missing path parameter "+name)
			}
		}
	}
	if len(stored) != 397 || bodies != 163 || queries != 76 || paths != 166 {
		violations = append(violations, fmt.Sprintf("coverage entries=%d bodies=%d queries=%d paths=%d, want 397/163/76/166", len(stored), bodies, queries, paths))
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("Agent request contract coverage failed:\n%s", strings.Join(violations, "\n"))
	}
}

func TestAIAgentRuntimeEnforcesEveryPathQueryAndBodyClassification(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	var violations []string
	for _, operation := range service.catalog {
		pathParams := agentContractTestObject(operation.PathSchema)
		if err := validateAgentOperationParameters(operation, pathParams, nil); err != nil {
			violations = append(violations, operation.Key+": valid path parameters: "+err.Error())
		}
		pathProperties, _ := operation.PathSchema["properties"].(map[string]any)
		for name, rawSchema := range pathProperties {
			invalid := make(map[string]any, len(pathParams))
			for field, value := range pathParams {
				invalid[field] = value
			}
			fieldSchema, _ := rawSchema.(map[string]any)
			invalid[name] = agentContractInvalidTypeValue(fieldSchema)
			if err := validateAgentOperationParameters(operation, invalid, nil); err == nil {
				violations = append(violations, operation.Key+": accepted invalid path type for "+name)
			}
		}
		invalidPath := make(map[string]any, len(pathParams)+1)
		for name, value := range pathParams {
			invalidPath[name] = value
		}
		invalidPath["__unknown_path"] = "value"
		if err := validateAgentOperationParameters(operation, invalidPath, nil); err == nil {
			violations = append(violations, operation.Key+": accepted unknown path parameter")
		}

		queryProperties, _ := operation.QuerySchema["properties"].(map[string]any)
		for name, rawSchema := range queryProperties {
			fieldSchema, _ := rawSchema.(map[string]any)
			query := map[string]any{name: agentContractTestValue(fieldSchema)}
			if err := validateAgentOperationParameters(operation, pathParams, query); err != nil {
				violations = append(violations, operation.Key+": valid query "+name+": "+err.Error())
			}
			if err := validateAgentOperationParameters(operation, pathParams, map[string]any{name: agentContractInvalidTypeValue(fieldSchema)}); err == nil {
				violations = append(violations, operation.Key+": accepted invalid query type for "+name)
			}
		}
		if err := validateAgentOperationParameters(operation, pathParams, map[string]any{"__unknown_query": "value"}); err == nil {
			violations = append(violations, operation.Key+": accepted unknown query parameter")
		}
		if len(operation.BodySchema) == 0 {
			if err := validateAgentOperationBodyContract(operation, nil); err != nil {
				violations = append(violations, operation.Key+": rejected omitted body: "+err.Error())
			}
			if err := validateAgentOperationBodyContract(operation, map[string]any{}); err == nil {
				violations = append(violations, operation.Key+": accepted a body on a bodyless operation")
			}
		} else {
			if err := validateAgentOperationBodyContract(operation, nil); err == nil {
				violations = append(violations, operation.Key+": accepted an omitted required body")
			}
			if err := validateAgentOperationBodyContract(operation, agentContractTestObject(operation.BodySchema)); err != nil {
				violations = append(violations, operation.Key+": generated contract-valid body: "+err.Error())
			}
		}
		if len(operation.QueryExample) > 0 {
			if err := validateAgentOperationParameters(operation, pathParams, operation.QueryExample); err != nil {
				violations = append(violations, operation.Key+": query example: "+err.Error())
			}
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("Agent runtime request contract enforcement failed:\n%s", strings.Join(violations, "\n"))
	}
}

func agentContractTestObject(schema map[string]any) map[string]any {
	result := make(map[string]any)
	properties, _ := schema["properties"].(map[string]any)
	required, _ := schema["required"].([]any)
	for _, rawName := range required {
		name := fmt.Sprint(rawName)
		fieldSchema, _ := properties[name].(map[string]any)
		result[name] = agentContractTestValue(fieldSchema)
	}
	groups, _ := schema["required_any"].([]any)
	for _, rawGroup := range groups {
		group, _ := rawGroup.([]any)
		if len(group) == 0 {
			continue
		}
		name := fmt.Sprint(group[0])
		if _, exists := result[name]; !exists {
			fieldSchema, _ := properties[name].(map[string]any)
			value := agentContractTestValue(fieldSchema)
			if items, ok := value.([]any); ok && len(items) == 0 {
				itemSchema, _ := fieldSchema["items"].(map[string]any)
				value = []any{agentContractTestValue(itemSchema)}
			}
			result[name] = value
		}
	}
	return result
}

func agentContractTestValue(schema map[string]any) any {
	if values, ok := schema["enum"].([]any); ok && len(values) > 0 {
		return values[0]
	}
	switch schema["type"] {
	case "boolean":
		return true
	case "integer", "number":
		value := float64(0)
		if minimum, ok := agentContractSchemaNumber(schema["minimum"]); ok {
			value = minimum
		}
		if minimum, ok := agentContractSchemaNumber(schema["exclusiveMinimum"]); ok {
			value = minimum + 1
		}
		return value
	case "array":
		count := 0
		if minimum, ok := agentContractSchemaNumber(schema["minimum"]); ok {
			count = int(minimum)
		}
		items, _ := schema["items"].(map[string]any)
		result := make([]any, count)
		for index := range result {
			result[index] = agentContractTestValue(items)
		}
		return result
	case "object":
		return agentContractTestObject(schema)
	default:
		switch schema["format"] {
		case "date-time":
			return "2030-01-02T03:04:05Z"
		case "date":
			return "2030-01-02"
		case "email":
			return "admin@example.com"
		case "url":
			return "https://example.com"
		case "semver":
			return "1.2.3"
		}
		minimum := 1
		if value, ok := agentContractSchemaNumber(schema["minimum"]); ok && int(value) > minimum {
			minimum = int(value)
		}
		return strings.Repeat("x", minimum)
	}
}

func agentContractInvalidTypeValue(schema map[string]any) any {
	switch schema["type"] {
	case "string":
		return true
	case "boolean", "integer", "number", "array", "object":
		return "invalid-type"
	default:
		return nil
	}
}

func TestAIAgentCatalogBodyExamplesMatchContracts(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	var violations []string
	for _, operation := range service.catalog {
		if len(operation.BodySchema) == 0 || len(operation.BodyExample) == 0 {
			continue
		}
		if err := validateAgentBodyContract(operation.BodySchema, operation.BodyExample, "body"); err != nil {
			violations = append(violations, operation.Key+": "+err.Error())
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("Agent body examples violate their contracts:\n%s", strings.Join(violations, "\n"))
	}
}

func TestAIAgentContractsContainNoOpaqueConcreteSchemas(t *testing.T) {
	var contracts map[string]agentOperationContract
	if err := json.Unmarshal(agentContractsJSON, &contracts); err != nil {
		t.Fatalf("decode Agent contracts: %v", err)
	}

	allowedUntyped := map[string]bool{
		"POST:/admin/accounts/batch-update-credentials:body.value": true,
	}
	var violations []string
	for endpoint, contract := range contracts {
		if len(contract.BodySchema) > 0 {
			auditAgentContractSchema(endpoint, "body", contract.BodySchema, allowedUntyped, &violations)
		}
		if len(contract.QuerySchema) > 0 {
			auditAgentContractSchema(endpoint, "query", contract.QuerySchema, nil, &violations)
		}
		if len(contract.PathSchema) > 0 {
			auditAgentContractSchema(endpoint, "path_params", contract.PathSchema, nil, &violations)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Fatalf("Agent contracts contain opaque or invalid schemas:\n%s", strings.Join(violations, "\n"))
	}
}

func TestAIAgentContractsPreserveCustomDecoderAndDomainAliasShapes(t *testing.T) {
	var contracts map[string]struct {
		BodySchema map[string]any `json:"body_schema"`
	}
	if err := json.Unmarshal(agentContractsJSON, &contracts); err != nil {
		t.Fatalf("decode Agent contracts: %v", err)
	}

	expected := map[string]string{
		"POST:/admin/groups:daily_limit_usd":                                     "number",
		"POST:/admin/groups:weekly_limit_usd":                                    "number",
		"POST:/admin/groups:monthly_limit_usd":                                   "number",
		"PUT:/admin/groups/:id:daily_limit_usd":                                  "number",
		"PUT:/admin/groups/:id:weekly_limit_usd":                                 "number",
		"PUT:/admin/groups/:id:monthly_limit_usd":                                "number",
		"POST:/admin/redeem-codes/batch-update:fields.expires_at":                "string",
		"POST:/admin/redeem-codes/batch-update:fields.group_id":                  "integer",
		"POST:/admin/announcements:targeting.any_of[].all_of[].group_ids":        "array",
		"PUT:/admin/announcements/:id:targeting.any_of[].all_of[].group_ids":     "array",
		"POST:/admin/groups:messages_dispatch_model_config.exact_model_mappings": "object",
		"PUT:/admin/groups/:id:models_list_config.models":                        "array",
		"POST:/admin/groups:reasoning_effort_mappings[].from":                    "string",
		"PUT:/admin/groups/:id:reasoning_effort_mappings[].to":                   "string",
	}
	for identity, expectedType := range expected {
		separator := strings.LastIndex(identity, ":")
		endpoint, fieldPath := identity[:separator], identity[separator+1:]
		contract, exists := contracts[endpoint]
		if !exists {
			t.Errorf("missing contract %s", endpoint)
			continue
		}
		schema := agentContractSchemaAt(contract.BodySchema, fieldPath)
		if schema["type"] != expectedType {
			t.Errorf("%s %s type = %v, want %s", endpoint, fieldPath, schema["type"], expectedType)
		}
	}
}

func TestAIAgentContractsExcludeKnownServerManagedFields(t *testing.T) {
	var contracts map[string]struct {
		BodySchema map[string]any `json:"body_schema"`
	}
	if err := json.Unmarshal(agentContractsJSON, &contracts); err != nil {
		t.Fatalf("decode Agent contracts: %v", err)
	}

	serverManaged := map[string][]string{
		"POST:/admin/ops/alert-rules":      {"id", "created_at", "updated_at", "last_triggered_at"},
		"PUT:/admin/ops/alert-rules/:id":   {"id", "created_at", "updated_at", "last_triggered_at"},
		"PUT:/admin/ops/runtime/logging":   {"source", "updated_at", "updated_by_user_id"},
		"PUT:/admin/ops/advanced-settings": {"ignore_invalid_api_key_errors"},
	}
	for endpoint, fields := range serverManaged {
		properties, _ := contracts[endpoint].BodySchema["properties"].(map[string]any)
		for _, field := range fields {
			if _, exists := properties[field]; exists {
				t.Errorf("%s exposes server-managed field %s", endpoint, field)
			}
		}
	}
	providerSchema := agentContractSchemaAt(contracts["PUT:/admin/settings/web-search-emulation"].BodySchema, "providers[]")
	providerProperties, _ := providerSchema["properties"].(map[string]any)
	for _, field := range []string{"api_key_configured", "quota_used"} {
		if _, exists := providerProperties[field]; exists {
			t.Errorf("web search provider contract exposes server-managed field %s", field)
		}
	}
}

func agentContractSchemaAt(schema map[string]any, path string) map[string]any {
	for _, component := range strings.Split(path, ".") {
		array := strings.HasSuffix(component, "[]")
		component = strings.TrimSuffix(component, "[]")
		properties, _ := schema["properties"].(map[string]any)
		schema, _ = properties[component].(map[string]any)
		if array {
			schema, _ = schema["items"].(map[string]any)
		}
	}
	return schema
}

func auditAgentContractSchema(endpoint, path string, schema map[string]any, allowedUntyped map[string]bool, violations *[]string) {
	if len(schema) == 0 {
		if path == "additionalProperties" || strings.HasSuffix(path, ".additionalProperties") || allowedUntyped[endpoint+":"+path] {
			return
		}
		*violations = append(*violations, fmt.Sprintf("%s %s has no type", endpoint, path))
		return
	}

	allowedKeywords := map[string]bool{
		"type": true, "properties": true, "additionalProperties": true, "items": true,
		"required": true, "required_any": true, "enum": true, "minimum": true, "maximum": true,
		"exclusiveMinimum": true, "exclusiveMaximum": true, "format": true, "default": true,
	}
	for keyword := range schema {
		if !allowedKeywords[keyword] {
			*violations = append(*violations, fmt.Sprintf("%s %s uses unenforced keyword %s", endpoint, path, keyword))
		}
	}
	if format, _ := schema["format"].(string); format != "" {
		supportedFormats := map[string]bool{"date-time": true, "date": true, "email": true, "http-url": true, "semver": true}
		if !supportedFormats[format] {
			*violations = append(*violations, fmt.Sprintf("%s %s uses unsupported format %s", endpoint, path, format))
		}
	}

	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		properties, hasProperties := schema["properties"].(map[string]any)
		additional, hasAdditional := schema["additionalProperties"].(map[string]any)
		if (!hasProperties || len(properties) == 0) && !hasAdditional {
			*violations = append(*violations, fmt.Sprintf("%s %s is an opaque object", endpoint, path))
			return
		}
		for field, raw := range properties {
			fieldSchema, ok := raw.(map[string]any)
			if !ok {
				*violations = append(*violations, fmt.Sprintf("%s %s.%s is not a schema", endpoint, path, field))
				continue
			}
			auditAgentContractSchema(endpoint, path+"."+field, fieldSchema, allowedUntyped, violations)
		}
		if hasAdditional {
			auditAgentContractSchema(endpoint, path+".additionalProperties", additional, allowedUntyped, violations)
		}
	case "array":
		items, ok := schema["items"].(map[string]any)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("%s %s is an array without item schema", endpoint, path))
			return
		}
		auditAgentContractSchema(endpoint, path+"[]", items, allowedUntyped, violations)
	case "string", "boolean", "integer", "number":
	case "":
		if !allowedUntyped[endpoint+":"+path] {
			*violations = append(*violations, fmt.Sprintf("%s %s has no type", endpoint, path))
		}
	default:
		*violations = append(*violations, fmt.Sprintf("%s %s uses unsupported type %q", endpoint, path, schemaType))
	}
}
