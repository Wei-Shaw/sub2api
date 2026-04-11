package websearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBraveProvider_Name(t *testing.T) {
	p := NewBraveProvider("key", nil)
	require.Equal(t, "brave", p.Name())
REDACTED

func TestBraveProvider_Search_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-key", r.Header.Get("X-Subscription-Token"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.Equal(t, "golang", r.URL.Query().Get("q"))
		require.Equal(t, "3", r.URL.Query().Get("count"))

		resp := braveResponse{REDACTED
		resp.Web.Results = []braveResult{
			{URL: "https://go.dev", Title: "Go", Description: "Go lang", Age: "1 day"REDACTED,
			{URL: "https://pkg.go.dev", Title: "Pkg", Description: "Packages"REDACTED,
			{URL: "https://tour.go.dev", Title: "Tour", Description: "A Tour of Go", Age: "3 days"REDACTED,
	REDACTED
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
REDACTED))
	defer srv.Close()

	p := NewBraveProvider("test-key", srv.Client())
	// Override the endpoint for testing
	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srv.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL REDACTED()

	resp, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 3REDACTED)
REDACTED
	require.Len(t, resp.Results, 3)
	require.Equal(t, "https://go.dev", resp.Results[0].URL)
	require.Equal(t, "Go lang", resp.Results[0].Snippet)
	require.Equal(t, "1 day", resp.Results[0].PageAge)
REDACTED

func TestBraveProvider_Search_DefaultMaxResults(t *testing.T) {
	var receivedCount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedCount = r.URL.Query().Get("count")
		resp := braveResponse{REDACTED
		json.NewEncoder(w).Encode(resp)
REDACTED))
	defer srv.Close()

	p := NewBraveProvider("key", srv.Client())
	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srv.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL REDACTED()

	_, _ = p.Search(context.Background(), SearchRequest{Query: "test", MaxResults: 0REDACTED)
	require.Equal(t, "5", receivedCount)
REDACTED

func TestBraveProvider_Search_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte("rate limited"))
REDACTED))
	defer srv.Close()

	p := NewBraveProvider("key", srv.Client())
	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srv.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL REDACTED()

	_, err := p.Search(context.Background(), SearchRequest{Query: "test"REDACTED)
	require.ErrorContains(t, err, "brave: status 429")
REDACTED

func TestBraveProvider_Search_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json"))
REDACTED))
	defer srv.Close()

	p := NewBraveProvider("key", srv.Client())
	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srv.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL REDACTED()

	_, err := p.Search(context.Background(), SearchRequest{Query: "test"REDACTED)
	require.ErrorContains(t, err, "brave: decode response")
REDACTED

func TestBraveProvider_Search_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := braveResponse{REDACTED
		json.NewEncoder(w).Encode(resp)
REDACTED))
	defer srv.Close()

	p := NewBraveProvider("key", srv.Client())
	origURL := *braveSearchURL
	u, _ := http.NewRequest("GET", srv.URL, nil)
	*braveSearchURL = *u.URL
	defer func() { *braveSearchURL = origURL REDACTED()

	resp, err := p.Search(context.Background(), SearchRequest{Query: "test"REDACTED)
REDACTED
	require.Empty(t, resp.Results)
REDACTED
