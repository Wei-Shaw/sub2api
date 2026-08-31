package cursor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
)

// TestE2ECursorChat sends a real request to Cursor's API.
// Skipped unless CURSOR_ACCESS_TOKEN is set.
func TestE2ECursorChat(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping e2e test")
	}

	machineID, macMachineID := cursorTelemetryIDs(t)

	creds := cursor.Credentials{
		AccessToken:   accessToken,
		MachineID:     machineID,
		MacMachineID:  macMachineID,
		ClientVersion: os.Getenv("CURSOR_CLIENT_VERSION"),
		ClientCommit:  os.Getenv("CURSOR_CLIENT_COMMIT"),
	}

	t.Logf("client_version=%q machine_id_len=%d mac_machine_id_len=%d",
		creds.ClientVersion, len(machineID), len(macMachineID))
	nalCreds := creds
	nalCreds.ClientLayout = "unifiedAgent"
	headers := cursor.BuildHeaders(nalCreds)
	t.Logf("x-cursor-client-os=%s arch=%s layout=%s timezone=%s commit_set=%v checksum_suffix=%s",
		headers["x-cursor-client-os"],
		headers["x-cursor-client-arch"],
		headers["x-cursor-client-layout"],
		headers["x-cursor-timezone"],
		headers["x-cursor-client-commit"] != "",
		trimChecksum(headers["x-cursor-checksum"]),
	)

	client := cursor.NewClient(creds)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if os.Getenv("CURSOR_FORCE_REFRESH") == "1" {
		refreshToken := os.Getenv("CURSOR_REFRESH_TOKEN")
		if refreshToken != "" {
			t.Log("Attempting token refresh...")
			newToken, err := cursor.RefreshToken(ctx, client.HTTPClient, refreshToken)
			if err != nil {
				t.Logf("Token refresh failed (continuing with original): %v", err)
			} else {
				t.Log("Token refreshed successfully")
				client.Creds.AccessToken = newToken
			}
		}
	}

	messages := []cursor.ChatMessage{
		{Role: "user", Content: "Say exactly: Hello from Sub2API Cursor integration test"},
	}

	if err := client.EstablishSession(ctx); err != nil {
		t.Logf("EstablishSession: %v", err)
	} else {
		t.Logf("EstablishSession OK")
	}

	var resp *http.Response
	var usedModel string
	models := []string{"default", "composer-2.5"}
	for _, model := range models {
		t.Logf("Trying NAL AgentService/Run model=%s", model)
		var err error
		resp, err = client.StreamChat(ctx, messages, model, cursor.ThinkingLevelUnspecified)
		if err != nil {
			t.Logf("StreamChat failed: %v", err)
			continue
		}
		usedModel = model
		break
	}
	if resp == nil || usedModel == "" {
		t.Fatal("All NAL models failed")
	}
	t.Logf("Using model=%s", usedModel)
	defer resp.Body.Close()

	t.Log("Connected to Cursor API, reading stream...")

	var totalText string
	frameIdx := 0
	for {
		frame, err := cursor.DecodeFrame(resp.Body)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Logf("Frame decode error (may be normal at end of stream): %v", err)
			break
		}
		t.Logf("Frame %d: flags=0x%02x payload_len=%d", frameIdx, frame.Flags, len(frame.Payload))
		if len(frame.Payload) < 200 {
			t.Logf("Frame %d raw: %x", frameIdx, frame.Payload)
		} else {
			t.Logf("Frame %d raw (first 200): %x", frameIdx, frame.Payload[:200])
		}
		frameIdx++

		events := cursor.ParseResponseFrame(frame.Payload)
		t.Logf("Frame %d: %d events parsed", frameIdx-1, len(events))
		for _, ev := range events {
			switch ev.Type {
			case "text":
				totalText += ev.Text
				fmt.Print(ev.Text)
			case "thinking":
				t.Logf("[thinking] %s", ev.Text)
			case "turn_ended":
				t.Log("turn_ended")
			}
		}

		if frame.Flags&cursor.FrameFlagEndStream != 0 {
			if msg := cursor.ConnectErrorJSON(frame); msg != "" {
				t.Fatalf("Connect error frame: %s", msg)
			}
			break
		}
		ended := false
		for _, ev := range events {
			if ev.Type == "turn_ended" {
				ended = true
			}
		}
		if ended {
			break
		}
	}
	fmt.Println()

	if totalText == "" {
		t.Fatal("No text received from Cursor API")
	}
	t.Logf("Total response length: %d characters", len(totalText))
}

func TestE2ECursorAvailableModels(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping e2e test")
	}

	machineID, macMachineID := cursorTelemetryIDs(t)
	client := cursor.NewClient(cursor.Credentials{
		AccessToken:   accessToken,
		MachineID:     machineID,
		MacMachineID:  macMachineID,
		ClientVersion: os.Getenv("CURSOR_CLIENT_VERSION"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	models, err := client.AvailableModels(ctx)
	if err != nil {
		t.Fatalf("AvailableModels: %v", err)
	}
	ids := cursor.ModelIDs(models)
	t.Logf("picker count=%d", len(ids))
	for _, m := range models {
		t.Logf("  %s display=%q aliases=%v legacy=%v", m.Name, m.DisplayName, m.Aliases, m.LegacySlugs)
	}
	if len(ids) < 3 {
		t.Fatalf("expected live picker catalog, got %v", ids)
	}
	hasDefault := false
	for _, id := range ids {
		if id == "default" {
			hasDefault = true
			break
		}
	}
	if !hasDefault {
		t.Fatalf("live catalog missing default: %v", ids)
	}
}

func TestE2ECursorRunSlugSnapshotMatchesLive(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping e2e test")
	}

	machineID, macMachineID := cursorTelemetryIDs(t)
	client := cursor.NewClient(cursor.Credentials{
		AccessToken:   accessToken,
		MachineID:     machineID,
		MacMachineID:  macMachineID,
		ClientVersion: os.Getenv("CURSOR_CLIENT_VERSION"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	models, err := client.AvailableModels(ctx)
	if err != nil {
		t.Fatalf("AvailableModels: %v", err)
	}

	for _, model := range models {
		live := append([]string{}, model.LegacySlugs...)
		snapshot := cursor.SnapshotLegacySlugs(model.Name)
		if !sameStringList(snapshot, live) {
			t.Errorf("%s legacy slugs drifted\n  snapshot=%v\n  live=%v", model.Name, snapshot, live)
		}
		res := cursor.ResolveRunModel(model.Name, cursor.RunOpts{}, models)
		if res.RunSlug == "" {
			t.Errorf("%s resolved to empty run slug", model.Name)
		}
		t.Logf("%s -> %s (legacy=%d)", model.Name, res.RunSlug, len(live))
	}
}

func sameStringList(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestE2ECursorDiagnoseEmptyModels(t *testing.T) {
	accessToken := os.Getenv("CURSOR_ACCESS_TOKEN")
	if accessToken == "" {
		t.Skip("CURSOR_ACCESS_TOKEN not set, skipping e2e test")
	}

	machineID, macMachineID := cursorTelemetryIDs(t)
	client := cursor.NewClient(cursor.Credentials{
		AccessToken:   accessToken,
		MachineID:     machineID,
		MacMachineID:  macMachineID,
		ClientVersion: os.Getenv("CURSOR_CLIENT_VERSION"),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := client.EstablishSession(ctx); err != nil {
		t.Logf("EstablishSession: %v", err)
	}

	picker, err := client.AvailableModels(ctx)
	if err != nil {
		t.Fatalf("AvailableModels: %v", err)
	}
	wanted := map[string]bool{
		"default": true, "composer-2.5": true, "gemini-3.1-pro": true,
		"grok-4.6": true, "claude-opus-5": true, "gpt-5.6-sol": true,
	}
	for _, m := range picker {
		if wanted[m.Name] {
			t.Logf("picker name=%q display=%q server=%q default_on=%v aliases=%v legacy=%v",
				m.Name, m.DisplayName, m.ServerModelName, m.DefaultOn, m.Aliases, m.LegacySlugs)
		}
	}

	messages := []cursor.ChatMessage{
		{Role: "user", Content: "In one sentence, what is 2+2?"},
	}
	for _, model := range []string{"default", "grok-4.6", "claude-opus-5", "gpt-5.6-sol"} {
		t.Run(model, func(t *testing.T) {
			diagnoseCursorModel(t, client, messages, model)
		})
	}

	for _, model := range []string{
		"cursor-grok-4.6-medium",
		"claude-opus-5-medium",
		"claude-opus-5-thinking-medium",
		"gpt-5.6-sol-medium",
	} {
		t.Run("legacy:"+model, func(t *testing.T) {
			diagnoseCursorModel(t, client, messages, model)
		})
	}

	t.Run("chatservice:grok-4.6", func(t *testing.T) {
		diagnoseChatService(t, client, messages, "grok-4.6")
	})
	t.Run("chatservice:claude-opus-5", func(t *testing.T) {
		diagnoseChatService(t, client, messages, "claude-opus-5")
	})
}

func diagnoseChatService(t *testing.T, client *cursor.Client, messages []cursor.ChatMessage, model string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	payload := cursor.BuildChatRequest(messages, model, cursor.ThinkingLevelUnspecified)
	frame, err := cursor.EncodeFrame(payload, false)
	if err != nil {
		t.Logf("encode: %v", err)
		return
	}
	url := cursor.BaseURLAPI + cursor.EndpointChatSimple
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(frame))
	if err != nil {
		t.Logf("request: %v", err)
		return
	}
	headers := cursor.BuildHeaders(client.Creds)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		t.Logf("ChatService error: %v", err)
		return
	}
	defer resp.Body.Close()
	t.Logf("ChatService status=%d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 800))
		t.Logf("ChatService body=%s", trimDiagnoseText(string(body)))
		return
	}
	var text strings.Builder
	frameIdx := 0
	for {
		fr, err := cursor.DecodeFrame(resp.Body)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Logf("decode: %v", err)
			break
		}
		if msg := cursor.ConnectErrorJSON(fr); msg != "" {
			t.Logf("frame=%d CONNECT_ERROR %s", frameIdx, trimDiagnoseText(msg))
			break
		}
		events := cursor.ParseResponseFrame(fr.Payload)
		for _, ev := range events {
			if ev.Type == "text" {
				text.WriteString(ev.Text)
			}
		}
		frameIdx++
		if fr.Flags&cursor.FrameFlagEndStream != 0 {
			break
		}
	}
	t.Logf("ChatService frames=%d text=%q", frameIdx, trimDiagnoseText(text.String()))
}

func diagnoseCursorModel(t *testing.T, client *cursor.Client, messages []cursor.ChatMessage, model string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	resp, err := client.StreamChat(ctx, messages, model, cursor.ThinkingLevelUnspecified)
	if err != nil {
		t.Logf("StreamChat error: %v", err)
		return
	}
	defer resp.Body.Close()

	var text, thinking strings.Builder
	frameIdx := 0
	for {
		frame, err := cursor.DecodeFrame(resp.Body)
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Logf("decode: %v", err)
			break
		}
		connectErr := cursor.ConnectErrorJSON(frame)
		events := cursor.ParseResponseFrame(frame.Payload)
		types := make([]string, 0, len(events))
		for _, ev := range events {
			types = append(types, ev.Type)
			switch ev.Type {
			case "text":
				text.WriteString(ev.Text)
			case "thinking":
				thinking.WriteString(ev.Text)
			}
		}
		if connectErr != "" {
			t.Logf("frame=%d CONNECT_ERROR %s", frameIdx, trimDiagnoseText(connectErr))
		} else if frameIdx < 3 || len(events) > 0 {
			t.Logf("frame=%d flags=0x%02x payload=%d events=%v interaction=%s",
				frameIdx, frame.Flags, len(frame.Payload), types,
				summarizeInteractionFields(frame.Payload),
			)
		}
		frameIdx++
		if frame.Flags&cursor.FrameFlagEndStream != 0 {
			break
		}
	}
	t.Logf("parsed_text_len=%d parsed_thinking_len=%d text=%q thinking=%q",
		text.Len(), thinking.Len(), trimDiagnoseText(text.String()), trimDiagnoseText(thinking.String()))
}

func summarizeProtoFields(data []byte) string {
	var parts []string
	pr := cursor.NewProtobufReader(data)
	for i := 0; i < 24; i++ {
		f, err := pr.Next()
		if f == nil || err != nil {
			break
		}
		if f.WireType == cursor.WireBytes {
			parts = append(parts, fmt.Sprintf("%d:bytes:%d", f.Num, len(f.Data)))
		} else {
			parts = append(parts, fmt.Sprintf("%d:varint:%d", f.Num, f.Varint))
		}
	}
	if len(parts) == 0 {
		return "unparsed"
	}
	return strings.Join(parts, ",")
}

func summarizeInteractionFields(data []byte) string {
	pr := cursor.NewProtobufReader(data)
	for {
		f, err := pr.Next()
		if f == nil || err != nil {
			return ""
		}
		if f.Num != 1 || f.WireType != cursor.WireBytes {
			continue
		}
		var parts []string
		inner := cursor.NewProtobufReader(f.Data)
		for i := 0; i < 32; i++ {
			sf, serr := inner.Next()
			if sf == nil || serr != nil {
				break
			}
			if sf.WireType == cursor.WireBytes {
				nested := summarizeProtoFields(sf.Data)
				parts = append(parts, fmt.Sprintf("%d:bytes:%d{%s}", sf.Num, len(sf.Data), nested))
			} else {
				parts = append(parts, fmt.Sprintf("%d:varint:%d", sf.Num, sf.Varint))
			}
		}
		return strings.Join(parts, ",")
	}
}

func extractPrintableStrings(data []byte, minLen, maxCount int) []string {
	var out []string
	start := -1
	flush := func(end int) {
		if start < 0 || end-start < minLen || len(out) >= maxCount {
			start = -1
			return
		}
		s := string(data[start:end])
		if strings.Contains(s, "eyJ") {
			s = "[redacted]"
		}
		out = append(out, trimDiagnoseText(s))
		start = -1
	}
	for i := 0; i < len(data); i++ {
		b := data[i]
		if b >= 32 && b < 127 {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
	}
	flush(len(data))
	return out
}

func trimDiagnoseText(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	if len(s) > 1200 {
		return s[:1200] + "…"
	}
	return s
}

func trimChecksum(cs string) string {
	if len(cs) > 16 {
		return "…" + cs[len(cs)-48:]
	}
	return cs
}

func cursorTelemetryIDs(t *testing.T) (machineID, macMachineID string) {
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
		t.Logf("storage.json not readable: %v", err)
		return machineID, macMachineID
	}
	var storage map[string]any
	if err := json.Unmarshal(raw, &storage); err != nil {
		t.Logf("storage.json parse failed: %v", err)
		return machineID, macMachineID
	}
	telMachine, _ := storage["telemetry.machineId"].(string)
	telMac, _ := storage["telemetry.macMachineId"].(string)

	// Official checksum uses telemetry IDs, not storage.serviceMachineId (UUID).
	if strings.Contains(machineID, "-") && telMachine != "" {
		t.Logf("CURSOR_MACHINE_ID looks like serviceMachineId UUID; using telemetry.machineId from storage.json")
		machineID = telMachine
	}
	if macMachineID == "" && telMac != "" {
		t.Logf("using telemetry.macMachineId from storage.json")
		macMachineID = telMac
	}
	return machineID, macMachineID
}
