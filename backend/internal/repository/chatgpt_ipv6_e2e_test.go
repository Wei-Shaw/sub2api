//go:build e2e

package repository

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptnetwork"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/tidwall/gjson"
)

// Opt-in live test; credentials are supplied only through the environment.
func TestChatGPTIPv6LiveCodex(t *testing.T) {
	token := os.Getenv("CGPT_IPV6_LIVE_TOKEN")
	if token == "" {
		t.Skip("live credentials not supplied")
	}
	t.Setenv(chatgptnetwork.EnvIPv6Only, "true")
	for _, kind := range []string{"responses", "oauth-image", "native-image"} {
		t.Run(kind, func(t *testing.T) {
			tr, err := buildUpstreamTransportWithTLSFingerprint(poolSettings{}, nil, tlsfingerprint.CodexCLIProfile())
			if err != nil {
				t.Fatal(err)
			}
			defer tr.CloseIdleConnections()
			client := &http.Client{Transport: chatgptnetwork.Wrap(tr, false), Timeout: 300 * time.Second}
			prompt := "Reply with OK."
			if kind != "responses" {
				prompt = "a yellow rubber duck on a white table, product photo"
			}
			path := "/backend-api/codex/responses"
			payload := map[string]any{"model": "gpt-6.1-sol", "instructions": "Follow the user request.", "input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": prompt}}}}, "stream": true, "store": false}
			if kind == "oauth-image" {
				payload["tools"] = []any{map[string]any{"type": "image_generation", "model": "gpt-image-2", "size": "1024x1024", "quality": "low"}}
				payload["tool_choice"] = map[string]any{"type": "image_generation"}
			}
			if kind == "native-image" {
				path = "/backend-api/codex/images/generations"
				payload = map[string]any{"model": "gpt-image-2", "prompt": prompt, "n": 1, "size": "1024x1024", "quality": "low"}
			}
			body, _ := json.Marshal(payload)
			var remotes []string
			ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{GotConn: func(i httptrace.GotConnInfo) { remotes = append(remotes, i.Conn.RemoteAddr().String()) }})
			req, _ := http.NewRequestWithContext(ctx, "POST", "https://chatgpt.com"+path, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Chatgpt-Account-Id", os.Getenv("CGPT_IPV6_LIVE_ACCOUNT_ID"))
			req.Header.Set("User-Agent", "codex_exec/0.163.0 (Mac OS 15.6.0; arm64) dumb (codex_exec; 0.163.0)")
			req.Header.Set("Originator", "codex_exec")
			start := time.Now()
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("kind=%s status=%d seconds=%.2f remotes=%v bytes=%d", kind, resp.StatusCode, time.Since(start).Seconds(), remotes, len(raw))
			for _, remote := range remotes {
				host, _, _ := net.SplitHostPort(remote)
				ip := net.ParseIP(host)
				if ip == nil || ip.To4() != nil {
					t.Fatal("not IPv6", remote)
				}
			}
			if resp.StatusCode != 200 {
				t.Fatalf("upstream status %d error code=%s", resp.StatusCode, gjson.GetBytes(raw, "error.code"))
			}
			encoded := ""
			completed := false
			if kind == "native-image" {
				encoded = gjson.GetBytes(raw, "data.0.b64_json").String()
			} else {
				for _, line := range strings.Split(string(raw), "\n") {
					if !strings.HasPrefix(line, "data: ") {
						continue
					}
					event := gjson.Parse(strings.TrimPrefix(line, "data: "))
					if event.Get("type").String() == "response.image_generation_call.completed" && event.Get("result").String() != "" {
						encoded = event.Get("result").String()
					}
					if event.Get("type").String() == "response.output_item.done" && event.Get("item.type").String() == "image_generation_call" && event.Get("item.result").String() != "" {
						encoded = event.Get("item.result").String()
					}

					if event.Get("type").String() == "response.completed" {
						completed = true
						for _, item := range event.Get("response.output").Array() {
							if item.Get("type").String() == "image_generation_call" && item.Get("result").String() != "" {
								encoded = item.Get("result").String()
							}
						}
					}
				}
			}
			if kind == "responses" {
				if !completed {
					t.Fatal("response.completed missing")
				}
				return
			}
			image, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || len(image) == 0 {
				t.Fatal("image missing or invalid")
			}
			decoded, err := png.Decode(bytes.NewReader(image))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("image=%v", decoded.Bounds())
			if out := os.Getenv("CGPT_IPV6_LIVE_OUTPUT"); out != "" {
				if err = os.MkdirAll(out, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(out, fmt.Sprintf("%s.png", kind)), image, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
