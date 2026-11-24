package service

import (
	"bytes"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestGatewayRequestBodyReuse_RestoresDerivedStateAndIsolatesAttempts(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4-5","model":"ignored","stream":true,"metadata":{"user_id":"session"},"system":null,"messages":[{"role":"user","content":"first"}],"messages":[],"thinking":{"type":"enabled"},"output_config":{"effort":"high"},"speed":"fast","max_tokens":123,"unknown":{"number":1.00}}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
	require.NoError(t, err)
	want := *parsed.Body.parsed
	groupID := int64(17)
	parsed.GroupID = &groupID
	parsed.SessionContext = &SessionContext{APIKeyID: 9}
	callbackCalled := false
	parsed.OnUpstreamAccepted = func() { callbackCalled = true }

	// Exported fields are mutable routing inputs, not the authoritative parse.
	clearGatewayRequestDerivedState(parsed)
	parsed.Model = "temporary-routing-override"
	first, err := parsed.CloneForBody(body)
	require.NoError(t, err)
	require.NotSame(t, parsed.Body, first.Body)
	require.Nil(t, first.OnUpstreamAccepted)
	require.Same(t, parsed.SessionContext, first.SessionContext)
	require.Same(t, parsed.GroupID, first.GroupID)
	require.Equal(t, "claude-sonnet-4-5", first.Model)
	require.Equal(t, `[{"role":"user","content":"first"}]`, string(first.MessagesRaw()))
	require.Equal(t, body, first.Body.Bytes())
	first.saveBodyState()
	require.Equal(t, want, *first.Body.parsed)

	require.NoError(t, parsed.ReplaceBody(body))
	parsed.saveBodyState()
	require.Equal(t, want, *parsed.Body.parsed)
	parsed.OnUpstreamAccepted()
	require.True(t, callbackCalled)
	second, err := parsed.CloneForBody(body)
	require.NoError(t, err)
	require.NoError(t, first.ReplaceBody([]byte(`{"model":"other","messages":[{"content":"replacement"}]}`)))
	require.Equal(t, "other", first.Model)
	require.Equal(t, "claude-sonnet-4-5", parsed.Model)
	require.Equal(t, body, parsed.Body.Bytes())
	require.Equal(t, body, second.Body.Bytes())
	require.Equal(t, string(parsed.MessagesRaw()), string(second.MessagesRaw()))
}

func TestGatewayRequestBodyReuse_InvalidatesReplacedAndUnparsedBodies(t *testing.T) {
	for _, mode := range []string{"manual", "replace-ref", "replace-storage", "mutate-in-place", "protocol"} {
		t.Run(mode, func(t *testing.T) {
			body := []byte(`{"model":"old","system":"system","messages":[1],"contents":[2]}`)
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
			require.NoError(t, err)
			switch mode {
			case "manual":
				parsed = &ParsedRequest{Body: NewRequestBodyRef(body), Model: "unparsed", protocol: domain.PlatformAnthropic}
			case "replace-ref":
				parsed.Body = NewRequestBodyRef(body)
			case "replace-storage":
				// Release the previous immutable view before reusing its storage.
				parsed.Body.Replace(nil)
				copy(body, bytes.Replace(body, []byte(`"old"`), []byte(`"new"`), 1))
				parsed.Body.Replace(body)
			case "mutate-in-place":
				// Violates the borrowing contract; the fingerprint still forces a reparse.
				copy(body, bytes.Replace(body, []byte(`"old"`), []byte(`"new"`), 1))
			case "protocol":
				parsed.protocol = domain.PlatformGemini
			}
			clone, err := parsed.CloneForBody(body)
			require.NoError(t, err)
			require.NoError(t, parsed.ReplaceBody(body))
			for _, actual := range []*ParsedRequest{parsed, clone} {
				if mode == "replace-storage" || mode == "mutate-in-place" {
					require.Equal(t, "new", actual.Model)
				} else {
					require.Equal(t, "old", actual.Model)
				}
				if mode == "protocol" {
					require.Equal(t, "[2]", string(actual.MessagesRaw()))
					require.Nil(t, actual.SystemRaw())
				} else {
					require.Equal(t, "[1]", string(actual.MessagesRaw()))
				}
			}
		})
	}
}

func TestGatewayRequestBodyReuse_ExplicitReplaceSameSliceInvalidates(t *testing.T) {
	body := []byte(`{"model":"old"}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), "")
	require.NoError(t, err)
	parsed.Body.Replace(body)
	require.Nil(t, parsed.Body.parsed)
	require.NoError(t, parsed.ReplaceBody(body))
	require.NotNil(t, parsed.Body.parsed)
}

func TestGatewayRequestBodyReuse_FailedParseCannotBeReused(t *testing.T) {
	for _, invalid := range []string{`{"messages":`, `{"model":123}`, `{"stream":"true"}`, `{"model":"claude-sonnet-4[1m]","stream":0}`} {
		t.Run(invalid, func(t *testing.T) {
			parsed, err := ParseGatewayRequest(NewRequestBodyRef([]byte(`{"model":"valid","system":"old","messages":[1]}`)), domain.PlatformAnthropic)
			require.NoError(t, err)
			require.Error(t, parsed.ReplaceBody([]byte(invalid)))
			require.Nil(t, parsed.Body.parsed)
			require.Nil(t, parsed.SystemRaw())
			require.Nil(t, parsed.MessagesRaw())
			_, err = parsed.CloneForBody(parsed.Body.Bytes())
			require.Error(t, err)
			require.Error(t, parsed.ReplaceBody(parsed.Body.Bytes()))
			require.Nil(t, parsed.Body.parsed)
			require.NoError(t, parsed.ReplaceBody([]byte(`{"model":"recovered","messages":[2]}`)))
			require.Equal(t, "recovered", parsed.Model)
			require.Equal(t, "[2]", string(parsed.MessagesRaw()))
		})
	}
}

func TestGatewayRequestBodyReuse_NormalizedBodyAndProtocolRanges(t *testing.T) {
	for _, tt := range []struct{ protocol, body string }{
		{domain.PlatformAnthropic, `{"model":"claude-sonnet-4[1m]","system":"sys","messages":[{"content":"hello"}]}`},
		{domain.PlatformGemini, `{"model":"gemini","systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"parts":[{"text":"hi"}]}]}`},
		{"responses", `{"model":"gpt-5.4","input":[{"type":"message","content":"hi"}]}`},
	} {
		t.Run(tt.protocol, func(t *testing.T) {
			original := []byte(tt.body)
			parsed, err := ParseGatewayRequest(NewRequestBodyRef(original), tt.protocol)
			require.NoError(t, err)
			for _, body := range [][]byte{original, parsed.Body.Bytes(), bytes.Clone(parsed.Body.Bytes())} {
				clone, err := parsed.CloneForBody(body)
				require.NoError(t, err)
				require.NoError(t, clone.ReplaceBody(clone.Body.Bytes()))
				require.Equal(t, parsed.Model, clone.Model)
				require.Equal(t, parsed.Body.Bytes(), clone.Body.Bytes())
				require.Equal(t, parsed.SystemRaw(), clone.SystemRaw())
				require.Equal(t, parsed.MessagesRaw(), clone.MessagesRaw())
				require.Equal(t, parsed.InputRaw(), clone.InputRaw())
			}
		})
	}
}

func TestGatewayRequestBodyReuse_ConcurrentIndependentAttempts(t *testing.T) {
	body := []byte(`{"model":"original","messages":[{"content":"hello"}]}`)
	parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			clone, err := parsed.CloneForBody(body)
			if err != nil {
				t.Error(err)
				return
			}
			if err := clone.ReplaceBody([]byte(`{"model":"attempt","messages":[]}`)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	require.Equal(t, "original", parsed.Model)
	require.Equal(t, body, parsed.Body.Bytes())
}

func BenchmarkGatewayRequestBodyReuse(b *testing.B) {
	for _, size := range []struct {
		name  string
		bytes int
	}{{"4KiB", 4 << 10}, {"1MiB", 1 << 20}, {"8MiB", 8 << 20}} {
		body := buildLargeAnthropicMessagesBody(size.bytes, false)
		parsed, err := ParseGatewayRequest(NewRequestBodyRef(body), domain.PlatformAnthropic)
		if err != nil {
			b.Fatal(err)
		}
		for _, reuse := range []bool{false, true} {
			name := "Reparse"
			if reuse {
				name = "Reuse"
			}
			b.Run(size.name+"/"+name, func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(body)))
				for i := 0; i < b.N; i++ {
					if !reuse {
						parsed.Body.Replace(body)
					}
					attempt, err := parsed.CloneForBody(body)
					if err != nil {
						b.Fatal(err)
					}
					// Unchanged compatibility and cache-control phases.
					for range 2 {
						if !reuse {
							attempt.Body.Replace(body)
						}
						if err := attempt.ReplaceBody(body); err != nil {
							b.Fatal(err)
						}
					}
					benchmarkIntSink = len(attempt.MessagesRaw())
				}
			})
			// Reparse deliberately invalidates the source snapshot.
			if err := parsed.ReplaceBody(body); err != nil {
				b.Fatal(err)
			}
		}
	}
}
