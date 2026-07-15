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
	c := &gin.Context{REDACTED
	SetOpenAIClientTransport(c, transport)
	return c
REDACTED

func countingOpenAIImageIntentClassifier(calls *atomic.Int64) openAIImageIntentClassifier {
	return func(endpoint string, requestedModel string, body []byte) bool {
		calls.Add(1)
		return IsImageGenerationIntent(endpoint, requestedModel, body)
REDACTED
REDACTED

func TestResolveOpenAIImageIntentHintCachesTrueAndFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		body []byte
		want bool
REDACTED{
		{name: "true", body: []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"REDACTED]REDACTED`), want: trueREDACTED,
		{name: "false is known", body: []byte(`{"model":"gpt-5.4","input":"write code"REDACTED`), want: falseREDACTED,
REDACTED

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
	REDACTED)
REDACTED
REDACTED

func TestResolveOpenAIImageIntentHintUsesHandlerSeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, seeded := range []bool{false, trueREDACTED {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		SetOpenAIImageIntentHint(c, seeded)
		var calls atomic.Int64

		got := resolveOpenAIImageIntentHint(c, "gpt-5.4", []byte(`{"model":"gpt-5.4"REDACTED`), countingOpenAIImageIntentClassifier(&calls))

		require.Equal(t, seeded, got)
		require.Zero(t, calls.Load())
REDACTED
REDACTED

func TestResolveOpenAIPassthroughImageIntentReusesCanonicalAcrossFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
	body := []byte(`{"model":"gpt-5.4","input":"write code"REDACTED`)
	var calls atomic.Int64
	classify := countingOpenAIImageIntentClassifier(&calls)

	for range 3 {
		require.False(t, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", body, "gpt-5.4", body, false, classify))
REDACTED
	require.Equal(t, int64(1), calls.Load())
REDACTED

func TestResolveOpenAIPassthroughImageIntentKeepsCompactMappingAttemptLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("text to image", func(t *testing.T) {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		body := []byte(`{"model":"draw-alias","input":"draw"REDACTED`)
		compactBody := []byte(`{"model":"gpt-image-2","input":"draw"REDACTED`)
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
REDACTED)

	t.Run("image to text", func(t *testing.T) {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		body := []byte(`{"model":"gpt-image-2","input":"draw"REDACTED`)
		compactBody := []byte(`{"model":"gpt-5.4","input":"draw"REDACTED`)
		var calls atomic.Int64
		classify := countingOpenAIImageIntentClassifier(&calls)

		require.False(t, resolveOpenAIPassthroughImageIntent(c, "gpt-image-2", body, "gpt-5.4", compactBody, true, classify))
		cached, known := getOpenAIImageIntentHint(c)
		require.True(t, known)
		require.True(t, cached)

		require.True(t, resolveOpenAIPassthroughImageIntent(c, "gpt-image-2", body, "gpt-image-2", body, false, classify))
		require.Equal(t, int64(2), calls.Load())
REDACTED)
REDACTED

func TestResolveOpenAIPassthroughImageIntentInvalidationDoesNotPolluteCanonical(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
	canonicalBody := []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"REDACTED]REDACTED`)
	strippedBody := []byte(`{"model":"gpt-5.4","tools":[]REDACTED`)
	var calls atomic.Int64
	classify := countingOpenAIImageIntentClassifier(&calls)

	require.False(t, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", canonicalBody, "gpt-5.4", strippedBody, true, classify))
	require.Equal(t, int64(2), calls.Load(), "unknown canonical and invalidated attempt are classified independently")
	cached, known := getOpenAIImageIntentHint(c)
	require.True(t, known)
	require.True(t, cached)

	require.True(t, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", canonicalBody, "gpt-5.4", canonicalBody, false, classify))
	require.Equal(t, int64(2), calls.Load())
REDACTED

func TestResolveOpenAIPassthroughImageIntentMappedBodyStartsUnknownThenSeedsCanonical(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
	canonicalBody := []byte(`{"model":"gpt-image-2","input":"draw"REDACTED`)
	strippedAttemptBody := []byte(`{"model":"gpt-5.4","input":"draw"REDACTED`)
	_, known := getOpenAIImageIntentHint(c)
	require.False(t, known)
	var calls atomic.Int64
	classify := countingOpenAIImageIntentClassifier(&calls)

	require.False(t, resolveOpenAIPassthroughImageIntent(c, "gpt-image-2", canonicalBody, "gpt-5.4", strippedAttemptBody, true, classify))
	require.Equal(t, int64(2), calls.Load())
	cached, known := getOpenAIImageIntentHint(c)
	require.True(t, known)
	require.True(t, cached)
REDACTED

func TestResolveOpenAIPassthroughImageIntentReusesAcrossInvariantMutations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		canonicalBody []byte
		attemptBody   []byte
		want          bool
REDACTED{
		{
			name:          "oauth sanitize fast policy and reasoning",
			canonicalBody: []byte(`{"model":"gpt-5.4","input":[{"type":"input_image","image_url":"data:image/png;base64,"REDACTED],"service_tier":"fast","reasoning":{"effort":"minimal"REDACTEDREDACTED`),
			attemptBody:   []byte(`{"model":"gpt-5.4","input":[],"service_tier":"priority","reasoning":{"effort":"none"REDACTED,"store":false,"stream":trueREDACTED`),
			want:          false,
	REDACTED,
		{
			name:          "namespace flatten",
			canonicalBody: []byte(`{"model":"gpt-5.4","tools":[{"type":"namespace","name":"code_tools"REDACTED]REDACTED`),
			attemptBody:   []byte(`{"model":"gpt-5.4","tools":[{"type":"function","name":"code_tools.run"REDACTED]REDACTED`),
			want:          false,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
			var calls atomic.Int64
			classify := countingOpenAIImageIntentClassifier(&calls)

			require.Equal(t, tt.want, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", tt.canonicalBody, "gpt-5.4", tt.attemptBody, false, classify))
			require.Equal(t, tt.want, resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", tt.canonicalBody, "gpt-5.4", tt.attemptBody, false, classify))
			require.Equal(t, int64(1), calls.Load())
	REDACTED)
REDACTED
REDACTED

func TestOpenAIGatewayServicePassthroughCompactImageIntentIsAttemptLocal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		canonicalModel string
		compactModel   string
		wantRejected   bool
		wantCanonical  bool
REDACTED{
		{
			name:           "text to image rejects",
			canonicalModel: "gpt-5.4",
			compactModel:   "gpt-image-2",
			wantRejected:   true,
	REDACTED,
		{
			name:           "image to text reaches upstream",
			canonicalModel: "gpt-image-2",
			compactModel:   "gpt-5.4",
			wantCanonical:  true,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"REDACTEDREDACTED,
				Body:       io.NopCloser(strings.NewReader(`{"id":"resp_compact","model":"` + tt.compactModel + `","usage":{"input_tokens":1,"output_tokens":1REDACTEDREDACTED`)),
		REDACTEDREDACTED
			svc := newOpenAIImageGenerationControlTestService(upstream)
			c, recorder := newOpenAIImageGenerationControlTestContext(false, "unit-test-agent/1.0")
			c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses/compact", nil)
			SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
			account := newOpenAIImageGenerationControlTestAccount()
			account.Extra = map[string]any{"openai_passthrough": trueREDACTED
			account.Credentials = map[string]any{
				"api_key": "sk-test",
				"compact_model_mapping": map[string]any{
					tt.canonicalModel: tt.compactModel,
			REDACTED,
		REDACTED
			body := []byte(`{"model":"` + tt.canonicalModel + `","stream":false,"input":"draw"REDACTED`)

			result, err := svc.Forward(context.Background(), c, account, body)

			cached, known := getOpenAIImageIntentHint(c)
			require.True(t, known)
			require.Equal(t, tt.wantCanonical, cached)
			if tt.wantRejected {
			REDACTED
				require.Nil(t, result)
				require.Equal(t, http.StatusForbidden, recorder.Code)
				require.Nil(t, upstream.lastReq)
				return
		REDACTED
		REDACTED
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, tt.compactModel, gjson.GetBytes(upstream.lastBody, "model").String())
	REDACTED)
REDACTED
REDACTED

func TestResolveOpenAIImageIntentHintExcludesWebSocketAndUnknownTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, transport := range []OpenAIClientTransport{OpenAIClientTransportWS, OpenAIClientTransportUnknownREDACTED {
		c := newOpenAIImageIntentHintTestContext(transport)
		var calls atomic.Int64
		classify := countingOpenAIImageIntentClassifier(&calls)
		body := []byte(`{"model":"gpt-5.4","input":"write code"REDACTED`)

		require.False(t, resolveOpenAIImageIntentHint(c, "gpt-5.4", body, classify))
		require.False(t, resolveOpenAIImageIntentHint(c, "gpt-5.4", body, classify))
		require.Equal(t, int64(2), calls.Load())
		_, known := getOpenAIImageIntentHint(c)
		require.False(t, known)
REDACTED
REDACTED

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
			body := []byte(`{"model":"gpt-5.4","input":"write code"REDACTED`)
			if image {
				body = []byte(`{"model":"gpt-5.4","tools":[{"type":"image_generation"REDACTED]REDACTED`)
		REDACTED
			results[index][0] = resolveOpenAIImageIntentHint(c, "gpt-5.4", body, classify)
			results[index][1] = resolveOpenAIImageIntentHint(c, "gpt-5.4", body, classify)
	REDACTED(i, i%2 == 0)
REDACTED
	wg.Wait()
	for i, result := range results {
		require.Equal(t, i%2 == 0, result[0])
		require.Equal(t, result[0], result[1])
REDACTED
	require.Equal(t, int64(requests), calls.Load())
REDACTED

var openAIImageIntentHintBenchmarkSink bool

func BenchmarkOpenAIPassthroughImageIntentHintLargeBody(b *testing.B) {
	body := []byte(`{"model":"gpt-5.4","input":"` + strings.Repeat("x", 4<<20) + `"REDACTED`)
	const attempts = 4

	b.Run("scan_each_attempt", func(b *testing.B) {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		b.ReportAllocs()
		calls := 0
		for range b.N {
			c.Set(openAIImageIntentHintContextKey, struct{REDACTED{REDACTED)
			for range attempts {
				calls++
				openAIImageIntentHintBenchmarkSink = IsImageGenerationIntent(openAIResponsesEndpoint, "gpt-5.4", body)
		REDACTED
	REDACTED
		b.ReportMetric(float64(calls)/float64(b.N), "classifier_calls/op")
REDACTED)

	b.Run("request_scoped_hint", func(b *testing.B) {
		c := newOpenAIImageIntentHintTestContext(OpenAIClientTransportHTTP)
		b.ReportAllocs()
		calls := 0
		classify := func(endpoint string, requestedModel string, candidate []byte) bool {
			calls++
			return IsImageGenerationIntent(endpoint, requestedModel, candidate)
	REDACTED
		for range b.N {
			c.Set(openAIImageIntentHintContextKey, struct{REDACTED{REDACTED)
			for range attempts {
				openAIImageIntentHintBenchmarkSink = resolveOpenAIPassthroughImageIntent(c, "gpt-5.4", body, "gpt-5.4", body, false, classify)
		REDACTED
	REDACTED
		b.ReportMetric(float64(calls)/float64(b.N), "classifier_calls/op")
REDACTED)
REDACTED
