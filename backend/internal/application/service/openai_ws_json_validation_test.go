package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPrepareOpenAIWSJSONDocumentsPreservesValidationBoundaries(t *testing.T) {
	terminal := `{"type":"response.completed","response":{"id":"resp_validation","usage":{"input_tokens":7,"output_tokens":9}}}`
	large := `{"type":"response.completed","response":{"id":"resp_large","padding":"` + strings.Repeat("x", maxOpenAIConcatenatedJSONBytes) + `","usage":{"input_tokens":7,"output_tokens":9}}}`
	for _, tc := range []struct {
		name, payload string
		valid, repair bool
	}{
		{"single", terminal, true, false},
		{"outer whitespace", " \n" + terminal + "\t", true, false},
		{"non JSON whitespace", terminal + "\f", false, false},
		{"non JSON whitespace before scalar", "\v0", false, false},
		{"non JSON unicode whitespace", "\u00a0" + terminal + "\u2003", false, false},
		{"fuzz non JSON trailing whitespace", "0  \f", false, false},
		{"concatenated", terminal + terminal, true, true},
		{"concatenated preserves repair whitespace", "\f" + terminal + terminal, true, true},
		{"invalid tail", terminal + "bad", false, false},
		{"empty", "", false, false},
		{"valid above repair limit", large, true, false},
		{"malformed above repair limit", large + "bad", false, false},
		{"too many documents", strings.Repeat(terminal, maxOpenAIConcatenatedJSONDocuments+1), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			documents, repaired, valid := prepareOpenAIWSJSONDocuments([]byte(tc.payload))
			require.Equal(t, tc.valid, valid)
			require.Equal(t, tc.repair, repaired)
			for _, document := range documents {
				require.True(t, json.Valid(document))
			}
		})
	}

	// Exercise the actual loop's oversized fallback for both client formats.
	for _, stream := range []bool{false, true} {
		svc, account := newOpenAIWSHeaderFixture(t)
		conn := &openAIWSCaptureConn{events: [][]byte{[]byte(large)}}
		svc.getOpenAIWSConnPool().setClientDialerForTest(&openAIWSCaptureDialer{conn: conn})
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		result, err := svc.forwardOpenAIWSV2(context.Background(), c, account, map[string]any{"model": "test"}, "token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, false, stream, "test", "test", time.Now(), 1, "", nil, nil, false)
		require.NoError(t, err)
		require.Equal(t, 7, result.Usage.InputTokens)
		require.Equal(t, 9, result.Usage.OutputTokens)
		require.False(t, conn.closed, "valid large terminal frames keep a clean connection")
	}
}

var benchmarkOpenAIWSJSONValid bool

func BenchmarkOpenAIWSJSONValidation(b *testing.B) {
	for _, size := range []int{1024, 256 << 10, 1 << 20} {
		payload := []byte(`{"type":"response.output_text.delta","delta":"` + strings.Repeat("x", size) + `"}`)
		for _, reuse := range []bool{false, true} {
			b.Run(fmt.Sprintf("%dKiB/reuse_%v", size>>10, reuse), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(payload)))
				for range b.N {
					if reuse {
						_, _, benchmarkOpenAIWSJSONValid = prepareOpenAIWSJSONDocuments(payload)
					} else {
						_, _ = splitOpenAIConcatenatedJSONDocuments(payload)
						benchmarkOpenAIWSJSONValid = json.Valid(payload)
					}
				}
			})
		}
	}
}

func FuzzOpenAIWSJSONValidation(f *testing.F) {
	for _, seed := range []string{`{"type":"response.created"}`, `{"type":"a"}{"type":"b"}`, `{"type":"x"}bad`, ``, `null`, `[]`, "0  \f", "\v0", "\u00a0{}\u2003"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 64<<10 {
			return
		}
		body := []byte(raw)
		wantDocs, wantRepaired := splitOpenAIConcatenatedJSONDocuments(body)
		wantValid := wantRepaired || json.Valid(body)
		documents, repaired, valid := prepareOpenAIWSJSONDocuments(body)
		require.Equal(t, wantRepaired, repaired)
		require.Equal(t, wantDocs, documents)
		require.Equal(t, wantValid, valid)
	})
}
