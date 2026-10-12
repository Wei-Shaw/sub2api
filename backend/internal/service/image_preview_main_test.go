package service

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	previewDir, err := os.MkdirTemp("", "buaga-image-preview-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	openAIImagePreviewDir = previewDir
	code := m.Run()
	_ = os.RemoveAll(previewDir)
	os.Exit(code)
}
