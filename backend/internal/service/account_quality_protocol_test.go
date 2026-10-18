package service

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"io"
	"net/http"
	"strings"
	"testing"
)

type qualityProtocolHTTP struct {
	HTTPUpstream
	endpoint   string
	status     int
	calls      int
	payload    string
	incomplete bool
}

func (h *qualityProtocolHTTP) response(req *http.Request, id int64) (*http.Response, error) {
	h.calls++
	raw, _ := io.ReadAll(req.Body)
	h.payload = string(raw)
	text := ""
	switch h.endpoint {
	case "messages":
		text = "data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"<answer>21</answer>\"}}\n\n"
		if !h.incomplete {
			text += "data: {\"type\":\"message_stop\"}\n\n"
		}
	case "chat_completions":
		text = "data: {\"choices\":[{\"delta\":{\"content\":\"<answer>21</answer>\"}}]}\n\n"
		if !h.incomplete {
			text += "data: {\"choices\":[{\"finish_reason\":\"stop\",\"delta\":{}}]}\n\ndata: [DONE]\n\n"
		}
	case "generate_content":
		text = "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"<answer>21</answer>\"}]},\"finishReason\":\"STOP\"}]}\n\n"
	}
	if h.status != 200 {
		text = "fixture failure"
	}
	return &http.Response{StatusCode: h.status, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(text))}, nil
}
func (h *qualityProtocolHTTP) DoWithTLS(req *http.Request, _ string, id int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return h.response(req, id)
}
func (h *qualityProtocolHTTP) Do(req *http.Request, _ string, id int64, _ int) (*http.Response, error) {
	return h.response(req, id)
}
func TestAccountQualityNativeProtocolTransportsAndObservationIsolation(t *testing.T) {
	for _, tc := range []struct{ platform, endpoint string }{{PlatformAnthropic, "messages"}, {"deepseek", "chat_completions"}, {"zhipu", "chat_completions"}, {PlatformGemini, "generate_content"}} {
		for _, status := range []int{200, 401} {
			t.Run(tc.platform+tc.endpoint+http.StatusText(status), func(t *testing.T) {
				repo := &qualityAccountRepo{account: &Account{ID: 7, Platform: tc.platform, Type: AccountTypeAPIKey, Status: "active", Schedulable: true, Concurrency: 1, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://example.invalid"}}}
				transport := &qualityProtocolHTTP{endpoint: tc.endpoint, status: status}
				svc := NewAccountTestService(repo, nil, nil, nil, nil, transport, &config.Config{}, nil)
				p := DefaultAccountQualityPolicy()
				p.Endpoint = tc.endpoint
				p.Effort = "unspecified"
				p.Model = "fixture-model"
				answer, detail, _ := svc.RunQualityProbe(context.Background(), 7, p, "fixture custom challenge")
				if status == 200 && (answer != "<answer>21</answer>" || detail != "") {
					t.Fatalf("native probe failed: %q %q", answer, detail)
				}
				if status == 401 && detail == "" {
					t.Fatal("authentication failure was treated as capability success")
				}
				if repo.mutated {
					t.Fatal("quality probe modified live account status")
				}
				if transport.calls != 1 || !strings.Contains(transport.payload, "fixture custom challenge") {
					t.Fatal("wrong account test payload or unexpected replay", transport.payload)
				}
			})
		}
	}
}
func TestAccountQualityNativeIncompleteStreamIsNotCapabilityPass(t *testing.T) {
	for _, ep := range []string{"messages", "chat_completions"} {
		repo := &qualityAccountRepo{account: &Account{ID: 7, Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Status: "active", Schedulable: true, Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://example.invalid"}}}
		h := &qualityProtocolHTTP{endpoint: ep, status: 200, incomplete: true}
		svc := NewAccountTestService(repo, nil, nil, nil, nil, h, &config.Config{}, nil)
		p := DefaultAccountQualityPolicy()
		p.Endpoint = ep
		p.Effort = "unspecified"
		p.Model = "fixture-model"
		_, detail, _ := svc.RunQualityProbe(context.Background(), 7, p, "fixture")
		if detail == "" {
			t.Fatal("incomplete stream was accepted", ep)
		}
	}
}
