//go:build unit

package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestS3BackupStore_UploadFile(t *testing.T) {
	var received []byte
	var receivedLength int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		receivedLength = r.ContentLength
		var err error
		received, err = io.ReadAll(r.Body)
	REDACTED
		w.WriteHeader(http.StatusOK)
REDACTED))
	defer server.Close()

	client, err := newS3Client(context.Background(), s3ClientParams{
		Endpoint:        server.URL,
		Region:          "auto",
		AccessKeyID:     "test-ak",
		SecretAccessKey: "test-sk",
		ForcePathStyle:  true,
REDACTED)
REDACTED

	content := []byte("streamed backup payload")
	filePath := t.TempDir() + "/part.gz"
	require.NoError(t, os.WriteFile(filePath, content, 0o600))

	store := &S3BackupStore{client: client, bucket: "backup-bucket"REDACTED
	size, err := store.UploadFile(context.Background(), "backup/part-1", filePath, "application/octet-stream")
REDACTED
	require.Equal(t, int64(len(content)), size)
	require.Equal(t, int64(len(content)), receivedLength)
	require.Equal(t, content, received)
REDACTED
