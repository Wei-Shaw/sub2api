package codexapp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	if len(events) != 2 {
		t.Fatalf("events count = %d, want 2: %#v", len(events), events)
	}
	if events[0].Type != EventAgentMessageDelta || events[0].Delta != "hello" {
		t.Fatalf("first event = %#v", events[0])
	}
	if events[1].Type != EventTurnCompleted || events[1].Status != "completed" {
		t.Fatalf("second event = %#v", events[1])
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

func startTestProvider(t *testing.T, scenario, markerDir string) *Provider {
	t.Helper()

	provider, err := Start(context.Background(), Config{
		Command: []string{os.Args[0], "-test.run=TestCodexAppServerHelperProcess", "--"},
		Env: append(os.Environ(),
			"GO_WANT_CODEX_APP_SERVER_HELPER=1",
			"CODEX_APP_SERVER_SCENARIO="+scenario,
			"CODEX_APP_SERVER_MARKER_DIR="+markerDir,
		),
		ClientInfo: ClientInfo{
			Name:    "sub2api_codex_provider",
			Title:   "Sub2API Codex Provider",
			Version: "test",
		},
		StartTimeout: 2 * time.Second,
	})
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
			writeHelperResponse(encoder, request.ID, map[string]any{"userAgent": "fake-codex-e2e"}, nil)
		case "initialized":
			// Notification: no response.
		case "thread/start":
			writeHelperResponse(encoder, request.ID, map[string]any{
				"thread": map[string]any{"id": "thr-e2e"},
			}, nil)
		case "turn/start":
			if scenario == "turn-error" {
				writeHelperResponse(encoder, request.ID, nil, map[string]any{"code": -32000, "message": "turn rejected"})
				continue
			}
			writeHelperResponse(encoder, request.ID, map[string]any{
				"turn": map[string]any{"id": "turn-e2e", "status": "inProgress"},
			}, nil)
			if scenario == "cancel" {
				writeMarker(markerDir, "turn-started")
				continue
			}
			_ = encoder.Encode(map[string]any{
				"method": "item/agentMessage/delta",
				"params": map[string]any{"threadId": "thr-e2e", "turnId": "turn-e2e", "delta": "hello"},
			})
			_ = encoder.Encode(map[string]any{
				"method": "turn/completed",
				"params": map[string]any{
					"threadId": "thr-e2e",
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
