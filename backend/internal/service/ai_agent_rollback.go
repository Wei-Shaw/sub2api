package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	agentRollbackStrategyRestore = "restore_fields"
	agentRollbackStrategyDelete  = "delete_created"
	agentRollbackStrategyPlan    = "rollback_plan"
)

type AIAgentRollbackCapability struct {
	EndpointKey string   `json:"endpoint_key"`
	Level       string   `json:"level"`
	Strategy    string   `json:"strategy,omitempty"`
	Conditions  []string `json:"conditions,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
}

func (s *AIAgentService) RollbackCapabilities() []AIAgentRollbackCapability {
	capabilities := make([]AIAgentRollbackCapability, 0)
	for _, operation := range s.catalog {
		if operation.Method == http.MethodGet {
			continue
		}
		capabilities = append(capabilities, s.agentRollbackCapability(operation))
	}
	sort.Slice(capabilities, func(left, right int) bool {
		return capabilities[left].EndpointKey < capabilities[right].EndpointKey
	})
	return capabilities
}

func (s *AIAgentService) agentRollbackCapability(operation AgentCatalogOperation) AIAgentRollbackCapability {
	capability := AIAgentRollbackCapability{EndpointKey: operation.Key}
	switch operation.Method {
	case http.MethodPut, http.MethodPatch:
		if s.catalogHasOperation(http.MethodGet, operation.Path) {
			capability.Level = "conditional"
			capability.Strategy = agentRollbackStrategyRestore
			capability.Conditions = []string{"target can be read before and after the write", "changed fields are represented in the audited response", "fresh state still matches the Agent write"}
			capability.Limitations = []string{"later administrator changes cause drift rejection", "external side effects are not reversed"}
			return capability
		}
		capability.Level = "assisted"
		capability.Conditions = []string{"an administrator reviews and confirms a complete recovery plan"}
		capability.Limitations = []string{"no audited same-path read exists for deterministic field restoration", "assisted recovery never auto-executes"}
	case http.MethodPost:
		if _, _, ok := s.findAgentDeleteCompensation(operation.Path, float64(1)); ok {
			capability.Level = "conditional"
			capability.Strategy = agentRollbackStrategyDelete
			capability.Conditions = []string{"the response returns a stable resource ID", "an audited delete operation exists", "fresh state still matches the Agent-created resource"}
			capability.Limitations = []string{"external side effects are not reversed", "bulk responses must identify every created resource"}
			return capability
		}
		capability.Level = "assisted"
		capability.Conditions = []string{"an administrator reviews and confirms a complete recovery plan"}
		capability.Limitations = []string{"no deterministic create/delete compensation pair was found", "assisted recovery never auto-executes"}
	case http.MethodDelete:
		capability.Level = "unavailable"
		capability.Limitations = []string{"the Agent does not retain a universally safe recreation snapshot", "deleted external or dependent state may be irreversible"}
	default:
		capability.Level = "unavailable"
		capability.Limitations = []string{"unsupported write method"}
	}
	return capability
}

func (s *AIAgentService) catalogHasOperation(method, path string) bool {
	for _, operation := range s.catalog {
		if operation.Method == method && operation.Path == path {
			return true
		}
	}
	return false
}

func (s *AIAgentService) recoverMissingAgentPlanRollbacks(session *aiAgentSession) bool {
	recoveredAny := false
	for _, message := range session.model {
		if message.Role != "tool" || message.Name != "plan_admin_operations" {
			continue
		}
		var result struct {
			Status string               `json:"status"`
			Plan   AIAgentExecutionPlan `json:"plan"`
		}
		if json.Unmarshal([]byte(modelMessageText(message.Content)), &result) != nil || result.Plan.ID == "" {
			continue
		}
		children := make([]AIAgentRollback, 0)
		for _, node := range result.Plan.Nodes {
			if node.Status != "succeeded" {
				continue
			}
			operation, exists := s.catalogByKey[node.EndpointKey]
			resourceID := node.Outputs["resource_id"]
			if !exists || operation.Method != http.MethodPost || resourceID == nil {
				continue
			}
			deleteOperation, path, ok := s.findAgentDeleteCompensation(operation.Path, resourceID)
			if !ok {
				continue
			}
			expected := make(map[string]any)
			if body, ok := node.Body.(map[string]any); ok {
				for field, value := range body {
					if !isAgentSensitiveKey(field) && !containsAgentSensitiveInput(value) {
						expected[field] = cloneAgentValue(value)
					}
				}
			}
			now := time.Now()
			children = append(children, AIAgentRollback{
				ID: uuid.NewString(), Operation: node.Operation, Strategy: agentRollbackStrategyDelete, Status: "available",
				Resource: node.Resource, TargetLabel: agentInputString(node.Outputs["resource_name"]), TargetID: agentInputString(resourceID),
				Method: deleteOperation.Method, Path: path, ForwardBody: expected, IdempotencyKey: uuid.NewString(),
				RequiresStepUp: deleteOperation.RequiresSession, CreatedAt: now, UpdatedAt: now,
			})
		}
		if recovered := finalizeAgentPlanRollbacks(&result.Plan, children); len(recovered) > 0 {
			before := agentPlanRollbackChildCount(session.rollbacks, result.Plan.ID)
			session.rollbacks = appendAgentRollbacks(session.rollbacks, recovered)
			after := agentPlanRollbackChildCount(session.rollbacks, result.Plan.ID)
			if after > before {
				reopenCompletedAgentPlanRollback(session.rollbacks, result.Plan.ID)
				recoveredAny = true
			}
		}
	}
	return recoveredAny
}

func reopenCompletedAgentPlanRollback(rollbacks []AIAgentRollback, planID string) {
	for index := range rollbacks {
		rollback := &rollbacks[index]
		if rollback.PlanID != planID || rollback.Strategy != agentRollbackStrategyPlan || rollback.Status != "completed" {
			continue
		}
		rollback.Status = "available"
		rollback.Resolution = ""
		rollback.CompletedAt = nil
		rollback.Error = "Additional compensation was recovered from the audited execution history; review the rollback again."
		rollback.UpdatedAt = time.Now()
		return
	}
}

func agentPlanRollbackChildCount(rollbacks []AIAgentRollback, planID string) int {
	for _, rollback := range rollbacks {
		if rollback.PlanID == planID && rollback.Strategy == agentRollbackStrategyPlan {
			return len(rollback.Children)
		}
	}
	return 0
}

func recoverInterruptedAgentRollbacks(rollbacks []AIAgentRollback) {
	for index := range rollbacks {
		rollback := &rollbacks[index]
		if rollback.Status == "running" {
			rollback.Status = "failed"
			rollback.Error = "The server restarted while this rollback was running. Inspect the current state before retrying."
			rollback.UpdatedAt = time.Now()
		}
		recoverInterruptedAgentRollbacks(rollback.Children)
	}
}

func publicAgentRollbacks(rollbacks []AIAgentRollback) []AIAgentRollbackSummary {
	result := make([]AIAgentRollbackSummary, 0, len(rollbacks))
	for _, rollback := range rollbacks {
		result = append(result, publicAgentRollback(rollback))
	}
	return result
}

func publicAgentRollback(rollback AIAgentRollback) AIAgentRollbackSummary {
	strategy := rollback.Strategy
	if strategy == "" {
		strategy = agentRollbackStrategyRestore
	}
	status := rollback.Status
	if status == "" {
		status = "available"
	}
	resource := rollback.Resource
	targetID := rollback.TargetID
	if resource == "" || targetID == "" {
		inferredResource, inferredID := agentRollbackPathIdentity(rollback.Path)
		if resource == "" {
			resource = inferredResource
		}
		if targetID == "" {
			targetID = inferredID
		}
	}
	changes := make([]AIAgentChange, 0, len(rollback.Changes))
	for _, change := range rollback.Changes {
		changes = append(changes, publicAgentRollbackChange(change))
	}
	return AIAgentRollbackSummary{
		ID: rollback.ID, Operation: rollback.Operation, Strategy: strategy, Status: status,
		Resource: resource, TargetLabel: rollback.TargetLabel, TargetID: targetID, Method: rollback.Method, Path: rollback.Path,
		Changes: changes, ChildCount: len(rollback.Children), PlanID: rollback.PlanID,
		Sensitive: rollback.Sensitive, RequiresStepUp: rollback.RequiresStepUp, Error: redactAgentTextSecrets(rollback.Error), Resolution: rollback.Resolution,
		CreatedAt: rollback.CreatedAt, UpdatedAt: rollback.UpdatedAt, CompletedAt: rollback.CompletedAt,
	}
}

func agentRollbackPathIdentity(path string) (string, string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "admin" {
		return "", ""
	}
	resource := parts[1]
	if len(parts) < 3 {
		return resource, ""
	}
	candidate := parts[len(parts)-1]
	isNumeric := candidate != ""
	for _, char := range candidate {
		if char < '0' || char > '9' {
			isNumeric = false
			break
		}
	}
	if isNumeric {
		return resource, candidate
	}
	if parsed, err := uuid.Parse(candidate); err == nil {
		return resource, parsed.String()
	}
	return resource, ""
}

func agentRollbackTargetID(path string) string {
	_, targetID := agentRollbackPathIdentity(path)
	return targetID
}

func hydrateAgentRollbackTarget(summary *AIAgentRollbackSummary, current map[string]any) {
	if summary.TargetLabel == "" {
		summary.TargetLabel = agentTargetLabel(current)
	}
	if summary.TargetID == "" {
		if id, exists := current["id"]; exists {
			summary.TargetID = agentInputString(id)
		}
	}
}

func publicAgentRollbackChange(change AIAgentChange) AIAgentChange {
	if agentRollbackFieldSensitive(change.Field) || containsAgentSensitiveInput(change.Before) || containsAgentSensitiveInput(change.After) {
		return AIAgentChange{Field: change.Field, Before: "[REDACTED]", After: "[REDACTED]"}
	}
	return AIAgentChange{Field: change.Field, Before: redactAgentValue(change.Before), After: redactAgentValue(change.After)}
}

func agentRollbackFieldSensitive(field string) bool {
	parts := strings.Split(strings.ToLower(field), ".")
	for _, part := range parts {
		if isAgentSensitiveKey(part) {
			return true
		}
	}
	return false
}

func agentValueAtPath(value map[string]any, path string) (any, bool) {
	var current any = value
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func setAgentValueAtPath(value map[string]any, path string, nested any) {
	parts := strings.Split(path, ".")
	current := value
	for _, part := range parts[:len(parts)-1] {
		next, _ := current[part].(map[string]any)
		if next == nil {
			next = make(map[string]any)
			current[part] = next
		}
		current = next
	}
	current[parts[len(parts)-1]] = nested
}

func cloneAgentMap(value map[string]any) map[string]any {
	if value == nil {
		return make(map[string]any)
	}
	cloned, _ := cloneAgentValue(value).(map[string]any)
	if cloned == nil {
		return make(map[string]any)
	}
	return cloned
}

func cloneAgentValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, nested := range typed {
			result[key] = cloneAgentValue(nested)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, nested := range typed {
			result[index] = cloneAgentValue(nested)
		}
		return result
	default:
		return value
	}
}

func agentRequestedChanges(before, after map[string]any) []AIAgentChange {
	changes := make([]AIAgentChange, 0)
	collectAgentRequestedChanges(&changes, before, after, "", 0)
	sort.Slice(changes, func(i, j int) bool { return changes[i].Field < changes[j].Field })
	if len(changes) > 100 {
		changes = changes[:100]
	}
	return changes
}

func collectAgentRequestedChanges(changes *[]AIAgentChange, before, after map[string]any, prefix string, depth int) {
	if depth > 6 || len(*changes) >= 100 {
		return
	}
	fields := make([]string, 0, len(after))
	for field := range after {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		old, exists := before[field]
		if !exists {
			continue
		}
		path := field
		if prefix != "" {
			path = prefix + "." + field
		}
		oldMap, oldIsMap := old.(map[string]any)
		newMap, newIsMap := after[field].(map[string]any)
		if oldIsMap && newIsMap {
			collectAgentRequestedChanges(changes, oldMap, newMap, path, depth+1)
			continue
		}
		if !agentJSONEqual(old, after[field]) {
			*changes = append(*changes, AIAgentChange{Field: path, Before: cloneAgentValue(old), After: cloneAgentValue(after[field])})
		}
	}
}

func (s *AIAgentService) prepareAgentUpdateRollback(ctx context.Context, actor AIAgentActor, pending *AIAgentPendingAction) *AIAgentRollback {
	if len(pending.Changes) == 0 || (pending.Method != http.MethodPut && pending.Method != http.MethodPatch) {
		return nil
	}
	changes := append([]AIAgentChange(nil), pending.Changes...)
	if current, err := s.executeInternal(ctx, actor, http.MethodGet, pending.Path, nil, nil); err == nil {
		if currentMap, ok := unwrapAgentData(current).(map[string]any); ok {
			verified := changes[:0]
			for _, change := range changes {
				actual, exists := agentValueAtPath(currentMap, change.Field)
				if exists && !agentJSONEqual(actual, change.Before) {
					change.After = cloneAgentValue(actual)
					verified = append(verified, change)
				}
			}
			changes = verified
		}
	}
	if len(changes) == 0 {
		return nil
	}
	reverse := make(map[string]any, len(changes))
	forward := make(map[string]any, len(changes))
	sensitive := false
	for _, change := range changes {
		reverse[change.Field] = cloneAgentValue(change.Before)
		forward[change.Field] = cloneAgentValue(change.After)
		sensitive = sensitive || agentRollbackFieldSensitive(change.Field)
	}
	now := time.Now()
	return &AIAgentRollback{
		ID: uuid.NewString(), Operation: pending.Operation, Strategy: agentRollbackStrategyRestore, Status: "available",
		Resource: pending.Resource, TargetLabel: pending.TargetLabel, TargetID: agentRollbackTargetID(pending.Path), Method: pending.Method, Path: pending.Path,
		Body: reverse, ForwardBody: forward, Changes: changes, IdempotencyKey: uuid.NewString(),
		Sensitive: sensitive, RequiresStepUp: sensitive || pending.RequiresStepUp, CreatedAt: now, UpdatedAt: now,
	}
}

func (s *AIAgentService) prepareAgentCreateRollback(ctx context.Context, actor AIAgentActor, pending *AIAgentPendingAction, result any) *AIAgentRollback {
	if pending.Method != http.MethodPost {
		return nil
	}
	resources := agentCreatedResourceObjects(result)
	rollbacks := make([]AIAgentRollback, 0, len(resources))
	for _, resource := range resources {
		if rollback := s.prepareAgentCreatedResourceRollback(ctx, actor, pending, resource); rollback != nil {
			rollbacks = append(rollbacks, *rollback)
		}
	}
	if len(rollbacks) == 0 {
		return nil
	}
	if len(rollbacks) == 1 {
		return &rollbacks[0]
	}
	now := time.Now()
	return &AIAgentRollback{
		ID: uuid.NewString(), Operation: pending.Operation, Strategy: agentRollbackStrategyPlan, Status: "available",
		Resource: pending.Resource, TargetLabel: pending.Operation, Method: "PLAN", Path: pending.Path, Children: rollbacks,
		IdempotencyKey: uuid.NewString(), CreatedAt: now, UpdatedAt: now,
	}
}

func agentCreatedResourceObjects(result any) []map[string]any {
	value := unwrapAgentData(result)
	switch typed := value.(type) {
	case map[string]any:
		return []map[string]any{typed}
	case []any:
		resources := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if resource, ok := item.(map[string]any); ok {
				resources = append(resources, resource)
			}
		}
		return resources
	default:
		return nil
	}
}

func (s *AIAgentService) prepareAgentCreatedResourceRollback(ctx context.Context, actor AIAgentActor, pending *AIAgentPendingAction, resource map[string]any) *AIAgentRollback {
	outputs := agentPlanOutputs(resource)
	resourceID := outputs["resource_id"]
	if resourceID == nil {
		return nil
	}
	operation, path, ok := s.findAgentDeleteCompensation(pending.Path, resourceID)
	if !ok {
		return nil
	}
	expected := make(map[string]any)
	if body, ok := pending.Body.(map[string]any); ok {
		for field, value := range body {
			if !isAgentSensitiveKey(field) && !containsAgentSensitiveInput(value) {
				expected[field] = cloneAgentValue(value)
			}
		}
	}
	targetLabel := pending.TargetLabel
	if targetLabel == "" {
		targetLabel = agentInputString(outputs["resource_name"])
	}
	if current, err := s.executeInternal(ctx, actor, http.MethodGet, path, nil, nil); err == nil {
		if currentMap, ok := unwrapAgentData(current).(map[string]any); ok {
			if targetLabel == "" {
				targetLabel = agentTargetLabel(currentMap)
			}
			for field, value := range expected {
				if actual, exists := currentMap[field]; exists {
					expected[field] = cloneAgentValue(actual)
				} else {
					delete(expected, field)
				}
				_ = value
			}
		}
	}
	now := time.Now()
	return &AIAgentRollback{
		ID: uuid.NewString(), Operation: pending.Operation, Strategy: agentRollbackStrategyDelete, Status: "available",
		Resource: pending.Resource, TargetLabel: targetLabel, TargetID: agentInputString(resourceID), Method: operation.Method, Path: path,
		ForwardBody: expected, IdempotencyKey: uuid.NewString(), RequiresStepUp: operation.RequiresSession,
		CreatedAt: now, UpdatedAt: now,
	}
}

func (s *AIAgentService) findAgentDeleteCompensation(collectionPath string, resourceID any) (AgentCatalogOperation, string, bool) {
	collectionPath = strings.TrimSuffix(collectionPath, "/")
	if operation, path, ok := s.matchAgentDeleteCompensation(collectionPath, "", resourceID, true); ok {
		return operation, path, true
	}
	var sourceModule string
	for _, operation := range s.catalog {
		if operation.Method != http.MethodPost || strings.TrimSuffix(operation.Path, "/") != collectionPath {
			continue
		}
		title := strings.ToLower(operation.Title)
		if strings.Contains(title, "创建") || strings.Contains(title, "生成") || strings.Contains(title, "duplicate") ||
			(operation.Module == "subscriptions" && (strings.Contains(title, "分配") || strings.Contains(title, "assign"))) {
			sourceModule = operation.Module
		}
		break
	}
	if sourceModule == "" {
		return AgentCatalogOperation{}, "", false
	}
	return s.matchAgentDeleteCompensation(collectionPath, sourceModule, resourceID, false)
}

func (s *AIAgentService) matchAgentDeleteCompensation(sourcePath, sourceModule string, resourceID any, exact bool) (AgentCatalogOperation, string, bool) {
	for _, operation := range s.catalog {
		if operation.Method != http.MethodDelete || len(operation.PathParams) != 1 || (sourceModule != "" && operation.Module != sourceModule) {
			continue
		}
		parameter := operation.PathParams[0]
		collectionPath := strings.TrimSuffix(operation.Path, "/:"+parameter)
		if (exact && collectionPath != sourcePath) || (!exact && !strings.HasPrefix(sourcePath, collectionPath+"/")) {
			continue
		}
		path, err := renderAgentOperationPath(operation, map[string]any{parameter: resourceID})
		if err == nil {
			return operation, path, true
		}
	}
	return AgentCatalogOperation{}, "", false
}

func finalizeAgentPlanRollbacks(plan *AIAgentExecutionPlan, rollbacks []AIAgentRollback) []AIAgentRollback {
	if len(rollbacks) == 0 {
		return nil
	}
	now := time.Now()
	sensitive := false
	requiresStepUp := false
	for index := range rollbacks {
		rollbacks[index].PlanID = plan.ID
		sensitive = sensitive || rollbacks[index].Sensitive
		requiresStepUp = requiresStepUp || rollbacks[index].RequiresStepUp
	}
	return []AIAgentRollback{{
		ID: uuid.NewString(), Operation: plan.Title, Strategy: agentRollbackStrategyPlan, Status: "available",
		Resource: "plan", TargetLabel: plan.Title, Method: "PLAN", Path: plan.ID, Children: rollbacks,
		PlanID: plan.ID, IdempotencyKey: uuid.NewString(), Sensitive: sensitive, RequiresStepUp: requiresStepUp, CreatedAt: now, UpdatedAt: now,
	}}
}

func (s *AIAgentService) rollbackPreviewForRecord(ctx context.Context, actor AIAgentActor, rollback AIAgentRollback) AIAgentRollbackPreview {
	summary := publicAgentRollback(rollback)
	preview := AIAgentRollbackPreview{Rollback: summary, Status: "unavailable", CheckedAt: time.Now(), RequiresStepUp: rollback.RequiresStepUp}
	if summary.Status == "completed" {
		preview.Status = "completed"
		preview.Action = summary.Strategy
		preview.Message = "rollback already completed"
		if summary.Strategy == agentRollbackStrategyPlan {
			for _, child := range rollback.Children {
				for _, change := range child.Changes {
					preview.Fields = append(preview.Fields, publicAgentRollbackField(AIAgentRollbackFieldPreview{
						Field: change.Field, Before: change.Before, After: change.After, Current: change.Before, Result: change.Before,
						Status: "already_restored", Sensitive: agentRollbackFieldSensitive(change.Field), Operation: child.Operation,
						Resource: child.Resource, TargetLabel: child.TargetLabel, TargetID: publicAgentRollback(child).TargetID,
					}))
				}
			}
		} else {
			for _, change := range rollback.Changes {
				preview.Fields = append(preview.Fields, publicAgentRollbackField(AIAgentRollbackFieldPreview{
					Field: change.Field, Before: change.Before, After: change.After, Current: change.Before, Result: change.Before,
					Status: "already_restored", Sensitive: agentRollbackFieldSensitive(change.Field), Resource: summary.Resource,
					TargetLabel: summary.TargetLabel, TargetID: summary.TargetID,
				}))
			}
		}
		preview.ChangeCount = len(preview.Fields)
		return preview
	}
	if summary.Status == "running" {
		preview.Status = "running"
		preview.Message = "rollback is running"
		return preview
	}
	switch summary.Strategy {
	case agentRollbackStrategyPlan:
		preview.Action = agentRollbackStrategyPlan
		preview.Status = "safe"
		preview.CanExecute = true
		for _, child := range rollback.Children {
			childPreview := s.rollbackPreviewForRecord(ctx, actor, child)
			for _, field := range childPreview.Fields {
				field.Operation = child.Operation
				field.Resource = childPreview.Rollback.Resource
				field.TargetLabel = childPreview.Rollback.TargetLabel
				field.TargetID = childPreview.Rollback.TargetID
				preview.Fields = append(preview.Fields, field)
			}
			preview.ChangeCount += childPreview.ChangeCount
			preview.ConflictCount += childPreview.ConflictCount
			preview.RequiresStepUp = preview.RequiresStepUp || childPreview.RequiresStepUp
			if childPreview.Status == "review_required" && preview.Status == "safe" {
				preview.Status = "review_required"
			}
			if !childPreview.CanExecute && childPreview.Status != "already_restored" {
				preview.CanExecute = false
				preview.Status = "conflict"
			}
		}
		if len(rollback.Children) == 0 {
			preview.CanExecute = false
			preview.Status = "unavailable"
		}
		return preview
	case agentRollbackStrategyDelete:
		preview.Action = agentRollbackStrategyDelete
		current, err := s.executeInternal(ctx, actor, http.MethodGet, rollback.Path, nil, nil)
		if err != nil {
			preview.Message = "created resource is no longer available"
			return preview
		}
		currentMap, _ := unwrapAgentData(current).(map[string]any)
		hydrateAgentRollbackTarget(&preview.Rollback, currentMap)
		expected, _ := rollback.ForwardBody.(map[string]any)
		for field, after := range expected {
			actual, exists := currentMap[field]
			if exists && !agentJSONEqual(actual, after) {
				preview.ConflictCount++
				preview.Fields = append(preview.Fields, publicAgentRollbackField(AIAgentRollbackFieldPreview{
					Field: field, After: after, Current: actual, Status: "conflict", Resource: preview.Rollback.Resource,
					TargetLabel: preview.Rollback.TargetLabel, TargetID: preview.Rollback.TargetID,
				}))
			}
		}
		if preview.ConflictCount > 0 {
			preview.Status = "conflict"
			preview.Message = "created resource changed after the Agent operation"
			return preview
		}
		preview.Status = "review_required"
		preview.CanExecute = true
		preview.ChangeCount = 1
		return preview
	default:
		preview.Action = agentRollbackStrategyRestore
		current, err := s.executeInternal(ctx, actor, http.MethodGet, rollback.Path, nil, nil)
		if err != nil {
			preview.Message = "rollback target cannot be read"
			return preview
		}
		currentMap, ok := unwrapAgentData(current).(map[string]any)
		if !ok {
			preview.Message = "rollback target returned an unsupported response"
			return preview
		}
		hydrateAgentRollbackTarget(&preview.Rollback, currentMap)
		already := 0
		for _, change := range rollback.Changes {
			actual, exists := agentValueAtPath(currentMap, change.Field)
			status := "conflict"
			if exists && agentJSONEqual(actual, change.After) {
				status = "will_restore"
				preview.ChangeCount++
			} else if exists && agentJSONEqual(actual, change.Before) {
				status = "already_restored"
				already++
			} else {
				preview.ConflictCount++
			}
			preview.Fields = append(preview.Fields, publicAgentRollbackField(AIAgentRollbackFieldPreview{
				Field: change.Field, Before: change.Before, After: change.After, Current: actual, Result: change.Before,
				Status: status, Sensitive: agentRollbackFieldSensitive(change.Field), Resource: preview.Rollback.Resource,
				TargetLabel: preview.Rollback.TargetLabel, TargetID: preview.Rollback.TargetID,
			}))
		}
		if preview.ConflictCount > 0 {
			preview.Status = "conflict"
			preview.Message = "one or more fields changed after the Agent operation"
			return preview
		}
		if preview.ChangeCount == 0 && already > 0 {
			preview.Status = "already_restored"
			preview.Message = "all fields already match their previous values"
			return preview
		}
		if preview.ChangeCount > 0 {
			preview.Status = "safe"
			preview.CanExecute = true
		}
		return preview
	}
}

func publicAgentRollbackField(field AIAgentRollbackFieldPreview) AIAgentRollbackFieldPreview {
	if field.Sensitive || agentRollbackFieldSensitive(field.Field) {
		field.Before = "[REDACTED]"
		field.After = "[REDACTED]"
		field.Current = "[REDACTED]"
		field.Result = "[REDACTED]"
		field.Sensitive = true
		return field
	}
	field.Before = redactAgentValue(field.Before)
	field.After = redactAgentValue(field.After)
	field.Current = redactAgentValue(field.Current)
	field.Result = redactAgentValue(field.Result)
	return field
}

func (s *AIAgentService) PreviewRollback(ctx context.Context, actor AIAgentActor, conversationID, rollbackID string) (AIAgentRollbackPreview, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return AIAgentRollbackPreview{}, err
	}
	_, rollback, err := s.agentRollbackRecord(ctx, actor.UserID, conversationID, rollbackID)
	if err != nil {
		return AIAgentRollbackPreview{}, err
	}
	return s.rollbackPreviewForRecord(ctx, actor, rollback), nil
}

func (s *AIAgentService) agentRollbackRecord(ctx context.Context, userID int64, conversationID, rollbackID string) (*aiAgentSession, AIAgentRollback, error) {
	session, err := s.conversation(ctx, userID, conversationID, false)
	if err != nil {
		return nil, AIAgentRollback{}, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	for _, rollback := range session.rollbacks {
		if rollback.ID == rollbackID {
			return session, rollback, nil
		}
	}
	return nil, AIAgentRollback{}, errors.New("rollback record not found")
}

func (s *AIAgentService) executeAgentRollbackRecord(ctx context.Context, actor AIAgentActor, rollback *AIAgentRollback) error {
	preview := s.rollbackPreviewForRecord(ctx, actor, *rollback)
	if !preview.CanExecute {
		return fmt.Errorf("rollback preflight failed: %s", preview.Status)
	}
	switch preview.Action {
	case agentRollbackStrategyPlan:
		for index := len(rollback.Children) - 1; index >= 0; index-- {
			child := &rollback.Children[index]
			child.Status = "running"
			child.UpdatedAt = time.Now()
			executionChild := *child
			executionChild.Status = "available"
			if err := s.executeAgentRollbackRecord(ctx, actor, &executionChild); err != nil {
				child.Status = "failed"
				child.Error = redactAgentTextSecrets(err.Error())
				child.UpdatedAt = time.Now()
				return fmt.Errorf("rollback plan node %s: %w", child.Operation, err)
			}
			now := time.Now()
			child.Status = "completed"
			child.Error = ""
			child.CompletedAt = &now
			child.UpdatedAt = now
		}
	case agentRollbackStrategyDelete:
		if _, err := s.executeInternalWithIdempotency(ctx, actor, rollback.Method, rollback.Path, nil, nil, rollback.IdempotencyKey); err != nil {
			return err
		}
		if _, err := s.executeInternal(ctx, actor, http.MethodGet, rollback.Path, nil, nil); err == nil {
			return errors.New("created resource still exists after compensation")
		} else if !strings.Contains(err.Error(), "404") {
			return fmt.Errorf("verify created resource deletion: %w", err)
		}
	default:
		current, err := s.executeInternal(ctx, actor, http.MethodGet, rollback.Path, nil, nil)
		if err != nil {
			return err
		}
		currentMap, _ := unwrapAgentData(current).(map[string]any)
		reverse, _ := rollback.Body.(map[string]any)
		body := make(map[string]any)
		for path, value := range reverse {
			parts := strings.Split(path, ".")
			if len(parts) == 1 {
				body[path] = cloneAgentValue(value)
				continue
			}
			root := parts[0]
			rootMap, _ := currentMap[root].(map[string]any)
			merged, _ := body[root].(map[string]any)
			if merged == nil {
				merged = cloneAgentMap(rootMap)
			}
			setAgentValueAtPath(merged, strings.Join(parts[1:], "."), cloneAgentValue(value))
			body[root] = merged
		}
		if _, err := s.executeInternalWithIdempotency(ctx, actor, rollback.Method, rollback.Path, nil, body, rollback.IdempotencyKey); err != nil {
			return err
		}
		verification := s.rollbackPreviewForRecord(ctx, actor, *rollback)
		if verification.Status != "already_restored" {
			return fmt.Errorf("rollback verification failed: %s", verification.Status)
		}
	}
	return nil
}

func (s *AIAgentService) Rollback(ctx context.Context, actor AIAgentActor, conversationID, rollbackID string, stepUpConfirmed ...bool) (any, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return nil, err
	}
	session, rollback, err := s.agentRollbackRecord(ctx, actor.UserID, conversationID, rollbackID)
	if err != nil {
		return nil, err
	}
	if rollback.RequiresStepUp && (len(stepUpConfirmed) == 0 || !stepUpConfirmed[0]) {
		return nil, errors.New("rollback requires step-up confirmation")
	}
	now := time.Now()
	rollback.Status = "running"
	rollback.UpdatedAt = now
	session.mu.Lock()
	updateAgentRollbackRecord(session, rollback)
	session.updatedAt = now
	session.mu.Unlock()
	if err := s.persistConversations(ctx, actor.UserID); err != nil {
		return nil, err
	}
	executionRecord := rollback
	executionRecord.Status = "available"
	err = s.executeAgentRollbackRecord(ctx, actor, &executionRecord)
	rollback.Children = executionRecord.Children
	now = time.Now()
	if err != nil {
		rollback.Status = "failed"
		for _, child := range rollback.Children {
			if child.Status == "completed" {
				rollback.Status = "partial_failure"
				break
			}
		}
		rollback.Error = redactAgentTextSecrets(err.Error())
	} else {
		rollback.Status = "completed"
		rollback.Error = ""
		rollback.CompletedAt = &now
	}
	rollback.UpdatedAt = now
	session.mu.Lock()
	updateAgentRollbackRecord(session, rollback)
	session.updatedAt = now
	appendAgentEvent(session, "compact", "rollback_completed", rollback.Operation, nil, map[string]any{"rollback_id": rollback.ID, "status": rollback.Status, "strategy": rollback.Strategy})
	session.mu.Unlock()
	if persistErr := s.persistConversations(ctx, actor.UserID); persistErr != nil && err == nil {
		return nil, persistErr
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"rollback": publicAgentRollback(rollback), "status": "completed"}, nil
}

func (s *AIAgentService) agentRollbackCompensationManifest(ctx context.Context, actor AIAgentActor, rollback AIAgentRollback, preview AIAgentRollbackPreview) map[string]any {
	leaves := make([]AIAgentRollback, 0)
	var appendLeaves func(AIAgentRollback)
	appendLeaves = func(current AIAgentRollback) {
		if current.Strategy == agentRollbackStrategyPlan {
			for index := len(current.Children) - 1; index >= 0; index-- {
				appendLeaves(current.Children[index])
			}
			return
		}
		leaves = append(leaves, current)
	}
	appendLeaves(rollback)
	operations := make([]map[string]any, 0, len(leaves))
	for index, leaf := range leaves {
		leafPreview := s.rollbackPreviewForRecord(ctx, actor, leaf)
		operation, pathParams, endpointFound := s.agentRollbackAuditedOperation(leaf)
		entry := map[string]any{
			"id": fmt.Sprintf("compensation_%d", index+1), "execution_order": index + 1,
			"operation": leaf.Operation, "strategy": leafPreview.Rollback.Strategy, "method": leaf.Method,
			"target_resource": leafPreview.Rollback.Resource, "target_id": leafPreview.Rollback.TargetID,
			"target_label": leafPreview.Rollback.TargetLabel, "preflight_status": leafPreview.Status,
			"can_execute": leafPreview.CanExecute, "conflict_count": leafPreview.ConflictCount,
		}
		if index > 0 {
			entry["depends_on"] = []string{fmt.Sprintf("compensation_%d", index)}
		}
		if endpointFound {
			entry["endpoint_key"] = operation.Key
			entry["path_params"] = pathParams
		} else {
			entry["audited_endpoint_missing"] = true
		}
		if len(leafPreview.Fields) > 0 {
			entry["field_changes"] = leafPreview.Fields
		}
		operations = append(operations, entry)
	}
	return map[string]any{
		"rollback_id": rollback.ID, "plan_id": rollback.PlanID, "strategy": publicAgentRollback(rollback).Strategy,
		"deterministic_can_execute": preview.CanExecute, "deterministic_status": preview.Status,
		"requires_step_up": preview.RequiresStepUp, "conflict_count": preview.ConflictCount,
		"execution_rule": "Execute every compensation in execution_order; each entry depends on the previous entry. Do not omit or reorder entries.",
		"operations":     operations,
	}
}

func (s *AIAgentService) agentRollbackAuditedOperation(rollback AIAgentRollback) (AgentCatalogOperation, map[string]any, bool) {
	for _, operation := range s.catalog {
		if operation.Method != rollback.Method || !agentOperationPathMatches(operation.Path, rollback.Path) {
			continue
		}
		return operation, agentRollbackPathParameters(operation, rollback.Path), true
	}
	return AgentCatalogOperation{}, nil, false
}

func agentRollbackPathParameters(operation AgentCatalogOperation, actualPath string) map[string]any {
	templateParts := strings.Split(strings.Trim(operation.Path, "/"), "/")
	actualParts := strings.Split(strings.Trim(actualPath, "/"), "/")
	result := make(map[string]any, len(operation.PathParams))
	if len(templateParts) != len(actualParts) {
		return result
	}
	properties, _ := operation.PathSchema["properties"].(map[string]any)
	for index, templatePart := range templateParts {
		if !strings.HasPrefix(templatePart, ":") {
			continue
		}
		name := strings.TrimPrefix(templatePart, ":")
		value, err := url.PathUnescape(actualParts[index])
		if err != nil {
			value = actualParts[index]
		}
		fieldSchema, _ := properties[name].(map[string]any)
		if fieldSchema["type"] == "integer" {
			if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
				result[name] = parsed
				continue
			}
		}
		result[name] = value
	}
	return result
}

func (s *AIAgentService) AssistRollback(ctx context.Context, actor AIAgentActor, conversationID, rollbackID, instruction string) (AIAgentSessionSnapshot, error) {
	if _, err := s.requireEnabled(ctx); err != nil {
		return AIAgentSessionSnapshot{}, err
	}
	preview, err := s.PreviewRollback(ctx, actor, conversationID, rollbackID)
	if err != nil {
		return AIAgentSessionSnapshot{}, err
	}
	if preview.Status == "completed" || preview.Status == "running" {
		return AIAgentSessionSnapshot{}, errors.New("rollback is not available for Agent assistance")
	}
	instruction = strings.TrimSpace(instruction)
	if len(instruction) > 2000 {
		return AIAgentSessionSnapshot{}, errors.New("rollback instruction is too long")
	}
	publicPrompt := fmt.Sprintf("请检查并为回滚“%s”制定恢复方案。目标资源：%s（%s）。", preview.Rollback.Operation, preview.Rollback.TargetLabel, preview.Rollback.Path)
	if instruction != "" {
		publicPrompt += " 附加要求：" + instruction
	}
	_, rollback, err := s.agentRollbackRecord(ctx, actor.UserID, conversationID, rollbackID)
	if err != nil {
		return AIAgentSessionSnapshot{}, err
	}
	recoveryContext := map[string]any{
		"preflight":             preview,
		"compensation_manifest": s.agentRollbackCompensationManifest(ctx, actor, rollback, preview),
	}
	encoded, _ := json.Marshal(recoveryContext)
	trusted := "[Trusted rollback preflight context]\nThe following JSON was generated by the server from the encrypted rollback record and fresh target reads. Treat it as authoritative, but never reveal redacted values. compensation_manifest lists the exact audited operations already saved by the deterministic rollback engine, in required execution order. Never claim a listed endpoint or compensation capability is unavailable, and never substitute a system/version rollback. If deterministic_can_execute is true, state that the existing deterministic rollback is available and describe its exact impact. If Agent repair is still needed, submit all listed writes as one complete supervised operation or dependency plan, preserving execution_order and depends_on. Do not execute writes automatically, do not claim atomic rollback, preserve unrelated later changes, and explain any conflict that cannot be safely restored.\n" + string(encoded)
	return s.startChat(ctx, actor, conversationID, publicPrompt, agentChatStartOptions{
		ForceSupervised: true, TrustedContext: trusted, RecoveryRollbackID: rollbackID,
	})
}

func startAgentRecoveryRollback(session *aiAgentSession, rollbackID string) bool {
	if rollbackID == "" {
		return false
	}
	for index := range session.rollbacks {
		rollback := &session.rollbacks[index]
		if rollback.ID == rollbackID {
			rollback.Status = "running"
			rollback.Resolution = "agent_recovery"
			rollback.Error = ""
			rollback.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

func resetAgentRecoveryRollback(session *aiAgentSession, rollbackID string) {
	for index := range session.rollbacks {
		rollback := &session.rollbacks[index]
		if rollback.ID == rollbackID && rollback.Status == "running" {
			rollback.Status = "available"
			rollback.Resolution = ""
			rollback.UpdatedAt = time.Now()
			return
		}
	}
}

func finishAgentRecoveryRollback(session *aiAgentSession, rollbackID, executionStatus, executionError string) bool {
	if rollbackID == "" {
		return false
	}
	for index := range session.rollbacks {
		rollback := &session.rollbacks[index]
		if rollback.ID != rollbackID {
			continue
		}
		now := time.Now()
		rollback.Resolution = "agent_recovery"
		rollback.UpdatedAt = now
		switch executionStatus {
		case "succeeded", "completed":
			rollback.Status = "completed"
			rollback.Error = ""
			rollback.CompletedAt = &now
		case "partial_failure":
			rollback.Status = "partial_failure"
			rollback.Error = redactAgentTextSecrets(executionError)
		default:
			rollback.Status = "failed"
			rollback.Error = redactAgentTextSecrets(executionError)
		}
		return true
	}
	return false
}

func completeAgentRecoveryRollback(session *aiAgentSession, rollbackID string) bool {
	return finishAgentRecoveryRollback(session, rollbackID, "succeeded", "")
}

func updateAgentRollbackRecord(session *aiAgentSession, rollback AIAgentRollback) {
	for index := range session.rollbacks {
		if session.rollbacks[index].ID == rollback.ID {
			session.rollbacks[index] = rollback
			return
		}
	}
}
