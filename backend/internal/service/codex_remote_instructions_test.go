package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

// stubCodexRemoteModelsServer serves a synthetic codex-rs models.json and points
// the package-level URL var at it, restoring on cleanup (prior art:
// chatgptCodexModelsURL package-var stubbing).
func stubCodexRemoteModelsServer(t *testing.T, models []codexRemoteModel, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		_, _ = w.Write([]byte(codexRemoteModelsJSON(models)))
	}))
	t.Cleanup(srv.Close)
	old := codexRemoteModelsURL
	codexRemoteModelsURL = srv.URL
	t.Cleanup(func() { codexRemoteModelsURL = old })
	return srv
}

func codexRemoteModelsJSON(models []codexRemoteModel) string {
	var b strings.Builder
	b.WriteString(`{"models":[`)
	for i, m := range models {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"slug":"` + m.Slug + `","model_messages":{"instructions_template":`)
		b.WriteString(`"` + strings.ReplaceAll(m.ModelMessages.InstructionsTemplate, `"`, `\"`) + `"}}`)
	}
	b.WriteString(`]}`)
	return b.String()
}

func newTestCodexRemoteSource(t *testing.T, models []codexRemoteModel) *codexRemoteInstructionsSource {
	t.Helper()
	stubCodexRemoteModelsServer(t, models, 0)
	src := newCodexRemoteInstructionsSource(&http.Client{})
	src.syncFetch = true
	return src
}

// setTestCodexRemoteSource installs src as the package-level lookup used by the
// integration points and restores the previous one on cleanup.
func setTestCodexRemoteSource(t *testing.T, src *codexRemoteInstructionsSource) func() {
	t.Helper()
	src.syncFetch = true
	old := remoteCodexInstructionsLookup
	remoteCodexInstructionsLookup = src.instructionsFor
	return func() { remoteCodexInstructionsLookup = old }
}

func TestCodexRemoteInstructionsSource(t *testing.T) {
	models := []codexRemoteModel{
		{
			Slug:          "gpt-99-fresh",
			ModelMessages: codexRemoteModelMessages{InstructionsTemplate: "You are Codex, an agent based on GPT-99."},
		},
	}

	t.Run("returns template for exact slug", func(t *testing.T) {
		s := newTestCodexRemoteSource(t, models)
		got, ok := s.instructionsFor("gpt-99-fresh")
		if !ok || got != "You are Codex, an agent based on GPT-99." {
			t.Fatalf("instructionsFor(gpt-99-fresh) = %q, %v; want exact template", got, ok)
		}
	})

	t.Run("fetch failure returns false", func(t *testing.T) {
		stubCodexRemoteModelsServer(t, nil, http.StatusInternalServerError)
		s := newCodexRemoteInstructionsSource(&http.Client{})
		s.syncFetch = true
		if _, ok := s.instructionsFor("gpt-99-fresh"); ok {
			t.Fatal("instructionsFor on fetch failure ok = true, want false")
		}
	})

	t.Run("async refresh keeps the request path unblocked and dedups", func(t *testing.T) {
		var requests int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requests, 1)
			time.Sleep(30 * time.Millisecond)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		old := codexRemoteModelsURL
		codexRemoteModelsURL = srv.URL
		t.Cleanup(func() { codexRemoteModelsURL = old })

		s := newCodexRemoteInstructionsSource(&http.Client{})
		const n = 10
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, _ = s.instructionsFor("gpt-99-fresh")
			}()
		}
		close(start)
		wg.Wait()

		// 后台刷新在途：等待恰好一个上游请求落地
		deadline := time.Now().Add(2 * time.Second)
		for atomic.LoadInt32(&requests) == 0 && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond) // 等 in-flight 期间的重复触发都落地
		if got := atomic.LoadInt32(&requests); got != 1 {
			t.Errorf("upstream requests = %d, want 1 (refreshing flag dedup)", got)
		}
	})
}

func TestCodexRemoteInstructionsIntegration(t *testing.T) {
	models := []codexRemoteModel{
		{
			Slug:          "gpt-99-fresh",
			ModelMessages: codexRemoteModelMessages{InstructionsTemplate: "You are Codex, an agent based on GPT-99."},
		},
	}
	restore := setTestCodexRemoteSource(t, newTestCodexRemoteSource(t, models))
	defer restore()

	t.Run("fallback descriptor uses remote template for unknown model", func(t *testing.T) {
		got := codexInstructionsTemplateForModel("gpt-99-fresh")
		if !strings.Contains(got, "GPT-99") {
			t.Fatal("fallback descriptor did not use remote template for unknown model")
		}
	})

	t.Run("known model keeps embed chain (no remote consult)", func(t *testing.T) {
		if got := codexInstructionsTemplateForModel("gpt-5.2"); got != openai.CodexBaseInstructionsForModel("gpt-5.2") {
			t.Fatal("known model template diverged from embed chain")
		}
	})

	t.Run("synth path uses remote template for unknown model", func(t3 *testing.T) {
		t := t3
		got := defaultCodexSynthInstructions("gpt-99-fresh")
		if !strings.Contains(got, "GPT-99") {
			t.Fatal("synth path did not use remote template for unknown model")
		}
	})

	t.Run("non-GPT remote slug gets identity rewritten", func(t4 *testing.T) {
		t := t4
		ossModels := []codexRemoteModel{
			{
				Slug:          "gpt-oss-next",
				ModelMessages: codexRemoteModelMessages{InstructionsTemplate: "You are GPT-99 running in the Codex CLI."},
			},
		}
		restore := setTestCodexRemoteSource(t, newTestCodexRemoteSource(t, ossModels))
		defer restore()
		got := codexInstructionsTemplateForModel("gpt-oss-next")
		if strings.Contains(got, "You are GPT-99") {
			t.Fatal("GPT identity was not rewritten for non-GPT-family model")
		}
	})

	t.Run("remote miss falls back to embed chain", func(t5 *testing.T) {
		t := t5
		stubCodexRemoteModelsServer(t, nil, http.StatusInternalServerError)
		restore := setTestCodexRemoteSource(t, newCodexRemoteInstructionsSource(&http.Client{}))
		defer restore()
		got := codexInstructionsTemplateForModel("gpt-99-other")
		if strings.TrimSpace(got) == "" {
			t.Fatal("empty instructions on remote failure")
		}
		if got != openai.CodexBaseInstructionsForModel("gpt-99-other") {
			t.Fatal("remote failure did not fall back to embed chain")
		}
	})
}
