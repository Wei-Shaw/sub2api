package httputil

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeLenientJSONRequestBody_accepts_client_control_chars_in_strings(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		path    string
		want    string
		wantRaw string
REDACTED{
		{
			name:    "null byte in message content",
			body:    []byte("{\"messages\":[{\"content\":\"hello\x00world\"REDACTED]REDACTED"),
			path:    "messages.0.content",
			want:    "hello\x00world",
			wantRaw: `"hello\u0000world"`,
	REDACTED,
		{
			name:    "ansi escape in message content",
			body:    []byte("{\"messages\":[{\"content\":\"hello\x1b[31mred\x1b[0m\"REDACTED]REDACTED"),
			path:    "messages.0.content",
			want:    "hello\x1b[31mred\x1b[0m",
			wantRaw: `"hello\u001b[31mred\u001b[0m"`,
	REDACTED,
		{
			name:    "leading UTF-8 BOM",
			body:    []byte("\xef\xbb\xbf{\"input\":\"hello\"REDACTED"),
			path:    "input",
			want:    "hello",
			wantRaw: `"hello"`,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			if gjson.ValidBytes(tt.body) {
				t.Fatalf("test payload should reproduce strict JSON rejection: %q", tt.body)
		REDACTED

			// When
			got, err := NormalizeLenientJSONRequestBody(tt.body, 1024)
			if err != nil {
				t.Fatalf("NormalizeLenientJSONRequestBody: %v", err)
		REDACTED

			// Then
			if !gjson.ValidBytes(got) {
				t.Fatalf("normalized body should be valid JSON: %q", got)
		REDACTED
			result := gjson.GetBytes(got, tt.path)
			if result.String() != tt.want {
				t.Fatalf("value mismatch: got %q want %q", result.String(), tt.want)
		REDACTED
			if result.Raw != tt.wantRaw {
				t.Fatalf("raw value mismatch: got %q want %q", result.Raw, tt.wantRaw)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestNormalizeLenientJSONRequestBody_keeps_invalid_structure_invalid(t *testing.T) {
	tests := []struct {
		name string
		body []byte
REDACTED{
		{
			name: "truncated JSON",
			body: []byte("{\"messages\":[{\"content\":\"hello\"REDACTED]"),
	REDACTED,
		{
			name: "control character outside string",
			body: []byte("{\"input\":\"hello\"REDACTED\x00"),
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got, err := NormalizeLenientJSONRequestBody(tt.body, 1024)
			if err != nil {
				t.Fatalf("NormalizeLenientJSONRequestBody: %v", err)
		REDACTED

			// Then
			if gjson.ValidBytes(got) {
				t.Fatalf("normalization must not repair invalid JSON structure: %q", got)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestNormalizeLenientJSONRequestBody_allows_http_requests_with_client_control_chars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Given
		body, err := ReadLenientJSONRequestBodyWithPrealloc(r, 1024)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
	REDACTED

		// When
		if !gjson.ValidBytes(body) {
			http.Error(w, "Failed to parse request body", http.StatusBadRequest)
			return
	REDACTED
		w.WriteHeader(http.StatusAccepted)
REDACTED))
	defer server.Close()

	tests := []struct {
		name string
		body []byte
		want int
REDACTED{
		{
			name: "null byte in JSON string",
			body: []byte("{\"model\":\"gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\x00world\"REDACTED]REDACTED"),
			want: http.StatusAccepted,
	REDACTED,
		{
			name: "ANSI escape in JSON string",
			body: []byte("{\"model\":\"gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\x1b[31mred\x1b[0m\"REDACTED]REDACTED"),
			want: http.StatusAccepted,
	REDACTED,
		{
			name: "leading UTF-8 BOM",
			body: []byte("\xef\xbb\xbf{\"model\":\"gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"REDACTED]REDACTED"),
			want: http.StatusAccepted,
	REDACTED,
		{
			name: "truncated JSON",
			body: []byte("{\"model\":\"gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"REDACTED]"),
			want: http.StatusBadRequest,
	REDACTED,
REDACTED

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewReader(tt.body))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
		REDACTED
			req.Header.Set("Content-Type", "application/json")

			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
		REDACTED
			defer func() { _ = resp.Body.Close() REDACTED()

			if resp.StatusCode != tt.want {
				t.Fatalf("status mismatch: got %d want %d", resp.StatusCode, tt.want)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestNormalizeLenientJSONRequestBody_rejects_expansion_past_limit(t *testing.T) {
	// Given
	body := []byte("{\"input\":\"\x00\x00\"REDACTED")

	// When
	_, err := NormalizeLenientJSONRequestBody(body, int64(len(body)+5))

	// Then
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected MaxBytesError, got %T %v", err, err)
REDACTED
	if maxErr.Limit != int64(len(body)+5) {
		t.Fatalf("limit mismatch: got %d want %d", maxErr.Limit, len(body)+5)
REDACTED
REDACTED
