package service

import (
	"context"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The browser helper is opt-in for installations that have Node and Chromium
// on the same machine as the server. Its capability is sent only over stdin.
type accountVaultAutoWorker struct {
	node, script, origin string
	mu                   sync.Mutex
	actors               map[int64]*autoWorkerState
}

type autoWorkerState struct {
	running   bool
	lastTry   time.Time
	status    string
	errorCode string
}

// The helper prints only a symbolic WorkerError code on fatal exit. Discard
// everything else, including browser diagnostics that may contain account data.
type autoWorkerErrorSink struct {
	mu   sync.Mutex
	tail string
	code string
	lastPause string
}

var autoWorkerErrorPattern = regexp.MustCompile(`助手停止：([A-Z][A-Z0-9_]{2,63})。`)
var autoWorkerPausePattern = regexp.MustCompile(`账号 #([0-9]{1,12})：已暂停（([A-Z][A-Z0-9_]{2,63})）`)

func (sink *autoWorkerErrorSink) Write(data []byte) (int, error) {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	length := len(data)
	if length > 1024 {
		data = data[length-1024:]
	}
	chunk := sink.tail + string(data)
	if match := autoWorkerErrorPattern.FindStringSubmatch(chunk); match != nil {
		sink.code = strings.ToLower(match[1])
	}
	if match := autoWorkerPausePattern.FindStringSubmatch(chunk); match != nil {
		// Log only the numeric vault ID and a bounded machine code. The helper's
		// free-form output can contain sensitive provider data and is discarded.
		pause := match[1] + ":" + match[2]
		if pause != sink.lastPause {
			log.Printf("account vault auto-worker paused: account_id=%s code=%s", match[1], match[2])
			sink.lastPause = pause
		}
	}
	if len(chunk) > 128 {
		chunk = chunk[len(chunk)-128:]
	}
	sink.tail = chunk
	return length, nil
}

func (sink *autoWorkerErrorSink) errorCode() string {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return sink.code
}

func newAccountVaultAutoWorker() *accountVaultAutoWorker {
	if os.Getenv("ACCOUNT_VAULT_AUTO_WORKER") != "1" {
		return nil
	}
	return &accountVaultAutoWorker{
		node:   os.Getenv("ACCOUNT_VAULT_AUTO_WORKER_NODE"),
		script: os.Getenv("ACCOUNT_VAULT_AUTO_WORKER_SCRIPT"),
		origin: os.Getenv("ACCOUNT_VAULT_AUTO_WORKER_ORIGIN"),
		actors: make(map[int64]*autoWorkerState),
	}
}

func validLocalWorkerOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	host := u.Hostname()
	if host != "localhost" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return false
	}
	return u.Port() != ""
}

func (w *accountVaultAutoWorker) configured() bool {
	if w == nil || !filepath.IsAbs(w.node) || !filepath.IsAbs(w.script) || !validLocalWorkerOrigin(w.origin) {
		return false
	}
	for _, path := range []string{w.node, w.script} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

func (w *accountVaultAutoWorker) state(actorID int64) (string, string) {
	if w == nil {
		return "manual", ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if state := w.actors[actorID]; state != nil {
		return state.status, state.errorCode
	}
	return "ready", ""
}

func autoWorkerEnvironment(node string) []string {
	keys := []string{"SystemRoot", "WINDIR", "USERPROFILE", "LOCALAPPDATA", "APPDATA", "TEMP", "TMP", "HOME", "HOMEDRIVE", "PROGRAMFILES", "PROGRAMFILES(X86)", "LANG", "HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY", "ACCOUNT_VAULT_BROWSER_PROXY"}
	env := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	env = append(env, "PATH="+filepath.Dir(node)+string(os.PathListSeparator)+os.Getenv("PATH"))
	return env
}

// ensure is called after an authorized queue/resume and when querying an
// already queued job. A failed launch is retried after a short backoff.
func (w *accountVaultAutoWorker) ensure(ctx context.Context, rotation *AccountVaultRotationService, actorID int64) (string, string) {
	if w == nil {
		return "manual", ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	state := w.actors[actorID]
	if state == nil {
		state = &autoWorkerState{}
		w.actors[actorID] = state
	}
	if state.running || time.Since(state.lastTry) < 30*time.Second {
		return state.status, state.errorCode
	}
	state.lastTry = time.Now()
	if !w.configured() {
		state.status, state.errorCode = "error", "not_configured"
		return state.status, state.errorCode
	}
	grant, err := rotation.CreateWorkerToken(ctx, actorID)
	if err != nil {
		state.status, state.errorCode = "error", "token_failed"
		return state.status, state.errorCode
	}
	errorSink := &autoWorkerErrorSink{}
	wait, launchCode := launchAutoWorker(w.node, w.script, w.origin, grant.Token, errorSink)
	grant.Token = ""
	if launchCode != "" {
		state.status, state.errorCode = "error", launchCode
		return state.status, state.errorCode
	}
	state.running, state.status, state.errorCode = true, "starting", ""
	go func() {
		err := wait()
		w.mu.Lock()
		defer w.mu.Unlock()
		state.running = false
		state.status = "error"
		state.errorCode = "worker_exited"
		if code := errorSink.errorCode(); code != "" {
			state.errorCode = code
		}
		if err == nil {
			state.status, state.errorCode = "stopped", ""
		}
		if state.errorCode != "" {
			log.Printf("account vault auto-worker stopped: code=%s", state.errorCode)
		}
	}()
	return state.status, state.errorCode
}

func (s *AccountVaultRotationService) decorateWorker(actorID int64, job *AccountVaultRotationDTO) {
	if job == nil || (job.Status != "queued" && job.Status != "running") {
		return
	}
	job.WorkerStatus, job.WorkerErrorCode = s.autoWorker.state(actorID)
}

func (s *AccountVaultRotationService) ensureWorker(ctx context.Context, actorID int64) {
	if s.autoWorker != nil {
		s.autoWorker.ensure(ctx, s, actorID)
	}
}
