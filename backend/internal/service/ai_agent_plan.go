package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const agentMaxPlanNodes = 50

var agentPlanNodeIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,47}$`)

type AIAgentExecutionPlan struct {
	ID              string            `json:"id"`
	Title           string            `json:"title"`
	FailurePolicy   string            `json:"failure_policy"`
	Fingerprint     string            `json:"fingerprint,omitempty"`
	Status          string            `json:"status"`
	Nodes           []AIAgentPlanNode `json:"nodes"`
	Sensitive       bool              `json:"sensitive,omitempty"`
	RequiresSession bool              `json:"requires_session,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type AIAgentPlanNode struct {
	ID              string          `json:"id"`
	EndpointKey     string          `json:"endpoint_key"`
	Operation       string          `json:"operation"`
	Action          string          `json:"action,omitempty"`
	Resource        string          `json:"resource,omitempty"`
	DependsOn       []string        `json:"depends_on,omitempty"`
	PathParams      map[string]any  `json:"path_params,omitempty"`
	Query           map[string]any  `json:"query,omitempty"`
	Body            any             `json:"body,omitempty"`
	Preview         []AIAgentChange `json:"preview,omitempty"`
	Status          string          `json:"status"`
	Error           string          `json:"error,omitempty"`
	Outputs         map[string]any  `json:"outputs,omitempty"`
	IdempotencyKey  string          `json:"idempotency_key,omitempty"`
	Sensitive       bool            `json:"sensitive,omitempty"`
	RequiresStepUp  bool            `json:"requires_step_up,omitempty"`
	RequiresSession bool            `json:"requires_session,omitempty"`
}

type agentPlanArguments struct {
	Title         string                  `json:"title"`
	FailurePolicy string                  `json:"failure_policy"`
	Nodes         []agentPlanNodeArgument `json:"nodes"`
}

type agentPlanNodeArgument struct {
	ID          string         `json:"id"`
	EndpointKey string         `json:"endpoint_key"`
	DependsOn   []string       `json:"depends_on"`
	PathParams  map[string]any `json:"path_params"`
	Query       map[string]any `json:"query"`
	Body        any            `json:"body"`
}

type agentPlanReference struct {
	NodeID string
	Output string
}

type agentExecutedPlanNode struct {
	node     *AIAgentPlanNode
	pending  *AIAgentPendingAction
	rollback *AIAgentRollback
}

func firstAgentString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (s *AIAgentService) prepareAgentExecutionPlan(ctx context.Context, actor AIAgentActor, prompt string, observed map[string]bool, input agentPlanArguments) (*AIAgentExecutionPlan, *AIAgentPendingAction, error) {
	encodedInput, _ := json.Marshal(input)
	if len(encodedInput) > 512<<10 {
		return nil, nil, errors.New("execution plan exceeds 512 KiB")
	}
	if len(input.Nodes) < 2 || len(input.Nodes) > agentMaxPlanNodes {
		return nil, nil, fmt.Errorf("a plan must contain between 2 and %d nodes", agentMaxPlanNodes)
	}
	policy := strings.TrimSpace(input.FailurePolicy)
	switch policy {
	case "stop_on_failure", "continue_independent", "rollback_on_failure":
	default:
		return nil, nil, errors.New("failure_policy must be stop_on_failure, continue_independent, or rollback_on_failure")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" || len([]rune(title)) > 120 {
		return nil, nil, errors.New("plan title is required and must be at most 120 characters")
	}

	now := time.Now()
	plan := &AIAgentExecutionPlan{
		ID: uuid.NewString(), Title: title, FailurePolicy: policy, Status: "awaiting_confirmation",
		Nodes: make([]AIAgentPlanNode, 0, len(input.Nodes)), CreatedAt: now, UpdatedAt: now,
	}
	knownIDs := make(map[string]struct{}, len(input.Nodes))
	for _, inputNode := range input.Nodes {
		if !agentPlanNodeIDPattern.MatchString(inputNode.ID) {
			return nil, nil, fmt.Errorf("invalid plan node id %q", inputNode.ID)
		}
		if _, duplicate := knownIDs[inputNode.ID]; duplicate {
			return nil, nil, fmt.Errorf("duplicate plan node id %q", inputNode.ID)
		}
		knownIDs[inputNode.ID] = struct{}{}
	}

	writeFingerprints := make(map[string]string, len(input.Nodes))
	for _, inputNode := range input.Nodes {
		operation, exists := s.catalogByKey[inputNode.EndpointKey]
		if !exists {
			return nil, nil, fmt.Errorf("node %s uses an operation outside the audited catalog", inputNode.ID)
		}
		if operation.Method == http.MethodGet {
			return nil, nil, fmt.Errorf("node %s is a read operation; perform lookups before creating the write plan", inputNode.ID)
		}
		dependencies, err := validateAgentPlanDependencies(inputNode, knownIDs)
		if err != nil {
			return nil, nil, err
		}
		previewPathParams, err := resolveAgentPlanValue(inputNode.PathParams, nil, true)
		if err != nil {
			return nil, nil, fmt.Errorf("node %s path parameters: %w", inputNode.ID, err)
		}
		pathParams, _ := previewPathParams.(map[string]any)
		previewQuery, err := resolveAgentPlanValue(inputNode.Query, nil, true)
		if err != nil {
			return nil, nil, fmt.Errorf("node %s query: %w", inputNode.ID, err)
		}
		query, _ := previewQuery.(map[string]any)
		if err := validateAgentOperationParameters(operation, pathParams, query); err != nil {
			return nil, nil, fmt.Errorf("node %s parameters are invalid: %w", inputNode.ID, err)
		}
		path, err := renderAgentOperationPath(operation, pathParams)
		if err != nil {
			return nil, nil, fmt.Errorf("node %s: %w", inputNode.ID, err)
		}
		previewBody, err := resolveAgentPlanValue(inputNode.Body, nil, true)
		if err != nil {
			return nil, nil, fmt.Errorf("node %s body: %w", inputNode.ID, err)
		}
		previewBody, err = normalizeAgentOperationBody(operation.Method, path, previewBody)
		requestedPreviewBody := previewBody
		if err == nil {
			previewBody, err = s.hydrateAgentSingletonPutBody(ctx, actor, operation, path, previewBody)
		}
		if err == nil {
			err = validateAgentOperationBodyContract(operation, previewBody)
		}
		if err == nil {
			err = validateAgentOperationSemantics(operation.Method, path, previewBody)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("node %s payload is invalid: %w", inputNode.ID, err)
		}
		normalizedNodeBody, err := normalizeAgentOperationBody(operation.Method, path, inputNode.Body)
		if err == nil {
			normalizedNodeBody, err = s.hydrateAgentSingletonPutBody(ctx, actor, operation, path, normalizedNodeBody)
		}
		normalizedValidationBody := normalizedNodeBody
		if err == nil && containsAgentPlanReference(normalizedNodeBody) {
			normalizedValidationBody, err = resolveAgentPlanValue(normalizedNodeBody, nil, true)
		}
		if err == nil {
			err = validateAgentOperationBodyContract(operation, normalizedValidationBody)
		}
		if err == nil {
			err = validateAgentOperationSemantics(operation.Method, path, normalizedValidationBody)
		}
		if err == nil && !containsAgentPlanReference(normalizedNodeBody) {
			handledByPlan, planErr := validateAgentPlanCrossResourceTransition(input.Nodes, inputNode, normalizedNodeBody)
			if planErr != nil {
				err = planErr
			} else if !handledByPlan {
				err = s.validateAgentCrossResourceSemantics(ctx, actor, operation, normalizedNodeBody)
			}
		}
		if err != nil {
			return nil, nil, fmt.Errorf("node %s payload is invalid: %w", inputNode.ID, err)
		}
		if !containsAgentPlanReference(inputNode.PathParams) && !agentTargetAuthorized(operation, inputNode.PathParams, prompt, observed) {
			return nil, nil, fmt.Errorf("node %s target must be read and uniquely identified before planning", inputNode.ID)
		}
		fingerprint := agentWriteFingerprint(operation.Method, operation.Path, inputNode.Query, map[string]any{"path_params": inputNode.PathParams, "body": normalizedNodeBody})
		if duplicateNode := writeFingerprints[fingerprint]; duplicateNode != "" {
			return nil, nil, fmt.Errorf("nodes %s and %s contain the same write", duplicateNode, inputNode.ID)
		}
		writeFingerprints[fingerprint] = inputNode.ID
		nodePreview := agentPendingBodyPreview(requestedPreviewBody)
		if (operation.Method == http.MethodPut || operation.Method == http.MethodPatch) && !containsAgentPlanReference(inputNode.PathParams) {
			previewPending, previewErr := s.preparePending(ctx, actor, operation, path, inputNode.Query, normalizedNodeBody)
			if previewErr != nil {
				return nil, nil, fmt.Errorf("node %s current-state validation failed: %w", inputNode.ID, previewErr)
			}
			if len(previewPending.Changes) > 0 {
				nodePreview = previewPending.Changes
			}
		}
		sensitiveQuery := containsAgentSensitiveInput(inputNode.Query)
		sensitiveBody := containsAgentSensitiveInput(normalizedNodeBody)
		if sensitiveQuery && !operation.RequiresSession {
			return nil, nil, fmt.Errorf("node %s contains a secret in query parameters", inputNode.ID)
		}
		node := AIAgentPlanNode{
			ID: inputNode.ID, EndpointKey: inputNode.EndpointKey, Operation: agentPendingOperationTitle(operation),
			Action: agentPendingAction(operation), Resource: operation.Module, DependsOn: dependencies,
			PathParams: inputNode.PathParams, Query: inputNode.Query, Body: normalizedNodeBody,
			Preview: nodePreview, Status: "planned", IdempotencyKey: uuid.NewString(),
			Sensitive: sensitiveQuery || sensitiveBody, RequiresStepUp: operation.RequiresSession || sensitiveQuery || sensitiveBody,
			RequiresSession: operation.RequiresSession,
		}
		plan.Sensitive = plan.Sensitive || node.Sensitive
		plan.RequiresSession = plan.RequiresSession || node.RequiresSession
		plan.Nodes = append(plan.Nodes, node)
	}
	if err := validateAgentPlanDAG(plan.Nodes); err != nil {
		return nil, nil, err
	}
	if err := validateAgentPlanBusinessSemantics(input.Nodes); err != nil {
		return nil, nil, err
	}
	plan.Fingerprint = agentExecutionPlanFingerprint(plan)

	pending := &AIAgentPendingAction{
		ID: uuid.NewString(), IdempotencyKey: uuid.NewString(), Operation: title, Action: "plan", Resource: "execution_plan",
		Method: "PLAN", Path: fmt.Sprintf("%d audited writes", len(plan.Nodes)), Sensitive: plan.Sensitive,
		RequiresStepUp: plan.Sensitive || plan.RequiresSession, Plan: plan, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute),
	}
	for _, node := range plan.Nodes {
		for _, field := range agentSensitiveFieldPaths(node.Body, "") {
			pending.SensitiveFields = append(pending.SensitiveFields, node.ID+"."+field)
		}
	}
	return plan, pending, nil
}

func validateAgentPlanCrossResourceTransition(nodes []agentPlanNodeArgument, current agentPlanNodeArgument, normalizedBody any) (bool, error) {
	switch current.EndpointKey {
	case "POST:/admin/subscriptions/assign", "POST:/admin/subscriptions/bulk-assign":
	default:
		return false, nil
	}
	body, _ := normalizedBody.(map[string]any)
	groupID := agentInputString(body["group_id"])
	if groupID == "" {
		return false, nil
	}
	byID := make(map[string]agentPlanNodeArgument, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	ancestors := make(map[string]bool)
	var visit func(string)
	visit = func(nodeID string) {
		for _, dependency := range byID[nodeID].DependsOn {
			if ancestors[dependency] {
				continue
			}
			ancestors[dependency] = true
			visit(dependency)
		}
	}
	visit(current.ID)
	plannedType := ""
	plannedNode := ""
	for _, node := range nodes {
		if !ancestors[node.ID] || node.EndpointKey != "PUT:/admin/groups/:id" || agentInputString(node.PathParams["id"]) != groupID {
			continue
		}
		nodeBody, _ := node.Body.(map[string]any)
		subscriptionType := strings.ToLower(agentInputString(nodeBody["subscription_type"]))
		if subscriptionType == "" {
			continue
		}
		if plannedType != "" && plannedType != subscriptionType {
			return true, fmt.Errorf("node %s has conflicting dependency updates for group %s subscription_type", current.ID, groupID)
		}
		plannedType = subscriptionType
		plannedNode = node.ID
	}
	if plannedType == "" {
		return false, nil
	}
	if plannedType != "subscription" {
		return true, fmt.Errorf("node %s requires dependency node %s to leave group %s as subscription_type=subscription", current.ID, plannedNode, groupID)
	}
	return true, nil
}

func validateAgentPlanBusinessSemantics(nodes []agentPlanNodeArgument) error {
	byID := make(map[string]agentPlanNodeArgument, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for _, node := range nodes {
		body, _ := node.Body.(map[string]any)
		requiresSubscriptionGroup := false
		switch node.EndpointKey {
		case "POST:/admin/redeem-codes/generate", "POST:/admin/redeem-codes/create-and-redeem":
			requiresSubscriptionGroup = strings.EqualFold(agentInputString(body["type"]), "subscription")
		case "POST:/admin/subscriptions/assign", "POST:/admin/subscriptions/bulk-assign":
			requiresSubscriptionGroup = true
		}
		if !requiresSubscriptionGroup {
			continue
		}
		groupReference, ok := body["group_id"].(map[string]any)
		if !ok {
			continue
		}
		reference, err := parseAgentPlanReference(agentInputString(groupReference["$ref"]))
		if err != nil {
			continue
		}
		source, exists := byID[reference.NodeID]
		if !exists || source.EndpointKey != "POST:/admin/groups" {
			continue
		}
		sourceBody, _ := source.Body.(map[string]any)
		if !strings.EqualFold(agentInputString(sourceBody["subscription_type"]), "subscription") {
			return fmt.Errorf("node %s requires new group node %s to set body.subscription_type to subscription", node.ID, source.ID)
		}
	}
	return nil
}

func validateAgentPlanDependencies(node agentPlanNodeArgument, knownIDs map[string]struct{}) ([]string, error) {
	dependencies := append([]string(nil), node.DependsOn...)
	sort.Strings(dependencies)
	dependencies = compactAgentStrings(dependencies)
	declared := make(map[string]bool, len(dependencies))
	for _, dependency := range dependencies {
		if dependency == node.ID {
			return nil, fmt.Errorf("node %s cannot depend on itself", node.ID)
		}
		if _, exists := knownIDs[dependency]; !exists {
			return nil, fmt.Errorf("node %s depends on unknown node %s", node.ID, dependency)
		}
		declared[dependency] = true
	}
	for _, value := range []any{node.PathParams, node.Query, node.Body} {
		for _, reference := range collectAgentPlanReferences(value) {
			if _, exists := knownIDs[reference.NodeID]; !exists {
				return nil, fmt.Errorf("node %s references unknown node %s", node.ID, reference.NodeID)
			}
			if !declared[reference.NodeID] {
				return nil, fmt.Errorf("node %s must declare %s in depends_on", node.ID, reference.NodeID)
			}
		}
	}
	return dependencies, nil
}

func compactAgentStrings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func validateAgentPlanDAG(nodes []AIAgentPlanNode) error {
	dependencies := make(map[string][]string, len(nodes))
	for _, node := range nodes {
		dependencies[node.ID] = node.DependsOn
	}
	states := make(map[string]uint8, len(nodes))
	var visit func(string) error
	visit = func(id string) error {
		if states[id] == 1 {
			return fmt.Errorf("execution plan contains a dependency cycle at %s", id)
		}
		if states[id] == 2 {
			return nil
		}
		states[id] = 1
		for _, dependency := range dependencies[id] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		states[id] = 2
		return nil
	}
	for _, node := range nodes {
		if err := visit(node.ID); err != nil {
			return err
		}
	}
	return nil
}

func collectAgentPlanReferences(value any) []agentPlanReference {
	var references []agentPlanReference
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if raw, exact := typed["$ref"]; exact && len(typed) == 1 {
				if reference, err := parseAgentPlanReference(fmt.Sprint(raw)); err == nil {
					references = append(references, reference)
				}
				return
			}
			for _, nested := range typed {
				walk(nested)
			}
		case []any:
			for _, nested := range typed {
				walk(nested)
			}
		}
	}
	walk(value)
	return references
}

func containsAgentPlanReference(value any) bool {
	return len(collectAgentPlanReferences(value)) > 0
}

func parseAgentPlanReference(value string) (agentPlanReference, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !agentPlanNodeIDPattern.MatchString(parts[0]) {
		return agentPlanReference{}, errors.New("references must use node_id.resource_id or node_id.resource_name")
	}
	if parts[1] != "resource_id" && parts[1] != "resource_name" {
		return agentPlanReference{}, fmt.Errorf("output %q is not allow-listed", parts[1])
	}
	return agentPlanReference{NodeID: parts[0], Output: parts[1]}, nil
}

func resolveAgentPlanValue(value any, outputs map[string]map[string]any, preview bool) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		if raw, exact := typed["$ref"]; exact && len(typed) == 1 {
			reference, err := parseAgentPlanReference(fmt.Sprint(raw))
			if err != nil {
				return nil, err
			}
			if preview {
				if reference.Output == "resource_id" {
					return float64(1), nil
				}
				return "resolved-resource", nil
			}
			value, exists := outputs[reference.NodeID][reference.Output]
			if !exists || value == nil || fmt.Sprint(value) == "" {
				return nil, fmt.Errorf("required output %s.%s is unavailable", reference.NodeID, reference.Output)
			}
			return value, nil
		}
		result := make(map[string]any, len(typed))
		for key, nested := range typed {
			resolved, err := resolveAgentPlanValue(nested, outputs, preview)
			if err != nil {
				return nil, err
			}
			result[key] = resolved
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, nested := range typed {
			resolved, err := resolveAgentPlanValue(nested, outputs, preview)
			if err != nil {
				return nil, err
			}
			result[index] = resolved
		}
		return result, nil
	default:
		return value, nil
	}
}

func validateAgentPlanForExecution(plan *AIAgentExecutionPlan) error {
	if plan == nil {
		return errors.New("execution plan is missing")
	}
	if len(plan.Nodes) < 2 || len(plan.Nodes) > agentMaxPlanNodes || validateAgentPlanDAG(plan.Nodes) != nil {
		return errors.New("execution plan structure is invalid")
	}
	if plan.Fingerprint == "" || plan.Fingerprint != agentExecutionPlanFingerprint(plan) {
		return errors.New("execution plan changed after confirmation was requested")
	}
	return nil
}

func (s *AIAgentService) executeAgentPlan(ctx context.Context, actor AIAgentActor, session *aiAgentSession, plan *AIAgentExecutionPlan, processDisplay string) (map[string]any, []AIAgentRollback, error) {
	if err := validateAgentPlanForExecution(plan); err != nil {
		return nil, nil, err
	}
	plan.Status = "running"
	plan.UpdatedAt = time.Now()
	for index := range plan.Nodes {
		if plan.Nodes[index].Status == "failed" || plan.Nodes[index].Status == "blocked" || plan.Nodes[index].Status == "rolled_back" || plan.Nodes[index].Status == "rollback_failed" {
			plan.Nodes[index].Status = "planned"
			plan.Nodes[index].Error = ""
		}
	}
	outputs := make(map[string]map[string]any, len(plan.Nodes))
	for index := range plan.Nodes {
		if plan.Nodes[index].Status == "succeeded" {
			outputs[plan.Nodes[index].ID] = plan.Nodes[index].Outputs
		}
	}
	executed := make([]agentExecutedPlanNode, 0, len(plan.Nodes))
	var rollbacks []AIAgentRollback
	failedCount := 0

	for {
		progressed := false
		unfinished := 0
		for index := range plan.Nodes {
			node := &plan.Nodes[index]
			if node.Status == "succeeded" || node.Status == "failed" || node.Status == "blocked" {
				continue
			}
			unfinished++
			dependencyFailed := false
			dependenciesReady := true
			for _, dependencyID := range node.DependsOn {
				dependency := agentPlanNodeByID(plan, dependencyID)
				if dependency == nil || dependency.Status == "failed" || dependency.Status == "blocked" {
					dependencyFailed = true
					break
				}
				if dependency.Status != "succeeded" {
					dependenciesReady = false
				}
			}
			if dependencyFailed {
				node.Status = "blocked"
				node.Error = "a dependency did not complete successfully"
				progressed = true
				continue
			}
			if !dependenciesReady {
				continue
			}
			progressed = true
			node.Status = "running"
			appendAgentEvent(session, processDisplay, "plan_node_started", node.Operation, nil, map[string]any{"plan_id": plan.ID, "node_id": node.ID, "operation": node.Operation})
			pending, err := s.resolveAgentPlanNode(ctx, actor, node, outputs)
			if err == nil {
				var rollback *AIAgentRollback
				var result any
				result, rollback, err = s.executePending(ctx, actor, pending)
				if err == nil {
					node.Outputs = agentPlanOutputs(result)
					outputs[node.ID] = node.Outputs
					node.Status = "succeeded"
					node.Error = ""
					executed = append(executed, agentExecutedPlanNode{node: node, pending: pending, rollback: rollback})
					if rollback != nil {
						rollbacks = append(rollbacks, *rollback)
					}
					appendAgentEvent(session, processDisplay, "plan_node_completed", node.Operation, nil, map[string]any{"plan_id": plan.ID, "node_id": node.ID, "operation": node.Operation, "status": "success"})
					continue
				}
			}
			node.Status = "failed"
			node.Error = redactAgentTextSecrets(err.Error())
			failedCount++
			if ctx.Err() != nil {
				markAgentPlanUnfinishedBlocked(plan)
				plan.Status = "stopped"
				plan.UpdatedAt = time.Now()
				return agentPlanResult(plan), finalizeAgentPlanRollbacks(plan, rollbacks), ctx.Err()
			}
			appendAgentEvent(session, processDisplay, "plan_node_failed", node.Operation, nil, map[string]any{"plan_id": plan.ID, "node_id": node.ID, "operation": node.Operation, "status": "error"})
			if plan.FailurePolicy != "continue_independent" {
				markAgentPlanUnfinishedBlocked(plan)
				if plan.FailurePolicy == "rollback_on_failure" {
					s.executeAgentPlanCompensations(ctx, actor, session, executed, processDisplay)
					plan.Fingerprint = agentExecutionPlanFingerprint(plan)
					rollbacks = nil
				}
				plan.Status = "failed"
				plan.UpdatedAt = time.Now()
				return agentPlanResult(plan), finalizeAgentPlanRollbacks(plan, rollbacks), fmt.Errorf("plan node %s failed: %w", node.ID, err)
			}
		}
		if unfinished == 0 || !progressed {
			break
		}
		if ctx.Err() != nil {
			plan.Status = "stopped"
			plan.UpdatedAt = time.Now()
			return agentPlanResult(plan), finalizeAgentPlanRollbacks(plan, rollbacks), ctx.Err()
		}
	}
	for index := range plan.Nodes {
		if plan.Nodes[index].Status == "blocked" {
			failedCount++
		}
	}
	if failedCount > 0 {
		plan.Status = "partial_failure"
	} else {
		plan.Status = "succeeded"
	}
	plan.UpdatedAt = time.Now()
	return agentPlanResult(plan), finalizeAgentPlanRollbacks(plan, rollbacks), nil
}

func (s *AIAgentService) runConfirmedAgentPlan(ctx context.Context, actor AIAgentActor, session *aiAgentSession, pendingID, processDisplay string) {
	jobKey := s.agentJobKey(actor.UserID, session.id)
	defer func() {
		s.jobsMu.Lock()
		delete(s.jobs, jobKey)
		s.jobsMu.Unlock()
	}()
	select {
	case s.concurrency <- struct{}{}:
		defer func() { <-s.concurrency }()
	case <-ctx.Done():
		session.mu.Lock()
		session.status = agentConversationStatusStopped
		session.errorMessage = "Response stopped"
		session.updatedAt = time.Now()
		session.mu.Unlock()
		s.persistConversationsDetached(actor.UserID)
		return
	}

	session.mu.Lock()
	pending := session.pending
	if pending == nil || pending.ID != pendingID || pending.Plan == nil {
		session.status = agentConversationStatusError
		session.errorMessage = "The confirmed execution plan is no longer pending."
		session.updatedAt = time.Now()
		session.mu.Unlock()
		s.persistConversationsDetached(actor.UserID)
		return
	}
	workingPlan := cloneAgentExecutionPlan(pending.Plan)
	recoveryRollbackID := pending.RecoveryRollbackID
	runID := session.activeRunID
	session.mu.Unlock()

	eventSession := &aiAgentSession{activeRunID: runID}
	result, rollbacks, err := s.executeAgentPlan(ctx, actor, eventSession, workingPlan, processDisplay)

	session.mu.Lock()
	pending = session.pending
	if pending == nil || pending.ID != pendingID || pending.Plan == nil {
		session.status = agentConversationStatusError
		session.errorMessage = "The confirmed execution plan is no longer pending."
		session.updatedAt = time.Now()
		session.mu.Unlock()
		s.persistConversationsDetached(actor.UserID)
		return
	}
	pending.Plan = workingPlan
	session.events = append(session.events, eventSession.events...)
	if len(session.events) > agentMaxProcessEvents {
		session.events = append([]AIAgentProcessEvent(nil), session.events[len(session.events)-agentMaxProcessEvents:]...)
	}
	if err != nil {
		if recoveryRollbackID == "" {
			session.rollbacks = appendAgentRollbacks(session.rollbacks, rollbacks)
		}
		recoveryStatus := "failed"
		for _, node := range workingPlan.Nodes {
			if node.Status == "succeeded" || node.Status == "rollback_failed" {
				recoveryStatus = "partial_failure"
				break
			}
		}
		finishAgentRecoveryRollback(session, recoveryRollbackID, recoveryStatus, err.Error())
		if ctx.Err() != nil {
			session.status = agentConversationStatusStopped
			session.errorMessage = "Response stopped"
			appendAgentEvent(session, processDisplay, "stopped", "Execution plan stopped", nil)
		} else {
			session.status = agentConversationStatusError
			session.errorMessage = redactAgentTextSecrets(err.Error())
			appendAgentEvent(session, processDisplay, "error", "Execution plan failed", map[string]any{"error": err.Error()})
		}
		session.updatedAt = time.Now()
		session.mu.Unlock()
		s.persistConversationsDetached(actor.UserID)
		return
	}

	session.pending = nil
	promoteAgentPending(session)
	recoveryHandled := finishAgentRecoveryRollback(session, recoveryRollbackID, workingPlan.Status, "Agent recovery plan did not complete every operation")
	if !recoveryHandled {
		session.rollbacks = appendAgentRollbacks(session.rollbacks, rollbacks)
	}
	queuedRemaining := len(session.pendingQueue)
	if session.pending != nil {
		queuedRemaining++
	}
	summary := fmt.Sprintf("Confirmed execution plan completed: %s", workingPlan.Title)
	message := AIAgentMessage{
		ID: uuid.NewString(), RunID: runID, Role: "assistant", Content: summary, Event: "plan_confirmed",
		Metadata: map[string]any{"plan_id": workingPlan.ID, "title": workingPlan.Title, "status": workingPlan.Status, "queued_remaining": queuedRemaining, "recovery_rollback_id": recoveryRollbackID, "result": redactAgentValue(result)}, CreatedAt: time.Now(),
	}
	session.model = append(session.model, agentModelMessage{Role: "user", Content: "[Trusted UI confirmation result] " + summary})
	session.public = append(session.public, message)
	session.status = agentConversationStatusIdle
	session.errorMessage = ""
	session.updatedAt = time.Now()
	trimAgentHistory(session)
	session.mu.Unlock()
	s.persistConversationsDetached(actor.UserID)
}

func (s *AIAgentService) resolveAgentPlanNode(ctx context.Context, actor AIAgentActor, node *AIAgentPlanNode, outputs map[string]map[string]any) (*AIAgentPendingAction, error) {
	operation, exists := s.catalogByKey[node.EndpointKey]
	if !exists {
		return nil, errors.New("operation is no longer present in the audited catalog")
	}
	resolvedPathParams, err := resolveAgentPlanValue(node.PathParams, outputs, false)
	if err != nil {
		return nil, err
	}
	pathParams, _ := resolvedPathParams.(map[string]any)
	path, err := renderAgentOperationPath(operation, pathParams)
	if err != nil {
		return nil, err
	}
	resolvedQuery, err := resolveAgentPlanValue(node.Query, outputs, false)
	if err != nil {
		return nil, err
	}
	query, _ := resolvedQuery.(map[string]any)
	if err := validateAgentOperationParameters(operation, pathParams, query); err != nil {
		return nil, err
	}
	body, err := resolveAgentPlanValue(node.Body, outputs, false)
	if err != nil {
		return nil, err
	}
	body, err = normalizeAgentOperationBody(operation.Method, path, body)
	if err == nil {
		err = validateAgentOperationBodyContract(operation, body)
	}
	if err == nil {
		err = validateAgentOperationSemantics(operation.Method, path, body)
	}
	if err != nil {
		return nil, err
	}
	pending, err := s.preparePending(ctx, actor, operation, path, query, body)
	if err != nil {
		return nil, err
	}
	pending.IdempotencyKey = node.IdempotencyKey
	pending.Sensitive = node.Sensitive
	pending.RequiresStepUp = node.RequiresStepUp
	return pending, nil
}

func agentPlanOutputs(result any) map[string]any {
	outputs := make(map[string]any)
	value := unwrapAgentData(result)
	object, ok := value.(map[string]any)
	if !ok {
		return outputs
	}
	if id, exists := object["id"]; exists && fmt.Sprint(id) != "" {
		outputs["resource_id"] = id
	}
	for _, field := range []string{"name", "title", "email", "code", "username"} {
		if name := agentInputString(object[field]); name != "" {
			outputs["resource_name"] = name
			break
		}
	}
	return outputs
}

func agentPlanNodeByID(plan *AIAgentExecutionPlan, id string) *AIAgentPlanNode {
	for index := range plan.Nodes {
		if plan.Nodes[index].ID == id {
			return &plan.Nodes[index]
		}
	}
	return nil
}

func markAgentPlanUnfinishedBlocked(plan *AIAgentExecutionPlan) {
	for index := range plan.Nodes {
		if plan.Nodes[index].Status == "planned" || plan.Nodes[index].Status == "running" {
			plan.Nodes[index].Status = "blocked"
			plan.Nodes[index].Error = "execution stopped after an earlier failure"
		}
	}
}

func (s *AIAgentService) executeAgentPlanCompensations(ctx context.Context, actor AIAgentActor, session *aiAgentSession, executed []agentExecutedPlanNode, processDisplay string) {
	for index := len(executed) - 1; index >= 0; index-- {
		item := executed[index]
		err := s.compensateAgentPlanNode(ctx, actor, item)
		if err != nil {
			item.node.Status = "rollback_failed"
			item.node.Error = redactAgentTextSecrets(err.Error())
			appendAgentEvent(session, processDisplay, "plan_node_rollback_failed", item.node.Operation, nil, map[string]any{"node_id": item.node.ID, "status": "error"})
			continue
		}
		item.node.Status = "rolled_back"
		if item.pending.Method == http.MethodPost {
			item.node.IdempotencyKey = uuid.NewString()
		}
		appendAgentEvent(session, processDisplay, "plan_node_rolled_back", item.node.Operation, nil, map[string]any{"node_id": item.node.ID, "status": "success"})
	}
}

func (s *AIAgentService) compensateAgentPlanNode(ctx context.Context, actor AIAgentActor, item agentExecutedPlanNode) error {
	if item.rollback != nil {
		return s.executeAgentRollbackRecord(ctx, actor, item.rollback)
	}
	if item.pending.Method != http.MethodPost {
		return errors.New("operation has no registered compensation")
	}
	resourceID := item.node.Outputs["resource_id"]
	if resourceID == nil {
		return errors.New("created resource did not return an ID for compensation")
	}
	collectionPath := strings.TrimSuffix(item.pending.Path, "/")
	for _, operation := range s.catalog {
		if operation.Method != http.MethodDelete || len(operation.PathParams) != 1 {
			continue
		}
		parameter := operation.PathParams[0]
		marker := "/:" + parameter
		if strings.TrimSuffix(operation.Path, marker) != collectionPath {
			continue
		}
		path, err := renderAgentOperationPath(operation, map[string]any{parameter: resourceID})
		if err != nil {
			return err
		}
		_, err = s.executeInternal(ctx, actor, operation.Method, path, nil, nil)
		return err
	}
	return errors.New("created resource has no audited delete compensation")
}

func agentPlanResult(plan *AIAgentExecutionPlan) map[string]any {
	nodes := make([]map[string]any, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		nodes = append(nodes, map[string]any{"id": node.ID, "operation": node.Operation, "status": node.Status, "error": node.Error, "outputs": node.Outputs})
	}
	return map[string]any{"plan_id": plan.ID, "title": plan.Title, "status": plan.Status, "nodes": nodes}
}

func agentPlanToolResult(status string, plan *AIAgentExecutionPlan, position int) map[string]any {
	return map[string]any{"status": status, "position": position, "plan": publicAgentExecutionPlan(plan), "sensitive": plan.Sensitive, "requires_step_up": plan.Sensitive || plan.RequiresSession}
}

func appendAgentRollbacks(existing, added []AIAgentRollback) []AIAgentRollback {
	if len(added) == 0 {
		return existing
	}
	result := append([]AIAgentRollback(nil), existing...)
	var prepend []AIAgentRollback
	for _, rollback := range added {
		merged := false
		if rollback.PlanID != "" && rollback.Strategy == agentRollbackStrategyPlan {
			for index := range result {
				if result[index].PlanID == rollback.PlanID && result[index].Strategy == agentRollbackStrategyPlan {
					mergeAgentPlanRollback(&result[index], rollback)
					merged = true
					break
				}
			}
		}
		if !merged {
			prepend = append(prepend, rollback)
		}
	}
	result = append(prepend, result...)
	if len(result) > 20 {
		result = result[:20]
	}
	return result
}

func mergeAgentPlanRollback(existing *AIAgentRollback, added AIAgentRollback) {
	seen := make(map[string]bool, len(existing.Children))
	for _, child := range existing.Children {
		seen[agentRollbackMergeKey(child)] = true
	}
	for _, child := range added.Children {
		key := agentRollbackMergeKey(child)
		if !seen[key] {
			existing.Children = append(existing.Children, child)
			seen[key] = true
		}
	}
	existing.Sensitive = existing.Sensitive || added.Sensitive
	existing.RequiresStepUp = existing.RequiresStepUp || added.RequiresStepUp
	existing.UpdatedAt = time.Now()
}

func agentRollbackMergeKey(rollback AIAgentRollback) string {
	return strings.Join([]string{rollback.Strategy, rollback.Method, rollback.Path, rollback.TargetID, rollback.Operation}, "\x00")
}

func agentExecutionPlanFingerprint(plan *AIAgentExecutionPlan) string {
	type fingerprintNode struct {
		ID             string         `json:"id"`
		EndpointKey    string         `json:"endpoint_key"`
		DependsOn      []string       `json:"depends_on,omitempty"`
		PathParams     map[string]any `json:"path_params,omitempty"`
		Query          map[string]any `json:"query,omitempty"`
		Body           any            `json:"body,omitempty"`
		IdempotencyKey string         `json:"idempotency_key"`
	}
	nodes := make([]fingerprintNode, 0, len(plan.Nodes))
	for _, node := range plan.Nodes {
		nodes = append(nodes, fingerprintNode{ID: node.ID, EndpointKey: node.EndpointKey, DependsOn: node.DependsOn, PathParams: node.PathParams, Query: node.Query, Body: node.Body, IdempotencyKey: node.IdempotencyKey})
	}
	encoded, _ := json.Marshal(map[string]any{"id": plan.ID, "title": plan.Title, "failure_policy": plan.FailurePolicy, "nodes": nodes})
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func cloneAgentExecutionPlan(plan *AIAgentExecutionPlan) *AIAgentExecutionPlan {
	if plan == nil {
		return nil
	}
	encoded, _ := json.Marshal(plan)
	var cloned AIAgentExecutionPlan
	if json.Unmarshal(encoded, &cloned) != nil {
		return nil
	}
	return &cloned
}

func publicAgentExecutionPlan(plan *AIAgentExecutionPlan) *AIAgentExecutionPlan {
	cloned := cloneAgentExecutionPlan(plan)
	if cloned == nil {
		return nil
	}
	cloned.Title = redactAgentTextSecrets(cloned.Title)
	for index := range cloned.Nodes {
		node := &cloned.Nodes[index]
		node.IdempotencyKey = ""
		node.Query, _ = redactAgentValue(node.Query).(map[string]any)
		node.Body = redactAgentValue(node.Body)
		node.Outputs, _ = redactAgentValue(node.Outputs).(map[string]any)
		for previewIndex := range node.Preview {
			node.Preview[previewIndex].Before = redactAgentValue(node.Preview[previewIndex].Before)
			node.Preview[previewIndex].After = redactAgentValue(node.Preview[previewIndex].After)
		}
	}
	return cloned
}
