package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// sanitizeOpenAIResponsesInputItemIDsWithSJSON is the previous implementation,
// kept as the reference: it deletes the IDs from a copy of each item with
// sjson and joins the rebuilt items.
func sanitizeOpenAIResponsesInputItemIDsWithSJSON(body []byte) ([]byte, bool, error) {
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return body, false, nil
	}
	var items []string
	changed := false
	var err error
	input.ForEach(func(_, item gjson.Result) bool {
		stripID, stripCallID := openAIResponsesInputItemSanitization(item)
		if !stripID && !stripCallID {
			items = append(items, item.Raw)
			return true
		}
		changed = true
		itemBody := []byte(item.Raw)
		if stripID {
			if itemBody, err = sjson.DeleteBytes(itemBody, "id"); err != nil {
				return false
			}
		}
		if stripCallID {
			if itemBody, err = sjson.DeleteBytes(itemBody, "call_id"); err != nil {
				return false
			}
		}
		items = append(items, string(itemBody))
		return true
	})
	if err != nil || !changed {
		return body, false, err
	}
	return replaceOpenAIRawInput(body, input, items), true, nil
}

func requireSameJSONIgnoringWhitespace(t testing.TB, want, got []byte) {
	t.Helper()
	var wantCompact, gotCompact bytes.Buffer
	require.NoError(t, json.Compact(&wantCompact, want))
	require.NoError(t, json.Compact(&gotCompact, got))
	require.Equal(t, wantCompact.String(), gotCompact.String())
}

var sanitizeInputItemIDCases = []string{
	`{"model":"gpt-5","input":[{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_1","output":"done"}],"stream":true}`,
	`{"input":[{"id":"ctco_1","type":"custom_tool_call_output","call_id":"call_1","output":"done"}]}`,
	`{"input":[{"type":"custom_tool_call_output","call_id":"call_1","output":"done","id":"ctco_1"}]}`,
	`{"input":[{"id":"bad"}]}`,
	`{"input":[{"type":"message","id":"bad","call_id":"c","content":"hi"}]}`,
	`{"input":[{"type":"message","call_id":"c","id":"bad"}]}`,
	`{"input":[{"call_id":"c","id":"bad","type":"message"}]}`,
	`{"input":[{"type":"message","id":"bad","id":"msg_ok","content":"dup"}]}`,
	`{"input":[{"type":"message","id":"msg_ok","id":"bad"}]}`,
	`{"input":[{"type":"message","id":7,"call_id":"c"}]}`,
	`{"input":[{"type":"message","content":[{"id":"nested","type":"input_text","text":"keep"}],"id":"bad"}]}`,
	`{"input":[{"type":"message","\u0069d":"bad","content":"escaped key"}]}`,
	`{"input":["text",42,null,[1,2],{"type":"reasoning","id":"bad","summary":[]},true]}`,
	`{"input":[{"type":"message","id":"msg_ok"},{"type":"reasoning","id":"rs_ok","call_id":"c"}],"tail":{"id":"bad"}}`,
	`{"input":[{"type":"message","id":"bad","content":"\u00e9\"\\\n"}],"opaque":9007199254740993}`,
	`{"input":[{"type":"custom_tool_call_output","call_id":"c","output":"a\"b\\c\nd","id":"ctco_1"}]}`,
	`{"input":[{"type":"message","content":[{"type":"input_text","text":"} ] { [ \" ,"}],"id":"bad","n":-1.5e3,"t":true,"z":null}]}`,
	`{"input":[{"type":"message", "n" : 12 , "t" : false , "id" : "bad" , "z" : null }]}`,
	`{"input":[]}`,
	`{"input":{"type":"message","id":"bad"}}`,
	`{"input":"bad"}`,
	`{"input":[ {"type" : "message" , "id" : "bad" , "content" : "spaced"} ,
		{"type":"custom_tool_call_output", "call_id":"c","id":"ctco_2"}
	] }`,
	" \n\t" + `{"input":[{"type":"message","id":"bad"}]}`,
}

func TestSanitizeOpenAIResponsesInputItemIDsMatchesSJSONDeletion(t *testing.T) {
	for i, raw := range sanitizeInputItemIDCases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			body := []byte(raw)
			want, wantChanged, wantErr := sanitizeOpenAIResponsesInputItemIDsWithSJSON(body)
			require.NoError(t, wantErr)
			got, gotChanged, err := sanitizeOpenAIResponsesInputItemIDs(body)
			require.NoError(t, err)
			require.Equal(t, wantChanged, gotChanged)
			if !strings.ContainsAny(raw, " \n\t") {
				// Compact bodies, which is what clients send, are byte-identical.
				require.Equal(t, string(want), string(got))
				return
			}
			requireSameJSONIgnoringWhitespace(t, want, got)
		})
	}
}

func FuzzSanitizeOpenAIResponsesInputItemIDsMatchesSJSONDeletion(f *testing.F) {
	for _, raw := range sanitizeInputItemIDCases {
		f.Add([]byte(raw))
	}
	f.Fuzz(func(t *testing.T, body []byte) {
		if !gjson.ValidBytes(body) {
			t.Skip()
		}
		want, wantChanged, wantErr := sanitizeOpenAIResponsesInputItemIDsWithSJSON(body)
		got, gotChanged, err := sanitizeOpenAIResponsesInputItemIDs(body)
		if wantErr != nil {
			return
		}
		require.NoError(t, err)
		require.Equal(t, wantChanged, gotChanged)
		if gotChanged {
			requireSameJSONIgnoringWhitespace(t, want, got)
		}
	})
}

func TestSanitizeOpenAIResponsesInputItemIDsDoesNotModifyInput(t *testing.T) {
	body := []byte(`{"input":[{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_1","output":"done"},{"type":"message","id":"bad","call_id":"c"}]}`)
	original := bytes.Clone(body)

	out, changed, err := sanitizeOpenAIResponsesInputItemIDs(body)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, original, body)
	require.False(t, gjson.GetBytes(out, "input.0.id").Exists())
	require.False(t, gjson.GetBytes(out, "input.1.call_id").Exists())
}

func TestSanitizeOpenAIResponsesInputItemIDsRejectsTruncatedItem(t *testing.T) {
	// Unreachable for validated bodies; a partly parsed item must not be
	// rewritten into a valid-looking object that silently lost members.
	body := []byte(`{"input":[{"type":"message","id":"bad","content":"x",}]}`)

	_, _, err := sanitizeOpenAIResponsesInputItemIDs(body)

	require.Error(t, err)
}

// minAllocatedBytes returns the fewest bytes fn allocated over a few runs, so
// that allocations by unrelated goroutines do not make the check flaky.
func minAllocatedBytes(fn func()) uint64 {
	var best uint64
	for i := 0; i < 5; i++ {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		fn()
		runtime.ReadMemStats(&after)
		if allocated := after.TotalAlloc - before.TotalAlloc; i == 0 || allocated < best {
			best = allocated
		}
	}
	return best
}

func TestSanitizeOpenAIResponsesInputItemIDsCopiesBodyOnce(t *testing.T) {
	body := codexToolOutputHistoryBody(32, 64<<10)
	var out []byte
	allocated := minAllocatedBytes(func() {
		var changed bool
		var err error
		out, changed, err = sanitizeOpenAIResponsesInputItemIDs(body)
		require.NoError(t, err)
		require.True(t, changed)
	})
	require.False(t, gjson.GetBytes(out, "input.1.id").Exists())
	require.LessOrEqual(t, allocated, uint64(len(body))*5/4, "allocated %d bytes for a %d byte body", allocated, len(body))
}

// applyCodexAccountIdentityClientMetadataRawWithSJSON is the previous
// implementation, kept as the reference.
func applyCodexAccountIdentityClientMetadataRawWithSJSON(body []byte, account *Account, apiKeyID int64) ([]byte, bool, error) {
	if len(body) == 0 || codexAccountIdentityNamespace(account) == "" {
		return body, false, nil
	}
	if !gjson.ParseBytes(body).IsObject() {
		return body, false, nil
	}
	next := body
	changed := false
	originalBodySessionID := ""
	if cm := gjson.GetBytes(body, "client_metadata"); cm.IsObject() {
		clientMetadata := map[string]any{}
		if err := json.Unmarshal([]byte(cm.Raw), &clientMetadata); err != nil {
			return body, false, err
		}
		originalBodySessionID, _ = clientMetadata["session_id"].(string)
		metadataChanged := applyCodexAccountIdentityFields(clientMetadata, account, apiKeyID)
		if applyCodexAccountIdentityEmbeddedMetadata(clientMetadata, account, apiKeyID) {
			metadataChanged = true
		}
		if metadataChanged {
			raw, err := json.Marshal(clientMetadata)
			if err != nil {
				return body, false, err
			}
			if next, err = sjson.SetRawBytes(next, "client_metadata", raw); err != nil {
				return body, false, err
			}
			changed = true
		}
	}
	if promptCacheKey := gjson.GetBytes(body, "prompt_cache_key"); promptCacheKey.Type == gjson.String && strings.TrimSpace(promptCacheKey.String()) != "" {
		raw := promptCacheKey.String()
		kind := "prompt-cache"
		if strings.TrimSpace(originalBodySessionID) != "" && raw == originalBodySessionID {
			kind = "session"
		}
		if scoped := scopeCodexAccountIdentityValue(account, apiKeyID, kind, raw); scoped != raw {
			var err error
			if next, err = sjson.SetBytes(next, "prompt_cache_key", scoped); err != nil {
				return body, false, err
			}
			changed = true
		}
	}
	return next, changed, nil
}

func TestApplyCodexAccountIdentityClientMetadataRawMatchesSJSON(t *testing.T) {
	cases := []string{
		`{"model":"gpt-5","prompt_cache_key":"client-session","client_metadata":{"session_id":"client-session","thread_id":"t"},"input":[]}`,
		`{"client_metadata":{"session_id":"s","x-codex-installation-id":"i"},"input":[],"prompt_cache_key":"other-key"}`,
		`{"model":"gpt-5","client_metadata":{"session_id":"s"}}`,
		`{"model":"gpt-5","prompt_cache_key":"only-key"}`,
		`{"model":"gpt-5","prompt_cache_key":"<k&é\"\\>","client_metadata":{"session_id":"<k&é\"\\>"}}`,
		`{"model":"gpt-5","prompt_cache_key":"   ","client_metadata":{"unrelated":"x"}}`,
		`{"model":"gpt-5","prompt_cache_key":"dup-1","prompt_cache_key":"dup-2","client_metadata":{"session_id":"dup-1"}}`,
		`{ "prompt_cache_key" : "spaced" , "client_metadata" : { "session_id" : "spaced" } }`,
		`{"model":"gpt-5","client_metadata":"not-an-object","prompt_cache_key":7}`,
		`[{"prompt_cache_key":"not-an-object-body"}]`,
		string(codexToolOutputHistoryBody(2, 32)),
	}
	account := codexIdentityTestAccount()
	for i, raw := range cases {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			body := []byte(raw)
			original := bytes.Clone(body)
			want, wantChanged, wantErr := applyCodexAccountIdentityClientMetadataRawWithSJSON(body, account, 77)
			require.NoError(t, wantErr)
			got, gotChanged, err := applyCodexAccountIdentityClientMetadataRaw(body, account, 77)
			require.NoError(t, err)
			require.Equal(t, wantChanged, gotChanged)
			require.Equal(t, string(want), string(got))
			require.Equal(t, original, body)
		})
	}
}

func TestApplyCodexAccountIdentityClientMetadataRawCopiesBodyOnce(t *testing.T) {
	body := codexToolOutputHistoryBody(32, 64<<10)
	account := codexIdentityTestAccount()
	var out []byte
	allocated := minAllocatedBytes(func() {
		var changed bool
		var err error
		out, changed, err = applyCodexAccountIdentityClientMetadataRaw(body, account, 77)
		require.NoError(t, err)
		require.True(t, changed)
	})
	require.NotEqual(t, "client-session", gjson.GetBytes(out, "prompt_cache_key").String())
	require.NotEqual(t, "client-thread", gjson.GetBytes(out, "client_metadata.thread_id").String())
	require.LessOrEqual(t, allocated, uint64(len(body))*5/4, "allocated %d bytes for a %d byte body", allocated, len(body))
}
