package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newOpenAIImageIntentHintTestContext(transport OpenAIClientTransport) *gin.Context {
	c := &gin.Context{}
	SetOpenAIClientTransport(c, transport)
	return c
}

func countingOpenAIImageIntentClassifier(calls *atomic.Int64) openAIImageIntentClassifier {
	return func(endpoint string, requestedModel string, body []byte) bool {
		calls.Add(1)
		return IsImageGenerationIntent(endpoint, requestedModel, body)
	}
}

func TestResolveOpenAIImageIntentHintCachesTrueAndFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body []byte
		want bool
	}{
		{name: "true", body: []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"}]}`), want: true},
		{name: "false is known", body: []byte(`{"model":"gpt-5.4","input":"write code"}`), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
			var calls atomic.Int64
			classify := countingOpenAIImageIntentClassifier(&calls)

			require.Equal(t, tt.want, resolveOpenAIImageIntentHint(c, "gpt-5.4", tt.body, classify))
			require.Equal(t, tt.want, resolveOpenAIImageIntentHint(c, "gpt-5.4", tt.body, classify))
			require.Equal(t, int64(1), calls.Load())
			cached, known := getOpenAIImageIntentHint(c)
			require.True(t, known)
			require.Equal(t, tt.want, cached)
		})
	}
}

func TestResolveOpenAIImageIntentHintUsesHandlerSeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, seeded := range []bool{false, true} {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		SetOpenAIImageIntentHint(c, seeded)
		var calls atomic.Int64

		got := resolveOpenAIImageIntentHint(c, "gpt-5.4", []byte(`{"model":"gpt-5.4"}`), countingOpenAIImageIntentClassifier(&calls))

		require.Equal(t, seeded, got)
		require.Zero(t, calls.Load())
	}
}

func TestResolveOpenAIPassthroughImageIntentReusesCanonicalAcrossFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
	body := []byte(`{"model":"gpt-5.4","input":"write code"}`)
	var calls atomic.Int64
	classify := countingOpenAIImageIntentClassifier(&calls)

	for range 3 {
		require.False(t, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", body, "gpt-5.4", body, false, classify))
	}
	require.Equal(t, int64(1), calls.Load())
}

func TestResolveOpenAIPassthroughImageIntentKeepsCompactMappingAttemptLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("text to image", func(t *testing.T) {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		body := []byte(`{"model":"draw-alias","input":"draw"}`)
		compactBody := []byte(`{"model":"gpt-image-2","input":"draw"}`)
		var calls atomic.Int64
		classify := countingOpenAIImageIntentClassifier(&calls)

		require.True(t, resolveOpenAIPassthroughImageIntent(c, "draw-alias", body, "gpt-image-2", compactBody, true, classify))
		cached, known := getOpenAIImageIntentHint(c)
		require.True(t, known)
		require.False(t, cached)

		require.False(t, resolveOpenAIPassthroughImageIntent(c, "draw-alias", body, "draw-alias", body, false, classify))
		require.Equal(t, int64(2), calls.Load())
		cached, known = getOpenAIImageIntentHint(c)
		require.True(t, known)
		require.False(t, cached)
	})

	t.Run("image to text", func(t *testing.T) {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		body := []byte(`{"model":"gpt-image-2","input":"draw"}`)
		compactBody := []byte(`{"model":"gpt-5.4","input":"draw"}`)
		var calls atomic.Int64
		classify := countingOpenAIImageIntentClassifier(&calls)

		require.False(t, resolveOpenAIPassthroughImageIntent(c, "gpt-image-2", body, "gpt-5.4", compactBody, true, classify))
		cached, known := getOpenAIImageIntentHint(c)
		require.True(t, known)
		require.True(t, cached)

		require.True(t, resolveOpenAIPassthroughImageIntent(c, "gpt-image-2", body, "gpt-image-2", body, false, classify))
		require.Equal(t, int64(2), calls.Load())
	})
}

func TestResolveOpenAIPassthroughImageIntentInvalidationDoesNotPolluteCanonical(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
	canonicalBody := []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"}]}`)
	strippedBody := []byte(`{"model":"gpt-5.4","tools":[]}`)
	var calls atomic.Int64
	classify := countingOpenAIImageIntentClassifier(&calls)

	require.False(t, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", canonicalBody, "gpt-5.4", strippedBody, true, classify))
	require.Equal(t, int64(2), calls.Load(), "unknown canonical and invalidated attempt are classified independently")
	cached, known := getOpenAIImageIntentHint(c)
	require.True(t, known)
	require.True(t, cached)

	require.True(t, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", canonicalBody, "gpt-5.4", canonicalBody, false, classify))
	require.Equal(t, int64(2), calls.Load())
}

func TestResolveOpenAIPassthroughImageIntentMappedBodyStartsUnknownThenSeedsCanonical(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
	canonicalBody := []byte(`{"model":"gpt-image-2","input":"draw"}`)
	strippedAttemptBody := []byte(`{"model":"gpt-5.4","input":"draw"}`)
	_, known := getOpenAIImageIntentHint(c)
	require.False(t, known)
	var calls atomic.Int64
	classify := countingOpenAIImageIntentClassifier(&calls)

	require.False(t, resolveOpenAIPassthroughImageIntent(c, "gpt-image-2", canonicalBody, "gpt-5.4", strippedAttemptBody, true, classify))
	require.Equal(t, int64(2), calls.Load())
	cached, known := getOpenAIImageIntentHint(c)
	require.True(t, known)
	require.True(t, cached)
}

func TestResolveOpenAIPassthroughImageIntentReusesAcrossInvariantMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		canonicalBody []byte
		attemptBody   []byte
		want          bool
	}{
		{
			name:          "oauth sanitize fast policy and reasoning",
			canonicalBody: []byte(`{"model":"gpt-5.4","input":[{"type":"input_image","image_url":"data:image/png;base64,"}],"service_tier":"fast","reasoning":{"effort":"minimal"}}`),
			attemptBody:   []byte(`{"model":"gpt-5.4","input":[],"service_tier":"priority","reasoning":{"effort":"none"},"store":false,"stream":true}`),
			want:          false,
		},
		{
			name:          "namespace flatten",
			canonicalBody: []byte(`{"model":"gpt-5.4","tools":[{"type":"namespace","name":"code_tools"}]}`),
			attemptBody:   []byte(`{"model":"gpt-5.4","tools":[{"type":"function","name":"code_tools.run"}]}`),
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
			var calls atomic.Int64
			classify := countingOpenAIImageIntentClassifier(&calls)

			require.Equal(t, tt.want, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", tt.canonicalBody, "gpt-5.4", tt.attemptBody, false, classify))
			require.Equal(t, tt.want, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", tt.canonicalBody, "gpt-5.4", tt.attemptBody, false, classify))
			require.Equal(t, int64(1), calls.Load())
		})
	}
}

func TestOpenAIGatewayServicePassthroughCompactImageIntentIsAttemptLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		canonicalModel string
		compactModel   string
		wantRejected   bool
		wantCanonical  bool
	}{
		{
			name:           "text to image rejects",
			canonicalModel: "gpt-5.4",
			compactModel:   "gpt-image-2",
			wantRejected:   true,
		},
		{
			name:           "image to text reaches upstream",
			canonicalModel: "gpt-image-2",
			compactModel:   "gpt-5.4",
			wantCanonical:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","model":"` + tt.compactModel + `","usage":{"input_tokens":1,"output_tokens":1}}`)),
			}}
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, recorder := newOpenAIImageGenerationControlTestContext(false, "unit-test-agent/1.0")
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact", nil)
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			account := newOpenAIImageGenerationControlTestAccount()
			account.Extra = map[string]any{"openai_passthrough": true}
			account.Credentials = map[string]any{
				"api_key": "sk-test",
				"compact_model_mapping": map[string]any{
					tt.canonicalModel: tt.compactModel,
				},
			}
			body := []byte(`{"model":"` + tt.canonicalModel + `","stream":false,"input":"draw"}`)

			result, err := svc.Forward(context.Background(), c, account, body)

			cached, known := getOpenAIImageIntentHint(c)
			require.True(t, known)
			require.Equal(t, tt.wantCanonical, cached)
			if tt.wantRejected {
				require.Error(t, err)
				require.Nil(t, result)
				require.Equal(t, http.StatusForbidden, recorder.Code)
				require.Nil(t, upstream.lastReq)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, tt.compactModel, gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
}

func TestResolveOpenAIImageIntentHintExcludesWebSocketAndUnknownTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, transport := range []OpenAIClientTransport{OpenAIClientTransportWS, OpenAIClientTransportUnknown} {
		c := newOpenAIImageIntentHintTestContext(transport)
		var calls atomic.Int64
		classify := countingOpenAIImageIntentClassifier(&calls)
		body := []byte(`{"model":"gpt-5.4","input":"write code"}`)

		require.False(t, resolveOpenAIImageIntentHint(c, "gpt-5.4", body, classify))
		require.False(t, resolveOpenAIImageIntentHint(c, "gpt-5.4", body, classify))
		require.Equal(t, int64(2), calls.Load())
		_, known := getOpenAIImageIntentHint(c)
		require.False(t, known)
	}
}

func TestResolveOpenAIImageIntentHintConcurrentRequestsAreIsolated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const requests = 32
	var calls atomic.Int64
	classify := countingOpenAIImageIntentClassifier(&calls)
	var wg sync.WaitGroup
	results := make([][2]bool, requests)

	for i := range requests {
		wg.Add(1)
		go func(index int, image bool) {
			defer wg.Done()
			c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
			body := []byte(`{"model":"gpt-5.4","input":"write code"}`)
			if image {
				body = []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"}]}`)
			}
			results[index][0] = resolveOpenAIImageIntentHint(c, "gpt-5.4", body, classify)
			results[index][1] = resolveOpenAIImageIntentHint(c, "gpt-5.4", body, classify)
		}(i, i%2 == 0)
	}
	wg.Wait()
	for i, result := range results {
		require.Equal(t, i%2 == 0, result[0])
		require.Equal(t, result[0], result[1])
	}
	require.Equal(t, int64(requests), calls.Load())
}

var openAIImageIntentHintBenchmarkSink bool

func BenchmarkOpenAIPassthroughImageIntentHintLargeBody(b *testing.B) {
	body := []byte(`{"model":"gpt-5.4","input":"` + strings.Repeat("x", 4<<20) + `"}`)
	const attempts = 4

	b.Run("scan_each_attempt", func(b *testing.B) {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		b.ReportAllocs()
		calls := 0
		for range b.N {
			c.Set(openAIImageIntentHintContextKey, struct{}{})
			for range attempts {
				calls++
				openAIImageIntentHintBenchmarkSink = IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.4", body)
			}
		}
		b.ReportMetric(float64(calls)/float64(b.N), "classifier_calls/op")
	})

	b.Run("request_scoped_hint", func(b *testing.B) {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		b.ReportAllocs()
		calls := 0
		classify := func(endpoint string, requestedModel string, candidate []byte) bool {
			calls++
			return IsImageGenerationIntent(endpoint, requestedModel, candidate)
		}
		for range b.N {
			c.Set(openAIImageIntentHintContextKey, struct{}{})
			for range attempts {
				openAIImageIntentHintBenchmarkSink = resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", body, "gpt-5.4", body, false, classify)
			}
		}
		b.ReportMetric(float64(calls)/float64(b.N), "classifier_calls/op")
	})
}
