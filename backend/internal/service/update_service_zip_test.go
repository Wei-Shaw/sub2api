package service

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractBinaryFromZip(t *testing.T) {
	// Create a temporary zip archive containing a fake sub2api.exe
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "sub2api_0.1.161_windows_amd64.zip")
	destPath := filepath.Join(tmpDir, "sub2api.exe")
	expectedContent := []byte("fake sub2api binary content")

	zipFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}
	zw := zip.NewWriter(zipFile)
	w, err := zw.Create("sub2api.exe")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write(expectedContent); err != nil {
		t.Fatalf("failed to write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	zipFile.Close()

	// Extract the binary
	if err := extractBinaryFromZip(archivePath, destPath); err != nil {
		t.Fatalf("extractBinaryFromZip failed: %v", err)
	}

	// Verify the extracted content
	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if !bytes.Equal(got, expectedContent) {
		t.Fatalf("extracted content mismatch: got %q, want %q", got, expectedContent)
	}
}

func TestExtractBinaryFromZipPathTraversal(t *testing.T) {
	// Create a zip archive with a path traversal entry
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "malicious.zip")
	destPath := filepath.Join(tmpDir, "sub2api.exe")

	zipFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}
	zw := zip.NewWriter(zipFile)
	w, err := zw.Create("../sub2api.exe")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write([]byte("evil")); err != nil {
		t.Fatalf("failed to write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip writer: %v", err)
	}
	zipFile.Close()

	// Extraction should reject path traversal
	if err := extractBinaryFromZip(archivePath, destPath); err == nil {
		t.Fatal("expected error for path traversal attempt, got nil")
	}
}

func TestExtractBinaryFromZipRealRelease(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real release download test in short mode")
	}

	const releaseURL = "https://github.com/Wei-Shaw/sub2api/releases/download/v0.1.161/sub2api_0.1.161_windows_amd64.zip"

	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "sub2api_0.1.161_windows_amd64.zip")
	destPath := filepath.Join(tmpDir, "sub2api.exe")

	resp, err := http.Get(releaseURL)
	if err != nil {
		t.Fatalf("failed to download release zip: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to download release zip: status %d", resp.StatusCode)
	}

	out, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("failed to create archive file: %v", err)
	}
	if _, err := io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		t.Fatalf("failed to save archive file: %v", err)
	}
	_ = out.Close()

	if err := extractBinaryFromZip(archivePath, destPath); err != nil {
		t.Fatalf("extractBinaryFromZip failed on real release: %v", err)
	}

	// Verify extracted file is a valid Windows PE executable
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read extracted binary: %v", err)
	}
	if len(data) < 2 || data[0] != 'M' || data[1] != 'Z' {
		t.Fatalf("extracted file is not a valid Windows PE executable: got %q", data[:2])
	}

	// Verify the COFF header offset and signature
	if len(data) < 64 {
		t.Fatalf("extracted binary too small")
	}
	offset := binary.LittleEndian.Uint32(data[60:64])
	if offset+4 > uint32(len(data)) || string(data[offset:offset+4]) != "PE\x00\x00" {
		t.Fatalf("extracted file is not a valid PE executable")
	}
}
