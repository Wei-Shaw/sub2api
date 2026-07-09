package repository

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type GitHubReleaseServiceSuite struct {
	suite.Suite
	srv     *httptest.Server
	client  *githubReleaseClient
	tempDir string
REDACTED

// testTransport redirects requests to the test server
type testTransport struct {
	testServerURL string
REDACTED

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the URL to point to our test server
	testURL := t.testServerURL + req.URL.Path
	if req.URL.RawQuery != "" {
		testURL += "?" + req.URL.RawQuery
REDACTED
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, testURL, req.Body)
	if err != nil {
		return nil, err
REDACTED
	newReq.Header = req.Header
	return http.DefaultTransport.RoundTrip(newReq)
REDACTED

func newTestGitHubReleaseClient() *githubReleaseClient {
	return &githubReleaseClient{
		httpClient:         &http.Client{REDACTED,
		downloadHTTPClient: &http.Client{REDACTED,
REDACTED
REDACTED

func (s *GitHubReleaseServiceSuite) SetupTest() {
	s.tempDir = s.T().TempDir()
REDACTED

func (s *GitHubReleaseServiceSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
		s.srv = nil
REDACTED
REDACTED

func (s *GitHubReleaseServiceSuite) TestDownloadFile_EnforcesMaxSize_ContentLength() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("a"), 100))
REDACTED))

	s.client = newTestGitHubReleaseClient()

	dest := filepath.Join(s.tempDir, "file1.bin")
	err := s.client.DownloadFile(context.Background(), s.srv.URL, dest, 10)
	require.Error(s.T(), err, "expected error for oversized download with Content-Length")

	_, statErr := os.Stat(dest)
	require.Error(s.T(), statErr, "expected file to not exist for rejected download")
REDACTED

func (s *GitHubReleaseServiceSuite) TestDownloadFile_EnforcesMaxSize_Chunked() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force chunked encoding (unknown Content-Length) by flushing headers before writing.
		w.WriteHeader(http.StatusOK)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
	REDACTED
		for i := 0; i < 10; i++ {
			_, _ = w.Write(bytes.Repeat([]byte("b"), 10))
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
		REDACTED
	REDACTED
REDACTED))

	s.client = newTestGitHubReleaseClient()

	dest := filepath.Join(s.tempDir, "file2.bin")
	err := s.client.DownloadFile(context.Background(), s.srv.URL, dest, 10)
	require.Error(s.T(), err, "expected error for oversized chunked download")

	_, statErr := os.Stat(dest)
	require.Error(s.T(), statErr, "expected file to be cleaned up for oversized chunked download")
REDACTED

func (s *GitHubReleaseServiceSuite) TestDownloadFile_Success() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
	REDACTED
		for i := 0; i < 10; i++ {
			_, _ = w.Write(bytes.Repeat([]byte("b"), 10))
			if fl, ok := w.(http.Flusher); ok {
				fl.Flush()
		REDACTED
	REDACTED
REDACTED))

	s.client = newTestGitHubReleaseClient()

	dest := filepath.Join(s.tempDir, "file3.bin")
	err := s.client.DownloadFile(context.Background(), s.srv.URL, dest, 200)
	require.NoError(s.T(), err, "expected success")

	b, err := os.ReadFile(dest)
	require.NoError(s.T(), err, "read")
	require.True(s.T(), strings.HasPrefix(string(b), "b"), "downloaded content should start with 'b'")
	require.Len(s.T(), b, 100, "downloaded content length mismatch")
REDACTED

func (s *GitHubReleaseServiceSuite) TestDownloadFile_404() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
REDACTED))

	s.client = newTestGitHubReleaseClient()

	dest := filepath.Join(s.tempDir, "notfound.bin")
	err := s.client.DownloadFile(context.Background(), s.srv.URL, dest, 100)
	require.Error(s.T(), err, "expected error for 404")

	_, statErr := os.Stat(dest)
	require.Error(s.T(), statErr, "expected file to not exist for 404")
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchChecksumFile_Success() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("sum"))
REDACTED))

	s.client = newTestGitHubReleaseClient()

	body, err := s.client.FetchChecksumFile(context.Background(), s.srv.URL)
	require.NoError(s.T(), err, "FetchChecksumFile")
	require.Equal(s.T(), "sum", string(body), "checksum body mismatch")
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchChecksumFile_Non200() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
REDACTED))

	s.client = newTestGitHubReleaseClient()

	_, err := s.client.FetchChecksumFile(context.Background(), s.srv.URL)
	require.Error(s.T(), err, "expected error for non-200")
REDACTED

func (s *GitHubReleaseServiceSuite) TestDownloadFile_ContextCancel() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
REDACTED))

	s.client = newTestGitHubReleaseClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dest := filepath.Join(s.tempDir, "cancelled.bin")
	err := s.client.DownloadFile(ctx, s.srv.URL, dest, 100)
	require.Error(s.T(), err, "expected error for cancelled context")
REDACTED

func (s *GitHubReleaseServiceSuite) TestDownloadFile_InvalidURL() {
	s.client = newTestGitHubReleaseClient()

	dest := filepath.Join(s.tempDir, "invalid.bin")
	err := s.client.DownloadFile(context.Background(), "://invalid-url", dest, 100)
	require.Error(s.T(), err, "expected error for invalid URL")
REDACTED

func (s *GitHubReleaseServiceSuite) TestDownloadFile_InvalidDestPath() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
REDACTED))

	s.client = newTestGitHubReleaseClient()

	// Use a path that cannot be created (directory doesn't exist)
	dest := filepath.Join(s.tempDir, "nonexistent", "subdir", "file.bin")
	err := s.client.DownloadFile(context.Background(), s.srv.URL, dest, 100)
	require.Error(s.T(), err, "expected error for invalid destination path")
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchChecksumFile_InvalidURL() {
	s.client = newTestGitHubReleaseClient()

	_, err := s.client.FetchChecksumFile(context.Background(), "://invalid-url")
	require.Error(s.T(), err, "expected error for invalid URL")
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchLatestRelease_Success() {
	releaseJSON := `{
		"tag_name": "v1.0.0",
		"name": "Release 1.0.0",
		"body": "Release notes",
		"html_url": "https://github.com/test/repo/releases/v1.0.0",
		"assets": [
			{
				"name": "app-linux-amd64.tar.gz",
				"browser_download_url": "https://github.com/test/repo/releases/download/v1.0.0/app-linux-amd64.tar.gz"
		REDACTED
		]
REDACTED`

	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(s.T(), "/repos/test/repo/releases/latest", r.URL.Path)
		require.Equal(s.T(), "application/vnd.github.v3+json", r.Header.Get("Accept"))
		require.Equal(s.T(), "Sub2API-Updater", r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(releaseJSON))
REDACTED))

	// Use custom transport to redirect requests to test server
	s.client = &githubReleaseClient{
		httpClient: &http.Client{
			Transport: &testTransport{testServerURL: s.srv.URLREDACTED,
	REDACTED,
		downloadHTTPClient: &http.Client{REDACTED,
REDACTED

	release, err := s.client.FetchLatestRelease(context.Background(), "test/repo")
	require.NoError(s.T(), err)
	require.Equal(s.T(), "v1.0.0", release.TagName)
	require.Equal(s.T(), "Release 1.0.0", release.Name)
	require.Len(s.T(), release.Assets, 1)
	require.Equal(s.T(), "app-linux-amd64.tar.gz", release.Assets[0].Name)
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchRecentReleases_Success() {
	releasesJSON := `[
		{
			"tag_name": "v1.0.1",
			"name": "Release 1.0.1",
			"html_url": "https://github.com/test/repo/releases/v1.0.1",
			"published_at": "2026-07-08T00:00:00Z",
			"prerelease": false,
			"assets": [
				{
					"name": "app-linux-amd64.tar.gz",
					"browser_download_url": "https://github.com/test/repo/releases/download/v1.0.1/app-linux-amd64.tar.gz"
			REDACTED
			]
	REDACTED,
		{
			"tag_name": "v1.0.1-rc1",
			"name": "Release 1.0.1-rc1",
			"prerelease": true
	REDACTED,
		{
			"tag_name": "v1.0.0",
			"name": "Release 1.0.0"
	REDACTED
	]`

	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(s.T(), "/repos/test/repo/releases", r.URL.Path)
		require.Equal(s.T(), "15", r.URL.Query().Get("per_page"))
		require.Equal(s.T(), "application/vnd.github.v3+json", r.Header.Get("Accept"))
		require.Equal(s.T(), "Sub2API-Updater", r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(releasesJSON))
REDACTED))

	s.client = &githubReleaseClient{
		httpClient: &http.Client{
			Transport: &testTransport{testServerURL: s.srv.URLREDACTED,
	REDACTED,
		downloadHTTPClient: &http.Client{REDACTED,
REDACTED

	releases, err := s.client.FetchRecentReleases(context.Background(), "test/repo", 15)
	require.NoError(s.T(), err)
	require.Len(s.T(), releases, 3)
	require.Equal(s.T(), "v1.0.1", releases[0].TagName)
	require.False(s.T(), releases[0].Prerelease)
	require.Len(s.T(), releases[0].Assets, 1)
	require.True(s.T(), releases[1].Prerelease)
	require.Equal(s.T(), "v1.0.0", releases[2].TagName)
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchRecentReleases_Non200() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
REDACTED))

	s.client = &githubReleaseClient{
		httpClient: &http.Client{
			Transport: &testTransport{testServerURL: s.srv.URLREDACTED,
	REDACTED,
		downloadHTTPClient: &http.Client{REDACTED,
REDACTED

	_, err := s.client.FetchRecentReleases(context.Background(), "test/repo", 15)
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "403")
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchLatestRelease_Non200() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
REDACTED))

	s.client = &githubReleaseClient{
		httpClient: &http.Client{
			Transport: &testTransport{testServerURL: s.srv.URLREDACTED,
	REDACTED,
		downloadHTTPClient: &http.Client{REDACTED,
REDACTED

	_, err := s.client.FetchLatestRelease(context.Background(), "test/repo")
	require.Error(s.T(), err)
	require.Contains(s.T(), err.Error(), "404")
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchLatestRelease_InvalidJSON() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json"))
REDACTED))

	s.client = &githubReleaseClient{
		httpClient: &http.Client{
			Transport: &testTransport{testServerURL: s.srv.URLREDACTED,
	REDACTED,
		downloadHTTPClient: &http.Client{REDACTED,
REDACTED

	_, err := s.client.FetchLatestRelease(context.Background(), "test/repo")
	require.Error(s.T(), err)
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchLatestRelease_ContextCancel() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
REDACTED))

	s.client = &githubReleaseClient{
		httpClient: &http.Client{
			Transport: &testTransport{testServerURL: s.srv.URLREDACTED,
	REDACTED,
		downloadHTTPClient: &http.Client{REDACTED,
REDACTED

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.client.FetchLatestRelease(ctx, "test/repo")
	require.Error(s.T(), err)
REDACTED

func (s *GitHubReleaseServiceSuite) TestFetchChecksumFile_ContextCancel() {
	s.srv = newLocalTestServer(s.T(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
REDACTED))

	s.client = newTestGitHubReleaseClient()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.client.FetchChecksumFile(ctx, s.srv.URL)
	require.Error(s.T(), err)
REDACTED

func TestGitHubReleaseServiceSuite(t *testing.T) {
	suite.Run(t, new(GitHubReleaseServiceSuite))
REDACTED
