package servertiming

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
REDACTED

type trackingBody struct {
	read bool
REDACTED

func (b *trackingBody) Read(_ []byte) (int, error) {
	b.read = true
	return 0, io.EOF
REDACTED

func (b *trackingBody) Close() error { return nil REDACTED

func TestWrapRoundTripperRecordsResponseHeaderLatency(t *testing.T) {
	startedAt := time.Now()
	collector := New(startedAt)
	body := &trackingBody{REDACTED
	baseCalled := false
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		baseCalled = true
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Header:     make(http.Header),
			Request:    req,
	REDACTED, nil
REDACTED)
	req, err := http.NewRequestWithContext(WithCollector(context.Background(), collector), http.MethodGet, "https://api.github.com/repos/example/project", nil)
	if err != nil {
		t.Fatal(err)
REDACTED

	resp, err := WrapRoundTripper(base).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if !baseCalled {
		t.Fatal("base RoundTripper was not called")
REDACTED
	if body.read {
		t.Fatal("RoundTripper instrumentation read the response body; timing must stop at response headers")
REDACTED
	header := collector.HeaderValue(time.Now(), "bypass")
	if !strings.Contains(header, `dep_github;dur=`) || !strings.Contains(header, `deps;dur=`) {
		t.Fatalf("dependency metrics missing from header: %q", header)
REDACTED
REDACTED

func TestWrapRoundTripperUsesContextModuleOverride(t *testing.T) {
	collector := New(time.Now())
	ctx := WithDependencyModule(WithCollector(context.Background(), collector), "data-managementd")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://private.example.test/path", nil)
	if err != nil {
		t.Fatal(err)
REDACTED
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header), Request: reqREDACTED, nil
REDACTED)

	if _, err := WrapRoundTripper(base).RoundTrip(req); err != nil {
		t.Fatal(err)
REDACTED
	header := collector.HeaderValue(time.Now(), "bypass")
	if !strings.Contains(header, "dep_data_managementd") {
		t.Fatalf("module override missing from header: %q", header)
REDACTED
	if strings.Contains(header, "private.example") {
		t.Fatalf("raw host leaked into header: %q", header)
REDACTED
REDACTED

func TestWrapRoundTripperSkipsInactiveContext(t *testing.T) {
	called := false
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: make(http.Header), Request: reqREDACTED, nil
REDACTED)
	req, err := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		t.Fatal(err)
REDACTED
	if _, err := WrapRoundTripper(base).RoundTrip(req); err != nil {
		t.Fatal(err)
REDACTED
	if !called {
		t.Fatal("inactive request did not reach base RoundTripper")
REDACTED
REDACTED

func TestDoRecordsWithoutChangingTransportType(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header), Request: reqREDACTED, nil
REDACTED)
	client := &http.Client{Transport: baseREDACTED
	collector := New(time.Now())
	req, err := http.NewRequestWithContext(WithCollector(context.Background(), collector), http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		t.Fatal(err)
REDACTED
	if _, err := Do(client, req); err != nil {
		t.Fatal(err)
REDACTED
	if _, ok := client.Transport.(roundTripFunc); !ok {
		t.Fatalf("Do changed client transport type to %T", client.Transport)
REDACTED
	if header := collector.HeaderValue(time.Now(), "bypass"); !strings.Contains(header, "dep_openai;dur=") {
		t.Fatalf("dependency metric missing from header: %q", header)
REDACTED
REDACTED

func TestDependencyModuleClassification(t *testing.T) {
	tests := map[string]string{
		"https://api.github.com/repos/a/b":                    "github",
		"https://api.openai.com/v1/models":                    "openai",
		"https://api.anthropic.com/v1/messages":               "anthropic",
		"https://generativelanguage.googleapis.com/v1/models": "gemini",
		"https://cloudcode-pa.googleapis.com/v1internal":      "antigravity",
		"https://storage.googleapis.com/bucket/object":        "google",
		"https://bucket.s3.amazonaws.com/object":              "s3",
		"https://api.stripe.com/v1/refunds":                   "payment",
		"https://dependency.example.test/path":                "http",
REDACTED
	for rawURL, want := range tests {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			t.Fatalf("NewRequest(%q): %v", rawURL, err)
	REDACTED
		if got := dependencyModule(req); got != want {
			t.Errorf("dependencyModule(%q) = %q, want %q", rawURL, got, want)
	REDACTED
REDACTED
REDACTED

func TestClientInstrumentationDoesNotMutateOriginal(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header), Request: reqREDACTED, nil
REDACTED)
	original := &http.Client{Transport: base, Timeout: time.SecondREDACTED
	instrumented := InstrumentClient(original)
	if instrumented == original {
		t.Fatal("InstrumentClient returned the original client")
REDACTED
	if _, ok := original.Transport.(roundTripFunc); !ok {
		t.Fatalf("InstrumentClient mutated the original transport to %T", original.Transport)
REDACTED
	if instrumented.Timeout != original.Timeout {
		t.Fatal("InstrumentClient did not preserve client settings")
REDACTED
	if WrapRoundTripper(instrumented.Transport) != instrumented.Transport {
		t.Fatal("WrapRoundTripper wrapped an already instrumented transport twice")
REDACTED
REDACTED
