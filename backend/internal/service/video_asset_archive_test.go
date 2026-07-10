package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestArchiveSucceededVideoResultWritesLocalPath(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA_DIR", dataDir)

	prev := videoAssetURLValidator
	videoAssetURLValidator = func(string) error { return nil }
	t.Cleanup(func() { videoAssetURLValidator = prev })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("fake-video-bytes"))
	}))
	t.Cleanup(srv.Close)

	repo := &memoryVideoGatewayRepo{tasks: map[int64]*VideoTask{}}
	task := &VideoTask{
		ID:        42,
		Status:    VideoStatusSucceeded,
		ResultURL: srv.URL + "/out.mp4",
	}
	repo.tasks[task.ID] = task

	svc := NewVideoGatewayService(repo, nil, nil)
	svc.ArchiveSucceededVideoResult(context.Background(), task)

	if task.LocalAssetPath == "" {
		t.Fatal("expected local_asset_path to be set")
	}
	if !strings.HasPrefix(task.LocalAssetPath, "assets/video/42/") {
		t.Fatalf("unexpected path %q", task.LocalAssetPath)
	}
	abs, err := ResolveLocalAssetAbsPath(task.LocalAssetPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "fake-video-bytes" {
		t.Fatalf("unexpected file body %q", body)
	}
}

func TestArchiveSucceededVideoResultFailureDoesNotPanic(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	prev := videoAssetURLValidator
	videoAssetURLValidator = func(string) error { return nil }
	t.Cleanup(func() { videoAssetURLValidator = prev })

	repo := &memoryVideoGatewayRepo{tasks: map[int64]*VideoTask{}}
	task := &VideoTask{
		ID:        7,
		Status:    VideoStatusSucceeded,
		ResultURL: "http://127.0.0.1:1/does-not-exist.mp4",
	}
	svc := NewVideoGatewayService(repo, nil, nil)
	svc.ArchiveSucceededVideoResult(context.Background(), task)
	if task.LocalAssetPath != "" {
		t.Fatalf("failed download should not set path, got %q", task.LocalAssetPath)
	}
}

func TestResolveLocalAssetAbsPathRejectsTraversal(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	if _, err := ResolveLocalAssetAbsPath("../etc/passwd"); err == nil {
		t.Fatal("expected rejection")
	}
	if _, err := ResolveLocalAssetAbsPath("assets/other/x"); err == nil {
		t.Fatal("expected rejection outside video assets")
	}
}

func TestGuessVideoExt(t *testing.T) {
	t.Parallel()
	if guessVideoExt("https://cdn.example/a.webm?x=1") != ".webm" {
		t.Fatal("webm")
	}
	if guessVideoExt("https://cdn.example/a") != ".mp4" {
		t.Fatal("default mp4")
	}
}
