package codexapp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProviderRunTurnStreamsAndCompletes(t *testing.T) {
	t.Parallel()

	provider := startTestProvider(t, "success", "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var events []Event
	result, err := provider.RunTurn(ctx, TurnRequest{
		Model:     "gpt-6-sol",
		Input:     []InputItem{{Type: "text", Text: "hello"}},
		Ephemeral: true,
	}, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.ThreadID != "thr-e2e" || result.TurnID != "turn-e2e" || result.Status != "completed" {
		t.Fatalf("RunTurn() result = %#v", result)
	}
	if result.Usage != (TokenUsage{InputTokens: 4, CachedInputTokens: 1, OutputTokens: 2, ReasoningOutputTokens: 1, TotalTokens: 6}) {
		t.Fatalf("RunTurn() usage = %#v", result.Usage)
	}
	if len(events) != 4 {
		t.Fatalf("events count = %d, want 4: %#v", len(events), events)
	}
	if events[0].Type != EventAgentMessageDelta || events[0].Delta != "hello" {
		t.Fatalf("first event = %#v", events[0])
	}
	if events[1].Type != EventThreadTokenUsage || events[1].Usage.TotalTokens != 3 {
		t.Fatalf("second event = %#v", events[1])
	}
	if events[2].Type != EventThreadTokenUsage || events[2].Usage.TotalTokens != 3 {
		t.Fatalf("third event = %#v", events[2])
	}
	if events[3].Type != EventTurnCompleted || events[3].Status != "completed" {
		t.Fatalf("fourth event = %#v", events[3])
	}
}

func TestProviderRunTurnReturnsRPCError(t *testing.T) {
	t.Parallel()

	provider := startTestProvider(t, "turn-error", "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := provider.RunTurn(ctx, TurnRequest{
		Model: "gpt-6-sol",
		Input: []InputItem{{Type: "text", Text: "hello"}},
	}, nil)
	if err == nil {
		t.Fatal("RunTurn() error = nil, want RPC error")
	}
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("RunTurn() error type = %T, want *RPCError: %v", err, err)
	}
	if rpcErr.Method != "turn/start" || rpcErr.Code != -32000 || rpcErr.Message != "turn rejected" {
		t.Fatalf("RPC error = %#v", rpcErr)
	}
}

func TestProviderRunTurnUsesExistingThread(t *testing.T) {
	t.Parallel()

	provider := startTestProvider(t, "success", "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := provider.RunTurn(ctx, TurnRequest{
		ThreadID: "thr-existing",
		Input:    []InputItem{{Type: "text", Text: "continue"}},
	}, nil)
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.ThreadID != "thr-existing" || result.Status != "completed" {
		t.Fatalf("RunTurn() result = %#v", result)
	}
}

func TestProviderCancellationInterruptsTurn(t *testing.T) {
	t.Parallel()

	markerDir := t.TempDir()
	provider := startTestProvider(t, "cancel", markerDir)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := provider.RunTurn(ctx, TurnRequest{
			Model: "gpt-6-sol",
			Input: []InputItem{{Type: "text", Text: "wait"}},
		}, nil)
		errCh <- err
	}()

	waitForFile(t, filepath.Join(markerDir, "turn-started"))
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunTurn() error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunTurn() did not return after cancellation")
	}
	waitForFile(t, filepath.Join(markerDir, "turn-interrupted"))
}

func TestProviderSinkErrorInterruptsTurn(t *testing.T) {
	t.Parallel()

	markerDir := t.TempDir()
	provider := startTestProvider(t, "success", markerDir)
	wantErr := errors.New("client write failed")

	_, err := provider.RunTurn(context.Background(), TurnRequest{
		Model: "gpt-6-sol",
		Input: []InputItem{{Type: "text", Text: "hello"}},
	}, func(Event) error { return wantErr })
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunTurn() error = %v, want %v", err, wantErr)
	}
	waitForFile(t, filepath.Join(markerDir, "turn-interrupted"))
}

func TestProviderMalformedMessageFailsTurn(t *testing.T) {
	t.Parallel()

	provider := startTestProvider(t, "malformed", "")
	_, err := provider.RunTurn(context.Background(), TurnRequest{
		Model: "gpt-6-sol",
		Input: []InputItem{{Type: "text", Text: "hello"}},
	}, nil)
	if err == nil || !contains(err.Error(), "decode codex app-server message") {
		t.Fatalf("RunTurn() error = %v, want malformed message error", err)
	}
}

func TestStartValidatesConfigAndInitialization(t *testing.T) {
	t.Parallel()

	if _, err := Start(context.Background(), Config{}); err == nil || !contains(err.Error(), "command is required") {
		t.Fatalf("Start(empty) error = %v", err)
	}
	if _, err := Start(context.Background(), Config{Command: []string{"unused"}}); err == nil || !contains(err.Error(), "clientInfo.name is required") {
		t.Fatalf("Start(no clientInfo) error = %v", err)
	}

	_, err := Start(context.Background(), testConfig("init-error", ""))
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Method != "initialize" || rpcErr.Code != -32001 {
		t.Fatalf("Start(init-error) error = %#v", err)
	}

	timeoutConfig := testConfig("init-timeout", "")
	timeoutConfig.StartTimeout = 30 * time.Millisecond
	_, err = Start(context.Background(), timeoutConfig)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start(init-timeout) error = %v, want deadline exceeded", err)
	}
}

func TestProviderValidationAndRPCErrorText(t *testing.T) {
	t.Parallel()

	var nilProvider *Provider
	if err := nilProvider.Close(context.Background()); err != nil {
		t.Fatalf("nil Close() error = %v", err)
	}
	if _, err := nilProvider.RunTurn(context.Background(), TurnRequest{}, nil); err == nil {
		t.Fatal("nil RunTurn() error = nil")
	}

	provider := startTestProvider(t, "success", "")
	if _, err := provider.RunTurn(context.Background(), TurnRequest{}, nil); err == nil || !contains(err.Error(), "input is required") {
		t.Fatalf("RunTurn(empty input) error = %v", err)
	}

	rpcErr := &RPCError{Method: "turn/start", Code: -32000, Message: "rejected"}
	if got := rpcErr.Error(); got != "codex app-server turn/start failed (-32000): rejected" {
		t.Fatalf("RPCError.Error() = %q", got)
	}
	var nilRPCError *RPCError
	if got := nilRPCError.Error(); got != "codex app-server RPC error" {
		t.Fatalf("nil RPCError.Error() = %q", got)
	}
}

func TestDecodeTurnEventIgnoresUnrelatedNotifications(t *testing.T) {
	t.Parallel()

	if event, matched, err := decodeTurnEvent(rpcNotification{Method: "thread/started"}, "thr", "turn"); err != nil || matched || event != (Event{}) {
		t.Fatalf("unknown event = %#v, matched=%v, err=%v", event, matched, err)
	}
	params, err := json.Marshal(map[string]any{"threadId": "other", "turnId": "turn", "delta": "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if _, matched, err := decodeTurnEvent(rpcNotification{Method: EventAgentMessageDelta, Params: params}, "thr", "turn"); err != nil || matched {
		t.Fatalf("mismatched delta matched=%v err=%v", matched, err)
	}
}

func TestDecodeTurnEventRejectsMalformedPayloads(t *testing.T) {
	t.Parallel()

	for _, method := range []string{EventAgentMessageDelta, EventThreadTokenUsage, EventTurnCompleted} {
		_, _, err := decodeTurnEvent(rpcNotification{Method: method, Params: json.RawMessage(`{`)}, "thr", "turn")
		if err == nil || !contains(err.Error(), "decode "+method+" notification") {
			t.Fatalf("decodeTurnEvent(%s) error = %v", method, err)
		}
	}

	params, err := json.Marshal(map[string]any{
		"threadId": "other",
		"turn":     map[string]any{"id": "turn", "status": "completed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, matched, err := decodeTurnEvent(rpcNotification{Method: EventTurnCompleted, Params: params}, "thr", "turn"); err != nil || matched {
		t.Fatalf("mismatched completed matched=%v err=%v", matched, err)
	}
}

func TestProviderCloseHonorsContextAndTimeoutDefaults(t *testing.T) {
	t.Parallel()

	provider := startTestProvider(t, "hang-close", "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := provider.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() error = %v, want deadline exceeded", err)
	}

	if got := interruptTimeout(Config{}); got != defaultInterruptTimeout {
		t.Fatalf("interruptTimeout(default) = %s", got)
	}
	if got := interruptTimeout(Config{InterruptTimeout: time.Second}); got != time.Second {
		t.Fatalf("interruptTimeout(custom) = %s", got)
	}

	if err := (*process)(nil).terminalError(); !errors.Is(err, io.EOF) {
		t.Fatalf("nil process terminalError() = %v", err)
	}
	client := &rpcClient{err: io.EOF}
	waitErr := errors.New("exit status 9")
	proc := &process{rpc: client, waitErr: waitErr}
	if err := proc.terminalError(); err == nil || !contains(err.Error(), "exit status 9") {
		t.Fatalf("process terminalError() = %v", err)
	}
}

func startTestProvider(t *testing.T, scenario, markerDir string) *Provider {
	t.Helper()

	provider, err := Start(context.Background(), testConfig(scenario, markerDir))
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := provider.Close(closeCtx); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return provider
}

func testConfig(scenario, markerDir string) Config {
	return Config{
		Command: []string{os.Args[0], "-test.run=TestCodexAppServerHelperProcess", "--"},
		Env: append(os.Environ(),
			"GO_WANT_CODEX_APP_SERVER_HELPER=1",
			"CODEX_APP_SERVER_SCENARIO="+scenario,
			"CODEX_APP_SERVER_MARKER_DIR="+markerDir,
			"GORACE=atexit_sleep_ms=0",
		),
		ClientInfo: ClientInfo{
			Name:    "sub2api_codex_provider",
			Title:   "Sub2API Codex Provider",
			Version: "test",
		},
		StartTimeout: 2 * time.Second,
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func contains(value, fragment string) bool {
	return strings.Contains(value, fragment)
}

func TestCodexAppServerHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CODEX_APP_SERVER_HELPER") != "1" {
		return
	}

	scenario := os.Getenv("CODEX_APP_SERVER_SCENARIO")
	markerDir := os.Getenv("CODEX_APP_SERVER_MARKER_DIR")
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			os.Exit(2)
		}

		switch request.Method {
		case "initialize":
			var params struct {
				ClientInfo ClientInfo `json:"clientInfo"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil || params.ClientInfo.Name != "sub2api_codex_provider" {
				os.Exit(3)
			}
			if scenario == "init-timeout" {
				continue
			}
			if scenario == "init-error" {
				writeHelperResponse(encoder, request.ID, nil, map[string]any{"code": -32001, "message": "initialize rejected"})
				continue
			}
			writeHelperResponse(encoder, request.ID, map[string]any{"userAgent": "fake-codex-e2e"}, nil)
		case "initialized":
			// Notification: no response.
		case "thread/start":
			writeHelperResponse(encoder, request.ID, map[string]any{
				"thread": map[string]any{"id": "thr-e2e"},
			}, nil)
		case "turn/start":
			var params struct {
				ThreadID string `json:"threadId"`
			}
			if err := json.Unmarshal(request.Params, &params); err != nil {
				os.Exit(5)
			}
			if params.ThreadID == "" {
				params.ThreadID = "thr-e2e"
			}
			if scenario == "turn-error" {
				writeHelperResponse(encoder, request.ID, nil, map[string]any{"code": -32000, "message": "turn rejected"})
				continue
			}
			writeHelperResponse(encoder, request.ID, map[string]any{
				"turn": map[string]any{"id": "turn-e2e", "status": "inProgress"},
			}, nil)
			if scenario == "malformed" {
				_, _ = fmt.Fprintln(os.Stdout, "{not-json")
				continue
			}
			if scenario == "cancel" {
				writeMarker(markerDir, "turn-started")
				continue
			}
			_ = encoder.Encode(map[string]any{
				"method": "item/agentMessage/delta",
				"params": map[string]any{"threadId": params.ThreadID, "turnId": "turn-e2e", "delta": "hello"},
			})
			_ = encoder.Encode(map[string]any{
				"method": "thread/tokenUsage/updated",
				"params": map[string]any{
					"threadId": params.ThreadID,
					"turnId":   "turn-e2e",
					"tokenUsage": map[string]any{
						"last": map[string]any{
							"inputTokens": 2, "cachedInputTokens": 0, "outputTokens": 1,
							"reasoningOutputTokens": 0, "totalTokens": 3,
						},
					},
				},
			})
			_ = encoder.Encode(map[string]any{
				"method": "thread/tokenUsage/updated",
				"params": map[string]any{
					"threadId": params.ThreadID,
					"turnId":   "turn-e2e",
					"tokenUsage": map[string]any{
						"last": map[string]any{
							"inputTokens": 2, "cachedInputTokens": 1, "outputTokens": 1,
							"reasoningOutputTokens": 1, "totalTokens": 3,
						},
					},
				},
			})
			_ = encoder.Encode(map[string]any{
				"method": "turn/completed",
				"params": map[string]any{
					"threadId": params.ThreadID,
					"turn":     map[string]any{"id": "turn-e2e", "status": "completed"},
				},
			})
		case "turn/interrupt":
			writeMarker(markerDir, "turn-interrupted")
			writeHelperResponse(encoder, request.ID, map[string]any{}, nil)
		default:
			writeHelperResponse(encoder, request.ID, nil, map[string]any{
				"code": -32601, "message": fmt.Sprintf("unsupported method %s", request.Method),
			})
		}
	}
	if scenario == "hang-close" {
		for {
			time.Sleep(time.Second)
		}
	}
	os.Exit(0)
}

func writeHelperResponse(encoder *json.Encoder, id json.RawMessage, result any, rpcErr map[string]any) {
	response := map[string]any{"id": id}
	if rpcErr != nil {
		response["error"] = rpcErr
	} else {
		response["result"] = result
	}
	_ = encoder.Encode(response)
}

func writeMarker(dir, name string) {
	if dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("ok"), 0o600); err != nil {
		os.Exit(4)
	}
}
