package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestE2ECursorGatewayChatCompletions exercises the OpenAI-compatible
// Cursor gateway against a live Cursor Pro account.
func TestE2ECursorGatewayChatCompletions(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping gateway e2e")
	}

	machineID, macMachineID := cursorGatewayTelemetryIDs(t)
	account := &Account{
		ID:       1,
		Name:     "cursor-e2e",
		Platform: PlatformCursor,
		Credentials: map[string]any{
			"access_token":   accessToken,
			"machine_id":     machineID,
			"mac_machine_id": macMachineID,
			"client_version": os.Getenv("CURSOR_CLIENT_VERSION"),
		},
	}

	gin.SetMode(gin.TestMode)
	svc := NewCursorGatewayService()
	body := []byte(`{"model":"default","stream":false,"messages":[{"role":"user","content":"Say exactly: Hello from Sub2API Cursor gateway"}]}`)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
	if err != nil {
		t.Fatalf("ForwardAsChatCompletions: %v", err)
	}
	if result == nil || result.Stream {
		t.Fatalf("unexpected result: %+v", result)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("parse response: %v body=%s", err, rec.Body.String())
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		t.Fatalf("empty completion: %s", rec.Body.String())
	}
	t.Logf("gateway completion: %q usage=%+v", parsed.Choices[0].Message.Content, result.Usage)
}

func TestE2ECursorGatewayVariantModels(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping gateway e2e")
	}

	machineID, macMachineID := cursorGatewayTelemetryIDs(t)
	account := &Account{
		ID:       1,
		Name:     "cursor-e2e",
		Platform: PlatformCursor,
		Credentials: map[string]any{
			"access_token":   accessToken,
			"machine_id":     machineID,
			"mac_machine_id": macMachineID,
			"client_version": os.Getenv("CURSOR_CLIENT_VERSION"),
		},
	}

	gin.SetMode(gin.TestMode)
	svc := NewCursorGatewayService()

	for _, model := range []string{"grok-4.6", "claude-opus-5", "gpt-5.6-sol"} {
		t.Run(model, func(t *testing.T) {
			body := []byte(`{"model":"` + model + `","stream":false,"messages":[{"role":"user","content":"Reply with exactly: 2 + 2 equals 4."}]}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			result, err := svc.ForwardAsChatCompletions(ctx, c, account, body)
			if err != nil {
				t.Fatalf("ForwardAsChatCompletions: %v body=%s", err, rec.Body.String())
			}
			if result == nil {
				t.Fatal("nil result")
			}

			var parsed struct {
				Choices []struct {
					Message struct {
						Content string `json:"content"`
					} `json:"message"`
				} `json:"choices"`
				Warnings []map[string]string `json:"warnings"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
				t.Fatalf("parse response: %v body=%s", err, rec.Body.String())
			}
			if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
				t.Fatalf("empty completion: %s", rec.Body.String())
			}
			t.Logf("variant=%s warnings=%v content=%q usage=%+v", rec.Header().Get("X-Sub2API-Model-Variant"), parsed.Warnings, parsed.Choices[0].Message.Content, result.Usage)
		})
	}
}

func cursorGatewayTelemetryIDs(t *testing.T) (machineID, macMachineID string) {
	t.Helper()
	machineID = os.Getenv("CURSOR_MACHINE_ID")
	macMachineID = os.Getenv("CURSOR_MAC_MACHINE_ID")

	home, err := os.UserHomeDir()
	if err != nil {
		return machineID, macMachineID
	}
	storagePath := filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "storage.json")
	raw, err := os.ReadFile(storagePath)
	if err != nil {
		return machineID, macMachineID
	}
	var storage map[string]any
	if err := json.Unmarshal(raw, &storage); err != nil {
		return machineID, macMachineID
	}
	telMachine, _ := storage["telemetry.machineId"].(string)
	telMac, _ := storage["telemetry.macMachineId"].(string)
	if strings.Contains(machineID, "-") && telMachine != "" {
		machineID = telMachine
	}
	if macMachineID == "" && telMac != "" {
		macMachineID = telMac
	}
	return machineID, macMachineID
}
