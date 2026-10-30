package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func reminderTestBody(t *testing.T, protocol string, texts []string) []byte {
	t.Helper()
	blocks := make([]map[string]string, 0, len(texts))
	for _, text := range texts {
		blocks = append(blocks, map[string]string{"type": "text", "text": text})
	}
	message := map[string]any{"role": "user", "content": blocks}
	var body any
	switch protocol {
	case ContentModerationProtocolGemini:
		body = map[string]any{"contents": []any{map[string]any{"role": "user", "parts": blocks}}}
	case ContentModerationProtocolOpenAIResponses, "unknown":
		body = map[string]any{"input": []any{message}}
	case ContentModerationProtocolOpenAIImages:
		body = map[string]any{"prompt": strings.Join(texts, "\n")}
	default:
		body = map[string]any{"messages": []any{message}}
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return raw
}

func TestContentModerationCheck_ReminderKeywordModes(t *testing.T) {
	for _, protocol := range []string{ContentModerationProtocolAnthropicMessages, ContentModerationProtocolOpenAIChat, ContentModerationProtocolOpenAIResponses, ContentModerationProtocolGemini, ContentModerationProtocolOpenAIImages, "unknown"} {
		for name, texts := range map[string][]string{
			"plain":    {"今晚打老虎"},
			"prefix":   {"<system-reminder>context</system-reminder> 今晚打老虎"},
			"suffix":   {"今晚打老虎 <system-reminder>context</system-reminder>"},
			"separate": {"<system-reminder>context</system-reminder>", "今晚打老虎"},
			"inside":   {"<system-reminder>今晚打老虎</system-reminder>"},
		} {
			for _, mode := range []string{ContentModerationKeywordModeKeywordOnly, ContentModerationKeywordModeAPIOnly, ContentModerationKeywordModeKeywordAndAPI} {
				t.Run(protocol+"/"+name+"/"+mode, func(t *testing.T) {
					body := reminderTestBody(t, protocol, texts)
					semantic := ExtractContentModerationInput(protocol, body)
					wantSemantic := ""
					if name == "plain" || (name == "separate" && protocol != ContentModerationProtocolOpenAIImages) {
						wantSemantic = "今晚打老虎"
					}
					require.Equal(t, wantSemantic, semantic.Text)
					require.Contains(t, extractContentModerationKeywordText(protocol, body), "今晚打老虎")
					var calls atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						var payload struct {
							Input string `json:"input"`
						}
						if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Error(err)
						}
						if payload.Input != semantic.Text {
							t.Errorf("semantic input changed: %q != %q", payload.Input, semantic.Text)
						}
						_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{}}})
					}))
					defer server.Close()
					cfg := defaultContentModerationConfig()
					cfg.Enabled = true
					cfg.Mode = ContentModerationModePreBlock
					cfg.KeywordBlockingMode = mode
					cfg.BlockedKeywords = []string{"今晚打老虎"}
					cfg.BaseURL = server.URL
					cfg.APIKeys = []string{"test"}
					cfg.RecordNonHits = false
					cfg.PreHashCheckEnabled = true
					raw, err := json.Marshal(cfg)
					require.NoError(t, err)
					repo := &contentModerationTestRepo{}
					cache := &contentModerationTestHashCache{}
					if mode != ContentModerationKeywordModeAPIOnly {
						cache.hashes = map[string]struct{}{semantic.Hash(): {}}
					}
					svc := NewContentModerationService(&contentModerationTestSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true", SettingKeyContentModerationConfig: string(raw)}}, repo, cache, nil, nil, nil, nil, nil)
					decision, err := svc.Check(context.Background(), ContentModerationCheckInput{Protocol: protocol, Body: body})
					require.NoError(t, err)
					if mode == ContentModerationKeywordModeAPIOnly {
						require.True(t, decision.Allowed)
						want := int32(0)
						if !semantic.IsEmpty() {
							want = 1
						}
						require.Equal(t, want, calls.Load())
						require.Empty(t, repo.snapshotLogs())
					} else {
						require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
						require.True(t, decision.Blocked)
						require.Zero(t, calls.Load())
						require.Empty(t, cache.snapshotChecked())
						logs := requireContentModerationLogCount(t, repo, 1)
						require.Equal(t, "今晚打老虎", logs[0].MatchedKeyword)
						require.Contains(t, logs[0].InputExcerpt, "今晚打老虎")
						require.Equal(t, int64(1), svc.asyncEnqueued.Load())
					}
				})
			}
		}
	}
}

func TestExtractContentModerationKeywordText_Boundaries(t *testing.T) {
	for _, tc := range []struct{ protocol, body string }{
		{ContentModerationProtocolAnthropicMessages, `{"messages":[{"role":"user","content":"old"},{"role":"user","content":[{"type":"tool_result","content":"今晚打老虎"}]}]}`},
		{ContentModerationProtocolAnthropicMessages, `{"messages":[{"role":"user","content":"old"},{"role":"assistant","content":"今晚打老虎"}]}`},
		{ContentModerationProtocolOpenAIChat, `{"messages":[{"role":"user","content":"old"},{"role":"tool","content":"今晚打老虎"}]}`},
		{ContentModerationProtocolOpenAIChat, `{"messages":[{"role":"user","content":"old"},{"role":"assistant","content":"今晚打老虎"}]}`},
		{ContentModerationProtocolOpenAIResponses, `{"input":[{"role":"user","content":"old"},{"type":"function_call_output","output":"今晚打老虎"}]}`},
		{ContentModerationProtocolOpenAIResponses, `{"input":[{"role":"user","content":"old"},{"role":"assistant","content":"今晚打老虎"}]}`},
		{ContentModerationProtocolGemini, `{"contents":[{"role":"user","parts":[{"text":"old"}]},{"role":"user","parts":[{"functionResponse":{"response":{"text":"今晚打老虎"}}}]}]}`},
		{ContentModerationProtocolGemini, `{"contents":[{"role":"user","parts":[{"text":"old"}]},{"role":"model","parts":[{"text":"今晚打老虎"}]}]}`},
	} {
		t.Run(tc.protocol+tc.body, func(t *testing.T) {
			require.Empty(t, extractContentModerationKeywordText(tc.protocol, []byte(tc.body)))
		})
	}
}

func TestContentModerationCheck_ReminderPolicyPreserved(t *testing.T) {
	for _, name := range []string{"keyword_miss", "combined_miss", "observe", "off", "disabled", "group_scope", "model_scope", "no_api_key", "zero_sample"} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var payload struct {
					Input string `json:"input"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload.Input != "safe text" {
					t.Errorf("unexpected semantic input: %q", payload.Input)
				}
				_ = json.NewEncoder(w).Encode(moderationAPIResponse{Results: []moderationAPIResult{{}}})
			}))
			defer server.Close()
			cfg := defaultContentModerationConfig()
			cfg.Enabled = true
			cfg.Mode = ContentModerationModePreBlock
			cfg.BlockedKeywords = []string{"今晚打老虎"}
			cfg.KeywordBlockingMode = ContentModerationKeywordModeKeywordAndAPI
			cfg.BaseURL = server.URL
			cfg.APIKeys = []string{"test"}
			cfg.RecordNonHits = false
			cfg.PreHashCheckEnabled = false
			switch name {
			case "keyword_miss":
				cfg.KeywordBlockingMode = ContentModerationKeywordModeKeywordOnly
				cfg.BlockedKeywords = []string{"absent"}
			case "combined_miss":
				cfg.BlockedKeywords = []string{"absent"}
			case "observe":
				cfg.Mode = ContentModerationModeObserve
			case "off":
				cfg.Mode = ContentModerationModeOff
			case "disabled":
				cfg.Enabled = false
			case "group_scope":
				cfg.AllGroups = false
				cfg.GroupIDs = []int64{42}
			case "model_scope":
				cfg.ModelFilter = ContentModerationModelFilter{Type: ContentModerationModelFilterInclude, Models: []string{"other-model"}}
			case "no_api_key":
				cfg.APIKeys = nil
			case "zero_sample":
				cfg.SampleRate = 0
			}
			raw, err := json.Marshal(cfg)
			require.NoError(t, err)
			repo := &contentModerationTestRepo{}
			svc := NewContentModerationService(&contentModerationTestSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true", SettingKeyContentModerationConfig: string(raw)}}, repo, nil, nil, nil, nil, nil, nil)
			body := reminderTestBody(t, ContentModerationProtocolAnthropicMessages, []string{"<system-reminder>今晚打老虎</system-reminder>", "safe text"})
			decision, err := svc.Check(context.Background(), ContentModerationCheckInput{Protocol: ContentModerationProtocolAnthropicMessages, Body: body})
			require.NoError(t, err)
			if name == "no_api_key" || name == "zero_sample" {
				require.True(t, decision.Blocked)
				requireContentModerationLogCount(t, repo, 1)
			} else {
				require.True(t, decision.Allowed)
			}
			switch name {
			case "observe":
				require.Eventually(t, func() bool { return calls.Load() == 1 }, time.Second, time.Millisecond*10)
			case "combined_miss":
				require.Equal(t, int32(1), calls.Load())
			default:
				require.Zero(t, calls.Load())
			}
		})
	}
}

func TestExtractContentModerationKeywordText_ResponsesInputForms(t *testing.T) {
	for _, body := range []string{
		`{"input":"<system-reminder>今晚打老虎</system-reminder>"}`,
		`{"input":{"role":"user","content":"<system-reminder>今晚打老虎</system-reminder>"}}`,
		`{"input":[{"type":"input_text","text":"<system-reminder>今晚打老虎</system-reminder>"}]}`,
	} {
		require.Contains(t, extractContentModerationKeywordText(ContentModerationProtocolOpenAIResponses, []byte(body)), "今晚打老虎")
		require.Empty(t, ExtractContentModerationText(ContentModerationProtocolOpenAIResponses, []byte(body)))
	}
}

func TestExtractContentModerationKeywordText_LongReminder(t *testing.T) {
	// Reminder context must not consume the semantic API's text limit and hide
	// the actual user keyword from the local check.
	body := reminderTestBody(t, ContentModerationProtocolAnthropicMessages, []string{
		"<system-reminder>" + strings.Repeat("x", maxModerationInputRunes) + "</system-reminder>", "今晚打老虎",
	})
	require.Contains(t, extractContentModerationKeywordText(ContentModerationProtocolAnthropicMessages, body), "今晚打老虎")
	require.Equal(t, "今晚打老虎", ExtractContentModerationText(ContentModerationProtocolAnthropicMessages, body))
}
