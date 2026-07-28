package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type aiAgentMemorySettings struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *aiAgentMemorySettings) Get(_ context.Context, key string) (*Setting, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}
func (s *aiAgentMemorySettings) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := s.Get(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}
func (s *aiAgentMemorySettings) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	s.values[key] = value
	s.mu.Unlock()
	return nil
}
func (s *aiAgentMemorySettings) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]string)
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}
func (s *aiAgentMemorySettings) SetMultiple(_ context.Context, values map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}
func (s *aiAgentMemorySettings) GetAll(_ context.Context) (map[string]string, error) {
	return s.GetMultiple(context.Background(), nil)
}
func (s *aiAgentMemorySettings) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.values, key)
	s.mu.Unlock()
	return nil
}

type aiAgentTestEncryptor struct{}

func (aiAgentTestEncryptor) Encrypt(value string) (string, error) { return "encrypted:" + value, nil }
func (aiAgentTestEncryptor) Decrypt(value string) (string, error) {
	const prefix = "encrypted:"
	if len(value) < len(prefix) || value[:len(prefix)] != prefix {
		return "", ErrSettingNotFound
	}
	return value[len(prefix):], nil
}

func writeAgentChatStream(writer http.ResponseWriter, payload string) {
	var completion agentCompletionResponse
	if json.Unmarshal([]byte(payload), &completion) != nil || len(completion.Choices) == 0 {
		panic("invalid test chat completion")
	}
	message := completion.Choices[0].Message
	toolCalls := make([]map[string]any, 0, len(message.ToolCalls))
	for index, call := range message.ToolCalls {
		toolCalls = append(toolCalls, map[string]any{"index": index, "id": call.ID, "type": call.Type, "function": call.Function})
	}
	delta, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"role": message.Role, "content": message.Content, "tool_calls": toolCalls}}}})
	writer.Header().Set("Content-Type", "text/event-stream")
	_, _ = fmt.Fprintf(writer, "data: %s\n\ndata: [DONE]\n\n", delta)
}

func newAIAgentHistoryTestService(t *testing.T, settings *aiAgentMemorySettings, server *httptest.Server) *AIAgentService {
	t.Helper()
	settings.mu.Lock()
	settings.values[agentSettingEnabled] = "true"
	settings.values[agentSettingBaseURL] = server.URL
	settings.values[agentSettingModel] = "test-model"
	settings.values[agentSettingAPIKey] = "encrypted:model-key"
	settings.values[agentSettingProtocol] = agentProtocolChatCompletions
	settings.values[agentSettingProcessDisplay] = "full"
	settings.mu.Unlock()
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, nil, nil)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	return service
}

func waitForConversationStatus(t *testing.T, service *AIAgentService, userID int64, conversationID, status string) AIAgentSessionSnapshot {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, err := service.Session(context.Background(), userID, conversationID)
		if err == nil && snapshot.Conversation.Status == status {
			return snapshot
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("conversation did not reach status %q", status)
	return AIAgentSessionSnapshot{}
}

func waitForAgentJob(t *testing.T, service *AIAgentService, userID int64, conversationID string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		service.jobsMu.Lock()
		_, running := service.jobs[service.agentJobKey(userID, conversationID)]
		service.jobsMu.Unlock()
		if !running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Agent job did not finish")
}

func TestAIAgentHighConfidenceCandidateExecutesWithoutCatalogSearch(t *testing.T) {
	var mu sync.Mutex
	modelCalls := 0
	firstRequest := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode model request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		encoded, _ := json.Marshal(payload)
		mu.Lock()
		modelCalls++
		call := modelCalls
		if call == 1 {
			firstRequest = string(encoded)
		}
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		if call == 1 {
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"direct_1","type":"function","function":{"name":"execute_admin_operation","arguments":"{\"endpoint_key\":\"POST:/admin/accounts\",\"body\":{\"name\":\"Fast OpenAI\",\"platform\":\"openai\",\"type\":\"apikey\",\"credentials\":{\"api_key\":\"sk-test-candidate-secret\"}}}"}}]}}]}`)
			return
		}
		writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"账号创建操作已准备，等待确认。"}}]}`)
	}))
	defer server.Close()

	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service := newAIAgentHistoryTestService(t, settings, server)
	conversation, err := service.CreateConversation(context.Background(), 17)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := service.StartChat(context.Background(), AIAgentActor{UserID: 17}, conversation.Conversation.ID, "创建一个 OpenAI 账号"); err != nil {
		t.Fatalf("StartChat() error = %v", err)
	}
	completed := waitForConversationStatus(t, service, 17, conversation.Conversation.ID, agentConversationStatusIdle)
	waitForAgentJob(t, service, 17, conversation.Conversation.ID)

	mu.Lock()
	gotCalls := modelCalls
	gotFirstRequest := firstRequest
	mu.Unlock()
	if gotCalls != 2 {
		t.Fatalf("model requests = %d, want 2 (direct tool call plus final response)", gotCalls)
	}
	if !strings.Contains(gotFirstRequest, "Local audited operation plans") || !strings.Contains(gotFirstRequest, "POST:/admin/accounts") || !strings.Contains(gotFirstRequest, `\"confidence\":\"high\"`) {
		t.Fatalf("first model request lacks the high-confidence local candidate: %s", gotFirstRequest)
	}
	if completed.Pending == nil || completed.Pending.EndpointKey != "POST:/admin/accounts" {
		t.Fatalf("pending action = %#v", completed.Pending)
	}
	if len(completed.Messages) != 2 || completed.Messages[0].RunID == "" || completed.Messages[0].RunID != completed.Messages[1].RunID {
		t.Fatalf("message run IDs = %#v", completed.Messages)
	}
	for _, event := range completed.Events {
		if event.RunID != completed.Messages[0].RunID {
			t.Fatalf("event %s run ID = %q, want %q", event.ID, event.RunID, completed.Messages[0].RunID)
		}
	}
	toolNames := make([]string, 0)
	for _, event := range completed.Events {
		if event.Kind == "tool" {
			toolNames = append(toolNames, fmt.Sprint(event.Metadata["tool"]))
		}
	}
	if len(toolNames) != 1 || toolNames[0] != "execute_admin_operation" {
		t.Fatalf("tool calls = %v, want direct execute without catalog search", toolNames)
	}
}

func TestAIAgentMultiIntentModelRunQueuesMultipleWrites(t *testing.T) {
	var mu sync.Mutex
	modelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode model request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		modelCalls++
		call := modelCalls
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		if call == 1 {
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"multi_1","type":"function","function":{"name":"execute_admin_operation","arguments":"{\"endpoint_key\":\"POST:/admin/groups\",\"body\":{\"name\":\"Alpha\"}}"}},{"id":"multi_2","type":"function","function":{"name":"execute_admin_operation","arguments":"{\"endpoint_key\":\"POST:/admin/accounts\",\"body\":{\"name\":\"Beta\",\"platform\":\"openai\",\"type\":\"apikey\",\"credentials\":{\"api_key\":\"sk-test-multi-secret\"}}}"}}]}}]}`)
			return
		}
		writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"两个操作已按顺序等待确认。"}}]}`)
	}))
	defer server.Close()

	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service := newAIAgentHistoryTestService(t, settings, server)
	conversation, err := service.CreateConversation(context.Background(), 19)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	prompt := "创建 Alpha 分组，然后创建 Beta OpenAI 账号"
	if _, err := service.StartChat(context.Background(), AIAgentActor{UserID: 19}, conversation.Conversation.ID, prompt); err != nil {
		t.Fatalf("StartChat() error = %v", err)
	}
	completed := waitForConversationStatus(t, service, 19, conversation.Conversation.ID, agentConversationStatusIdle)
	waitForAgentJob(t, service, 19, conversation.Conversation.ID)

	mu.Lock()
	gotCalls := modelCalls
	mu.Unlock()
	if gotCalls != 2 {
		t.Fatalf("model requests = %d, want 2", gotCalls)
	}
	service.sessionsMu.Lock()
	stored := service.sessions[19][conversation.Conversation.ID]
	service.sessionsMu.Unlock()
	stored.mu.Lock()
	defer stored.mu.Unlock()
	if stored.pending == nil || stored.pending.EndpointKey != "POST:/admin/groups" || len(stored.pendingQueue) != 1 || stored.pendingQueue[0].EndpointKey != "POST:/admin/accounts" {
		t.Fatalf("staged writes: pending=%#v queue=%#v", stored.pending, stored.pendingQueue)
	}
	toolCalls := 0
	for _, event := range completed.Events {
		if event.Kind == "tool" && event.Metadata["tool"] == "execute_admin_operation" {
			toolCalls++
		}
	}
	if toolCalls != 2 {
		t.Fatalf("execute tool events = %d, want 2", toolCalls)
	}
}

func TestAIAgentProviderContextErrorCompressesAndRetries(t *testing.T) {
	var mu sync.Mutex
	var requestSizes []int
	adminCalls := 0
	adminIdempotencyKey := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/api/v1/admin/test/context-checkpoint" {
			mu.Lock()
			adminCalls++
			adminIdempotencyKey = request.Header.Get("Idempotency-Key")
			mu.Unlock()
			_, _ = writer.Write([]byte(`{"code":0,"data":{"id":42,"name":"checkpoint-target"}}`))
			return
		}
		var payload json.RawMessage
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode model payload: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		requestSizes = append(requestSizes, len(payload))
		call := len(requestSizes)
		mu.Unlock()
		switch call {
		case 1:
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"checkpoint-call","type":"function","function":{"name":"execute_admin_operation","arguments":"{\"endpoint_key\":\"POST:/admin/test/context-checkpoint\",\"body\":{\"name\":\"created-before-compression\"}}"}}]}}]}`)
		case 2, 3, 4:
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = writer.Write([]byte(`{"error":{"message":"maximum context length exceeded"}}`))
		default:
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"context retry completed"}}]}`)
		}
	}))
	defer server.Close()

	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service := newAIAgentHistoryTestService(t, settings, server)
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	service.cfg = &config.Config{Server: config.ServerConfig{Port: port}}
	service.client = server.Client()
	service.internalAuth, _ = NewAgentInternalAuth()
	operation := AgentCatalogOperation{Key: "POST:/admin/test/context-checkpoint", Method: http.MethodPost, Path: "/admin/test/context-checkpoint", Title: "Context checkpoint", BodySchema: agentTestOpenBodySchema()}
	service.catalogByKey[operation.Key] = operation
	settings.mu.Lock()
	settings.values[agentSettingContextWindow] = "1m"
	settings.values[agentSettingAutoApprove] = "true"
	settings.mu.Unlock()
	conversation, err := service.CreateConversation(context.Background(), 28)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	service.sessionsMu.Lock()
	stored := service.sessions[28][conversation.Conversation.ID]
	service.sessionsMu.Unlock()
	stored.mu.Lock()
	for index := 0; index < 8; index++ {
		stored.model = append(stored.model,
			agentModelMessage{Role: "user", Content: fmt.Sprintf("old request %d %s", index, strings.Repeat("context ", 8000))},
			agentModelMessage{Role: "assistant", Content: fmt.Sprintf("old answer %d %s", index, strings.Repeat("result ", 4000))},
		)
	}
	stored.mu.Unlock()
	if _, err := service.StartChat(context.Background(), AIAgentActor{UserID: 28}, conversation.Conversation.ID, "current request"); err != nil {
		t.Fatalf("StartChat() error = %v", err)
	}
	completed := waitForConversationStatus(t, service, 28, conversation.Conversation.ID, agentConversationStatusIdle)
	waitForAgentJob(t, service, 28, conversation.Conversation.ID)
	mu.Lock()
	sizes := append([]int(nil), requestSizes...)
	gotAdminCalls := adminCalls
	gotIdempotencyKey := adminIdempotencyKey
	mu.Unlock()
	if len(sizes) != 5 || sizes[2] >= sizes[1] || sizes[3] >= sizes[2] || sizes[4] >= sizes[3] {
		t.Fatalf("progressive model request sizes = %v", sizes)
	}
	if gotAdminCalls != 1 || gotIdempotencyKey == "" {
		t.Fatalf("completed write calls=%d idempotency_key=%q during context retries; events=%#v", gotAdminCalls, gotIdempotencyKey, completed.Events)
	}
	if len(completed.Messages) == 0 || completed.Messages[len(completed.Messages)-1].Content != "context retry completed" {
		t.Fatalf("completed messages = %#v", completed.Messages)
	}
	retryCompressions := 0
	for _, event := range completed.Events {
		if event.Kind == "context_compressed" && event.Metadata["provider_retry"] == true && event.Metadata["quality_check"] == "passed" {
			retryCompressions++
		}
	}
	if retryCompressions != 3 {
		t.Fatalf("provider retry compression events = %d, events=%#v", retryCompressions, completed.Events)
	}
}

func TestAIAgentUnsupportedCapabilityClaimForcesNestedSkillSearch(t *testing.T) {
	var mu sync.Mutex
	modelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		modelCalls++
		call := modelCalls
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"目前没有合适的接口支持这个管理能力。"}}]}`)
		case 2:
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"skill-search","type":"function","function":{"name":"search_admin_operations","arguments":"{\"query\":\"生成未使用的兑换码\"}"}}]}}]}`)
		default:
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"已通过审计 Skill 索引找到候选接口。"}}]}`)
		}
	}))
	defer server.Close()
	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service := newAIAgentHistoryTestService(t, settings, server)
	conversation, err := service.CreateConversation(context.Background(), 29)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := service.StartChat(context.Background(), AIAgentActor{UserID: 29}, conversation.Conversation.ID, "帮我处理这个后台能力"); err != nil {
		t.Fatalf("StartChat() error = %v", err)
	}
	completed := waitForConversationStatus(t, service, 29, conversation.Conversation.ID, agentConversationStatusIdle)
	waitForAgentJob(t, service, 29, conversation.Conversation.ID)
	mu.Lock()
	calls := modelCalls
	mu.Unlock()
	if calls != 3 {
		t.Fatalf("model calls = %d, want correction + Skill search + answer", calls)
	}
	if got := completed.Messages[len(completed.Messages)-1].Content; got != "已通过审计 Skill 索引找到候选接口。" {
		t.Fatalf("final response = %q", got)
	}
	corrections := 0
	searches := 0
	for _, event := range completed.Events {
		if event.Kind == "capability_corrected" {
			corrections++
		}
		if event.Kind == "tool" && event.Metadata["tool"] == "search_admin_operations" {
			searches++
		}
	}
	if corrections != 1 || searches != 1 {
		t.Fatalf("capability corrections=%d searches=%d events=%#v", corrections, searches, completed.Events)
	}
}

func TestAIAgentMissingFieldClaimForcesExactContractInspection(t *testing.T) {
	var mu sync.Mutex
	modelCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		modelCalls++
		call := modelCalls
		mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		switch call {
		case 1:
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"更新接口未开放 subscription_type 字段，因此无法修改。"}}]}`)
		case 2:
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"contract-inspection","type":"function","function":{"name":"search_admin_operations","arguments":"{\"endpoint_key\":\"PUT:/admin/groups/:id\"}"}}]}}]}`)
		default:
			writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"已核对完整合同，更新接口支持 subscription_type。"}}]}`)
		}
	}))
	defer server.Close()
	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service := newAIAgentHistoryTestService(t, settings, server)
	conversation, err := service.CreateConversation(context.Background(), 30)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := service.StartChat(context.Background(), AIAgentActor{UserID: 30}, conversation.Conversation.ID, "将分组改成订阅类型"); err != nil {
		t.Fatalf("StartChat() error = %v", err)
	}
	completed := waitForConversationStatus(t, service, 30, conversation.Conversation.ID, agentConversationStatusIdle)
	waitForAgentJob(t, service, 30, conversation.Conversation.ID)
	mu.Lock()
	calls := modelCalls
	mu.Unlock()
	if calls != 3 || completed.Messages[len(completed.Messages)-1].Content != "已核对完整合同，更新接口支持 subscription_type。" {
		t.Fatalf("contract-inspection flow calls=%d messages=%#v", calls, completed.Messages)
	}
	foundContract := false
	for _, event := range completed.Events {
		if event.Kind == "tool_result" && event.Metadata["status"] == "contract_resolved" {
			foundContract = true
		}
	}
	if !foundContract {
		t.Fatalf("exact contract inspection event missing: %#v", completed.Events)
	}
}

func TestAIAgentRollbackAssistanceAlwaysStagesWritesForConfirmation(t *testing.T) {
	var mu sync.Mutex
	modelCalls := 0
	writes := 0
	trustedManifest := false
	concurrency := float64(10)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/3":
			mu.Lock()
			current := concurrency
			mu.Unlock()
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{"id": 3, "name": "Account", "concurrency": current}})
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/admin/accounts/3":
			var body map[string]any
			_ = json.NewDecoder(request.Body).Decode(&body)
			mu.Lock()
			writes++
			concurrency, _ = body["concurrency"].(float64)
			current := concurrency
			mu.Unlock()
			_ = json.NewEncoder(writer).Encode(map[string]any{"code": 0, "data": map[string]any{"id": 3, "name": "Account", "concurrency": current}})
		case request.Method == http.MethodPost && request.URL.Path == "/v1/chat/completions":
			requestBody, _ := io.ReadAll(request.Body)
			mu.Lock()
			modelCalls++
			call := modelCalls
			if call == 1 {
				text := string(requestBody)
				trustedManifest = strings.Contains(text, "compensation_manifest") && strings.Contains(text, "PUT:/admin/accounts/:id") && strings.Contains(text, `\"id\":3`)
			}
			mu.Unlock()
			switch call {
			case 1:
				writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"recovery-read","type":"function","function":{"name":"execute_admin_operation","arguments":"{\"endpoint_key\":\"GET:/admin/accounts/:id\",\"path_params\":{\"id\":3}}"}}]}}]}`)
			case 2:
				writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"recovery-write","type":"function","function":{"name":"execute_admin_operation","arguments":"{\"endpoint_key\":\"PUT:/admin/accounts/:id\",\"path_params\":{\"id\":3},\"body\":{\"concurrency\":5}}"}}]}}]}`)
			default:
				writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"恢复方案已经生成，等待管理员确认。"}}]}`)
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
			_, _ = writer.Write([]byte(`{"error":"not found"}`))
		}
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	_, portText, _ := net.SplitHostPort(parsed.Host)
	port, _ := strconv.Atoi(portText)
	settings := &aiAgentMemorySettings{values: map[string]string{
		agentSettingEnabled: "true", agentSettingBaseURL: server.URL, agentSettingModel: "test-model", agentSettingAPIKey: "encrypted:model-key",
		agentSettingProtocol: agentProtocolChatCompletions, agentSettingProcessDisplay: "full", agentSettingAutoApprove: "true",
	}}
	internalAuth, _ := NewAgentInternalAuth()
	service, err := NewAIAgentService(settings, aiAgentTestEncryptor{}, &config.Config{Server: config.ServerConfig{Port: port}}, internalAuth)
	if err != nil {
		t.Fatalf("NewAIAgentService() error = %v", err)
	}
	service.client = server.Client()
	conversation, err := service.CreateConversation(context.Background(), 31)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	session, _ := service.conversation(context.Background(), 31, conversation.Conversation.ID, false)
	now := time.Now()
	session.mu.Lock()
	session.rollbacks = []AIAgentRollback{{
		ID: "assist-rollback", Operation: "Update account", Strategy: agentRollbackStrategyRestore, Status: "available",
		TargetLabel: "Account", Method: http.MethodPut, Path: "/admin/accounts/3",
		Body: map[string]any{"concurrency": float64(5)}, ForwardBody: map[string]any{"concurrency": float64(10)},
		Changes: []AIAgentChange{{Field: "concurrency", Before: float64(5), After: float64(10)}}, CreatedAt: now, UpdatedAt: now,
	}}
	session.mu.Unlock()
	if _, err := service.AssistRollback(context.Background(), AIAgentActor{UserID: 31}, conversation.Conversation.ID, "assist-rollback", "只恢复并发"); err != nil {
		t.Fatalf("AssistRollback() error = %v", err)
	}
	completed := waitForConversationStatus(t, service, 31, conversation.Conversation.ID, agentConversationStatusIdle)
	waitForAgentJob(t, service, 31, conversation.Conversation.ID)
	mu.Lock()
	writeCount := writes
	callCount := modelCalls
	manifestIncluded := trustedManifest
	mu.Unlock()
	if writeCount != 0 || callCount != 3 {
		t.Fatalf("assisted rollback writes=%d model_calls=%d", writeCount, callCount)
	}
	if !manifestIncluded {
		t.Fatal("assisted rollback model context omitted the exact audited compensation manifest")
	}
	if completed.Pending == nil || completed.Pending.Path != "/admin/accounts/3" || completed.Pending.RecoveryRollbackID != "" {
		t.Fatalf("assisted rollback did not stage a pending recovery: pending=%#v messages=%#v events=%#v", completed.Pending, completed.Messages, completed.Events)
	}
	session.mu.Lock()
	privatePending := clonePending(session.pending)
	session.mu.Unlock()
	if privatePending == nil || privatePending.RecoveryRollbackID != "assist-rollback" {
		t.Fatalf("pending recovery is not linked to its source rollback: %#v", privatePending)
	}
	if _, err := service.Confirm(context.Background(), AIAgentActor{UserID: 31}, conversation.Conversation.ID, completed.Pending.ID, false); err != nil {
		t.Fatalf("confirm assisted rollback: %v", err)
	}
	resolved, err := service.Session(context.Background(), 31, conversation.Conversation.ID)
	if err != nil {
		t.Fatalf("load resolved recovery: %v", err)
	}
	if len(resolved.Rollbacks) != 1 || resolved.Rollbacks[0].ID != "assist-rollback" || resolved.Rollbacks[0].Status != "completed" || resolved.Rollbacks[0].Resolution != "agent_recovery" {
		t.Fatalf("assisted recovery created a new rollback instead of resolving its source: %#v", resolved.Rollbacks)
	}
	mu.Lock()
	writeCount = writes
	currentConcurrency := concurrency
	mu.Unlock()
	if writeCount != 1 || currentConcurrency != float64(5) {
		t.Fatalf("confirmed assisted recovery writes=%d concurrency=%v", writeCount, currentConcurrency)
	}
}

func TestAIAgentStreamingMessageIsVisibleBeforeCompletion(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial \"}}]}\n\n"))
		writer.(http.Flusher).Flush() //nolint:errcheck // http.Flusher has no error result to handle.
		<-release
		_, _ = writer.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()
	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service := newAIAgentHistoryTestService(t, settings, server)
	conversation, err := service.CreateConversation(context.Background(), 52)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := service.StartChat(context.Background(), AIAgentActor{UserID: 52}, conversation.Conversation.ID, "stream this"); err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var partial AIAgentSessionSnapshot
	for time.Now().Before(deadline) {
		partial, _ = service.Session(context.Background(), 52, conversation.Conversation.ID)
		if len(partial.Messages) == 2 && partial.Messages[1].Streaming {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(partial.Messages) != 2 || !partial.Messages[1].Streaming || partial.Messages[1].Content != "partial " || partial.Conversation.Status != agentConversationStatusRunning {
		t.Fatalf("partial streaming snapshot = %#v", partial)
	}
	close(release)
	completed := waitForConversationStatus(t, service, 52, conversation.Conversation.ID, agentConversationStatusIdle)
	if len(completed.Messages) != 2 || completed.Messages[1].Streaming || completed.Messages[1].Content != "partial answer" {
		t.Fatalf("completed streaming snapshot = %#v", completed.Messages)
	}
}

func TestAIAgentBackgroundChatSurvivesRequestCancellationAndPersists(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-release
		writer.Header().Set("Content-Type", "application/json")
		writeAgentChatStream(writer, `{"choices":[{"message":{"role":"assistant","content":"completed after refresh"}}]}`)
	}))
	defer server.Close()
	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service := newAIAgentHistoryTestService(t, settings, server)
	conversation, err := service.CreateConversation(context.Background(), 7)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	started, err := service.StartChat(requestCtx, AIAgentActor{UserID: 7}, conversation.Conversation.ID, "keep working")
	if err != nil {
		t.Fatalf("StartChat() error = %v", err)
	}
	cancelRequest()
	if started.Conversation.Status != agentConversationStatusRunning {
		t.Fatalf("status = %q", started.Conversation.Status)
	}
	close(release)
	completed := waitForConversationStatus(t, service, 7, conversation.Conversation.ID, agentConversationStatusIdle)
	waitForAgentJob(t, service, 7, conversation.Conversation.ID)
	if got := completed.Messages[len(completed.Messages)-1].Content; got != "completed after refresh" {
		t.Fatalf("assistant message = %q", got)
	}

	reloaded := newAIAgentHistoryTestService(t, settings, server)
	persisted, err := reloaded.Session(context.Background(), 7, conversation.Conversation.ID)
	if err != nil {
		t.Fatalf("reloaded Session() error = %v", err)
	}
	if len(persisted.Messages) != 2 || persisted.Messages[1].Content != "completed after refresh" {
		t.Fatalf("persisted messages = %#v", persisted.Messages)
	}
}

func TestAIAgentStopCancelsRunningModelRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		select {
		case <-request.Context().Done():
		case <-releaseRequest:
		}
	}))
	defer server.Close()
	settings := &aiAgentMemorySettings{values: make(map[string]string)}
	service := newAIAgentHistoryTestService(t, settings, server)
	conversation, err := service.CreateConversation(context.Background(), 9)
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}
	if _, err := service.StartChat(context.Background(), AIAgentActor{UserID: 9}, conversation.Conversation.ID, "stop this"); err != nil {
		t.Fatalf("StartChat() error = %v", err)
	}
	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("model request did not start")
	}
	if !service.Stop(9, conversation.Conversation.ID) {
		t.Fatal("Stop() returned false")
	}
	stopped := waitForConversationStatus(t, service, 9, conversation.Conversation.ID, agentConversationStatusStopped)
	close(releaseRequest)
	if len(stopped.Events) == 0 || stopped.Events[len(stopped.Events)-1].Kind != "stopped" {
		t.Fatalf("events = %#v", stopped.Events)
	}
}
