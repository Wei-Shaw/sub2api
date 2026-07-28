package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

func agentTestOpenBodySchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": map[string]any{}}
}

func agentTestMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value type = %T, want map", value)
	}
	return result
}

func TestAIAgentCatalogSnapshotContainsAuditedAdminRoutes(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	if got, want := len(service.catalog), 397; got != want {
		t.Fatalf("catalog size = %d, want %d", got, want)
	}
	accountCreate := service.catalogByKey["POST:/admin/accounts"]
	credentials, _ := accountCreate.BodyExample["credentials"].(map[string]any)
	if accountCreate.BodyExample["type"] != "apikey" || credentials["api_key"] == nil ||
		accountCreate.BodyExample["concurrency"] != float64(10) || accountCreate.BodyExample["priority"] != float64(1) {
		t.Fatalf("account creation body example is incomplete: %#v", accountCreate.BodyExample)
	}
	contractCount := 0
	requiredContracts := 0
	for _, operation := range service.catalog {
		if len(operation.BodySchema) > 0 {
			contractCount++
		}
		if required, _ := operation.BodySchema["required"].([]any); len(required) > 0 {
			requiredContracts++
			if err := validateAgentBodyContract(operation.BodySchema, map[string]any{}, "body"); err == nil {
				t.Fatalf("empty payload passed required contract for %s", operation.Key)
			}
		}
	}
	if contractCount != 163 {
		t.Fatalf("body contract count = %d, want 163", contractCount)
	}
	if requiredContracts != 89 {
		t.Fatalf("required body contract count = %d, want 89", requiredContracts)
	}
	for _, endpointKey := range []string{"POST:/admin/groups", "PUT:/admin/groups/:id"} {
		properties, _ := service.catalogByKey[endpointKey].BodySchema["properties"].(map[string]any)
		for _, field := range []string{"daily_limit_usd", "weekly_limit_usd", "monthly_limit_usd"} {
			fieldSchema, _ := properties[field].(map[string]any)
			if fieldSchema["type"] != "number" {
				t.Fatalf("%s %s contract = %#v, want number", endpointKey, field, fieldSchema)
			}
			if err := validateAgentBodyContract(fieldSchema, float64(150), "body."+field); err != nil {
				t.Fatalf("%s numeric limit rejected: %v", field, err)
			}
			if err := validateAgentBodyContract(fieldSchema, map[string]any{"value": float64(150)}, "body."+field); err == nil {
				t.Fatalf("%s object limit unexpectedly accepted", field)
			}
		}
	}
	for _, forbidden := range []string{
		"POST:/admin/ai-agent/chat",
		"POST:/admin/ai-agent/actions/:id/confirm",
	} {
		if _, exists := service.catalogByKey[forbidden]; exists {
			t.Fatalf("Agent control endpoint must not be callable as a tool: %s", forbidden)
		}
	}
}

func TestAIAgentRollbackCapabilitiesCoverEveryWriteOperation(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	capabilities := service.RollbackCapabilities()
	if len(capabilities) != 229 {
		t.Fatalf("rollback capability count = %d, want 229", len(capabilities))
	}
	byKey := make(map[string]AIAgentRollbackCapability, len(capabilities))
	for _, capability := range capabilities {
		if capability.Level == "" {
			t.Fatalf("rollback capability has no level: %#v", capability)
		}
		byKey[capability.EndpointKey] = capability
	}
	for key, expected := range map[string]AIAgentRollbackCapability{
		"PUT:/admin/groups/:id":    {Level: "conditional", Strategy: agentRollbackStrategyRestore},
		"POST:/admin/groups":       {Level: "conditional", Strategy: agentRollbackStrategyDelete},
		"DELETE:/admin/groups/:id": {Level: "unavailable"},
	} {
		actual := byKey[key]
		if actual.Level != expected.Level || actual.Strategy != expected.Strategy {
			t.Fatalf("rollback capability %s = %#v, want level=%s strategy=%s", key, actual, expected.Level, expected.Strategy)
		}
	}
	session := newAIAgentSession("rollback-contract")
	contract := service.inspectAgentOperationContract(session, "PUT:/admin/groups/:id", "")
	if !strings.Contains(contract, `"rollback_support"`) || !strings.Contains(contract, `"strategy":"restore_fields"`) {
		t.Fatalf("exact operation contract omitted rollback support: %s", contract)
	}
}

func TestAIAgentCrossResourceValidationRejectsNonSubscriptionGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/groups/7" {
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":7,"name":"standard","subscription_type":"standard"}}`))
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	service, err := NewAIAgentService(nil, nil, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	operation := service.catalogByKey["POST:/admin/redeem-codes/generate"]
	err = service.validateAgentCrossResourceSemantics(context.Background(), AIAgentActor{UserID: 1}, operation, map[string]any{"type": "subscription", "group_id": float64(7)})
	if err == nil || !strings.Contains(err.Error(), "subscription group") {
		t.Fatalf("non-subscription group validation error = %v", err)
	}
}

func TestAIAgentLocalOperationSuggestions(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	cases := []struct {
		prompt string
		want   string
	}{
		{"创建一个 OpenAI 账号", "POST:/admin/accounts"},
		{"删除用户 42", "DELETE:/admin/users/:id"},
		{"查看用户余额历史", "GET:/admin/users/:id/balance-history"},
		{"重置 Web Search 用量", "POST:/admin/settings/web-search-emulation/reset-usage"},
	}
	for _, test := range cases {
		candidates := service.suggestOperations(test.prompt, 4)
		if len(candidates) == 0 {
			t.Fatalf("no local candidate for %q", test.prompt)
		}
		top := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			top = append(top, fmt.Sprintf("%s=%.1f/%s", candidate.Operation.Key, candidate.Score, candidate.Confidence))
		}
		t.Logf("%s => %s", test.prompt, strings.Join(top, ", "))
		if got := candidates[0].Operation.Key; got != test.want {
			t.Fatalf("top candidate for %q = %s, want %s (candidates %s)", test.prompt, got, test.want, strings.Join(top, ", "))
		}
		if candidates[0].Confidence != "high" {
			t.Fatalf("top candidate for %q confidence = %s, want high (candidates %s)", test.prompt, candidates[0].Confidence, strings.Join(top, ", "))
		}
	}
	for _, ambiguousPrompt := range []string{"查看", "账号"} {
		candidates := service.suggestOperations(ambiguousPrompt, 4)
		if len(candidates) == 0 {
			t.Fatalf("no candidates for ambiguous prompt %q", ambiguousPrompt)
		}
		t.Logf("ambiguous %s => %s %.1f/%s", ambiguousPrompt, candidates[0].Operation.Key, candidates[0].Score, candidates[0].Confidence)
		if candidates[0].Confidence == "high" {
			t.Fatalf("ambiguous prompt %q incorrectly received high confidence: %#v", ambiguousPrompt, candidates[0])
		}
	}
	if candidates := service.suggestOperations("qzxvplm-kjhgfdsa", 4); len(candidates) != 0 {
		t.Fatalf("unknown prompt returned candidates: %#v", candidates)
	}

	prompt := service.agentPlanningPrompt("创建一个 OpenAI 账号")
	if !strings.Contains(prompt, "POST:/admin/accounts") || !strings.Contains(prompt, "Local audited operation plans") {
		t.Fatalf("planning prompt did not include local candidate: %s", prompt)
	}
	if strings.Contains(prompt, "body_schema") {
		t.Fatal("local candidate prompt must not inject the full body schema")
	}
	searchResult, err := json.Marshal(service.searchOperationSummaries("创建一个 OpenAI 账号", 12))
	if err != nil {
		t.Fatalf("marshal search result: %v", err)
	}
	if strings.Contains(string(searchResult), "body_schema") {
		t.Fatal("search result must use the concise operation contract")
	}
}

func TestAIAgentResourceAmbiguityAndMultiIntentPlanning(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}

	explicit := "把 VIP 分组的倍率修改为 1.5"
	if ambiguity := service.agentResourceAmbiguity(explicit); ambiguity != nil {
		t.Fatalf("explicit group prompt was marked ambiguous: %#v", ambiguity)
	}
	candidates := service.suggestOperations(explicit, 4)
	if len(candidates) > 0 {
		t.Logf("explicit group multiplier => %s %.1f/%s", candidates[0].Operation.Key, candidates[0].Score, candidates[0].Confidence)
	}
	if len(candidates) == 0 || candidates[0].Operation.Key != "PUT:/admin/groups/:id" || candidates[0].Confidence != "high" {
		t.Fatalf("explicit group multiplier candidates = %#v", candidates)
	}
	summary := agentOperationSummary(candidates[0])
	lookup, _ := summary["target_lookup"].(map[string]any)
	if lookup["endpoint_key"] != "GET:/admin/groups" {
		t.Fatalf("group target lookup = %#v", lookup)
	}

	ambiguous := "把 VIP 的倍率修改为 1.5"
	ambiguity := service.agentResourceAmbiguity(ambiguous)
	t.Logf("ambiguous multiplier => %#v", ambiguity)
	if ambiguity == nil || !strings.Contains(fmt.Sprint(ambiguity["message"]), "账号") || !strings.Contains(fmt.Sprint(ambiguity["message"]), "分组") {
		t.Fatalf("resource ambiguity = %#v", ambiguity)
	}
	planningPrompt, blockReason := service.agentPlanningContext(ambiguous)
	if blockReason == "" || !strings.Contains(planningPrompt, "tools are disabled") {
		t.Fatalf("ambiguous planning context = %q, %q", planningPrompt, blockReason)
	}
	blocked := service.executeTool(context.Background(), AIAgentActor{}, &aiAgentSession{toolBlockReason: blockReason}, ambiguous,
		agentToolCall{Function: agentToolFunction{Name: "search_admin_operations", Arguments: `{"query":"倍率"}`}}, false, map[string]string{})
	if !strings.Contains(blocked, `"status":"clarification_required"`) {
		t.Fatalf("blocked tool result = %s", blocked)
	}

	contextSession := &aiAgentSession{public: []AIAgentMessage{{Role: "assistant", Content: "已定位到 VIP 分组。"}}}
	resourceHint := agentRecentResourceHint(contextSession)
	contextualPrompt, contextualBlock := service.agentPlanningContextWithHint("把倍率改成 1.5", resourceHint)
	if contextualBlock != "" || !strings.Contains(contextualPrompt, "PUT:/admin/groups/:id") {
		t.Fatalf("resource context was not inherited: prompt=%q block=%q", contextualPrompt, contextualBlock)
	}

	contextualMultiPrompt, contextualMultiBlock := service.agentPlanningContext("修改 Alpha 分组的倍率，然后修改 Beta 的倍率")
	if contextualMultiBlock != "" || strings.Count(contextualMultiPrompt, "PUT:/admin/groups/:id") < 2 {
		t.Fatalf("same-message resource context was not inherited: %q, %q", contextualMultiPrompt, contextualMultiBlock)
	}

	multiPrompt := service.agentPlanningPrompt("查看用户余额历史，然后重置 Web Search 用量")
	t.Logf("multi-intent plan => balance-history + reset-usage candidates")
	for _, expected := range []string{"GET:/admin/users/:id/balance-history", "POST:/admin/settings/web-search-emulation/reset-usage", `"intent"`} {
		if !strings.Contains(multiPrompt, expected) {
			t.Fatalf("multi-intent plan lacks %q: %s", expected, multiPrompt)
		}
	}
}

func TestAIAgentCompoundGroupRedeemIntentUsesGenerateCapability(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	prompt := "创建个openai分组，倍率0.1，设置为专属分组，然后给我一个兑换码"
	clauses := agentIntentClauses(prompt)
	if len(clauses) != 2 || !strings.Contains(clauses[0], "倍率0.1") || !strings.Contains(clauses[0], "专属分组") {
		t.Fatalf("compound intent clauses = %#v", clauses)
	}
	planningPrompt, blockReason := service.agentPlanningContextWithHints(prompt, "用户", "查看最近注册的用户")
	if blockReason != "" {
		t.Fatalf("current group intent was polluted by stale user context: %s", blockReason)
	}
	if !strings.Contains(planningPrompt, "POST:/admin/groups") || !strings.Contains(planningPrompt, "Search only for an intent whose candidates are ambiguous") {
		t.Fatalf("compound plan did not preserve group creation and require Skill resolution: %s", planningPrompt)
	}
	if hint := agentApplicableIntentHint("继续修复并重试", "创建分组并生成兑换码"); hint == "" {
		t.Fatal("retry follow-up did not inherit the unfinished user intent")
	}
	followUpPrompt, followUpBlock := service.agentPlanningContextWithHints("你输出给我就行", "分组", "不是金额，是这个新创建的openai分组的兑换码，不过期，生成一个就行")
	if followUpBlock != "" || !strings.Contains(followUpPrompt, "POST:/admin/redeem-codes/generate") {
		t.Fatalf("referential follow-up lost redeem intent: prompt=%q block=%q", followUpPrompt, followUpBlock)
	}
	explicitGenerate := service.suggestOperations("生成一个新创建分组的兑换码", 4)
	if len(explicitGenerate) == 0 || explicitGenerate[0].Operation.Key != "POST:/admin/redeem-codes/generate" || explicitGenerate[0].Confidence != "high" {
		t.Fatalf("explicit generate candidates = %#v", explicitGenerate)
	}
	session := newAIAgentSession("skill-search")
	session.capabilitySearches = make(map[string]string)
	session.expandedSkills = make(map[string]string)
	resolved := service.searchAgentCapability(session, "这个分组的兑换码")
	if len(resolved) > agentMaxToolOutput {
		t.Fatalf("nested Skill result exceeds tool output budget: %d bytes", len(resolved))
	}
	for _, expected := range []string{"skill_path", "operation_manifest", "redeem_codes", "POST:/admin/redeem-codes/generate", "POST:/admin/redeem-codes/create-and-redeem"} {
		if !strings.Contains(resolved, expected) {
			t.Fatalf("nested Skill result lacks %q: %s", expected, resolved)
		}
	}
	cached := service.searchAgentCapability(session, "分组的兑换码")
	if !strings.Contains(cached, `"cached":true`) {
		t.Fatalf("equivalent capability search was not cached: %s", cached)
	}
	reusedSkill := service.searchAgentCapability(session, "查看兑换码列表")
	if !strings.Contains(reusedSkill, `"status":"skill_reused"`) || !strings.Contains(reusedSkill, `"reused":true`) || strings.Contains(reusedSkill, `"operation_manifest":`) {
		t.Fatalf("expanded Skill was not reused compactly: %s", reusedSkill)
	}
	recentUsers := service.suggestOperations("查看最近注册的用户", 4)
	if len(recentUsers) == 0 || recentUsers[0].Operation.Module != "users" {
		t.Fatalf("canonical user Skill was not preferred: %#v", recentUsers)
	}
	generate := service.catalogByKey["POST:/admin/redeem-codes/generate"]
	if err := validateAgentOperationSemantics(http.MethodPost, generate.Path, map[string]any{"count": float64(1), "type": "subscription"}); err == nil || !strings.Contains(err.Error(), "group_id") {
		t.Fatalf("subscription redeem code without group was accepted: %v", err)
	}
	if err := validateAgentOperationSemantics(http.MethodPost, generate.Path, map[string]any{
		"count": float64(1), "type": "subscription", "group_id": float64(7), "expires_at": "2027-01-01T00:00:00Z", "expires_in_days": float64(30),
	}); err == nil || !strings.Contains(err.Error(), "cannot both") {
		t.Fatalf("conflicting expiry fields were accepted: %v", err)
	}
	if correction := agentCapabilityClaimCorrection("目前没有合适的接口支持此操作", 0, 0, 0); !strings.Contains(correction, "search_admin_operations") {
		t.Fatal("unverified capability claim was not forced through the Skill index")
	}
	if correction := agentCapabilityClaimCorrection("目前没有合适的接口支持此操作", 1, 0, 0); !strings.Contains(correction, "Reuse") {
		t.Fatal("post-search capability claim did not require reuse and contract comparison")
	}
	planNodes := []agentPlanNodeArgument{
		{ID: "create_group", EndpointKey: "POST:/admin/groups", Body: map[string]any{"name": "openai", "subscription_type": "standard"}},
		{ID: "generate_code", EndpointKey: "POST:/admin/redeem-codes/generate", DependsOn: []string{"create_group"}, Body: map[string]any{
			"count": float64(1), "type": "subscription", "group_id": map[string]any{"$ref": "create_group.resource_id"},
		}},
	}
	if err := validateAgentPlanBusinessSemantics(planNodes); err == nil || !strings.Contains(err.Error(), "subscription_type") {
		t.Fatalf("non-subscription group plan was accepted: %v", err)
	}
	groupBody, ok := planNodes[0].Body.(map[string]any)
	if !ok {
		t.Fatalf("group plan body type = %T, want map", planNodes[0].Body)
	}
	groupBody["subscription_type"] = "subscription"
	if err := validateAgentPlanBusinessSemantics(planNodes); err != nil {
		t.Fatalf("valid subscription group plan was rejected: %v", err)
	}
}

func TestAIAgentEnglishResourceAliasesRankMatchingSkill(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	tests := []struct {
		query string
		key   string
	}{
		{query: "update user allowed_groups assign group to user", key: "PUT:/admin/users/:id"},
		{query: "delete user by user id", key: "DELETE:/admin/users/:id"},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			candidates := service.suggestOperations(test.query, 6)
			if len(candidates) == 0 || candidates[0].Operation.Key != test.key {
				t.Fatalf("suggestOperations(%q) = %#v, want first %s", test.query, candidates, test.key)
			}
		})
	}
}

func TestAIAgentLargeContractsPrioritizeFieldsAndRequireExactInspection(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	candidates := service.suggestOperations("把 codex飞天 改为订阅分组", 4)
	if len(candidates) == 0 || candidates[0].Operation.Key != "PUT:/admin/groups/:id" {
		t.Fatalf("subscription group update candidates = %#v", candidates)
	}
	summary := agentOperationSummary(candidates[0])
	fields, _ := summary["body_fields"].([]string)
	if !strings.Contains(fmt.Sprint(fields), "subscription_type") {
		t.Fatalf("query-relevant field was omitted from projected contract: %#v", summary)
	}
	if summary["body_fields_truncated"] != true || summary["body_field_count"] != 46 {
		t.Fatalf("large contract projection metadata = %#v", summary)
	}
	session := newAIAgentSession("contract-inspection")
	contract := service.inspectAgentOperationContract(session, "PUT:/admin/groups/:id", "")
	if len(contract) > agentMaxToolOutput || !strings.Contains(contract, `"body_fields_complete":true`) ||
		!strings.Contains(contract, `"subscription_type":{"enum":["standard","subscription"]`) {
		t.Fatalf("complete group update contract = %s", contract)
	}
	cached := service.inspectAgentOperationContract(session, "PUT:/admin/groups/:id", "")
	if !strings.Contains(cached, `"cached":true`) {
		t.Fatalf("operation contract lookup was not cached: %s", cached)
	}
	nested := service.inspectAgentOperationContract(session, "PUT:/admin/groups/:id", "reasoning_effort_mappings[].from")
	if !strings.Contains(nested, `"status":"field_contract_resolved"`) || !strings.Contains(nested, `"enum":["minimal","low","medium","high","xhigh","max"]`) {
		t.Fatalf("nested field contract lookup = %s", nested)
	}
	queryContract := service.inspectAgentOperationContract(session, "GET:/admin/users", "")
	if !strings.Contains(queryContract, `"query_field_contracts"`) || !strings.Contains(queryContract, `"search":{"maximum":2048,"type":"string"}`) || strings.Contains(queryContract, `"keyword"`) {
		t.Fatalf("query contract lookup = %s", queryContract)
	}
	subscriptionContract := service.inspectAgentOperationContract(session, "POST:/admin/subscriptions/assign", "")
	if !strings.Contains(subscriptionContract, `"business_rules"`) || !strings.Contains(subscriptionContract, `PUT:/admin/groups/:id set subscription_type=subscription`) {
		t.Fatalf("subscription workflow contract = %s", subscriptionContract)
	}
	correction := agentCapabilityClaimCorrection("更新接口未开放 subscription_type 字段，因此无法修改", 1, 0, 0)
	if !strings.Contains(correction, "endpoint_key") || !strings.Contains(correction, "body_field_contracts") {
		t.Fatalf("missing-field claim was not forced through exact contract inspection: %s", correction)
	}
}

func TestAIAgentSubscriptionWorkflowSkillAndDefaults(t *testing.T) {
	workflows := agentWorkflowSkillHints("你这订阅没分配呀，把现有 group 分配给 user")
	encoded, _ := json.Marshal(workflows)
	if !strings.Contains(string(encoded), "user_subscription_assignment") || !strings.Contains(string(encoded), "POST:/admin/subscriptions/assign") || !strings.Contains(string(encoded), "PUT:/admin/groups/:id") {
		t.Fatalf("subscription workflow Skills = %s", encoded)
	}
	for _, test := range []struct {
		name string
		body map[string]any
		want int
	}{
		{name: "omitted", body: map[string]any{"user_id": float64(4), "group_id": float64(6)}, want: 30},
		{name: "non-positive", body: map[string]any{"user_id": float64(4), "group_id": float64(6), "validity_days": float64(-1)}, want: 30},
		{name: "explicit", body: map[string]any{"user_id": float64(4), "group_id": float64(6), "validity_days": float64(60)}, want: 60},
	} {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := normalizeAgentOperationBody(http.MethodPost, "/admin/subscriptions/assign", test.body)
			if err != nil {
				t.Fatalf("normalizeAgentOperationBody() error = %v", err)
			}
			normalizedBody, ok := normalized.(map[string]any)
			if !ok {
				t.Fatalf("normalized body type = %T, want map", normalized)
			}
			value, _ := agentOptionalNumericValue(normalizedBody["validity_days"])
			if int(value) != test.want {
				t.Fatalf("validity_days = %v, want %d", value, test.want)
			}
		})
	}
}

func TestAIAgentAccountDefaultsPersistInExecutionPlan(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	marker := AgentCatalogOperation{Key: "POST:/admin/test/marker", Method: http.MethodPost, Path: "/admin/test/marker", Title: "Marker", BodySchema: agentTestOpenBodySchema()}
	service.catalogByKey[marker.Key] = marker
	plan, _, err := service.prepareAgentExecutionPlan(context.Background(), AIAgentActor{}, "create account and marker", map[string]bool{}, agentPlanArguments{
		Title: "account defaults", FailurePolicy: "stop_on_failure", Nodes: []agentPlanNodeArgument{
			{ID: "account", EndpointKey: "POST:/admin/accounts", Body: map[string]any{
				"name": "default account", "platform": "openai", "type": "apikey", "credentials": map[string]any{"api_key": "secret"},
			}},
			{ID: "marker", EndpointKey: marker.Key, Body: map[string]any{"value": float64(1)}},
		},
	})
	if err != nil {
		t.Fatalf("prepareAgentExecutionPlan() error = %v", err)
	}
	body := agentTestMap(t, plan.Nodes[0].Body)
	if body["concurrency"] != 10 || body["priority"] != 1 {
		t.Fatalf("account plan body lacks scheduling defaults: %#v", body)
	}
	preview, _ := json.Marshal(plan.Nodes[0].Preview)
	if !strings.Contains(string(preview), `"field":"concurrency"`) || !strings.Contains(string(preview), `"field":"priority"`) {
		t.Fatalf("account plan preview lacks scheduling defaults: %s", preview)
	}
}

func TestAIAgentPlanAllowsCreatedGroupReferenceInUserAllowedGroups(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	plan, pending, err := service.prepareAgentExecutionPlan(context.Background(), AIAgentActor{UserID: 1}, "create user and exclusive group", nil, agentPlanArguments{
		Title: "Create user and group", FailurePolicy: "stop_on_failure", Nodes: []agentPlanNodeArgument{
			{ID: "create_group", EndpointKey: "POST:/admin/groups", Body: map[string]any{
				"name": "exclusive group", "platform": "openai", "is_exclusive": true, "rate_multiplier": 0.11,
				"daily_limit_usd": 150.0, "weekly_limit_usd": 800.0, "monthly_limit_usd": 1600.0,
			}},
			{ID: "create_user", EndpointKey: "POST:/admin/users", DependsOn: []string{"create_group"}, Body: map[string]any{
				"username": "new-user", "email": "new-user@example.com", "password": "secret-password",
				"allowed_groups": []any{map[string]any{"$ref": "create_group.resource_id"}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("prepareAgentExecutionPlan() error = %v", err)
	}
	if plan == nil || pending == nil || len(plan.Nodes) != 2 {
		t.Fatalf("plan = %#v, pending = %#v", plan, pending)
	}
	userBody, _ := plan.Nodes[1].Body.(map[string]any)
	groups, _ := userBody["allowed_groups"].([]any)
	if len(groups) != 1 || !containsAgentPlanReference(groups[0]) {
		t.Fatalf("stored user group reference = %#v", userBody)
	}
}

func TestAIAgentPlanUsesDependencyGroupStateForSubscriptionAssignment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/groups/6" {
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":6,"name":"subscription candidate","platform":"openai","subscription_type":"standard"}}`))
			return
		}
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	service, err := NewAIAgentService(nil, nil, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	nodes := []agentPlanNodeArgument{
		{ID: "convert_group", EndpointKey: "PUT:/admin/groups/:id", PathParams: map[string]any{"id": float64(6)}, Body: map[string]any{"subscription_type": "subscription"}},
		{ID: "assign_subscription", EndpointKey: "POST:/admin/subscriptions/assign", DependsOn: []string{"convert_group"}, Body: map[string]any{"user_id": float64(4), "group_id": float64(6)}},
	}
	plan, _, err := service.prepareAgentExecutionPlan(context.Background(), AIAgentActor{UserID: 1}, "convert group 6 and assign it to user 4", map[string]bool{"6": true}, agentPlanArguments{
		Title: "Convert and assign subscription", FailurePolicy: "stop_on_failure", Nodes: nodes,
	})
	if err != nil {
		t.Fatalf("prepareAgentExecutionPlan() error = %v", err)
	}
	if plan == nil || len(plan.Nodes) != 2 || plan.Nodes[1].DependsOn[0] != "convert_group" {
		t.Fatalf("plan = %#v", plan)
	}
	withoutDependency := nodes
	withoutDependency[1].DependsOn = nil
	if _, _, err := service.prepareAgentExecutionPlan(context.Background(), AIAgentActor{UserID: 1}, "convert group 6 and assign it to user 4", map[string]bool{"6": true}, agentPlanArguments{
		Title: "Unsafe conversion", FailurePolicy: "stop_on_failure", Nodes: withoutDependency,
	}); err == nil || !strings.Contains(err.Error(), "subscription group") {
		t.Fatalf("plan without dependency error = %v", err)
	}
}

func TestAIAgentRollbackManifestPreservesReverseCompensationOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/users/3":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":3,"username":"new-user","email":"new-user@example.com","allowed_groups":[5]}}`))
		case "/api/v1/admin/groups/5":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":5,"name":"exclusive group","platform":"openai"}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	service, err := NewAIAgentService(nil, nil, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	now := time.Now()
	rollback := AIAgentRollback{
		ID: "plan-rollback", PlanID: "plan-1", Operation: "Create user and group", Strategy: agentRollbackStrategyPlan, Status: "available",
		Children: []AIAgentRollback{
			{ID: "group", Operation: "Create group", Strategy: agentRollbackStrategyDelete, Status: "available", Method: http.MethodDelete, Path: "/admin/groups/5", Resource: "groups", TargetID: "5", ForwardBody: map[string]any{"name": "exclusive group", "platform": "openai"}, CreatedAt: now, UpdatedAt: now},
			{ID: "user", Operation: "Create user", Strategy: agentRollbackStrategyDelete, Status: "available", Method: http.MethodDelete, Path: "/admin/users/3", Resource: "users", TargetID: "3", ForwardBody: map[string]any{"username": "new-user", "email": "new-user@example.com", "allowed_groups": []any{float64(5)}}, CreatedAt: now, UpdatedAt: now},
		},
		CreatedAt: now, UpdatedAt: now,
	}
	preview := service.rollbackPreviewForRecord(context.Background(), AIAgentActor{UserID: 1}, rollback)
	manifest := service.agentRollbackCompensationManifest(context.Background(), AIAgentActor{UserID: 1}, rollback, preview)
	operations, _ := manifest["operations"].([]map[string]any)
	if manifest["deterministic_can_execute"] != true || len(operations) != 2 {
		t.Fatalf("manifest = %#v", manifest)
	}
	if operations[0]["endpoint_key"] != "DELETE:/admin/users/:id" || fmt.Sprint(operations[0]["target_id"]) != "3" {
		t.Fatalf("first compensation = %#v", operations[0])
	}
	if operations[1]["endpoint_key"] != "DELETE:/admin/groups/:id" || fmt.Sprint(operations[1]["target_id"]) != "5" || fmt.Sprint(operations[1]["depends_on"]) != "[compensation_1]" {
		t.Fatalf("second compensation = %#v", operations[1])
	}
}

func TestAIAgentInvalidPlanCannotDowngradeToSeparateWrites(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	operation := AgentCatalogOperation{Key: "POST:/admin/test/plan-guard", Method: http.MethodPost, Path: "/admin/test/plan-guard", Title: "Guarded write", BodySchema: agentTestOpenBodySchema()}
	service.catalogByKey[operation.Key] = operation
	session := newAIAgentSession("plan-guard")
	invalidPlan := agentToolCall{Function: agentToolFunction{Name: "plan_admin_operations", Arguments: `{
		"title":"guard partial completion","failure_policy":"stop_on_failure","nodes":[
			{"id":"one","endpoint_key":"POST:/admin/test/plan-guard","body":{"value":1}},
			{"id":"two","endpoint_key":"POST:/admin/test/plan-guard","body":{"value":1}}
		]}`}}
	result := service.executeTool(context.Background(), AIAgentActor{}, session, "execute dependent writes", invalidPlan, true, map[string]string{})
	if !strings.Contains(result, `"status":"invalid_plan"`) || !session.planRequired {
		t.Fatalf("invalid plan did not activate plan guard: %s", result)
	}
	direct := agentToolCall{Function: agentToolFunction{Name: "execute_admin_operation", Arguments: `{"endpoint_key":"POST:/admin/test/plan-guard","body":{"value":1}}`}}
	result = service.executeTool(context.Background(), AIAgentActor{}, session, "execute dependent writes", direct, true, map[string]string{})
	if !strings.Contains(result, `"status":"plan_required"`) || !strings.Contains(result, "partial completion") {
		t.Fatalf("invalid plan downgraded to a separate write: %s", result)
	}
}

func TestAIAgentSupervisedWritesQueueAndPromote(t *testing.T) {
	var executed []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		executed = append(executed, request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":{"ok":true}}`))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, err := NewAgentInternalAuth()
	if err != nil {
		t.Fatalf("NewAgentInternalAuth() error = %v", err)
	}
	settings := &aiAgentMemorySettings{values: map[string]string{agentSettingEnabled: "true"}}
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	firstOperation := AgentCatalogOperation{Key: "POST:/admin/test/first", Method: http.MethodPost, Path: "/admin/test/first", Title: "First", BodySchema: agentTestOpenBodySchema()}
	secondOperation := AgentCatalogOperation{Key: "POST:/admin/test/second", Method: http.MethodPost, Path: "/admin/test/second", Title: "Second", BodySchema: agentTestOpenBodySchema()}
	service.catalogByKey[firstOperation.Key] = firstOperation
	service.catalogByKey[secondOperation.Key] = secondOperation
	session := newAIAgentSession("multi")
	service.sessions[21] = map[string]*aiAgentSession{session.id: session}
	service.loaded[21] = true

	firstCall := agentToolCall{Function: agentToolFunction{Name: "execute_admin_operation", Arguments: `{"endpoint_key":"POST:/admin/test/first","body":{"value":1}}`}}
	secondCall := agentToolCall{Function: agentToolFunction{Name: "execute_admin_operation", Arguments: `{"endpoint_key":"POST:/admin/test/second","body":{"value":2}}`}}
	completedWrites := make(map[string]string)
	firstResult := service.executeTool(context.Background(), AIAgentActor{UserID: 21}, session, "执行两个命令", firstCall, false, completedWrites)
	secondResult := service.executeTool(context.Background(), AIAgentActor{UserID: 21}, session, "执行两个命令", secondCall, false, completedWrites)
	if !strings.Contains(firstResult, `"status":"confirmation_required"`) || !strings.Contains(secondResult, `"status":"confirmation_queued"`) {
		t.Fatalf("queue results = %s / %s", firstResult, secondResult)
	}
	if session.pending == nil || len(session.pendingQueue) != 1 {
		t.Fatalf("pending=%#v queue=%#v", session.pending, session.pendingQueue)
	}
	if err := service.persistConversations(context.Background(), 21); err != nil {
		t.Fatalf("persist queued writes: %v", err)
	}
	reloaded, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("reload service: %v", err)
	}
	reloadedSession, err := reloaded.conversation(context.Background(), 21, session.id, false)
	if err != nil || reloadedSession.pending == nil || len(reloadedSession.pendingQueue) != 1 {
		t.Fatalf("reloaded pending queue: session=%#v err=%v", reloadedSession, err)
	}
	firstID := session.pending.ID
	secondID := session.pendingQueue[0].ID
	if _, err := service.Confirm(context.Background(), AIAgentActor{UserID: 21}, session.id, firstID, false); err != nil {
		t.Fatalf("confirm first: %v", err)
	}
	if session.pending == nil || session.pending.ID != secondID || len(session.pendingQueue) != 0 {
		t.Fatalf("second action was not promoted: pending=%#v queue=%#v", session.pending, session.pendingQueue)
	}
	if _, err := service.Confirm(context.Background(), AIAgentActor{UserID: 21}, session.id, secondID, false); err != nil {
		t.Fatalf("confirm second: %v", err)
	}
	if session.pending != nil || len(executed) != 2 || executed[0] != "/api/v1/admin/test/first" || executed[1] != "/api/v1/admin/test/second" {
		t.Fatalf("executed=%v pending=%#v", executed, session.pending)
	}
}

func TestAIAgentExecutionPlanBindsDependenciesAndPersists(t *testing.T) {
	var executed []string
	var childBody map[string]any
	var idempotencyKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		executed = append(executed, request.URL.Path)
		idempotencyKeys = append(idempotencyKeys, request.Header.Get("Idempotency-Key"))
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/admin/test/parents":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":71,"name":"Parent"}}`))
		case "/api/v1/admin/test/children":
			if err := json.NewDecoder(request.Body).Decode(&childBody); err != nil {
				t.Errorf("decode child body: %v", err)
			}
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":72,"name":"Child"}}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, err := NewAgentInternalAuth()
	if err != nil {
		t.Fatalf("NewAgentInternalAuth() error = %v", err)
	}
	settings := &aiAgentMemorySettings{values: map[string]string{agentSettingEnabled: "true"}}
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	parentOperation := AgentCatalogOperation{Key: "POST:/admin/test/parents", Method: http.MethodPost, Path: "/admin/test/parents", Title: "Create parent", Module: "test", BodySchema: agentTestOpenBodySchema()}
	childOperation := AgentCatalogOperation{Key: "POST:/admin/test/children", Method: http.MethodPost, Path: "/admin/test/children", Title: "Create child", Module: "test", BodySchema: agentTestOpenBodySchema()}
	service.catalogByKey[parentOperation.Key] = parentOperation
	service.catalogByKey[childOperation.Key] = childOperation
	service.catalog = append(service.catalog, parentOperation, childOperation)
	session := newAIAgentSession("dependent plan")
	service.sessions[22] = map[string]*aiAgentSession{session.id: session}
	service.loaded[22] = true

	call := agentToolCall{Function: agentToolFunction{Name: "plan_admin_operations", Arguments: `{
		"title":"Create parent and child",
		"failure_policy":"stop_on_failure",
		"nodes":[
			{"id":"create_parent","endpoint_key":"POST:/admin/test/parents","body":{"name":"Parent"}},
			{"id":"create_child","endpoint_key":"POST:/admin/test/children","depends_on":["create_parent"],"body":{"name":"Child","parent_id":{"$ref":"create_parent.resource_id"}}}
		]}`}}
	result := service.executeTool(context.Background(), AIAgentActor{UserID: 22}, session, "create dependent resources", call, false, map[string]string{})
	if !strings.Contains(result, `"status":"confirmation_required"`) || session.pending == nil || session.pending.Plan == nil {
		t.Fatalf("plan staging result=%s pending=%#v", result, session.pending)
	}
	if err := service.persistConversations(context.Background(), 22); err != nil {
		t.Fatalf("persist plan: %v", err)
	}
	reloaded, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("reload service: %v", err)
	}
	reloaded.client = server.Client()
	reloaded.catalogByKey[parentOperation.Key] = parentOperation
	reloaded.catalogByKey[childOperation.Key] = childOperation
	reloaded.catalog = append(reloaded.catalog, parentOperation, childOperation)
	reloadedSession, err := reloaded.conversation(context.Background(), 22, session.id, false)
	if err != nil || reloadedSession.pending == nil || reloadedSession.pending.Plan == nil {
		t.Fatalf("reloaded plan: pending=%#v err=%v", reloadedSession.pending, err)
	}
	pendingID := reloadedSession.pending.ID
	confirmation, err := reloaded.Confirm(context.Background(), AIAgentActor{UserID: 22}, session.id, pendingID, false)
	if err != nil {
		t.Fatalf("confirm plan: %v", err)
	}
	if accepted, _ := confirmation.(map[string]any)["accepted"].(bool); !accepted {
		t.Fatalf("plan confirmation was not accepted for background execution: %#v", confirmation)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		reloadedSession.mu.Lock()
		status := reloadedSession.status
		reloadedSession.mu.Unlock()
		if status != agentConversationStatusRunning && status != agentConversationStatusStopping {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := fmt.Sprint(childBody["parent_id"]); got != "71" {
		t.Fatalf("resolved child parent_id = %q, want 71; body=%#v", got, childBody)
	}
	if len(executed) != 2 || executed[0] != "/api/v1/admin/test/parents" || executed[1] != "/api/v1/admin/test/children" {
		t.Fatalf("execution order = %v", executed)
	}
	if idempotencyKeys[0] == "" || idempotencyKeys[1] == "" || idempotencyKeys[0] == idempotencyKeys[1] {
		t.Fatalf("idempotency keys = %v", idempotencyKeys)
	}
	if reloadedSession.pending != nil {
		t.Fatalf("completed plan remained pending: %#v", reloadedSession.pending)
	}
}

func TestAIAgentExecutionPlanRejectsUnsafeReferencesAndCycles(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	operation := AgentCatalogOperation{Key: "POST:/admin/test/nodes", Method: http.MethodPost, Path: "/admin/test/nodes", Title: "Create node", BodySchema: agentTestOpenBodySchema()}
	service.catalogByKey[operation.Key] = operation
	actor := AIAgentActor{UserID: 23}
	unsafe := agentPlanArguments{Title: "unsafe", FailurePolicy: "stop_on_failure", Nodes: []agentPlanNodeArgument{
		{ID: "one", EndpointKey: operation.Key, Body: map[string]any{}},
		{ID: "two", EndpointKey: operation.Key, DependsOn: []string{"one"}, Body: map[string]any{"token": map[string]any{"$ref": "one.credentials"}}},
	}}
	if _, _, err := service.prepareAgentExecutionPlan(context.Background(), actor, "test", nil, unsafe); err == nil || !strings.Contains(err.Error(), "allow-listed") {
		t.Fatalf("unsafe output reference error = %v", err)
	}
	cyclic := agentPlanArguments{Title: "cycle", FailurePolicy: "stop_on_failure", Nodes: []agentPlanNodeArgument{
		{ID: "one", EndpointKey: operation.Key, DependsOn: []string{"two"}, Body: map[string]any{"name": "one"}},
		{ID: "two", EndpointKey: operation.Key, DependsOn: []string{"one"}, Body: map[string]any{"name": "two"}},
	}}
	if _, _, err := service.prepareAgentExecutionPlan(context.Background(), actor, "test", nil, cyclic); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
	duplicate := agentPlanArguments{Title: "duplicate", FailurePolicy: "continue_independent", Nodes: []agentPlanNodeArgument{
		{ID: "one", EndpointKey: operation.Key, Body: map[string]any{"name": "same"}},
		{ID: "two", EndpointKey: operation.Key, Body: map[string]any{"name": "same"}},
	}}
	if _, _, err := service.prepareAgentExecutionPlan(context.Background(), actor, "test", nil, duplicate); err == nil || !strings.Contains(err.Error(), "same write") {
		t.Fatalf("duplicate write error = %v", err)
	}
}

func TestAIAgentExecutionPlanPublicViewRedactsCredentials(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	session := newAIAgentSession("sensitive plan")
	secret := "sk-plan-secret-123456789"
	call := agentToolCall{Function: agentToolFunction{Name: "plan_admin_operations", Arguments: fmt.Sprintf(`{
		"title":"Create account and group",
		"failure_policy":"continue_independent",
		"nodes":[
			{"id":"account","endpoint_key":"POST:/admin/accounts","body":{"name":"Plan account","platform":"openai","type":"apikey","credentials":{"api_key":%q}}},
			{"id":"group","endpoint_key":"POST:/admin/groups","body":{"name":"Plan group"}}
		]}`, secret)}}
	result := service.executeTool(context.Background(), AIAgentActor{UserID: 25}, session, "create account and group", call, false, map[string]string{})
	if strings.Contains(result, secret) {
		t.Fatalf("tool result leaked plan credential: %s", result)
	}
	public, err := json.Marshal(publicAgentPending(session.pending))
	if err != nil {
		t.Fatalf("marshal public pending plan: %v", err)
	}
	if strings.Contains(string(public), secret) || strings.Contains(string(public), `"api_key":"`+secret+`"`) {
		t.Fatalf("public pending plan leaked credential: %s", public)
	}
	stored, _ := json.Marshal(session.pending)
	if !strings.Contains(string(stored), secret) {
		t.Fatal("encrypted pending source did not retain the credential needed for execution")
	}
}

func TestAIAgentExecutionPlanStopsDependentsAndCompensates(t *testing.T) {
	var executed []string
	parentDeleted := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		executed = append(executed, request.Method+" "+request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/test/parents":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":81,"name":"Parent"}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/test/parents/81":
			if parentDeleted {
				writer.WriteHeader(http.StatusNotFound)
				_, _ = writer.Write([]byte(`{"error":"not found"}`))
			} else {
				_, _ = writer.Write([]byte(`{"code":0,"data":{"id":81,"name":"Parent"}}`))
			}
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/test/children":
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"error":"child rejected"}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/api/v1/admin/test/parents/81":
			parentDeleted = true
			_, _ = writer.Write([]byte(`{"code":0,"data":{"deleted":true}}`))
		default:
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"error":"unexpected request"}`))
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	service, err := NewAIAgentService(nil, nil, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	operations := []AgentCatalogOperation{
		{Key: "POST:/admin/test/parents", Method: http.MethodPost, Path: "/admin/test/parents", Title: "Create parent", BodySchema: agentTestOpenBodySchema()},
		{Key: "DELETE:/admin/test/parents/:id", Method: http.MethodDelete, Path: "/admin/test/parents/:id", PathParams: []string{"id"}, Title: "Delete parent"},
		{Key: "POST:/admin/test/children", Method: http.MethodPost, Path: "/admin/test/children", Title: "Create child", BodySchema: agentTestOpenBodySchema()},
		{Key: "POST:/admin/test/grants", Method: http.MethodPost, Path: "/admin/test/grants", Title: "Create grant", BodySchema: agentTestOpenBodySchema()},
	}
	for _, operation := range operations {
		service.catalogByKey[operation.Key] = operation
		service.catalog = append(service.catalog, operation)
	}
	session := newAIAgentSession("rollback plan")
	call := agentToolCall{Function: agentToolFunction{Name: "plan_admin_operations", Arguments: `{
		"title":"Create dependent resources",
		"failure_policy":"rollback_on_failure",
		"nodes":[
			{"id":"parent","endpoint_key":"POST:/admin/test/parents","body":{"name":"Parent"}},
			{"id":"child","endpoint_key":"POST:/admin/test/children","depends_on":["parent"],"body":{"parent_id":{"$ref":"parent.resource_id"}}},
			{"id":"grant","endpoint_key":"POST:/admin/test/grants","depends_on":["child"],"body":{"child_id":{"$ref":"child.resource_id"}}}
		]}`}}
	result := service.executeTool(context.Background(), AIAgentActor{UserID: 24}, session, "create dependent resources", call, true, map[string]string{}, "compact")
	if !strings.Contains(result, `"status":"error"`) {
		t.Fatalf("plan failure result = %s", result)
	}
	want := []string{"POST /api/v1/admin/test/parents", "GET /api/v1/admin/test/parents/81", "POST /api/v1/admin/test/children", "GET /api/v1/admin/test/parents/81", "DELETE /api/v1/admin/test/parents/81", "GET /api/v1/admin/test/parents/81"}
	if fmt.Sprint(executed) != fmt.Sprint(want) {
		t.Fatalf("executed = %v, want %v", executed, want)
	}
	var planResult map[string]any
	if err := json.Unmarshal([]byte(result), &planResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	plan, _ := planResult["plan"].(map[string]any)
	nodes, _ := plan["nodes"].([]any)
	statuses := make([]string, 0, len(nodes))
	for _, raw := range nodes {
		node, _ := raw.(map[string]any)
		statuses = append(statuses, fmt.Sprint(node["status"]))
	}
	if fmt.Sprint(statuses) != fmt.Sprint([]string{"rolled_back", "failed", "blocked"}) {
		t.Fatalf("plan statuses = %v", statuses)
	}
}

func TestAIAgentFailedAutoPlanPersistsCompletedNodeRollbacks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/test/parents":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":31,"name":"Parent"}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/test/parents/31":
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":31,"name":"Parent"}}`))
		case request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/test/children":
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"error":"rejected"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	service, err := NewAIAgentService(nil, nil, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	for _, operation := range []AgentCatalogOperation{
		{Key: "POST:/admin/test/parents", Method: http.MethodPost, Path: "/admin/test/parents", Title: "Create parent", BodySchema: agentTestOpenBodySchema()},
		{Key: "DELETE:/admin/test/parents/:id", Method: http.MethodDelete, Path: "/admin/test/parents/:id", PathParams: []string{"id"}, Title: "Delete parent"},
		{Key: "POST:/admin/test/children", Method: http.MethodPost, Path: "/admin/test/children", Title: "Create child", BodySchema: agentTestOpenBodySchema()},
	} {
		service.catalogByKey[operation.Key] = operation
		service.catalog = append(service.catalog, operation)
	}
	session := newAIAgentSession("partial plan")
	call := agentToolCall{Function: agentToolFunction{Name: "plan_admin_operations", Arguments: `{"title":"Partial create","failure_policy":"stop_on_failure","nodes":[{"id":"parent","endpoint_key":"POST:/admin/test/parents","body":{"name":"Parent"}},{"id":"child","endpoint_key":"POST:/admin/test/children","depends_on":["parent"],"body":{"parent_id":{"$ref":"parent.resource_id"}}}]}`}}
	result := service.executeTool(context.Background(), AIAgentActor{UserID: 1}, session, "create resources", call, true, map[string]string{}, "compact")
	if !strings.Contains(result, `"status":"error"`) || !strings.Contains(result, `"rollback_available":true`) {
		t.Fatalf("failed plan result = %s", result)
	}
	if len(session.rollbacks) != 1 || session.rollbacks[0].Strategy != agentRollbackStrategyPlan || len(session.rollbacks[0].Children) != 1 {
		t.Fatalf("failed plan rollbacks = %#v", session.rollbacks)
	}
}

func TestAIAgentPartialPlanRollbacksMergeAcrossRetries(t *testing.T) {
	now := time.Now()
	child := func(id, path string) AIAgentRollback {
		return AIAgentRollback{ID: id, Operation: "Create", Strategy: agentRollbackStrategyDelete, Method: http.MethodDelete, Path: path, CreatedAt: now, UpdatedAt: now}
	}
	existing := []AIAgentRollback{{ID: "parent", PlanID: "plan-1", Strategy: agentRollbackStrategyPlan, Children: []AIAgentRollback{child("a", "/admin/users/1")}, CreatedAt: now, UpdatedAt: now}}
	added := []AIAgentRollback{{ID: "duplicate-parent", PlanID: "plan-1", Strategy: agentRollbackStrategyPlan, Children: []AIAgentRollback{child("a-new-id", "/admin/users/1"), child("b", "/admin/groups/2")}, CreatedAt: now, UpdatedAt: now}}
	merged := appendAgentRollbacks(existing, added)
	if len(merged) != 1 || merged[0].ID != "parent" || len(merged[0].Children) != 2 {
		t.Fatalf("merged plan rollbacks = %#v", merged)
	}
}

func TestAIAgentRecoversMissingCreateRollbacksFromFailedPlanHistory(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	plan := AIAgentExecutionPlan{
		ID: "failed-plan", Title: "Create user and group", Status: "failed", FailurePolicy: "stop_on_failure", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Nodes: []AIAgentPlanNode{
			{ID: "user", EndpointKey: "POST:/admin/users", Operation: "Create user", Resource: "users", Body: map[string]any{"username": "user", "email": "user@example.com", "password": "[REDACTED]"}, Status: "succeeded", Outputs: map[string]any{"resource_id": float64(21), "resource_name": "user"}},
			{ID: "group", EndpointKey: "POST:/admin/groups", Operation: "Create group", Resource: "groups", Body: map[string]any{"name": "group", "platform": "openai"}, Status: "succeeded", Outputs: map[string]any{"resource_id": float64(22), "resource_name": "group"}},
			{ID: "limits", EndpointKey: "PUT:/admin/groups/:id", Operation: "Set limits", Resource: "groups", Status: "failed"},
		},
	}
	toolResult, _ := json.Marshal(map[string]any{"status": "error", "plan": plan})
	session := newAIAgentSession("failed plan")
	session.model = []agentModelMessage{{Role: "tool", Name: "plan_admin_operations", Content: string(toolResult)}}
	if !service.recoverMissingAgentPlanRollbacks(session) {
		t.Fatal("failed plan history did not recover rollbacks")
	}
	if len(session.rollbacks) != 1 || session.rollbacks[0].PlanID != plan.ID || len(session.rollbacks[0].Children) != 2 {
		t.Fatalf("recovered rollbacks = %#v", session.rollbacks)
	}
	if session.rollbacks[0].Children[0].Path != "/admin/users/21" || session.rollbacks[0].Children[1].Path != "/admin/groups/22" {
		t.Fatalf("recovered rollback paths = %#v", session.rollbacks[0].Children)
	}
	if service.recoverMissingAgentPlanRollbacks(session) || len(session.rollbacks) != 1 {
		t.Fatalf("historical recovery was not idempotent: %#v", session.rollbacks)
	}
}

func TestAIAgentRecoversMissingSubscriptionCompensationFromSuccessfulPlan(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	now := time.Now()
	plan := AIAgentExecutionPlan{
		ID: "successful-subscription-plan", Title: "Convert and assign", Status: "succeeded", FailurePolicy: "stop_on_failure", CreatedAt: now, UpdatedAt: now,
		Nodes: []AIAgentPlanNode{
			{ID: "convert", EndpointKey: "PUT:/admin/groups/:id", Operation: "Update group", Resource: "groups", Status: "succeeded"},
			{ID: "assign", EndpointKey: "POST:/admin/subscriptions/assign", Operation: "Assign subscription", Resource: "subscriptions", Body: map[string]any{"user_id": float64(4), "group_id": float64(6), "validity_days": float64(30)}, Status: "succeeded", Outputs: map[string]any{"resource_id": float64(31)}},
		},
	}
	toolResult, _ := json.Marshal(map[string]any{"status": "succeeded", "plan": plan})
	session := newAIAgentSession("successful subscription plan")
	session.model = []agentModelMessage{{Role: "tool", Name: "plan_admin_operations", Content: string(toolResult)}}
	session.rollbacks = []AIAgentRollback{{
		ID: "existing-parent", PlanID: plan.ID, Operation: plan.Title, Strategy: agentRollbackStrategyPlan, Status: "completed", CompletedAt: &now,
		Children:  []AIAgentRollback{{ID: "group-restore", Operation: "Update group", Strategy: agentRollbackStrategyRestore, Method: http.MethodPut, Path: "/admin/groups/6", CreatedAt: now, UpdatedAt: now}},
		CreatedAt: now, UpdatedAt: now,
	}}
	if !service.recoverMissingAgentPlanRollbacks(session) {
		t.Fatal("successful plan history did not recover missing subscription compensation")
	}
	if len(session.rollbacks) != 1 || len(session.rollbacks[0].Children) != 2 || session.rollbacks[0].Children[1].Path != "/admin/subscriptions/31" || session.rollbacks[0].Status != "available" || session.rollbacks[0].CompletedAt != nil {
		t.Fatalf("recovered subscription rollback = %#v", session.rollbacks)
	}
	if service.recoverMissingAgentPlanRollbacks(session) {
		t.Fatalf("successful plan recovery was not idempotent: %#v", session.rollbacks)
	}
}

func TestAIAgentCreateRollbackSupportsArrayResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.Method != http.MethodGet || !strings.HasPrefix(request.URL.Path, "/api/v1/admin/redeem-codes/") {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		id := strings.TrimPrefix(request.URL.Path, "/api/v1/admin/redeem-codes/")
		_, _ = fmt.Fprintf(writer, `{"code":0,"data":{"id":%s,"code":"code-%s","type":"subscription","status":"unused","group_id":3,"validity_days":30}}`, id, id)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	service, err := NewAIAgentService(nil, aiAgentTestEncryptor{}, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	pending := &AIAgentPendingAction{
		Operation: "Generate redeem codes", Resource: "redeem_codes", Method: http.MethodPost, Path: "/admin/redeem-codes/generate",
		Body: map[string]any{"count": float64(2), "type": "subscription", "group_id": float64(3), "validity_days": float64(30)},
	}
	created := func(id float64) map[string]any {
		return map[string]any{"id": id, "code": fmt.Sprintf("code-%.0f", id), "type": "subscription", "status": "unused", "group_id": float64(3), "validity_days": float64(30)}
	}

	single := service.prepareAgentCreateRollback(context.Background(), AIAgentActor{UserID: 1}, pending, map[string]any{"data": []any{created(3)}})
	if single == nil || single.Strategy != agentRollbackStrategyDelete || single.Path != "/admin/redeem-codes/3" || single.TargetID != "3" {
		t.Fatalf("single array create rollback = %#v", single)
	}
	batch := service.prepareAgentCreateRollback(context.Background(), AIAgentActor{UserID: 1}, pending, map[string]any{"data": []any{created(3), created(4)}})
	if batch == nil || batch.Strategy != agentRollbackStrategyPlan || len(batch.Children) != 2 {
		t.Fatalf("batch array create rollback = %#v", batch)
	}
	if batch.Children[0].Path != "/admin/redeem-codes/3" || batch.Children[1].Path != "/admin/redeem-codes/4" {
		t.Fatalf("batch rollback paths = %#v", batch.Children)
	}
	if operation, path, ok := service.findAgentDeleteCompensation("/admin/redeem-codes/batch-update", float64(3)); ok {
		t.Fatalf("update action incorrectly matched create compensation: %s %s", operation.Key, path)
	}
}

func TestAIAgentRollbackPreviewExecutesVerifiedNestedRestoreAndRetainsAuditRecord(t *testing.T) {
	state := map[string]any{
		"id": float64(3), "name": "Account", "concurrency": float64(10),
		"credentials": map[string]any{"api_key": "new-secret", "base_url": "https://api.example.com"},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodGet:
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": state})
		case http.MethodPut:
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode rollback body: %v", err)
			}
			for field, value := range body {
				state[field] = value
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": state})
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	settings := &aiAgentMemorySettings{values: map[string]string{agentSettingEnabled: "true"}}
	internalAuth, _ := NewAgentInternalAuth()
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	conversation, err := service.conversation(context.Background(), 91, "", true)
	if err != nil {
		t.Fatalf("conversation() error = %v", err)
	}
	now := time.Now()
	rollback := AIAgentRollback{
		ID: "verified-rollback", Operation: "Update account", Strategy: agentRollbackStrategyRestore, Status: "available",
		TargetLabel: "Account", Method: http.MethodPut, Path: "/admin/accounts/3", Sensitive: true, RequiresStepUp: true,
		Body:        map[string]any{"concurrency": float64(5), "credentials.api_key": "old-secret"},
		ForwardBody: map[string]any{"concurrency": float64(10), "credentials.api_key": "new-secret"},
		Changes: []AIAgentChange{
			{Field: "concurrency", Before: float64(5), After: float64(10)},
			{Field: "credentials.api_key", Before: "old-secret", After: "new-secret"},
		},
		IdempotencyKey: "rollback-idempotency", CreatedAt: now, UpdatedAt: now,
	}
	conversation.mu.Lock()
	conversation.rollbacks = []AIAgentRollback{rollback}
	conversation.mu.Unlock()
	preview, err := service.PreviewRollback(context.Background(), AIAgentActor{UserID: 91}, conversation.id, rollback.ID)
	if err != nil || preview.Status != "safe" || preview.ChangeCount != 2 || !preview.CanExecute {
		t.Fatalf("PreviewRollback() = %#v, %v", preview, err)
	}
	encodedPreview, _ := json.Marshal(preview)
	if strings.Contains(string(encodedPreview), "old-secret") || strings.Contains(string(encodedPreview), "new-secret") {
		t.Fatalf("public rollback preview leaked a credential: %s", encodedPreview)
	}
	if _, err := service.Rollback(context.Background(), AIAgentActor{UserID: 91}, conversation.id, rollback.ID, true); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	credentials, _ := state["credentials"].(map[string]any)
	if state["concurrency"] != float64(5) || credentials["api_key"] != "old-secret" || credentials["base_url"] != "https://api.example.com" {
		t.Fatalf("verified rollback state = %#v", state)
	}
	snapshot, err := service.Session(context.Background(), 91, conversation.id)
	if err != nil || len(snapshot.Rollbacks) != 1 || snapshot.Rollbacks[0].Status != "completed" {
		t.Fatalf("completed rollback audit record = %#v, %v", snapshot.Rollbacks, err)
	}
	publicSnapshot, _ := json.Marshal(snapshot)
	if strings.Contains(string(publicSnapshot), "old-secret") || strings.Contains(string(publicSnapshot), "new-secret") || strings.Contains(string(publicSnapshot), "forward_body") {
		t.Fatalf("public session leaked private rollback data: %s", publicSnapshot)
	}
}

func TestAIAgentPlanRollbackPreflightsAndExecutesInReverseOrder(t *testing.T) {
	states := map[string]float64{"/api/v1/admin/items/1": 11, "/api/v1/admin/items/2": 22}
	var writes []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		value, exists := states[request.URL.Path]
		if !exists {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		if request.Method == http.MethodPut {
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			value, _ = body["value"].(float64)
			states[request.URL.Path] = value
			writes = append(writes, request.URL.Path)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{"id": request.URL.Path, "value": value}})
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	service := &AIAgentService{cfg: &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth: internalAuth, client: server.Client()}
	child := func(id string, before, after float64) AIAgentRollback {
		return AIAgentRollback{
			ID: id, Operation: "Update item " + id, Strategy: agentRollbackStrategyRestore, Status: "available",
			Method: http.MethodPut, Path: "/admin/items/" + id, Body: map[string]any{"value": before},
			ForwardBody: map[string]any{"value": after}, Changes: []AIAgentChange{{Field: "value", Before: before, After: after}},
			IdempotencyKey: "rollback-" + id, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
	}
	plan := AIAgentRollback{
		ID: "plan", Operation: "Update items", Strategy: agentRollbackStrategyPlan, Status: "available", Method: "PLAN", Path: "plan",
		Children: []AIAgentRollback{child("1", 1, 11), child("2", 2, 22)}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	preview := service.rollbackPreviewForRecord(context.Background(), AIAgentActor{UserID: 1}, plan)
	if !preview.CanExecute || preview.Status != "safe" || preview.ChangeCount != 2 ||
		preview.Fields[0].Resource != "items" || preview.Fields[0].TargetID == "" || preview.Fields[1].TargetID == "" {
		t.Fatalf("plan rollback preview = %#v", preview)
	}
	if err := service.executeAgentRollbackRecord(context.Background(), AIAgentActor{UserID: 1}, &plan); err != nil {
		t.Fatalf("executeAgentRollbackRecord() error = %v", err)
	}
	if fmt.Sprint(writes) != fmt.Sprint([]string{"/api/v1/admin/items/2", "/api/v1/admin/items/1"}) || states["/api/v1/admin/items/1"] != 1 || states["/api/v1/admin/items/2"] != 2 {
		t.Fatalf("reverse plan rollback writes=%v states=%v", writes, states)
	}
	if plan.Children[0].Status != "completed" || plan.Children[1].Status != "completed" {
		t.Fatalf("plan child audit states = %#v", plan.Children)
	}
}

func TestAIAgentRollbackPreviewBlocksFieldDrift(t *testing.T) {
	service := &AIAgentService{client: http.DefaultClient}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":{"id":3,"name":"OpenAI账号","concurrency":99}}`))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	service.cfg = &config.Config{Server: config.ServerConfig{Port: port}}
	service.internalAuth = internalAuth
	service.client = server.Client()
	preview := service.rollbackPreviewForRecord(context.Background(), AIAgentActor{UserID: 1}, AIAgentRollback{
		ID: "conflict", Strategy: agentRollbackStrategyRestore, Status: "available", Method: http.MethodPut, Path: "/admin/accounts/3",
		Changes: []AIAgentChange{{Field: "concurrency", Before: float64(5), After: float64(10)}},
	})
	if preview.Status != "conflict" || preview.CanExecute || preview.ConflictCount != 1 || preview.Fields[0].Current != float64(99) ||
		preview.Rollback.Resource != "accounts" || preview.Rollback.TargetID != "3" || preview.Rollback.TargetLabel != "OpenAI账号" {
		t.Fatalf("drift preview = %#v", preview)
	}
}

func TestAIAgentRunningExecutionPlanRecoversAsStoppedAfterRestart(t *testing.T) {
	settings := &aiAgentMemorySettings{values: map[string]string{agentSettingEnabled: "true"}}
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	session := newAIAgentSession("restart plan")
	plan := &AIAgentExecutionPlan{
		ID: uuid.NewString(), Title: "Restart plan", FailurePolicy: "stop_on_failure", Status: "running",
		Nodes: []AIAgentPlanNode{
			{ID: "one", EndpointKey: "POST:/admin/test/one", Status: "planned", IdempotencyKey: uuid.NewString()},
			{ID: "two", EndpointKey: "POST:/admin/test/two", DependsOn: []string{"one"}, Status: "planned", IdempotencyKey: uuid.NewString()},
		}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	plan.Fingerprint = agentExecutionPlanFingerprint(plan)
	session.status = agentConversationStatusRunning
	session.pending = &AIAgentPendingAction{ID: uuid.NewString(), Plan: plan, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(10 * time.Minute)}
	session.rollbacks = []AIAgentRollback{{ID: "interrupted", Strategy: agentRollbackStrategyRestore, Status: "running", CreatedAt: time.Now()}}
	service.sessions[27] = map[string]*aiAgentSession{session.id: session}
	service.loaded[27] = true
	if err := service.persistConversations(context.Background(), 27); err != nil {
		t.Fatalf("persist running plan: %v", err)
	}
	reloaded, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, nil, nil)
	if err != nil {
		t.Fatalf("reload service: %v", err)
	}
	recovered, err := reloaded.conversation(context.Background(), 27, session.id, false)
	if err != nil {
		t.Fatalf("load recovered plan: %v", err)
	}
	if recovered.status != agentConversationStatusStopped || recovered.pending == nil || recovered.pending.Plan.Status != "stopped" {
		t.Fatalf("recovered status=%s pending=%#v", recovered.status, recovered.pending)
	}
	if recovered.pending.Plan.Fingerprint != plan.Fingerprint || recovered.pending.Plan.Nodes[0].IdempotencyKey != plan.Nodes[0].IdempotencyKey {
		t.Fatal("restart changed the immutable plan or its idempotency keys")
	}
	if len(recovered.rollbacks) != 1 || recovered.rollbacks[0].Status != "failed" || !strings.Contains(recovered.rollbacks[0].Error, "server restarted") {
		t.Fatalf("interrupted rollback recovery = %#v", recovered.rollbacks)
	}
}

func TestAIAgentConfirmedExecutionPlanRunsInBackgroundAndStops(t *testing.T) {
	started := make(chan struct{}, 1)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-request.Context().Done()
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, _ := NewAgentInternalAuth()
	settings := &aiAgentMemorySettings{values: map[string]string{agentSettingEnabled: "true"}}
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	for _, operation := range []AgentCatalogOperation{
		{Key: "POST:/admin/test/slow-one", Method: http.MethodPost, Path: "/admin/test/slow-one", Title: "Slow one"},
		{Key: "POST:/admin/test/slow-two", Method: http.MethodPost, Path: "/admin/test/slow-two", Title: "Slow two"},
	} {
		service.catalogByKey[operation.Key] = operation
	}
	session := newAIAgentSession("stoppable plan")
	service.sessions[26] = map[string]*aiAgentSession{session.id: session}
	service.loaded[26] = true
	call := agentToolCall{Function: agentToolFunction{Name: "plan_admin_operations", Arguments: `{
		"title":"Stoppable plan","failure_policy":"stop_on_failure","nodes":[
			{"id":"one","endpoint_key":"POST:/admin/test/slow-one"},
			{"id":"two","endpoint_key":"POST:/admin/test/slow-two","depends_on":["one"]}
		]}`}}
	result := service.executeTool(context.Background(), AIAgentActor{UserID: 26}, session, "run stoppable plan", call, false, map[string]string{})
	if !strings.Contains(result, `"status":"confirmation_required"`) {
		t.Fatalf("stage plan = %s", result)
	}
	if _, err := service.Confirm(context.Background(), AIAgentActor{UserID: 26}, session.id, session.pending.ID, false); err != nil {
		t.Fatalf("confirm plan: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("background plan did not start")
	}
	if !service.Stop(26, session.id) {
		t.Fatal("Stop() did not find the running plan")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session.mu.Lock()
		status := session.status
		session.mu.Unlock()
		if status == agentConversationStatusStopped {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	session.mu.Lock()
	status := session.status
	pending := session.pending
	session.mu.Unlock()
	if status != agentConversationStatusStopped || pending == nil || requests.Load() != 1 {
		t.Fatalf("stopped plan status=%s pending=%#v requests=%d", status, pending, requests.Load())
	}
}

func TestAIAgentContextWindowParsingAndPersistence(t *testing.T) {
	cases := []struct {
		input      string
		normalized string
		tokens     int
	}{
		{"", "150k", 150000},
		{"150K", "150k", 150000},
		{"1m", "1m", 1000000},
		{"150000", "150k", 150000},
		{"250001", "250001", 250001},
	}
	for _, test := range cases {
		normalized, tokens, err := normalizeAIAgentContextWindow(test.input)
		if err != nil || normalized != test.normalized || tokens != test.tokens {
			t.Fatalf("normalizeAIAgentContextWindow(%q) = %q, %d, %v; want %q, %d", test.input, normalized, tokens, err, test.normalized, test.tokens)
		}
	}
	for _, invalid := range []string{"15k", "1.5m", "150kb", "0", "9m", "hello"} {
		if _, _, err := normalizeAIAgentContextWindow(invalid); err == nil {
			t.Fatalf("invalid context window %q was accepted", invalid)
		}
	}

	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	config, err := service.Config(context.Background())
	if err != nil || config.Enabled || config.ContextWindow != "150k" || config.ContextWindowTokens != 150000 {
		t.Fatalf("default context config = %#v, %v", config, err)
	}
	value := "1m"
	config, err = service.UpdateConfig(context.Background(), UpdateAIAgentConfigInput{ContextWindow: &value})
	if err != nil || config.ContextWindow != "1m" || config.ContextWindowTokens != 1000000 || settings.values[agentSettingContextWindow] != "1m" {
		t.Fatalf("updated context config = %#v, stored=%q, err=%v", config, settings.values[agentSettingContextWindow], err)
	}
}

func TestAIAgentEnabledDefaultsOffAndDisablingStopsWork(t *testing.T) {
	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	config, err := service.Config(context.Background())
	if err != nil || config.Enabled || config.ExecutionTopology != "single_instance" || config.MultiInstanceSafe {
		t.Fatalf("default config = %#v, err=%v", config, err)
	}
	if _, err := service.CreateConversation(context.Background(), 1); !errors.Is(err, ErrAIAgentDisabled) {
		t.Fatalf("default-disabled CreateConversation() error = %v", err)
	}
	enabled := true
	config, err = service.UpdateConfig(context.Background(), UpdateAIAgentConfigInput{Enabled: &enabled})
	if err != nil || !config.Enabled || settings.values[agentSettingEnabled] != "true" {
		t.Fatalf("enabled config = %#v, stored=%q, err=%v", config, settings.values[agentSettingEnabled], err)
	}

	jobContext, cancel := context.WithCancel(context.Background())
	service.jobs["test-job"] = cancel
	disabled := false
	config, err = service.UpdateConfig(context.Background(), UpdateAIAgentConfigInput{Enabled: &disabled})
	if err != nil || config.Enabled || settings.values[agentSettingEnabled] != "false" {
		t.Fatalf("disabled config = %#v, stored=%q, err=%v", config, settings.values[agentSettingEnabled], err)
	}
	select {
	case <-jobContext.Done():
	default:
		t.Fatal("disabling the Agent did not cancel running work")
	}
	if _, err := service.StartChat(context.Background(), AIAgentActor{UserID: 1}, "", "hello"); !errors.Is(err, ErrAIAgentDisabled) {
		t.Fatalf("disabled StartChat() error = %v", err)
	}
}

func TestAIAgentContextCompressionPreservesCurrentToolChain(t *testing.T) {
	secret := "sk-context-secret-123456789"
	var history []agentModelMessage
	for index := 0; index < 10; index++ {
		history = append(history,
			agentModelMessage{Role: "user", Content: fmt.Sprintf("old request %d %s %s", index, secret, strings.Repeat("历史内容", 900))},
			agentModelMessage{Role: "assistant", Content: fmt.Sprintf("old answer %d %s", index, strings.Repeat("result ", 700))},
		)
	}
	currentCall := agentToolCall{ID: "current-call", Type: "function", Function: agentToolFunction{Name: "execute_admin_operation", Arguments: `{"endpoint_key":"GET:/admin/users","query":{"search":"alice@example.com"}}`}}
	history = append(history,
		agentModelMessage{Role: "user", Content: "find the exact current user"},
		agentModelMessage{Role: "assistant", ToolCalls: []agentToolCall{currentCall}},
		agentModelMessage{Role: "tool", Name: "execute_admin_operation", ToolCallID: "current-call", Content: `{"status":"success","data":{"id":42,"email":"alice@example.com"}}`},
	)
	config := AIAgentConfig{Protocol: agentProtocolResponses, ContextWindow: "16k", ContextWindowTokens: 16000}
	compacted, report, err := prepareAgentModelContext(config, history)
	if err != nil {
		t.Fatalf("prepareAgentModelContext() error = %v", err)
	}
	if !report.Compressed || report.AfterTokens > report.InputBudget || report.DroppedTurns == 0 {
		t.Fatalf("compression report = %#v", report)
	}
	memory := modelMessageText(compacted[0].Content)
	if !strings.HasPrefix(memory, agentCompressedMemoryPrefix) || strings.Contains(memory, secret) {
		t.Fatalf("compressed memory is missing or leaked a secret: %s", memory)
	}
	if len(compacted) < 3 {
		t.Fatalf("compacted history contains only %d messages", len(compacted))
	}
	last := compacted[len(compacted)-3:]
	if last[0].Role != "user" || last[1].Role != "assistant" || len(last[1].ToolCalls) != 1 || last[1].ToolCalls[0].ID != "current-call" || last[2].Role != "tool" || last[2].ToolCallID != "current-call" {
		t.Fatalf("current tool chain changed during compression: %#v", last)
	}
}

func TestAIAgentContextCompressionCheckpointsCompletedToolCycles(t *testing.T) {
	firstCall := agentToolCall{ID: "call-one", Type: "function", Function: agentToolFunction{Name: "execute_admin_operation", Arguments: `{"endpoint_key":"GET:/admin/groups"}`}}
	secondCall := agentToolCall{ID: "call-two", Type: "function", Function: agentToolFunction{Name: "execute_admin_operation", Arguments: `{"endpoint_key":"GET:/admin/users/:id","path_params":{"id":42}}`}}
	activeOutput := `{"status":"success","data":{"id":42,"email":"alice@example.com"}}`
	history := []agentModelMessage{
		{Role: "user", Content: "find VIP group, then inspect user 42"},
		{Role: "assistant", ToolCalls: []agentToolCall{firstCall}},
		{Role: "tool", Name: "execute_admin_operation", ToolCallID: "call-one", Content: fmt.Sprintf(`{"status":"success","data":{"id":101,"name":"VIP","description":%q},"token":"sk-cycle-secret-123456789"}`, strings.Repeat("large result ", 7000))},
		{Role: "assistant", ToolCalls: []agentToolCall{secondCall}},
		{Role: "tool", Name: "execute_admin_operation", ToolCallID: "call-two", Content: activeOutput},
	}
	config := AIAgentConfig{Protocol: agentProtocolResponses, ContextWindow: "16k", ContextWindowTokens: 16000}
	compacted, report, err := prepareAgentModelContext(config, history)
	if err != nil {
		t.Fatalf("prepareAgentModelContext() error = %v", err)
	}
	if !report.Compressed || report.AfterTokens > report.InputBudget {
		t.Fatalf("compression report = %#v", report)
	}
	encoded, _ := json.Marshal(compacted)
	contextText := string(encoded)
	for _, expected := range []string{"find VIP group", "Completed execution checkpoint", "id=101", "name=VIP", "call-two", "alice@example.com"} {
		if !strings.Contains(contextText, expected) {
			t.Fatalf("compacted context lacks %q: %s", expected, contextText)
		}
	}
	if strings.Contains(contextText, "sk-cycle-secret") || strings.Contains(contextText, "call-one") {
		t.Fatalf("checkpoint leaked a secret or retained a completed tool protocol ID: %s", contextText)
	}
	if got := modelMessageText(compacted[len(compacted)-1].Content); got != activeOutput {
		t.Fatalf("active tool output changed: %s", got)
	}
	tampered := append([]agentModelMessage(nil), compacted...)
	tampered[len(tampered)-1].Content = `{"status":"changed"}`
	if err := validateAgentContextContinuity(history, tampered); err == nil {
		t.Fatal("active tool-chain tampering passed continuity validation")
	}
}

func TestAIAgentContextWindowProviderRetryCompactsMoreAggressively(t *testing.T) {
	history := []agentModelMessage{
		{Role: "user", Content: strings.Repeat("older request ", 8000)},
		{Role: "assistant", Content: strings.Repeat("older response ", 8000)},
		{Role: "user", Content: "current request"},
	}
	config := AIAgentConfig{Protocol: agentProtocolResponses, ContextWindow: "1m", ContextWindowTokens: 1000000}
	initial, initialReport, err := prepareAgentModelContext(config, history)
	if err != nil || initialReport.Compressed || len(initial) != len(history) {
		t.Fatalf("initial context unexpectedly compressed: report=%#v err=%v", initialReport, err)
	}
	retried, retryReport, err := prepareAgentModelContextRetry(config, history, 70)
	if err != nil || !retryReport.Compressed || retryReport.AfterTokens >= retryReport.BeforeTokens || len(retried) >= len(history) {
		t.Fatalf("provider retry compression = %#v, messages=%d, err=%v", retryReport, len(retried), err)
	}
	if !isAgentContextWindowError(errors.New("maximum context length exceeded")) || isAgentContextWindowError(errors.New("TLS timeout")) {
		t.Fatal("provider context error classification is incorrect")
	}
}

func TestAIAgentContextCompressionKeepsProviderSignedBlocks(t *testing.T) {
	signed := json.RawMessage(`{"type":"thinking","thinking":"private","signature":"signed-value"}`)
	history := []agentModelMessage{
		{Role: "user", Content: strings.Repeat("older context ", 9000)},
		{Role: "assistant", Content: "older answer"},
		{Role: "user", Content: "current request"},
		{Role: "assistant", AnthropicContent: []json.RawMessage{signed}, Content: "current answer"},
	}
	config := AIAgentConfig{Protocol: agentProtocolMessages, ContextWindow: "32k", ContextWindowTokens: 32000}
	compacted, _, err := prepareAgentModelContext(config, history)
	if err != nil {
		t.Fatalf("prepareAgentModelContext() error = %v", err)
	}
	last := compacted[len(compacted)-1]
	if len(last.AnthropicContent) != 1 || string(last.AnthropicContent[0]) != string(signed) {
		t.Fatalf("signed Messages block changed: %#v", last.AnthropicContent)
	}
}

func TestNormalizeAIAgentProtocolAndThinkingMode(t *testing.T) {
	for _, protocol := range []string{agentProtocolChatCompletions, agentProtocolResponses, agentProtocolMessages} {
		if got, err := normalizeAIAgentProtocol(protocol); err != nil || got != protocol {
			t.Fatalf("normalizeAIAgentProtocol(%q) = %q, %v", protocol, got, err)
		}
	}
	if _, err := normalizeAIAgentProtocol("custom"); err == nil {
		t.Fatal("expected unsupported protocol to be rejected")
	}
	for _, mode := range []string{"off", "compact", "full"} {
		if got, err := normalizeAIAgentProcessDisplay(mode); err != nil || got != mode {
			t.Fatalf("normalizeAIAgentProcessDisplay(%q) = %q, %v", mode, got, err)
		}
	}
	if _, err := normalizeAIAgentProcessDisplay("verbose"); err == nil {
		t.Fatal("expected unsupported process display mode to be rejected")
	}
	for _, mode := range []string{"", "low", "xhigh", "4096", "adaptive"} {
		if got, err := normalizeAIAgentThinkingMode(mode); err != nil || got != mode {
			t.Fatalf("normalizeAIAgentThinkingMode(%q) = %q, %v", mode, got, err)
		}
	}
	if _, err := normalizeAIAgentThinkingMode(`high\"}],\"tools\":[`); err == nil {
		t.Fatal("expected structured thinking mode injection to be rejected")
	}
}

func TestNormalizeAIAgentBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"models.example/v1", "https://models.example"},
		{"https://models.example/api/v1/", "https://models.example/api"},
	}
	for _, test := range tests {
		got, err := normalizeAIAgentBaseURL(test.input)
		if err != nil {
			t.Fatalf("normalizeAIAgentBaseURL(%q) error = %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("normalizeAIAgentBaseURL(%q) = %q, want %q", test.input, got, test.want)
		}
	}
	if _, err := normalizeAIAgentBaseURL("https://user:secret@models.example/v1"); err == nil {
		t.Fatal("expected embedded credentials to be rejected")
	}
}

func TestRenderAgentOperationPathEscapesParameters(t *testing.T) {
	operation := AgentCatalogOperation{Path: "/admin/users/:id", PathParams: []string{"id"}}
	path, err := renderAgentOperationPath(operation, map[string]any{"id": "user/42"})
	if err != nil {
		t.Fatalf("renderAgentOperationPath() error = %v", err)
	}
	if path != "/admin/users/user%2F42" {
		t.Fatalf("path = %q", path)
	}
}

func TestAIAgentSensitiveWriteRequiresConfirmationInsteadOfBlocking(t *testing.T) {
	service := &AIAgentService{catalogByKey: map[string]AgentCatalogOperation{
		"POST:/admin/accounts": {Key: "POST:/admin/accounts", Module: "accounts", Method: http.MethodPost, Path: "/admin/accounts", Title: "创建", BodySchema: agentTestOpenBodySchema()},
	}}
	session := &aiAgentSession{observed: make(map[string]bool)}
	call := agentToolCall{Function: agentToolFunction{Name: "execute_admin_operation", Arguments: `{"endpoint_key":"POST:/admin/accounts","body":{"name":"OpenAI account","platform":"openai","api_key":"sk-sensitive","base_url":"https://api.example.com"}}`}}
	result := service.executeTool(context.Background(), AIAgentActor{}, session, "create an account", call, false, make(map[string]string))
	if !strings.Contains(result, `"status":"confirmation_required"`) {
		t.Fatalf("tool result = %s", result)
	}
	if session.pending == nil || !session.pending.Sensitive || !session.pending.RequiresStepUp {
		t.Fatalf("pending action = %#v", session.pending)
	}
	if session.pending.Operation != "创建账号" || session.pending.TargetLabel != "OpenAI account" || len(session.pending.Preview) == 0 {
		t.Fatalf("human-readable pending action = %#v", session.pending)
	}
	previewJSON, _ := json.Marshal(session.pending.Preview)
	if strings.Contains(string(previewJSON), "sk-sensitive") || strings.Contains(string(previewJSON), "credentials") {
		t.Fatalf("pending preview leaked credentials: %s", previewJSON)
	}
	body := agentTestMap(t, session.pending.Body)
	credentials := agentTestMap(t, body["credentials"])
	if body["type"] != "apikey" || credentials["api_key"] != "sk-sensitive" || body["api_key"] != nil || body["concurrency"] != 10 || body["priority"] != 1 {
		t.Fatalf("normalized server-side body = %#v", body)
	}
	previewJSON, _ = json.Marshal(session.pending.Preview)
	if !strings.Contains(string(previewJSON), `"field":"concurrency"`) || !strings.Contains(string(previewJSON), `"after":10`) ||
		!strings.Contains(string(previewJSON), `"field":"priority"`) || !strings.Contains(string(previewJSON), `"after":1`) {
		t.Fatalf("account defaults are missing from confirmation preview: %s", previewJSON)
	}
	publicBody := agentTestMap(t, publicAgentPending(session.pending).Body)
	if publicBody["credentials"] != "[REDACTED]" {
		t.Fatalf("public pending body = %#v", publicBody)
	}
}

func TestAIAgentTextSecretsAreRedactedFromPublicHistory(t *testing.T) {
	input := "create it with sk-99ff7b1ee7fe9d8818ace2831 and API Key: another-secret-value"
	redacted := redactAgentTextSecrets(input)
	if strings.Contains(redacted, "99ff7b1") || strings.Contains(redacted, "another-secret") {
		t.Fatalf("redacted text leaked a credential: %s", redacted)
	}
	conversation := &aiAgentSession{
		id: "conversation", title: input, status: agentConversationStatusIdle,
		public: []AIAgentMessage{{Role: "user", Content: input}},
	}
	snapshot := snapshotAIAgentSession(conversation)
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(string(encoded), "99ff7b1") || strings.Contains(string(encoded), "another-secret") {
		t.Fatalf("public snapshot leaked a credential: %s", encoded)
	}
}

func TestAIAgentAutoApproveExecutesSensitiveWriteWithoutPendingConfirmation(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&receivedBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":{"id":1}}`))
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	internalAuth, err := NewAgentInternalAuth()
	if err != nil {
		t.Fatalf("NewAgentInternalAuth() error = %v", err)
	}
	service := &AIAgentService{
		cfg: &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth: internalAuth, client: server.Client(),
		catalogByKey: map[string]AgentCatalogOperation{
			"POST:/admin/accounts": {Key: "POST:/admin/accounts", Method: http.MethodPost, Path: "/admin/accounts", Title: "Create account", BodySchema: agentTestOpenBodySchema()},
		},
	}
	session := &aiAgentSession{observed: make(map[string]bool)}
	call := agentToolCall{Function: agentToolFunction{Name: "execute_admin_operation", Arguments: `{"endpoint_key":"POST:/admin/accounts","body":{"name":"OpenAI account","platform":"openai","api_key":"sk-sensitive","base_url":"https://api.example.com"}}`}}
	result := service.executeTool(context.Background(), AIAgentActor{UserID: 1}, session, "create an account", call, true, make(map[string]string))
	if !strings.Contains(result, `"status":"success"`) || session.pending != nil {
		t.Fatalf("result = %s, pending = %#v", result, session.pending)
	}
	receivedCredentials, _ := receivedBody["credentials"].(map[string]any)
	if receivedBody["type"] != "apikey" || receivedCredentials["api_key"] != "sk-sensitive" || receivedBody["api_key"] != nil ||
		receivedBody["concurrency"] != float64(10) || receivedBody["priority"] != float64(1) {
		t.Fatalf("received body = %#v", receivedBody)
	}
	explicit, err := normalizeAgentOperationBody(http.MethodPost, "/admin/accounts", map[string]any{
		"name": "custom", "platform": "openai", "type": "apikey", "credentials": map[string]any{"api_key": "secret"},
		"concurrency": float64(7), "priority": float64(3),
	})
	if err != nil {
		t.Fatalf("normalize explicit account defaults: %v", err)
	}
	explicitBody := agentTestMap(t, explicit)
	if explicitBody["concurrency"] != float64(7) || explicitBody["priority"] != float64(3) {
		t.Fatalf("explicit account scheduling values were overwritten: %#v", explicitBody)
	}
}

func TestAIAgentBodyContractValidatesRequiredTypesAndEnums(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	proxySchema := service.catalogByKey["POST:/admin/proxies"].BodySchema
	if err := validateAgentBodyContract(proxySchema, map[string]any{"name": "proxy"}, "body"); err == nil || !strings.Contains(err.Error(), "host") {
		t.Fatalf("missing proxy fields error = %v", err)
	}
	accountSchema := service.catalogByKey["POST:/admin/accounts"].BodySchema
	accountProperties, _ := accountSchema["properties"].(map[string]any)
	concurrencyContract, _ := accountProperties["concurrency"].(map[string]any)
	priorityContract, _ := accountProperties["priority"].(map[string]any)
	if concurrencyContract["default"] != float64(10) || priorityContract["default"] != float64(1) {
		t.Fatalf("account scheduling defaults are missing from contract: %#v / %#v", concurrencyContract, priorityContract)
	}
	contract := service.inspectAgentOperationContract(newAIAgentSession("account-contract"), "POST:/admin/accounts", "")
	if !strings.Contains(contract, `"concurrency":{"default":10`) || !strings.Contains(contract, `"priority":{"default":1`) {
		t.Fatalf("account defaults are missing from exact contract inspection: %s", contract)
	}
	invalid := map[string]any{"name": "account", "platform": "openai", "type": "invalid", "credentials": map[string]any{"api_key": "secret"}}
	if err := validateAgentBodyContract(accountSchema, invalid, "body"); err == nil || !strings.Contains(err.Error(), "must be one of") {
		t.Fatalf("account type error = %v", err)
	}
	refreshSchema := service.catalogByKey["POST:/admin/openai/refresh-token"].BodySchema
	if err := validateAgentBodyContract(refreshSchema, map[string]any{}, "body"); err == nil || !strings.Contains(err.Error(), "refresh_token") {
		t.Fatalf("refresh token alternative error = %v", err)
	}
	batchDeleteSchema := service.catalogByKey["POST:/admin/proxies/batch-delete"].BodySchema
	if err := validateAgentBodyContract(batchDeleteSchema, map[string]any{"ids": []any{}}, "body"); err == nil || !strings.Contains(err.Error(), "at least 1 items") {
		t.Fatalf("batch delete minimum error = %v", err)
	}
	if err := validateAgentOperationSemantics(http.MethodPost, "/admin/users/batch-limits", map[string]any{"all": false, "concurrency": 2}); err == nil {
		t.Fatal("batch limits accepted all=false without user_ids")
	}
}

func TestAIAgentLargeContractsAndToolOutputsStayBounded(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	settingsSession := newAIAgentSession("settings-contract")
	settingsContract := service.inspectAgentOperationContract(settingsSession, "PUT:/admin/settings", "")
	if !strings.Contains(settingsContract, `"status":"contract_too_large"`) || !strings.Contains(settingsContract, "exact field path") {
		t.Fatalf("large settings contract did not direct field inspection: %s", settingsContract)
	}
	settingsField := service.inspectAgentOperationContract(settingsSession, "PUT:/admin/settings", "custom_menu_items[].url")
	if !strings.Contains(settingsField, `"status":"field_contract_resolved"`) || !strings.Contains(settingsField, `"maximum":2048`) {
		t.Fatalf("settings field contract lookup = %s", settingsField)
	}
	publicSchema := publicAgentBodySchema(service.catalogByKey["PUT:/admin/settings"].BodySchema)
	encodedSchema, _ := json.Marshal(publicSchema)
	if len(encodedSchema) > 3000 {
		t.Fatalf("public settings schema is too large: %d bytes", len(encodedSchema))
	}
	large := marshalAgentToolResult(map[string]any{"status": "success", "data": strings.Repeat("中文", agentMaxToolOutput)})
	bounded := boundedAgentToolOutput(large)
	if len(bounded) > agentMaxToolOutput {
		t.Fatalf("bounded output is too large: %d bytes", len(bounded))
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(bounded), &result); err != nil || result["status"] != "tool_output_truncated" || result["original_status"] != "success" {
		t.Fatalf("bounded output = %s, error = %v", bounded, err)
	}
}

func TestAIAgentInvalidContractPayloadNeverCreatesPendingAction(t *testing.T) {
	service, err := NewAIAgentService(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	session := &aiAgentSession{observed: make(map[string]bool)}
	call := agentToolCall{Function: agentToolFunction{
		Name:      "execute_admin_operation",
		Arguments: `{"endpoint_key":"POST:/admin/proxies","body":{"name":"incomplete proxy"}}`,
	}}
	result := service.executeTool(context.Background(), AIAgentActor{}, session, "create a proxy", call, false, make(map[string]string))
	if !strings.Contains(result, `"status":"invalid_payload"`) || !strings.Contains(result, "host") || !strings.Contains(result, "body_schema") {
		t.Fatalf("tool result = %s", result)
	}
	if session.pending != nil {
		t.Fatalf("invalid payload created pending action: %#v", session.pending)
	}
}

func TestAIAgentAccountCreationRejectsIncompletePayloadBeforeConfirmation(t *testing.T) {
	_, err := normalizeAgentOperationBody(http.MethodPost, "/admin/accounts", map[string]any{"name": "Incomplete"})
	if err == nil || !strings.Contains(err.Error(), "body.platform") {
		t.Fatalf("normalizeAgentOperationBody() error = %v", err)
	}
}

func TestAIAgentSensitivePendingCannotUseRegularConfirmEndpoint(t *testing.T) {
	pending := &AIAgentPendingAction{ID: "action", RequiresStepUp: true, ExpiresAt: time.Now().Add(time.Minute)}
	conversation := &aiAgentSession{id: "conversation", pending: pending}
	service := &AIAgentService{
		settings: &aiAgentMemorySettings{values: map[string]string{agentSettingEnabled: "true"}},
		sessions: map[int64]map[string]*aiAgentSession{1: {"conversation": conversation}},
		active:   map[int64]string{1: "conversation"},
		loaded:   map[int64]bool{1: true},
	}
	if _, err := service.Confirm(context.Background(), AIAgentActor{UserID: 1}, "conversation", "action", false); err == nil || !strings.Contains(err.Error(), "step-up") {
		t.Fatalf("Confirm() error = %v", err)
	}
	if conversation.pending != pending {
		t.Fatal("rejected confirmation removed the pending action")
	}
}

func TestAIAgentCatalogEventUsesNonSecretEndpointKeyLabel(t *testing.T) {
	detail := []any{map[string]any{"key": "POST:/admin/accounts", "title": "Create"}}
	converted := agentToolResultEventDetail(agentToolCall{Function: agentToolFunction{Name: "search_admin_operations"}}, detail)
	conversation := &aiAgentSession{}
	appendAgentEvent(conversation, "full", "tool_result", "", converted)
	if len(conversation.events) != 1 || !strings.Contains(conversation.events[0].Detail, `"endpoint_key":"POST:/admin/accounts"`) {
		t.Fatalf("event detail = %s", conversation.events[0].Detail)
	}
}

func TestAIAgentWriteFingerprintIsStable(t *testing.T) {
	first := agentWriteFingerprint("PUT", "/admin/groups/7", map[string]any{"b": 2, "a": 1}, map[string]any{"status": "inactive"})
	second := agentWriteFingerprint("PUT", "/admin/groups/7", map[string]any{"a": 1, "b": 2}, map[string]any{"status": "inactive"})
	if first != second {
		t.Fatalf("fingerprint must be deterministic: %q != %q", first, second)
	}
}

func TestAIAgentSensitiveFieldDetectionDoesNotBlockIDs(t *testing.T) {
	if isAgentSensitiveKey("api_key_id") {
		t.Fatal("api_key_id is an identifier, not a secret")
	}
	for _, key := range []string{"key", "api_key", "password", "access_token", "credentials", "client_secret"} {
		if !isAgentSensitiveKey(key) {
			t.Fatalf("expected %q to be sensitive", key)
		}
	}
	if !containsAgentSensitiveInput(map[string]any{"profile": map[string]any{"refresh_token": "secret"}}) {
		t.Fatal("expected nested secret to be detected")
	}
	redacted := agentTestMap(t, redactAgentValue(map[string]any{"key": "ai_agent_conversations_7_encrypted", "value": "ciphertext"}))
	if redacted["value"] != "[REDACTED]" {
		t.Fatalf("encrypted Agent history value was not redacted: %#v", redacted)
	}
}

func TestAgentInternalAuthBindsLoopbackMethodAndURI(t *testing.T) {
	auth, err := NewAgentInternalAuth()
	if err != nil {
		t.Fatalf("NewAgentInternalAuth() error = %v", err)
	}
	identity := AgentInternalIdentity{UserID: 7, Concurrency: 5, SessionID: "session-1"}
	token, err := auth.Sign(identity, http.MethodPut, "/api/v1/admin/groups/9?x=1")
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	request := &http.Request{
		Method:     http.MethodPut,
		RemoteAddr: "127.0.0.1:4567",
		URL:        &url.URL{Path: "/api/v1/admin/groups/9", RawQuery: "x=1"},
		Header:     make(http.Header),
	}
	request.Header.Set(AgentInternalAuthHeader, token)
	claims, err := auth.Validate(request)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if claims.UserID != identity.UserID || claims.SessionID != identity.SessionID {
		t.Fatalf("claims = %#v", claims)
	}

	request.Method = http.MethodDelete
	if _, err := auth.Validate(request); err == nil {
		t.Fatal("expected method mismatch to be rejected")
	}
	request.Method = http.MethodPut
	request.RemoteAddr = "203.0.113.10:4567"
	if _, err := auth.Validate(request); err == nil {
		t.Fatal("expected non-loopback request to be rejected")
	}
}
