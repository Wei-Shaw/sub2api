//go:build unit

package service

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitBackupFile_ReassemblesExactBytes(t *testing.T) {
	src := writeBackupArchiveFixture(t, []byte("0123456789abcdefg"))
	parts, err := splitBackupFile(src, 5)
REDACTED
	require.Len(t, parts, 4)

	var got bytes.Buffer
	for i, part := range parts {
		require.Equal(t, i+1, part.Index)
		require.LessOrEqual(t, part.SizeBytes, int64(5))
		data, readErr := os.ReadFile(part.Path)
		require.NoError(t, readErr)
		require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(data)), part.SHA256)
		got.Write(data)
REDACTED
	require.Equal(t, []byte("0123456789abcdefg"), got.Bytes())
REDACTED

func TestSplitBackupFile_RejectsInvalidInput(t *testing.T) {
	src := writeBackupArchiveFixture(t, []byte("data"))

	_, err := splitBackupFile(src, 0)
REDACTED

	empty := writeBackupArchiveFixture(t, nil)
	_, err = splitBackupFile(empty, 5)
REDACTED

	_, err = splitBackupFile(filepathForMissingBackupArchive(t), 5)
REDACTED
REDACTED

func writeBackupArchiveFixture(t *testing.T, content []byte) string {
REDACTED
	path := filepathForBackupArchive(t)
	require.NoError(t, os.WriteFile(path, content, 0o600))
	return path
REDACTED

func filepathForBackupArchive(t *testing.T) string {
REDACTED
	return t.TempDir() + "/archive.gz"
REDACTED

func filepathForMissingBackupArchive(t *testing.T) string {
REDACTED
	return t.TempDir() + "/missing.gz"
REDACTED
