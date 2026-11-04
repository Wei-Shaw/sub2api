//go:build e2e

package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/chatgptnetwork"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSIPv6LiveToggle(t *testing.T) {
	token := os.Getenv("CGPT_IPV6_LIVE_TOKEN")
	if token == "" {
		t.Skip("live credentials not supplied")
	}
	old := chatgptnetwork.Enabled()
	defer chatgptnetwork.SetEnabled(old)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	settings := NewSettingService(&wsLiveSettings{values: map[string]string{}}, cfg)
	remotes := map[string]string{}
	modes := map[string]bool{}
	for _, enabled := range []bool{false, true, false} {
		stored, err := settings.GetAllSettings(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		stored.ChatGPTIPv6Only = enabled
		if err = settings.UpdateSettings(context.Background(), stored); err != nil {
			t.Fatal(err)
		}
		if chatgptnetwork.Enabled() != enabled {
			t.Fatal("saved setting not applied")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		var remote string
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{GotConn: func(i httptrace.GotConnInfo) { remote = i.Conn.RemoteAddr().String() }})
		h := http.Header{}
		h.Set("Authorization", "Bearer "+token)
		h.Set("Chatgpt-Account-Id", os.Getenv("CGPT_IPV6_LIVE_ACCOUNT_ID"))
		h.Set("User-Agent", CodexCanonicalUserAgent())
		h.Set("Originator", "codex_cli_rs")
		h.Set("OpenAI-Beta", openAIWSBetaV2Value)
		start := time.Now()
		conn, err := pool.Acquire(ctx, openAIWSAcquireRequest{Account: &Account{ID: 9004, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, WSURL: "wss://chatgpt.com/backend-api/codex/responses", Headers: h})
		if err != nil {
			cancel()
			t.Fatalf("enabled=%v remote=%s error=%v", enabled, remote, err)
		}
		if conn.Reused() {
			remote = remotes[conn.ConnID()]
			if modes[conn.ConnID()] != enabled {
				t.Fatal("reused incompatible network mode")
			}
		} else {
			remotes[conn.ConnID()] = remote
			modes[conn.ConnID()] = enabled
		}
		host, _, _ := net.SplitHostPort(remote)
		ip := net.ParseIP(host)
		if ip == nil || (enabled && ip.To4() != nil) {
			conn.Release()
			cancel()
			t.Fatalf("unexpected remote %s", remote)
		}
		err = conn.WriteJSONContext(ctx, map[string]any{"type": "response.create", "model": "gpt-6.1-sol", "instructions": "Follow the user request.", "input": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": "Reply with OK."}}}}, "store": false})
		if err != nil {
			conn.Release()
			cancel()
			t.Fatal(err)
		}
		completed := false
		events := 0
		for events < 200 {
			raw, e := conn.ReadMessageContext(ctx)
			if e != nil {
				err = e
				break
			}
			events++
			kind := gjson.GetBytes(raw, "type").String()
			if kind == "response.completed" {
				completed = true
				break
			}
			if kind == "error" || kind == "response.failed" {
				t.Logf("upstream type=%s code=%s", kind, gjson.GetBytes(raw, "error.code"))
				break
			}
		}
		conn.Release()
		cancel()
		t.Logf("enabled=%v remote=%s handshake=101 reused=%v completed=%v events=%d elapsed=%.2f", enabled, remote, conn.Reused(), completed, events, time.Since(start).Seconds())
		if !completed {
			t.Fatalf("completion missing: %v", err)
		}
	}
}

type wsLiveSettings struct{ values map[string]string }

func (r *wsLiveSettings) GetAll(context.Context) (map[string]string, error) { return r.values, nil }
func (r *wsLiveSettings) GetValue(_ context.Context, k string) (string, error) {
	return r.values[k], nil
}
func (r *wsLiveSettings) Get(_ context.Context, k string) (*Setting, error) {
	return &Setting{Key: k, Value: r.values[k]}, nil
}
func (r *wsLiveSettings) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	v := map[string]string{}
	for _, k := range keys {
		if s, ok := r.values[k]; ok {
			v[k] = s
		}
	}
	return v, nil
}
func (r *wsLiveSettings) SetMultiple(_ context.Context, v map[string]string) error {
	for k, s := range v {
		r.values[k] = s
	}
	return nil
}
func (r *wsLiveSettings) Set(_ context.Context, k, v string) error { r.values[k] = v; return nil }
func (r *wsLiveSettings) Delete(_ context.Context, k string) error { delete(r.values, k); return nil }
