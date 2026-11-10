package service

import (
	"bytes"
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// codexToolOutputHistoryBody builds a Codex-style request whose history is
// mostly custom tool outputs carrying ctco_ IDs, which the sanitizer strips.
// The outputs are multi-line text with escaped quotes, as tool output is, and
// the stripped ID comes before the output in half of the items and after it
// in the other half.
func codexToolOutputHistoryBody(items, outputBytes int) []byte {
	body := []byte(`{"model":"gpt-5.6-codex","stream":true,"prompt_cache_key":"client-session","client_metadata":{"session_id":"client-session","thread_id":"client-thread"},"input":[`)
	line := `src/main.go:12: \"value\" changed\n`
	output := strings.Repeat(line, outputBytes/len(line))
	for i := 0; i < items; i++ {
		if i > 0 {
			body = append(body, ',')
		}
		body = fmt.Appendf(body, `{"type":"custom_tool_call","id":"ctc_%d","call_id":"call_%d","name":"apply_patch","input":"patch"},`, i, i)
		if i%2 == 0 {
			body = fmt.Appendf(body, `{"type":"custom_tool_call_output","id":"ctco_%d","call_id":"call_%d","output":"%s"}`, i, i, output)
		} else {
			body = fmt.Appendf(body, `{"type":"custom_tool_call_output","call_id":"call_%d","output":"%s","id":"ctco_%d"}`, i, output, i)
		}
	}
	return append(body, `]}`...)
}

func codexIdentityTestAccount() *Account {
	return &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"chatgpt_account_id": "chatgpt-account-11"}}
}

func BenchmarkSanitizeOpenAIResponsesInputItemIDsCodexHistory(b *testing.B) {
	body := codexToolOutputHistoryBody(40, 64<<10)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)
		if err != nil || !changed || len(out) >= len(body) {
			b.Fatalf("tool output IDs were not stripped: changed=%v err=%v", changed, err)
		}
		runtime.KeepAlive(out)
	}
}

func BenchmarkApplyCodexAccountIdentityClientMetadataRaw(b *testing.B) {
	body := codexToolOutputHistoryBody(40, 64<<10)
	account := codexIdentityTestAccount()
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, changed, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 77)
		if err != nil || !changed || bytes.Equal(out, body) {
			b.Fatalf("identity was not scoped: changed=%v err=%v", changed, err)
		}
		runtime.KeepAlive(out)
	}
}
