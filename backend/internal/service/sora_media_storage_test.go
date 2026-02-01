//go:build unit

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestSoraMediaStorage_StoreFromURLs(t *testing.T) {
	tmpDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
REDACTED))
	defer server.Close()

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:                   "local",
				LocalPath:              tmpDir,
				MaxConcurrentDownloads: 1,
		REDACTED,
	REDACTED,
REDACTED

	storage := NewSoraMediaStorage(cfg)
	urls, err := storage.StoreFromURLs(context.Background(), "image", []string{server.URL + "/img.png"REDACTED)
REDACTED
	require.Len(t, urls, 1)
	require.True(t, strings.HasPrefix(urls[0], "/image/"))
	require.True(t, strings.HasSuffix(urls[0], ".png"))

	localPath := filepath.Join(tmpDir, filepath.FromSlash(strings.TrimPrefix(urls[0], "/")))
	require.FileExists(t, localPath)
REDACTED

func TestSoraMediaStorage_FallbackToUpstream(t *testing.T) {
	tmpDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
REDACTED))
	defer server.Close()

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:               "local",
				LocalPath:          tmpDir,
				FallbackToUpstream: true,
		REDACTED,
	REDACTED,
REDACTED

	storage := NewSoraMediaStorage(cfg)
	url := server.URL + "/broken.png"
	urls, err := storage.StoreFromURLs(context.Background(), "image", []string{urlREDACTED)
REDACTED
	require.Equal(t, []string{urlREDACTED, urls)
REDACTED

func TestSoraMediaStorage_MaxDownloadBytes(t *testing.T) {
	tmpDir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("too-large"))
REDACTED))
	defer server.Close()

	cfg := &config.Config{
		Sora: config.SoraConfig{
			Storage: config.SoraStorageConfig{
				Type:             "local",
				LocalPath:        tmpDir,
				MaxDownloadBytes: 1,
		REDACTED,
	REDACTED,
REDACTED

	storage := NewSoraMediaStorage(cfg)
	_, err := storage.StoreFromURLs(context.Background(), "image", []string{server.URL + "/img.png"REDACTED)
REDACTED
REDACTED
