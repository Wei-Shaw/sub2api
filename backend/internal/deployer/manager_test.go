package deployer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRunner struct {
	mu             sync.Mutex
	candidate      bool
	reloadFailures int
	commands       []string
	runtimeState   *atomic.Value
	versions       map[string]string
	unhealthy      map[string]bool
	stopped        map[string]bool
}

type ambiguousReloadRunner struct {
	base         *fakeRunner
	upstreamPath string
	livePort     *atomic.Int64
	reloadCalls  atomic.Int64
}

type stickyConnectionReloadRunner struct {
	base         *fakeRunner
	upstreamPath string
	liveSlot     *atomic.Value
	slotsByPort  map[int]string
}

type delayedHealthRunner struct {
	base      *fakeRunner
	remaining atomic.Int64
	calls     atomic.Int64
}

type routeDriftRunner struct {
	base         *delayedHealthRunner
	upstreamPath string
	driftPort    int
	drifted      atomic.Bool
}

type foreignOwnershipRunner struct {
	base   *fakeRunner
	labels string
}

type identityFailureRunner struct {
	base          *fakeRunner
	name          string
	expectedID    string
	replacementID string
	transient     bool
	renamedTo     string
	listError     bool
	absent        bool
	commands      []string
}

func (r *identityFailureRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	command := strings.Join(append([]string{name}, args...), " ")
	r.commands = append(r.commands, command)
	if strings.Contains(command, "container inspect --format") && args[len(args)-1] == r.expectedID {
		if r.renamedTo != "" {
			return fmt.Sprintf(
				`{"ID":%q,"Name":%q,"Labels":{"com.docker.compose.project":"sub2api","com.docker.compose.service":"sub2api"}}`,
				r.expectedID, "/"+r.renamedTo,
			), nil
		}
		return "", errors.New("transient inspect failure")
	}
	if strings.Contains(command, "container ls --all --no-trunc") {
		filter := ""
		for index, arg := range args {
			if arg == "--filter" && index+1 < len(args) {
				filter = args[index+1]
			}
		}
		if filter == "id="+r.expectedID {
			if r.transient {
				return fmt.Sprintf("%q", r.expectedID), nil
			}
			return "", nil
		}
		if strings.HasPrefix(filter, "name=") {
			if r.listError {
				return "", errors.New("daemon list failure")
			}
			if r.absent {
				return "", nil
			}
			if r.replacementID != "" {
				return fmt.Sprintf("%q", r.replacementID), nil
			}
			return fmt.Sprintf("%q", r.expectedID), nil
		}
	}
	return r.base.Run(ctx, env, name, args...)
}

func (r *foreignOwnershipRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	command := strings.Join(append([]string{name}, args...), " ")
	if strings.Contains(command, "container inspect --format") {
		container := fakeContainerName(args[len(args)-1])
		return fmt.Sprintf(`{"ID":%q,"Name":%q,"Labels":%s}`, fakeContainerID(container), "/"+container, r.labels), nil
	}
	return r.base.Run(ctx, env, name, args...)
}

func fakeContainerID(name string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte("container:"+name)))
}

func fakeContainerName(target string) string {
	for _, name := range []string{"sub2api", "sub2api-green", "sub2api-blue", "blue"} {
		if target == fakeContainerID(name) {
			return name
		}
	}
	return target
}

func (r *delayedHealthRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	command := strings.Join(append([]string{name}, args...), " ")
	if strings.Contains(command, "inspect --format {{if .State.Health}}") {
		r.calls.Add(1)
		if r.remaining.Add(-1) >= 0 {
			return "starting", nil
		}
	}
	return r.base.Run(ctx, env, name, args...)
}

func (r *routeDriftRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	output, err := r.base.Run(ctx, env, name, args...)
	command := strings.Join(append([]string{name}, args...), " ")
	if err == nil && strings.Contains(command, "inspect --format {{if .State.Health}}") && r.drifted.CompareAndSwap(false, true) {
		content := []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", r.driftPort))
		if writeErr := atomicWrite(r.upstreamPath, content, 0644); writeErr != nil {
			return "", writeErr
		}
	}
	return output, err
}

func (r *stickyConnectionReloadRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	if name == "/usr/bin/systemctl" {
		port, err := readUpstreamPort(r.upstreamPath)
		if err != nil {
			return "", err
		}
		r.liveSlot.Store(r.slotsByPort[port])
		return "reloaded", nil
	}
	return r.base.Run(ctx, env, name, args...)
}

func (r *ambiguousReloadRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	if name == "/usr/bin/systemctl" {
		call := r.reloadCalls.Add(1)
		if call == 1 {
			port, err := readUpstreamPort(r.upstreamPath)
			if err != nil {
				return "", err
			}
			r.livePort.Store(int64(port))
			return "reload returned an error after applying configuration", errors.New("ambiguous reload result")
		}
		return "restore reload returned an error without applying configuration", errors.New("ambiguous restore reload result")
	}
	return r.base.Run(ctx, env, name, args...)
}

func (f *fakeRunner) Run(_ context.Context, _ map[string]string, name string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	command := strings.Join(append([]string{name}, args...), " ")
	f.commands = append(f.commands, command)
	if name == "fake-nginx-dump" && len(args) == 2 {
		site, err := os.ReadFile(args[0])
		if err != nil {
			return "", err
		}
		upstream, err := os.ReadFile(args[1])
		if err != nil {
			return "", err
		}
		return string(site) + "\n" + string(upstream), nil
	}
	if strings.Contains(command, "image inspect --format {{json .RepoDigests}}") {
		return `["ghcr.io/ssharkkky/sub2api@sha256:` + strings.Repeat("a", 64) + `"]`, nil
	}
	if strings.Contains(command, "image inspect --format {{json .Config.Labels}}") {
		return `{"org.opencontainers.image.source":"https://github.com/ssharkkky/sub2api","io.tokensupply.sub2api.update-protocol":"2"}`, nil
	}
	if strings.Contains(command, "container inspect --format") {
		container := fakeContainerName(args[len(args)-1])
		if container == "sub2api-green" && !f.candidate {
			return "", fmt.Errorf("container inspect failed")
		}
		return fmt.Sprintf(
			`{"ID":%q,"Name":%q,"Labels":{"com.docker.compose.project":"sub2api","com.docker.compose.service":"sub2api"}}`,
			fakeContainerID(container), "/"+container,
		), nil
	}
	if strings.Contains(command, "container ls --all --no-trunc") {
		return "", nil
	}
	if strings.Contains(command, "inspect --format {{.State.Running}}") {
		container := fakeContainerName(args[len(args)-1])
		if f.stopped[container] {
			return "false", nil
		}
		return "true", nil
	}
	if strings.Contains(command, " stop --time ") {
		container := fakeContainerName(args[len(args)-1])
		if f.stopped == nil {
			f.stopped = make(map[string]bool)
		}
		f.stopped[container] = true
		return "stopped", nil
	}
	if strings.Contains(command, " start ") {
		container := fakeContainerName(args[len(args)-1])
		delete(f.stopped, container)
		return "started", nil
	}
	if strings.Contains(command, "inspect --format {{json .Config.Labels}}") {
		return `{"com.docker.compose.project":"sub2api","com.docker.compose.service":"sub2api"}`, nil
	}
	if strings.Contains(command, " compose ") && strings.Contains(command, " run ") {
		f.candidate = true
		return "candidate-id", nil
	}
	if strings.Contains(command, "kill --signal=USR1") && f.runtimeState != nil {
		f.runtimeState.Store("active")
		return "signaled", nil
	}
	if strings.Contains(command, "inspect sub2api-green") && !f.candidate {
		return "", fmt.Errorf("not found")
	}
	if strings.Contains(command, "inspect --format {{if .State.Health}}") {
		for container := range f.unhealthy {
			if strings.HasSuffix(command, " "+container) || strings.HasSuffix(command, " "+fakeContainerID(container)) {
				return "unhealthy", nil
			}
		}
		return "healthy", nil
	}
	if strings.Contains(command, "inspect --format {{.Config.Image}}") {
		return "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1", nil
	}
	for container, version := range f.versions {
		if strings.Contains(command, "exec "+container+" /app/sub2api --version") || strings.Contains(command, "exec "+fakeContainerID(container)+" /app/sub2api --version") {
			return "Sub2API " + version + " (commit: test)", nil
		}
	}
	if strings.Contains(command, "exec "+fakeContainerID("sub2api-green")+" /app/sub2api --version") {
		return "Sub2API 0.1.2-ts.1 (commit: test)", nil
	}
	if strings.Contains(command, "exec "+fakeContainerID("sub2api")+" /app/sub2api --version") {
		return "Sub2API 0.1.1-ts.1 (commit: test)", nil
	}
	if name == "/usr/bin/systemctl" && f.reloadFailures > 0 {
		f.reloadFailures--
		return "reload failed", fmt.Errorf("reload failed")
	}
	return "ok", nil
}

func testConfig(t *testing.T, candidatePort int) Config {
	t.Helper()
	dir := t.TempDir()
	initialListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	initialPort := initialListener.Addr().(*net.TCPAddr).Port
	initialServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = initialServer.Serve(initialListener) }()
	t.Cleanup(func() { _ = initialServer.Close() })
	upstream := filepath.Join(dir, "upstream.conf")
	if err := os.WriteFile(upstream, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", initialPort)), 0644); err != nil {
		t.Fatal(err)
	}
	site := filepath.Join(dir, "site.conf")
	if err := os.WriteFile(site, []byte("proxy_pass http://sub2api_managed;\n"), 0644); err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		port, readErr := readUpstreamPort(upstream)
		if readErr != nil {
			http.Error(w, readErr.Error(), http.StatusBadGateway)
			return
		}
		request, requestErr := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(port)+r.URL.Path, nil)
		if requestErr != nil {
			http.Error(w, requestErr.Error(), http.StatusBadGateway)
			return
		}
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			http.Error(w, requestErr.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	t.Cleanup(proxy.Close)
	return Config{
		SocketPath:      filepath.Join(dir, "deployer.sock"),
		SocketMode:      0660,
		SocketGID:       os.Getgid(),
		StatePath:       filepath.Join(dir, "state.json"),
		ImageStatePath:  filepath.Join(dir, ".deployer.env"),
		ImageRepository: "ghcr.io/ssharkkky/sub2api",
		RequiredImageLabels: map[string]string{
			"org.opencontainers.image.source":        "https://github.com/ssharkkky/sub2api",
			"io.tokensupply.sub2api.update-protocol": "2",
		},
		DockerBinary:        "docker",
		ComposeWorkDir:      dir,
		ComposeProject:      "sub2api",
		ComposeEnvFiles:     []string{filepath.Join(dir, ".env"), filepath.Join(dir, ".deployer.env")},
		ComposeFiles:        []string{filepath.Join(dir, "compose.yaml")},
		ComposeService:      "sub2api",
		ImageEnvironment:    "SUB2API_IMAGE",
		ContainerPort:       8080,
		DeploymentStatePath: filepath.Join(dir, "runtime", "active-slot"),
		DeploymentStateFile: "/run/sub2api-deployment/active-slot",
		Slots:               []Slot{{Name: "blue", Host: "127.0.0.1", Port: initialPort}, {Name: "sub2api-green", Host: "127.0.0.1", Port: candidatePort}},
		InitialContainer:    "sub2api",
		InitialVersion:      "0.1.1-ts.1",
		NginxUpstreamPath:   upstream,
		NginxSitePath:       site,
		NginxUpstreamName:   "sub2api_managed",
		NginxProbeURL:       proxy.URL + "/health",
		NginxTestCommand:    []string{"/usr/sbin/nginx", "-t"},
		NginxDumpCommand:    []string{"fake-nginx-dump", site, upstream},
		NginxReloadCommand: []string{
			"/usr/bin/systemctl", "reload", "nginx",
		},
		HealthPath:        "/health",
		HealthTimeout:     Duration{Duration: time.Second},
		StabilizeDuration: Duration{Duration: 10 * time.Millisecond},
		DrainDuration:     Duration{Duration: time.Millisecond},
		DrainTimeout:      Duration{Duration: time.Second},
		StopTimeout:       Duration{Duration: time.Second},
	}
}

func TestManagedDeploymentSucceedsAndPinsDigest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	var runtimeState atomic.Value
	runtimeState.Store("standby")
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":%q,"slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, runtimeState.Load().(string))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	runner := &fakeRunner{runtimeState: &runtimeState}
	cfg := testConfig(t, port)
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Start(DeployRequest{
		Action:                 "update",
		TargetVersion:          "0.1.2-ts.1",
		ExpectedCurrentVersion: "0.1.1-ts.1",
		RequestID:              "test-request-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		job, err = manager.Job(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != JobStatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if job.Status != JobStatusSucceeded {
		t.Fatalf("deployment status=%s error=%s rollback=%s", job.Status, job.Error, job.RollbackError)
	}
	if manager.Health().ActivePort != port {
		t.Fatalf("active port=%d, want %d", manager.Health().ActivePort, port)
	}
	imageState, err := os.ReadFile(cfg.ImageStatePath)
	if err != nil {
		t.Fatal(err)
	}
	want := "SUB2API_IMAGE=ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64)
	if strings.TrimSpace(string(imageState)) != want {
		t.Fatalf("image state=%q, want %q", strings.TrimSpace(string(imageState)), want)
	}
	upstreamPort, err := readUpstreamPort(cfg.NginxUpstreamPath)
	if err != nil || upstreamPort != port {
		t.Fatalf("upstream port=%d err=%v", upstreamPort, err)
	}
	if !job.BackgroundActivated {
		t.Fatal("job did not record background activation")
	}
	marker, err := os.ReadFile(cfg.DeploymentStatePath)
	if err != nil || strings.TrimSpace(string(marker)) != "sub2api-green" {
		t.Fatalf("active slot marker=%q err=%v", marker, err)
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	standbyIndex := strings.Index(commands, "-e DEPLOYMENT_STANDBY=true")
	stopIndex := strings.Index(commands, "stop --time 1 "+fakeContainerID("sub2api"))
	activateIndex := strings.Index(commands, "kill --signal=USR1 "+fakeContainerID("sub2api-green"))
	if standbyIndex < 0 || stopIndex < 0 || activateIndex < 0 || stopIndex > activateIndex {
		t.Fatalf("candidate was not started standby and activated after old stop:\n%s", commands)
	}
}

func TestDrainTimeoutKeepsBothContainersAndReconcileCompletesLater(t *testing.T) {
	oldListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	oldPort := oldListener.Addr().(*net.TCPAddr).Port
	var blockers atomic.Int64
	blockers.Store(1)
	oldServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := blockers.Load()
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":"active"},"drain":{"supported":true,"active_requests":%d,"hijacked_connections":0,"blockers":%d}}`, count, count)
	})}
	go func() { _ = oldServer.Serve(oldListener) }()
	t.Cleanup(func() { _ = oldServer.Close() })

	candidateListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	candidatePort := candidateListener.Addr().(*net.TCPAddr).Port
	var runtimeState atomic.Value
	runtimeState.Store("standby")
	candidateServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":%q,"slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, runtimeState.Load().(string))
	})}
	go func() { _ = candidateServer.Serve(candidateListener) }()
	t.Cleanup(func() { _ = candidateServer.Close() })

	cfg := testConfig(t, candidatePort)
	cfg.Slots[0].Name = "sub2api-blue"
	cfg.Slots[0].Port = oldPort
	cfg.DrainDuration = Duration{Duration: 5 * time.Millisecond}
	cfg.DrainTimeout = Duration{Duration: 60 * time.Millisecond}
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", oldPort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := &Job{
		ID:                   "drain-timeout-0001",
		Action:               "update",
		TargetVersion:        "0.1.2-ts.1",
		TargetImage:          "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
		Status:               JobStatusRunning,
		Stage:                StageStartingCandidate,
		OldContainer:         "sub2api",
		OldContainerID:       fakeContainerID("sub2api"),
		OldSlot:              "sub2api-blue",
		OldSlotCaptured:      true,
		CandidateContainer:   "sub2api-green",
		CandidateContainerID: fakeContainerID("sub2api-green"),
		CandidateSlot:        "sub2api-green",
		CandidatePort:        candidatePort,
		TrafficState:         TrafficStateOld,
		CreatedAt:            now,
		StartedAt:            now,
		UpdatedAt:            now,
	}
	runner := &fakeRunner{candidate: true, runtimeState: &runtimeState}
	manager := &Manager{
		cfg:        cfg,
		runner:     runner,
		httpClient: &http.Client{Timeout: time.Second},
		now:        time.Now,
		state: State{
			ActiveSlot:        "sub2api-blue",
			ActiveContainer:   "sub2api",
			ActiveContainerID: fakeContainerID("sub2api"),
			ActivePort:        oldPort,
			ActiveVersion:     "0.1.1-ts.1",
			ActiveImage:       "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
			Job:               job,
		},
	}

	manager.finishCandidateDeployment(context.Background(), job.ID, cfg.Slots[1], true)
	degraded, err := manager.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if degraded.Status != JobStatusDegraded || !manager.Health().Degraded || degraded.TrafficState != TrafficStateCandidate {
		t.Fatalf("drain timeout did not preserve a degraded candidate route: job=%+v health=%+v", degraded, manager.Health())
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	if strings.Contains(commands, "stop --time 1 "+fakeContainerID("sub2api")) || strings.Contains(commands, "kill --signal=USR1 "+fakeContainerID("sub2api-green")) {
		t.Fatalf("drain timeout stopped the old owner or activated candidate workers:\n%s", commands)
	}
	if port, err := readUpstreamPort(cfg.NginxUpstreamPath); err != nil || port != candidatePort {
		t.Fatalf("candidate route was not preserved: port=%d err=%v", port, err)
	}

	blockers.Store(0)
	manager.cfg.DrainTimeout = Duration{Duration: time.Second}
	if err := manager.Reconcile(context.Background(), "sub2api-green"); err != nil {
		t.Fatalf("reconcile after drain completed: %v", err)
	}
	if health := manager.Health(); health.Degraded || health.ActiveContainer != "sub2api-green" {
		t.Fatalf("reconciliation did not activate candidate: %+v", health)
	}
	if runtimeState.Load().(string) != "active" {
		t.Fatalf("candidate background runtime state=%q, want active", runtimeState.Load())
	}
}

func TestLegacyDrainRequiresExplicitReconcileOverride(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active"}}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	cfg := testConfig(t, 18081)
	manager := &Manager{cfg: cfg, httpClient: &http.Client{Timeout: time.Second}, now: time.Now}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := manager.waitForOldDrain(context.Background(), port, false); !errors.Is(err, ErrDrainUnobservable) {
		t.Fatalf("legacy drain error=%v, want ErrDrainUnobservable", err)
	}
	if err := manager.waitForOldDrain(context.Background(), port, true); err != nil {
		t.Fatalf("explicit legacy override failed: %v", err)
	}
}

func TestNginxReloadFailureRestoresPreviousUpstream(t *testing.T) {
	cfg := testConfig(t, 18081)
	runner := &fakeRunner{reloadFailures: 1}
	manager := &Manager{cfg: cfg, runner: runner, httpClient: &http.Client{Timeout: time.Second}, now: time.Now}
	_, err := manager.switchTraffic(context.Background(),
		trafficEndpoint{Port: 18081, Slot: "sub2api-green"},
		trafficEndpoint{Port: cfg.Slots[0].Port, Slot: "", AllowEmpty: true},
	)
	if err == nil {
		t.Fatal("expected reload failure")
	}
	port, readErr := readUpstreamPort(cfg.NginxUpstreamPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if port != cfg.Slots[0].Port {
		t.Fatalf("restored port=%s, want %d", strconv.Itoa(port), cfg.Slots[0].Port)
	}
}

func TestPersistSuccessfulDeploymentDoesNotCommitMemoryStateWhenDiskWriteFails(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.StatePath = t.TempDir()
	manager := &Manager{
		cfg: cfg,
		now: time.Now,
		state: State{
			ActiveSlot:      "blue",
			ActiveContainer: "sub2api",
			ActivePort:      cfg.Slots[0].Port,
			ActiveVersion:   "0.1.1-ts.1",
			ActiveImage:     "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		},
	}
	job := &Job{
		TargetVersion:      "0.1.2-ts.1",
		TargetImage:        "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
		CandidateSlot:      "sub2api-green",
		CandidateContainer: "sub2api-green",
		CandidatePort:      18081,
	}

	if err := manager.persistSuccessfulDeployment(job); err == nil {
		t.Fatal("expected state persistence failure")
	}
	if manager.state.ActiveContainer != "sub2api" || manager.state.ActivePort != cfg.Slots[0].Port || manager.state.PreviousContainer != "" {
		t.Fatalf("in-memory deployment state changed after failed persistence: %+v", manager.state)
	}
}

func TestStartIsIdempotentForSameRequest(t *testing.T) {
	cfg := testConfig(t, 18081)
	runner := &fakeRunner{}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	req := DeployRequest{Action: "update", TargetVersion: "0.1.2-ts.1", RequestID: "same-request-0001"}
	first, err := manager.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("job ids differ: %s != %s", first.ID, second.ID)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		current, jobErr := manager.Job(first.ID)
		if jobErr != nil {
			t.Fatal(jobErr)
		}
		if current.Status != JobStatusRunning {
			if current.Status != JobStatusFailed {
				t.Fatalf("status=%s, want failed", current.Status)
			}
			port, readErr := readUpstreamPort(cfg.NginxUpstreamPath)
			if readErr != nil || port != cfg.Slots[0].Port {
				t.Fatalf("rollback upstream port=%d err=%v", port, readErr)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("deployment goroutine did not finish")
}

func TestStartRejectsReusedRequestIDWithDifferentExpectedVersion(t *testing.T) {
	cfg := testConfig(t, 18081)
	manager, err := NewManager(cfg, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	req := DeployRequest{
		Action:                 "update",
		TargetVersion:          "0.1.2-ts.1",
		ExpectedCurrentVersion: "0.1.1-ts.1",
		RequestID:              "expected-conflict-0001",
	}
	job, err := manager.Start(req)
	if err != nil {
		t.Fatal(err)
	}
	req.ExpectedCurrentVersion = "0.1.0-ts.1"
	if _, err := manager.Start(req); !errors.Is(err, ErrRequestConflict) {
		t.Fatalf("Start error=%v, want ErrRequestConflict", err)
	}
	_ = waitForFinishedJob(t, manager, job.ID)
}

func TestRequestIDRemainsIdempotentAfterLaterJob(t *testing.T) {
	cfg := testConfig(t, 18081)
	manager, err := NewManager(cfg, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	firstRequest := DeployRequest{Action: "update", TargetVersion: "0.1.2-ts.1", RequestID: "historical-request-0001"}
	first, err := manager.Start(firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	first = waitForFinishedJob(t, manager, first.ID)
	second, err := manager.Start(DeployRequest{Action: "update", TargetVersion: "0.1.2-ts.1", RequestID: "historical-request-0002"})
	if err != nil {
		t.Fatal(err)
	}

	archived, err := manager.Job(first.ID)
	if err != nil {
		t.Fatalf("Job archived request: %v", err)
	}
	if archived.ID != first.ID || archived.Status != first.Status || !archived.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("archived job changed: got %+v, want %+v", archived, first)
	}
	replayed, err := manager.Start(firstRequest)
	if err != nil {
		t.Fatalf("Start archived request: %v", err)
	}
	if replayed.ID != first.ID || !replayed.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("historical replay returned a new job: %+v", replayed)
	}
	_ = waitForFinishedJob(t, manager, second.ID)

	persisted, err := loadState(cfg.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if found := findJobByRequestID(persisted, first.ID); found == nil || found.ID != first.ID {
		t.Fatalf("persisted state lost archived job: %+v", persisted.JobHistory)
	}
}

func TestArchiveTerminalJobPrunesByAgeAndCount(t *testing.T) {
	now := time.Now().UTC()
	state := State{Job: &Job{ID: "current-terminal", Status: JobStatusSucceeded, UpdatedAt: now}}
	for i := 0; i < maxJobHistory+10; i++ {
		state.JobHistory = append(state.JobHistory, Job{
			ID:        fmt.Sprintf("history-%02d", i),
			Status:    JobStatusSucceeded,
			UpdatedAt: now.Add(-time.Duration(i) * time.Hour),
		})
	}
	state.JobHistory = append(state.JobHistory, Job{
		ID:        "expired-history",
		Status:    JobStatusSucceeded,
		UpdatedAt: now.Add(-jobHistoryTTL - time.Hour),
	})

	archiveTerminalJob(&state, now)
	if len(state.JobHistory) != maxJobHistory {
		t.Fatalf("history length=%d, want %d", len(state.JobHistory), maxJobHistory)
	}
	if state.JobHistory[0].ID != "current-terminal" {
		t.Fatalf("newest terminal job was not archived first: %+v", state.JobHistory[0])
	}
	if findJobByRequestID(state, "expired-history") != nil {
		t.Fatal("expired job remained in bounded history")
	}
}

func TestRestartRecoveryFinishesHealthySwitchedCandidate(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	var runtimeState atomic.Value
	runtimeState.Store("active")
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":%q,"slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, runtimeState.Load().(string))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	cfg := testConfig(t, port)
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", port)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:      "blue",
		ActiveContainer: "sub2api",
		ActivePort:      cfg.Slots[0].Port,
		ActiveVersion:   "0.1.1-ts.1",
		ActiveImage:     "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		Job: &Job{
			ID:                 "restart-recovery-1",
			Action:             "update",
			TargetVersion:      "0.1.2-ts.1",
			TargetImage:        "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
			Status:             JobStatusRunning,
			Stage:              StageStabilizing,
			OldContainer:       "sub2api",
			OldSlot:            "blue",
			CandidateContainer: "sub2api-green",
			CandidateSlot:      "sub2api-green",
			CandidatePort:      port,
			TrafficSwitched:    true,
			CreatedAt:          now,
			StartedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{candidate: true, runtimeState: &runtimeState}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Job("restart-recovery-1")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusSucceeded {
		t.Fatalf("status=%s error=%s", job.Status, job.Error)
	}
	if manager.Health().ActiveContainer != "sub2api-green" {
		t.Fatalf("active container=%s", manager.Health().ActiveContainer)
	}
	if manager.state.PreviousContainer != "sub2api" {
		t.Fatalf("previous container=%s", manager.state.PreviousContainer)
	}
}

func TestRestartRecoveryActivatesCandidateWhenPreviousContainerIsAlreadyStopped(t *testing.T) {
	for _, testCase := range []struct {
		name               string
		stage              string
		markerBeforeSignal bool
	}{
		{name: "crash_after_stop_before_handoff_state", stage: StageDraining},
		{name: "crash_after_marker_before_signal", stage: StageActivating, markerBeforeSignal: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			oldListener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			oldPort := oldListener.Addr().(*net.TCPAddr).Port
			oldServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active","slot":"blue"},"drain":{"supported":true,"active_requests":1,"hijacked_connections":0,"blockers":1}}`))
			})}
			go func() { _ = oldServer.Serve(oldListener) }()
			t.Cleanup(func() { _ = oldServer.Close() })

			candidateListener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			candidatePort := candidateListener.Addr().(*net.TCPAddr).Port
			var runtimeState atomic.Value
			runtimeState.Store("standby")
			candidateServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":%q,"slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, runtimeState.Load().(string))
			})}
			go func() { _ = candidateServer.Serve(candidateListener) }()
			t.Cleanup(func() { _ = candidateServer.Close() })

			cfg := testConfig(t, candidatePort)
			cfg.Slots[0].Port = oldPort
			cfg.DrainDuration = Duration{Duration: time.Millisecond}
			cfg.DrainTimeout = Duration{Duration: 25 * time.Millisecond}
			if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", candidatePort)), 0644); err != nil {
				t.Fatal(err)
			}
			if testCase.markerBeforeSignal {
				if err := atomicWrite(cfg.DeploymentStatePath, []byte("sub2api-green\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}

			now := time.Now().UTC()
			state := State{
				ActiveSlot:        "blue",
				ActiveContainer:   "sub2api",
				ActiveContainerID: fakeContainerID("sub2api"),
				ActivePort:        oldPort,
				ActiveVersion:     "0.1.1-ts.1",
				ActiveImage:       "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
				Job: &Job{
					ID:                   "restart-stopped-old-" + testCase.name,
					Action:               "update",
					TargetVersion:        "0.1.2-ts.1",
					FromVersion:          "0.1.1-ts.1",
					FromImage:            "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
					TargetImage:          "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
					Status:               JobStatusRunning,
					Stage:                testCase.stage,
					OldContainer:         "sub2api",
					OldContainerID:       fakeContainerID("sub2api"),
					OldSlot:              "blue",
					OldSlotCaptured:      true,
					HandoffPrepared:      true,
					HandoffContainer:     "sub2api",
					HandoffContainerID:   fakeContainerID("sub2api"),
					CandidateContainer:   "sub2api-green",
					CandidateContainerID: fakeContainerID("sub2api-green"),
					CandidateSlot:        "sub2api-green",
					CandidatePort:        candidatePort,
					TrafficState:         TrafficStateCandidate,
					TrafficSwitched:      true,
					CreatedAt:            now,
					StartedAt:            now,
					UpdatedAt:            now,
				},
				UpdatedAt: now,
			}
			if err := saveState(cfg.StatePath, state); err != nil {
				t.Fatal(err)
			}
			runner := &fakeRunner{
				candidate:    true,
				runtimeState: &runtimeState,
				versions: map[string]string{
					"sub2api":       "0.1.1-ts.1",
					"sub2api-green": "0.1.2-ts.1",
				},
				stopped: map[string]bool{"sub2api": true},
			}

			manager, err := NewManager(cfg, runner)
			if err != nil {
				t.Fatal(err)
			}
			job, err := manager.Job(state.Job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if job.Status != JobStatusSucceeded || !job.BackgroundActivated {
				t.Fatalf("stopped-old recovery did not complete: %+v", job)
			}
			if runtimeState.Load().(string) != "active" {
				t.Fatalf("candidate runtime state=%q, want active", runtimeState.Load())
			}
			marker, err := os.ReadFile(cfg.DeploymentStatePath)
			if err != nil || strings.TrimSpace(string(marker)) != "sub2api-green" {
				t.Fatalf("candidate marker=%q err=%v", marker, err)
			}
			runner.mu.Lock()
			commands := strings.Join(runner.commands, "\n")
			runner.mu.Unlock()
			if !strings.Contains(commands, "kill --signal=USR1 "+fakeContainerID("sub2api-green")) {
				t.Fatalf("recovery did not activate candidate background services:\n%s", commands)
			}
			if strings.Contains(commands, "stop --time 1 "+fakeContainerID("sub2api")) {
				t.Fatalf("recovery tried to stop an already stopped previous container:\n%s", commands)
			}
		})
	}
}

func TestRestartRecoverySkipsDrainAfterJournalBoundOldContainerStopped(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	candidatePort := listener.Addr().(*net.TCPAddr).Port
	var runtimeState atomic.Value
	runtimeState.Store("standby")
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":%q,"slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, runtimeState.Load().(string))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	deadListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	oldPort := deadListener.Addr().(*net.TCPAddr).Port
	if err := deadListener.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig(t, candidatePort)
	cfg.Slots[0].Port = oldPort
	cfg.DrainTimeout = Duration{Duration: 20 * time.Millisecond}
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", candidatePort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	oldID := fakeContainerID("sub2api")
	candidateID := fakeContainerID("sub2api-green")
	state := State{
		ActiveSlot:        "blue",
		ActiveContainer:   "sub2api",
		ActiveContainerID: oldID,
		ActivePort:        oldPort,
		ActiveVersion:     "0.1.1-ts.1",
		ActiveImage:       "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		Job: &Job{
			ID:                   "restart-after-stop-0001",
			Action:               "update",
			TargetVersion:        "0.1.2-ts.1",
			FromVersion:          "0.1.1-ts.1",
			FromImage:            "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
			TargetImage:          "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
			Status:               JobStatusRunning,
			Stage:                StageDraining,
			OldContainer:         "sub2api",
			OldContainerID:       oldID,
			HandoffPrepared:      true,
			HandoffContainer:     "sub2api",
			HandoffContainerID:   oldID,
			OldSlot:              "blue",
			OldSlotCaptured:      true,
			CandidateContainer:   "sub2api-green",
			CandidateContainerID: candidateID,
			CandidateSlot:        "sub2api-green",
			CandidatePort:        candidatePort,
			TrafficState:         TrafficStateCandidate,
			TrafficSwitched:      true,
			CreatedAt:            now,
			StartedAt:            now,
			UpdatedAt:            now,
		},
		UpdatedAt: now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		candidate:    true,
		runtimeState: &runtimeState,
		stopped:      map[string]bool{"sub2api": true},
		versions: map[string]string{
			"sub2api":       "0.1.1-ts.1",
			"sub2api-green": "0.1.2-ts.1",
		},
	}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Job("restart-after-stop-0001")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusSucceeded || !job.BackgroundActivated {
		t.Fatalf("recovery did not activate candidate after old container was already stopped: %+v", job)
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	if strings.Contains(commands, "stop --time 1 "+oldID) {
		t.Fatalf("recovery tried to stop the already-stopped journal-bound container:\n%s", commands)
	}
	if !strings.Contains(commands, "kill --signal=USR1 "+candidateID) {
		t.Fatalf("recovery did not activate candidate background services:\n%s", commands)
	}
	healthProbe := "inspect --format {{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}} " + candidateID
	if count := strings.Count(commands, healthProbe); count != 1 {
		t.Fatalf("already-stopped handoff ran candidate stabilization instead of immediate activation; health probes=%d:\n%s", count, commands)
	}
}

func TestRestartRecoveryCompletesDurableHandoffBeforeOldContainerStop(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	candidatePort := listener.Addr().(*net.TCPAddr).Port
	var runtimeState atomic.Value
	runtimeState.Store("standby")
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":%q,"slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, runtimeState.Load().(string))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	cfg := testConfig(t, candidatePort)
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", candidatePort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	oldID := fakeContainerID("sub2api")
	candidateID := fakeContainerID("sub2api-green")
	state := State{
		ActiveSlot:        "blue",
		ActiveContainer:   "sub2api",
		ActiveContainerID: oldID,
		ActivePort:        cfg.Slots[0].Port,
		ActiveVersion:     "0.1.1-ts.1",
		ActiveImage:       "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		Job: &Job{
			ID:                   "restart-before-stop-0001",
			Action:               "update",
			TargetVersion:        "0.1.2-ts.1",
			FromVersion:          "0.1.1-ts.1",
			FromImage:            "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
			TargetImage:          "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
			Status:               JobStatusRunning,
			Stage:                StageDraining,
			OldContainer:         "sub2api",
			OldContainerID:       oldID,
			HandoffPrepared:      true,
			HandoffContainer:     "sub2api",
			HandoffContainerID:   oldID,
			OldSlot:              "blue",
			OldSlotCaptured:      true,
			CandidateContainer:   "sub2api-green",
			CandidateContainerID: candidateID,
			CandidateSlot:        "sub2api-green",
			CandidatePort:        candidatePort,
			TrafficState:         TrafficStateCandidate,
			TrafficSwitched:      true,
			CreatedAt:            now,
			StartedAt:            now,
			UpdatedAt:            now,
		},
		UpdatedAt: now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		candidate:    true,
		runtimeState: &runtimeState,
		versions: map[string]string{
			"sub2api":       "0.1.1-ts.1",
			"sub2api-green": "0.1.2-ts.1",
		},
	}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Job("restart-before-stop-0001")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusSucceeded || !job.BackgroundActivated {
		t.Fatalf("recovery did not complete durable old-container handoff: %+v", job)
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	stopIndex := strings.Index(commands, "stop --time 1 "+oldID)
	activateIndex := strings.Index(commands, "kill --signal=USR1 "+candidateID)
	if stopIndex < 0 || activateIndex < 0 || stopIndex > activateIndex {
		t.Fatalf("recovery did not stop the journal-bound old container before activation:\n%s", commands)
	}
}

func TestRestartRecoveryRequiresFullCandidateStabilization(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	var healthCalls atomic.Int64
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if healthCalls.Add(1) >= 4 {
			http.Error(w, "candidate became unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	cfg := testConfig(t, port)
	cfg.StabilizeDuration = Duration{Duration: 50 * time.Millisecond}
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", port)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:      "blue",
		ActiveContainer: "sub2api",
		ActivePort:      cfg.Slots[0].Port,
		ActiveVersion:   "0.1.1-ts.1",
		ActiveImage:     "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		Job: &Job{
			ID:                 "restart-stabilization-1",
			Action:             "update",
			TargetVersion:      "0.1.2-ts.1",
			FromVersion:        "0.1.1-ts.1",
			FromImage:          "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
			TargetImage:        "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
			Status:             JobStatusRunning,
			Stage:              StageStabilizing,
			OldContainer:       "sub2api",
			OldSlot:            "blue",
			OldSlotCaptured:    true,
			CandidateContainer: "sub2api-green",
			CandidateSlot:      "sub2api-green",
			CandidatePort:      port,
			TrafficState:       TrafficStateCandidate,
			TrafficSwitched:    true,
			CreatedAt:          now,
			StartedAt:          now,
			UpdatedAt:          now,
		},
		UpdatedAt: now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		candidate: true,
		versions: map[string]string{
			"sub2api":       "0.1.1-ts.1",
			"sub2api-green": "0.1.2-ts.1",
		},
	}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Job("restart-stabilization-1")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusFailed || !job.RollbackPerformed {
		t.Fatalf("recovery promoted a candidate that failed stabilization: %+v", job)
	}
	if manager.Health().ActiveContainer != "sub2api" {
		t.Fatalf("active container=%s, want previous container", manager.Health().ActiveContainer)
	}
}

func TestRestartRecoveryCompletesRollbackAlreadyInProgress(t *testing.T) {
	oldListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	oldPort := oldListener.Addr().(*net.TCPAddr).Port
	oldServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = oldServer.Serve(oldListener) }()
	t.Cleanup(func() { _ = oldServer.Close() })

	candidateListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	candidatePort := candidateListener.Addr().(*net.TCPAddr).Port
	candidateServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = candidateServer.Serve(candidateListener) }()
	t.Cleanup(func() { _ = candidateServer.Close() })

	cfg := testConfig(t, candidatePort)
	cfg.Slots[0].Port = oldPort
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", candidatePort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:      "blue",
		ActiveContainer: "sub2api",
		ActivePort:      oldPort,
		ActiveVersion:   "0.1.1-ts.1",
		ActiveImage:     "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		Job: &Job{
			ID:                 "restart-rollback-1",
			Action:             "update",
			TargetVersion:      "0.1.2-ts.1",
			FromVersion:        "0.1.1-ts.1",
			FromImage:          "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
			TargetImage:        "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
			Status:             JobStatusRunning,
			Stage:              StageRollingBack,
			OldContainer:       "sub2api",
			OldSlot:            "blue",
			CandidateContainer: "sub2api-green",
			CandidateSlot:      "sub2api-green",
			CandidatePort:      candidatePort,
			TrafficSwitched:    true,
			CreatedAt:          now,
			StartedAt:          now,
			UpdatedAt:          now,
		},
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		candidate: true,
		versions: map[string]string{
			"sub2api":       "0.1.1-ts.1",
			"sub2api-green": "0.1.2-ts.1",
		},
	}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Job("restart-rollback-1")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusFailed || !job.RollbackPerformed {
		t.Fatalf("status=%s rollback_performed=%v error=%s", job.Status, job.RollbackPerformed, job.Error)
	}
	port, err := readUpstreamPort(cfg.NginxUpstreamPath)
	if err != nil || port != oldPort {
		t.Fatalf("restored upstream port=%d err=%v", port, err)
	}
	if manager.Health().ActiveContainer != "sub2api" {
		t.Fatalf("active container=%s", manager.Health().ActiveContainer)
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	if !strings.Contains(commands, "stop --time 1 "+fakeContainerID("sub2api-green")) {
		t.Fatalf("candidate was not stopped while completing rollback:\n%s", commands)
	}
}

func TestRestartRecoveryRejectsSameNameCandidateReplacement(t *testing.T) {
	candidateListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	candidatePort := candidateListener.Addr().(*net.TCPAddr).Port
	candidateServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = candidateServer.Serve(candidateListener) }()
	t.Cleanup(func() { _ = candidateServer.Close() })

	cfg := testConfig(t, candidatePort)
	cfg.HealthTimeout = Duration{Duration: 20 * time.Millisecond}
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", candidatePort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:        "blue",
		ActiveContainer:   cfg.InitialContainer,
		ActiveContainerID: fakeContainerID(cfg.InitialContainer),
		ActivePort:        cfg.Slots[0].Port,
		ActiveVersion:     "0.1.1-ts.1",
		ActiveImage:       "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		Job: &Job{
			ID:                   "restart-replacement-0001",
			Action:               "update",
			TargetVersion:        "0.1.2-ts.1",
			FromVersion:          "0.1.1-ts.1",
			FromImage:            "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
			TargetImage:          "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
			Status:               JobStatusRunning,
			Stage:                StageStabilizing,
			OldContainer:         cfg.InitialContainer,
			OldContainerID:       fakeContainerID(cfg.InitialContainer),
			OldSlot:              "blue",
			OldSlotCaptured:      true,
			CandidateContainer:   "sub2api-green",
			CandidateContainerID: fakeContainerID("sub2api-green"),
			CandidateSlot:        "sub2api-green",
			CandidatePort:        candidatePort,
			TrafficState:         TrafficStateCandidate,
			TrafficSwitched:      true,
			CreatedAt:            now,
			StartedAt:            now,
			UpdatedAt:            now,
		},
		UpdatedAt: now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	replacementID := fakeContainerID("replacement")
	runner := &identityFailureRunner{
		base:          &fakeRunner{candidate: true},
		name:          "sub2api-green",
		expectedID:    fakeContainerID("sub2api-green"),
		replacementID: replacementID,
	}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Job("restart-replacement-0001")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusRollbackFailed || !manager.Health().Degraded {
		t.Fatalf("replacement did not fail closed during recovery: job=%+v health=%+v", job, manager.Health())
	}
	commands := strings.Join(runner.commands, "\n")
	if strings.Contains(commands, "docker stop --time 1 "+replacementID) || strings.Contains(commands, "docker kill --signal=USR1 "+replacementID) {
		t.Fatalf("replacement container received a destructive command:\n%s", commands)
	}
}

func TestRollbackToRetainedPreviousContainerSkipsPullAndSwapsSlots(t *testing.T) {
	activeListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	activePort := activeListener.Addr().(*net.TCPAddr).Port
	activeServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = activeServer.Serve(activeListener) }()
	t.Cleanup(func() { _ = activeServer.Close() })

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	previousPort := listener.Addr().(*net.TCPAddr).Port
	var runtimeState atomic.Value
	runtimeState.Store("standby")
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":%q,"slot":"blue"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, runtimeState.Load().(string))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	cfg := testConfig(t, activePort)
	cfg.Slots[0].Port = previousPort
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", activePort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:        "sub2api-green",
		ActiveContainer:   "sub2api-green",
		ActivePort:        activePort,
		ActiveVersion:     "0.1.2-ts.1",
		ActiveImage:       "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("b", 64),
		PreviousSlot:      "blue",
		PreviousContainer: "sub2api-blue",
		PreviousPort:      previousPort,
		PreviousVersion:   "0.1.1-ts.1",
		PreviousImage:     "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("a", 64),
		UpdatedAt:         now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		candidate:    true,
		runtimeState: &runtimeState,
		versions: map[string]string{
			"sub2api-blue":  "0.1.1-ts.1",
			"sub2api-green": "0.1.2-ts.1",
		},
	}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Start(DeployRequest{Action: "rollback", TargetVersion: "0.1.1-ts.1", RequestID: "retained-rollback-0001"})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForFinishedJob(t, manager, job.ID)
	if job.Status != JobStatusSucceeded {
		t.Fatalf("status=%s error=%s rollback=%s", job.Status, job.Error, job.RollbackError)
	}
	if manager.state.ActiveContainer != "sub2api-blue" || manager.state.PreviousContainer != "sub2api-green" {
		t.Fatalf("active=%s previous=%s", manager.state.ActiveContainer, manager.state.PreviousContainer)
	}
	marker, err := os.ReadFile(cfg.DeploymentStatePath)
	if err != nil || strings.TrimSpace(string(marker)) != "blue" {
		t.Fatalf("active slot marker=%q err=%v", marker, err)
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	if strings.Contains(commands, " pull ") || strings.Contains(commands, "image inspect --format {{json .Config.Labels}}") {
		t.Fatalf("retained rollback unexpectedly pulled or revalidated an image:\n%s", commands)
	}
}

func TestRollbackToLegacyInitialContainerAllowsEmptyRuntimeSlot(t *testing.T) {
	activeListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	activePort := activeListener.Addr().(*net.TCPAddr).Port
	activeServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = activeServer.Serve(activeListener) }()
	t.Cleanup(func() { _ = activeServer.Close() })

	cfg := testConfig(t, activePort)
	cfg.StabilizeDuration = Duration{Duration: time.Nanosecond}
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", activePort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:        "sub2api-green",
		ActiveContainer:   "sub2api-green",
		ActivePort:        activePort,
		ActiveVersion:     "0.1.2-ts.1",
		ActiveImage:       "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat("b", 64),
		PreviousSlot:      "blue",
		PreviousContainer: cfg.InitialContainer,
		PreviousPort:      cfg.Slots[0].Port,
		PreviousVersion:   "0.1.1-ts.1",
		PreviousImage:     "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		UpdatedAt:         now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{candidate: true, versions: map[string]string{
		"sub2api":       "0.1.1-ts.1",
		"sub2api-green": "0.1.2-ts.1",
	}}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Start(DeployRequest{Action: "rollback", TargetVersion: "0.1.1-ts.1", RequestID: "legacy-retained-rollback-0001"})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForFinishedJob(t, manager, job.ID)
	if job.Status != JobStatusSucceeded {
		t.Fatalf("legacy retained rollback status=%s error=%s rollback=%s", job.Status, job.Error, job.RollbackError)
	}
	if manager.Health().ActiveContainer != cfg.InitialContainer {
		t.Fatalf("active container=%s, want legacy initial %s", manager.Health().ActiveContainer, cfg.InitialContainer)
	}
}

func TestActivationFailureRestoresOldTrafficAndStopsCandidate(t *testing.T) {
	oldListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	oldPort := oldListener.Addr().(*net.TCPAddr).Port
	oldServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = oldServer.Serve(oldListener) }()
	t.Cleanup(func() { _ = oldServer.Close() })

	candidateListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	candidatePort := candidateListener.Addr().(*net.TCPAddr).Port
	candidateServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"standby","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = candidateServer.Serve(candidateListener) }()
	t.Cleanup(func() { _ = candidateServer.Close() })

	cfg := testConfig(t, candidatePort)
	cfg.Slots[0].Port = oldPort
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", oldPort)), 0644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{versions: map[string]string{
		"sub2api":       "0.1.1-ts.1",
		"sub2api-green": "0.1.2-ts.1",
	}}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Start(DeployRequest{Action: "update", TargetVersion: "0.1.2-ts.1", RequestID: "activation-failure-0001"})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForFinishedJob(t, manager, job.ID)
	if job.Status != JobStatusFailed || !job.RollbackPerformed {
		t.Fatalf("status=%s rollback_performed=%t error=%s rollback=%s", job.Status, job.RollbackPerformed, job.Error, job.RollbackError)
	}
	port, err := readUpstreamPort(cfg.NginxUpstreamPath)
	if err != nil || port != oldPort {
		t.Fatalf("restored upstream=%d err=%v", port, err)
	}
	marker, err := os.ReadFile(cfg.DeploymentStatePath)
	if err != nil || strings.TrimSpace(string(marker)) != "blue" {
		t.Fatalf("restored active slot marker=%q err=%v", marker, err)
	}
}

func TestFailedTrafficRestoreLeavesServingCandidateRunning(t *testing.T) {
	cfg := testConfig(t, 18081)
	runner := &fakeRunner{candidate: true, unhealthy: map[string]bool{"sub2api": true}}
	manager := &Manager{cfg: cfg, runner: runner, httpClient: &http.Client{Timeout: time.Second}, now: time.Now}
	job := &Job{
		ID:                 "restore-failure-0001",
		Status:             JobStatusRunning,
		OldContainer:       "sub2api",
		OldSlot:            "blue",
		CandidateContainer: "sub2api-green",
		CandidatePort:      18081,
		TrafficSwitched:    true,
	}
	if err := manager.restoreOldDeployment(context.Background(), job, false); err == nil {
		t.Fatal("expected restore failure")
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	if strings.Contains(commands, "stop --time 1 "+fakeContainerID("sub2api-green")) {
		t.Fatalf("serving candidate was stopped after rollback failed:\n%s", commands)
	}
}

func waitForFinishedJob(t *testing.T, manager *Manager, id string) *Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := manager.Job(id)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != JobStatusRunning {
			return job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("deployment job did not finish")
	return nil
}

func TestAmbiguousReloadClassifiesLiveCandidateByRoutedSlot(t *testing.T) {
	candidateListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	candidatePort := candidateListener.Addr().(*net.TCPAddr).Port
	candidateServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"standby","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = candidateServer.Serve(candidateListener) }()
	t.Cleanup(func() { _ = candidateServer.Close() })

	cfg := testConfig(t, candidatePort)
	var livePort atomic.Int64
	livePort.Store(int64(cfg.Slots[0].Port))
	proxy := newLivePortProxy(t, &livePort)
	cfg.NginxProbeURL = proxy.URL + "/health"
	runner := &ambiguousReloadRunner{
		base:         &fakeRunner{},
		upstreamPath: cfg.NginxUpstreamPath,
		livePort:     &livePort,
	}
	manager := &Manager{cfg: cfg, runner: runner, httpClient: &http.Client{Timeout: time.Second}, now: time.Now}
	route, err := manager.switchTraffic(context.Background(),
		trafficEndpoint{Port: candidatePort, Slot: "sub2api-green"},
		trafficEndpoint{Port: cfg.Slots[0].Port, Slot: "", AllowEmpty: true},
	)
	if err == nil {
		t.Fatal("expected an ambiguous reload error")
	}
	if !route.Known || route.Port != candidatePort {
		t.Fatalf("observed route=%+v, want live candidate port %d", route, candidatePort)
	}
	diskPort, readErr := readUpstreamPort(cfg.NginxUpstreamPath)
	if readErr != nil || diskPort != cfg.Slots[0].Port {
		t.Fatalf("restored disk port=%d err=%v, want old port %d", diskPort, readErr, cfg.Slots[0].Port)
	}
}

func TestTrafficConfirmationUsesFreshConnectionAfterReload(t *testing.T) {
	cfg := testConfig(t, 18081)
	var liveSlot atomic.Value
	liveSlot.Store("")
	var connectionSlots sync.Map
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assigned, _ := connectionSlots.LoadOrStore(r.RemoteAddr, liveSlot.Load().(string))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":"standby","slot":%q},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, assigned.(string))
	}))
	t.Cleanup(proxy.Close)
	cfg.NginxProbeURL = proxy.URL

	runner := &stickyConnectionReloadRunner{
		base:         &fakeRunner{},
		upstreamPath: cfg.NginxUpstreamPath,
		liveSlot:     &liveSlot,
		slotsByPort: map[int]string{
			cfg.Slots[0].Port: "",
			cfg.Slots[1].Port: cfg.Slots[1].Name,
		},
	}
	manager := &Manager{cfg: cfg, runner: runner, httpClient: &http.Client{Timeout: time.Second}, now: time.Now}
	if err := manager.verifyRoutedHealth(context.Background(), "", true); err != nil {
		t.Fatalf("prime old route connection: %v", err)
	}

	route, err := manager.switchTraffic(context.Background(),
		trafficEndpoint{Port: cfg.Slots[1].Port, Slot: cfg.Slots[1].Name},
		trafficEndpoint{Port: cfg.Slots[0].Port, AllowEmpty: true},
	)
	if err != nil {
		t.Fatalf("switch traffic: %v", err)
	}
	if !route.Known || route.Port != cfg.Slots[1].Port {
		t.Fatalf("confirmed route=%+v, want candidate port %d", route, cfg.Slots[1].Port)
	}
	connectionCount := 0
	connectionSlots.Range(func(_, _ any) bool {
		connectionCount++
		return true
	})
	if connectionCount < 2 {
		t.Fatalf("route confirmation reused the pre-reload connection; connections=%d", connectionCount)
	}
}

func TestEffectiveNginxValidationRejectsCommentsAndDuplicateProxyPass(t *testing.T) {
	if hasProxyPassDirective([]byte("# proxy_pass http://sub2api_managed;\n"), "sub2api_managed") {
		t.Fatal("commented proxy_pass was treated as active")
	}
	cfg := testConfig(t, 18081)
	duplicate := "server { proxy_pass http://sub2api_managed; }\nserver { proxy_pass http://sub2api_managed; }\n"
	if err := os.WriteFile(cfg.NginxSitePath, []byte(duplicate), 0644); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{cfg: cfg, runner: &fakeRunner{}, now: time.Now}
	err := manager.validateManagedRoute(context.Background(), cfg.Slots[0].Port)
	if err == nil || !strings.Contains(err.Error(), "exactly one active proxy_pass") {
		t.Fatalf("validation error=%v, want duplicate effective proxy_pass rejection", err)
	}
}

func TestConfigRequiresLocalExactNginxHealthProbe(t *testing.T) {
	cfg := testConfig(t, 18081)
	if err := cfg.validate(); err != nil {
		t.Fatalf("valid test config rejected: %v", err)
	}
	for _, probeURL := range []string{
		"https://127.0.0.1/health",
		"http://tokensupply.net/health",
		"http://127.0.0.1/not-health",
		"http://127.0.0.1/health?through=proxy",
	} {
		invalid := cfg
		invalid.NginxProbeURL = probeURL
		if err := invalid.validate(); err == nil {
			t.Fatalf("invalid nginx probe URL %q was accepted", probeURL)
		}
	}
}

func TestConfigRequiresComposeProject(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.ComposeProject = ""
	if err := cfg.validate(); err == nil || !strings.Contains(err.Error(), "compose_project is required") {
		t.Fatalf("validation error=%v, want required compose_project", err)
	}
}

func TestConfigRejectsUnsafeContainerIdentifiers(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "compose service", mutate: func(cfg *Config) { cfg.ComposeService = "-sub2api" }},
		{name: "initial container", mutate: func(cfg *Config) { cfg.InitialContainer = "-sub2api" }},
		{name: "one-character container", mutate: func(cfg *Config) { cfg.InitialContainer = "x" }},
		{name: "slot", mutate: func(cfg *Config) { cfg.Slots[0].Name = "-blue" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := testConfig(t, 18081)
			testCase.mutate(&cfg)
			if err := cfg.validate(); err == nil {
				t.Fatal("unsafe identifier was accepted")
			}
		})
	}
}

func TestContainerActionsFailClosedOnIdentityUncertainty(t *testing.T) {
	cfg := testConfig(t, 18081)
	name := "sub2api-green"
	expectedID := fakeContainerID(name)
	replacementID := fakeContainerID("replacement")
	for _, testCase := range []struct {
		name   string
		runner *identityFailureRunner
	}{
		{
			name: "transient inspect failure while ID remains present",
			runner: &identityFailureRunner{base: &fakeRunner{candidate: true}, name: name,
				expectedID: expectedID, transient: true},
		},
		{
			name: "same name replacement",
			runner: &identityFailureRunner{base: &fakeRunner{candidate: true}, name: name,
				expectedID: expectedID, replacementID: replacementID},
		},
		{
			name: "exact name lookup failure",
			runner: &identityFailureRunner{base: &fakeRunner{candidate: true}, name: name,
				expectedID: expectedID, listError: true},
		},
		{
			name: "persisted ID was renamed",
			runner: &identityFailureRunner{base: &fakeRunner{candidate: true}, name: name,
				expectedID: expectedID, renamedTo: "renamed-container"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			manager := &Manager{cfg: cfg, runner: testCase.runner, httpClient: &http.Client{Timeout: time.Second}, now: time.Now}
			ref := containerRef{Name: name, ID: expectedID}
			if err := manager.stopContainer(context.Background(), ref); err == nil {
				t.Fatal("uncertain container identity was accepted for stop")
			}
			if err := manager.ensureBackgroundActive(context.Background(), ref, cfg.Slots[1].Port, cfg.Slots[1].Name); err == nil {
				t.Fatal("uncertain container identity was accepted for activation")
			}
			commands := strings.Join(testCase.runner.commands, "\n")
			for _, forbidden := range []string{"docker stop ", "docker kill ", "docker rm ", "docker update ", "docker start "} {
				if strings.Contains(commands, forbidden) {
					t.Fatalf("identity uncertainty emitted destructive command %q:\n%s", forbidden, commands)
				}
			}
		})
	}
}

func TestProvenAbsentContainerOnlyAllowsIdempotentStopAndRemove(t *testing.T) {
	cfg := testConfig(t, 18081)
	name := "sub2api-green"
	runner := &identityFailureRunner{
		base: &fakeRunner{candidate: true}, name: name, expectedID: fakeContainerID(name), absent: true,
	}
	manager := &Manager{cfg: cfg, runner: runner, httpClient: &http.Client{Timeout: time.Second}, now: time.Now}
	ref := containerRef{Name: name, ID: runner.expectedID}
	if err := manager.stopContainer(context.Background(), ref); err != nil {
		t.Fatalf("stop proven-absent container: %v", err)
	}
	if err := manager.removeContainerIfPresent(context.Background(), ref); err != nil {
		t.Fatalf("remove proven-absent container: %v", err)
	}
	if err := manager.startContainer(context.Background(), ref); err == nil {
		t.Fatal("start accepted a proven-absent container")
	}
	commands := strings.Join(runner.commands, "\n")
	for _, forbidden := range []string{"docker stop ", "docker rm ", "docker start "} {
		if strings.Contains(commands, forbidden) {
			t.Fatalf("proven absence emitted command %q:\n%s", forbidden, commands)
		}
	}
}

func TestCandidatePersistenceFailureLeavesUnpersistedContainerUntouched(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.StatePath = t.TempDir()
	now := time.Now().UTC()
	runner := &fakeRunner{}
	manager := &Manager{
		cfg:    cfg,
		runner: runner,
		now:    time.Now,
		state: State{Job: &Job{
			ID:        "candidate-persist-failure-0001",
			Status:    JobStatusRunning,
			Stage:     StageStartingCandidate,
			CreatedAt: now,
			StartedAt: now,
			UpdatedAt: now,
		}},
	}
	if err := manager.startCandidate(context.Background(), "candidate-persist-failure-0001", cfg.Slots[1], "example.invalid/sub2api@sha256:"+strings.Repeat("a", 64)); err == nil || !strings.Contains(err.Error(), "persist candidate container identity") {
		t.Fatalf("startCandidate error=%v, want persistence failure", err)
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	for _, forbidden := range []string{"docker update --restart", "docker rm -f", "docker stop --time"} {
		if strings.Contains(commands, forbidden) {
			t.Fatalf("unpersisted candidate received command %q:\n%s", forbidden, commands)
		}
	}
}

func TestNewManagerRejectsInvalidPersistedContainerIDBeforeDocker(t *testing.T) {
	cfg := testConfig(t, 18081)
	state := State{
		ActiveSlot:        "blue",
		ActiveContainer:   cfg.InitialContainer,
		ActiveContainerID: "--host=attacker.invalid",
		ActivePort:        cfg.Slots[0].Port,
		UpdatedAt:         time.Now().UTC(),
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	if _, err := NewManager(cfg, runner); err == nil || !strings.Contains(err.Error(), "invalid persisted ID") {
		t.Fatalf("NewManager error=%v, want invalid persisted ID", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("invalid persisted ID reached Docker:\n%s", strings.Join(runner.commands, "\n"))
	}
}

func TestForeignSameNameContainerNeverReceivesDestructiveCommand(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		labels string
	}{
		{name: "foreign-project", labels: `{"com.docker.compose.project":"foreign","com.docker.compose.service":"sub2api"}`},
		{name: "foreign-service", labels: `{"com.docker.compose.project":"sub2api","com.docker.compose.service":"foreign"}`},
		{name: "missing-labels", labels: `{}`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := testConfig(t, 18081)
			base := &fakeRunner{candidate: true}
			runner := &foreignOwnershipRunner{base: base, labels: testCase.labels}
			manager := &Manager{
				cfg:        cfg,
				runner:     runner,
				httpClient: &http.Client{Timeout: time.Second},
				now:        time.Now,
			}
			container := cfg.Slots[1].Name

			if err := manager.startCandidate(context.Background(), "foreign-candidate-job", cfg.Slots[1], "example.invalid/sub2api@sha256:"+strings.Repeat("a", 64)); err == nil {
				t.Fatal("candidate without exact ownership labels was accepted")
			}
			foreignRef := containerRef{Name: container, ID: fakeContainerID(container)}
			if err := manager.startContainer(context.Background(), foreignRef); err == nil {
				t.Fatal("unowned container was accepted for start")
			}
			if err := manager.stopContainer(context.Background(), foreignRef); err == nil {
				t.Fatal("unowned container was accepted for stop")
			}
			if err := manager.removeContainerIfPresent(context.Background(), foreignRef); err == nil {
				t.Fatal("unowned container was accepted for removal")
			}
			if err := manager.ensureBackgroundActive(context.Background(), foreignRef, cfg.Slots[1].Port, cfg.Slots[1].Name); err == nil {
				t.Fatal("unowned container was accepted for activation signal")
			}

			base.mu.Lock()
			commands := strings.Join(base.commands, "\n")
			base.mu.Unlock()
			if !strings.Contains(commands, "docker compose --project-name "+cfg.ComposeProject+" --project-directory") {
				t.Fatalf("candidate compose command did not pin project name:\n%s", commands)
			}
			for _, forbidden := range []string{
				"docker update --restart",
				"docker start " + container,
				"docker stop --time",
				"docker rm -f " + container,
				"docker kill --signal=USR1 " + container,
			} {
				if strings.Contains(commands, forbidden) {
					t.Fatalf("unowned container received destructive command %q:\n%s", forbidden, commands)
				}
			}
		})
	}
}

func TestCompletionPersistenceFailureIsTerminalAndDegradedInMemory(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.StatePath = t.TempDir()
	now := time.Now().UTC()
	manager := &Manager{
		cfg: cfg,
		now: time.Now,
		state: State{Job: &Job{
			ID:        "terminal-persist-0001",
			Status:    JobStatusRunning,
			Stage:     StageActivating,
			CreatedAt: now,
			StartedAt: now,
			UpdatedAt: now,
		}},
	}
	if err := manager.complete("terminal-persist-0001", "Deployment completed", ""); err == nil {
		t.Fatal("expected completion persistence failure")
	}
	job, err := manager.Job("terminal-persist-0001")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusDegraded || job.Stage != StageFailed || job.FinishedAt == nil {
		t.Fatalf("job remained non-terminal after persistence failure: %+v", job)
	}
	if health := manager.Health(); !health.Degraded || health.Status != "unhealthy" || health.JobRunning {
		t.Fatalf("health did not expose terminal persistence failure: %+v", health)
	}
}

func TestStateTransitionMutationIsNotAppliedBeforeDurableWrite(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.StatePath = t.TempDir()
	now := time.Now().UTC()
	manager := &Manager{
		cfg: cfg,
		now: time.Now,
		state: State{Job: &Job{
			ID:           "write-ahead-transition-0001",
			Status:       JobStatusRunning,
			Stage:        StageCheckingCandidate,
			TrafficState: TrafficStateOld,
			CreatedAt:    now,
			StartedAt:    now,
			UpdatedAt:    now,
		}},
	}
	err := manager.updateJob("write-ahead-transition-0001", StageSwitchingTraffic, "switching", func(job *Job) {
		job.TrafficState = TrafficStateSwitchPending
	})
	if err == nil {
		t.Fatal("expected state transition persistence failure")
	}
	job, jobErr := manager.Job("write-ahead-transition-0001")
	if jobErr != nil {
		t.Fatal(jobErr)
	}
	if job.Stage != StageCheckingCandidate || job.TrafficState != TrafficStateOld {
		t.Fatalf("failed write-ahead transition leaked into memory: %+v", job)
	}
}

func TestRecoveredTerminalPersistenceFailuresAreVisible(t *testing.T) {
	for _, recoveredStatus := range []string{JobStatusSucceeded, JobStatusRollbackFailed} {
		t.Run(recoveredStatus, func(t *testing.T) {
			cfg := testConfig(t, 18081)
			cfg.StatePath = t.TempDir()
			now := time.Now().UTC()
			job := Job{
				ID:        "recovered-terminal-" + recoveredStatus,
				Status:    JobStatusRunning,
				Stage:     StageRollingBack,
				CreatedAt: now,
				StartedAt: now,
				UpdatedAt: now,
			}
			manager := &Manager{cfg: cfg, now: time.Now, state: State{Job: &job}}
			if err := manager.finishRecoveredJob(job, recoveredStatus, "recovery terminal", "recovery detail"); err == nil {
				t.Fatal("expected recovered terminal persistence failure")
			}
			visible, err := manager.Job(job.ID)
			if err != nil {
				t.Fatal(err)
			}
			if visible.Status != JobStatusDegraded || visible.Stage != StageFailed || visible.FinishedAt == nil {
				t.Fatalf("recovered job remained non-terminal: %+v", visible)
			}
			if !manager.Health().Degraded || manager.Health().JobRunning {
				t.Fatalf("recovered persistence failure was not exposed in health: %+v", manager.Health())
			}
		})
	}
}

func TestDegradedLatchBlocksStartUntilSafeReconciliation(t *testing.T) {
	cfg := testConfig(t, 18081)
	now := time.Now().UTC()
	state := State{
		ActiveSlot:      "blue",
		ActiveContainer: "sub2api",
		ActivePort:      cfg.Slots[0].Port,
		ActiveVersion:   "0.1.1-ts.1",
		ActiveImage:     "ghcr.io/ssharkkky/sub2api:0.1.1-ts.1",
		Degraded:        true,
		DegradedReason:  "rollback route was uncertain",
		Job: &Job{
			ID:                 "failed-rollback-0001",
			Status:             JobStatusRollbackFailed,
			Stage:              StageFailed,
			OldSlot:            "blue",
			OldContainer:       "sub2api",
			CandidateSlot:      "sub2api-green",
			CandidateContainer: "sub2api-green",
			CandidatePort:      cfg.Slots[1].Port,
			CreatedAt:          now,
			StartedAt:          now,
			UpdatedAt:          now,
		},
		UpdatedAt: now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{candidate: true}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	request := DeployRequest{Action: "update", TargetVersion: "0.1.2-ts.1", RequestID: "blocked-until-reconcile-0001"}
	if _, err := manager.Start(request); !errors.Is(err, ErrDeployerDegraded) {
		t.Fatalf("Start error=%v, want ErrDeployerDegraded", err)
	}
	if err := manager.Reconcile(context.Background(), "sub2api-green"); err == nil {
		t.Fatal("unhealthy/non-routed candidate reconciliation unexpectedly succeeded")
	}
	if _, err := manager.Start(request); !errors.Is(err, ErrDeployerDegraded) {
		t.Fatalf("Start after failed reconciliation error=%v, want ErrDeployerDegraded", err)
	}
	if err := manager.Reconcile(context.Background(), "blue"); err != nil {
		t.Fatalf("reconcile active slot: %v", err)
	}
	job, err := manager.Start(request)
	if err != nil {
		t.Fatalf("Start after successful reconciliation: %v", err)
	}
	_ = waitForFinishedJob(t, manager, job.ID)
}

func TestReconcileRefusesWhileDaemonSocketIsLive(t *testing.T) {
	cfg := testConfig(t, 18081)
	socketDir, err := os.MkdirTemp("/tmp", "sub2api-deployer-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	cfg.SocketPath = filepath.Join(socketDir, "deployer.sock")
	listener, err := net.Listen("unix", cfg.SocketPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	if err := RequireDaemonStopped(cfg.SocketPath); err == nil || !strings.Contains(err.Error(), "daemon is still accepting connections") {
		t.Fatalf("pre-construction guard error=%v, want live daemon rejection", err)
	}
	manager := &Manager{cfg: cfg, now: time.Now, state: State{Degraded: true}}
	err = manager.Reconcile(context.Background(), "blue")
	if err == nil || !strings.Contains(err.Error(), "daemon is still accepting connections") {
		t.Fatalf("Reconcile error=%v, want live daemon guard", err)
	}
}

func TestDegradedHealthAndStartEndpointsReturnServiceUnavailable(t *testing.T) {
	cfg := testConfig(t, 18081)
	manager := &Manager{cfg: cfg, now: time.Now, state: State{Degraded: true, DegradedReason: "manual reconciliation required"}}
	server := NewHTTPServer(cfg, manager)

	healthRecorder := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(healthRecorder, httptest.NewRequest(http.MethodGet, "/v1/health", nil))
	if healthRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("health status=%d, want %d", healthRecorder.Code, http.StatusServiceUnavailable)
	}

	deploymentRecorder := httptest.NewRecorder()
	body := strings.NewReader(`{"action":"update","target_version":"0.1.2-ts.1","request_id":"degraded-http-0001"}`)
	server.server.Handler.ServeHTTP(deploymentRecorder, httptest.NewRequest(http.MethodPost, "/v1/deployments", body))
	if deploymentRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("deployment status=%d body=%s, want %d", deploymentRecorder.Code, deploymentRecorder.Body.String(), http.StatusServiceUnavailable)
	}
}

func TestStartupWaitsForPersistedActiveContainerToBecomeHealthy(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.HealthTimeout = Duration{Duration: 2 * time.Second}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:      cfg.Slots[0].Name,
		ActiveContainer: cfg.InitialContainer,
		ActivePort:      cfg.Slots[0].Port,
		ActiveVersion:   cfg.InitialVersion,
		UpdatedAt:       now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &delayedHealthRunner{base: &fakeRunner{}}
	runner.remaining.Store(2)
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	if health := manager.Health(); health.Degraded {
		t.Fatalf("delayed active container permanently degraded startup: %+v", health)
	}
	if calls := runner.calls.Load(); calls < 3 {
		t.Fatalf("active health checks=%d, want at least 3", calls)
	}
}

func TestStartupDoesNotWaitOnWrongPersistedNginxRoute(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.HealthTimeout = Duration{Duration: 2 * time.Second}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:      cfg.Slots[0].Name,
		ActiveContainer: cfg.InitialContainer,
		ActivePort:      cfg.Slots[0].Port,
		ActiveVersion:   cfg.InitialVersion,
		UpdatedAt:       now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", cfg.Slots[1].Port)), 0644); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	manager, err := NewManager(cfg, &fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("startup waited %s on a wrong effective route", elapsed)
	}
	if health := manager.Health(); !health.Degraded || !strings.Contains(health.DegradedReason, "expected") {
		t.Fatalf("wrong startup route did not latch degraded: %+v", health)
	}
}

func TestStartupStopsWaitingWhenPersistedNginxRouteDrifts(t *testing.T) {
	cfg := testConfig(t, 18081)
	cfg.HealthTimeout = Duration{Duration: 2 * time.Second}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:      cfg.Slots[0].Name,
		ActiveContainer: cfg.InitialContainer,
		ActivePort:      cfg.Slots[0].Port,
		ActiveVersion:   cfg.InitialVersion,
		UpdatedAt:       now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	delayed := &delayedHealthRunner{base: &fakeRunner{}}
	delayed.remaining.Store(100)
	runner := &routeDriftRunner{
		base:         delayed,
		upstreamPath: cfg.NginxUpstreamPath,
		driftPort:    cfg.Slots[1].Port,
	}
	started := time.Now()
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("startup waited %s after the effective route drifted", elapsed)
	}
	if health := manager.Health(); !health.Degraded || !strings.Contains(health.DegradedReason, "changed") {
		t.Fatalf("route drift did not latch degraded: %+v", health)
	}
}

func TestSecondDeploymentPreparationRemovesLegacyContainerFromInactiveSlot(t *testing.T) {
	greenListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	greenPort := greenListener.Addr().(*net.TCPAddr).Port
	greenServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = greenServer.Serve(greenListener) }()
	t.Cleanup(func() { _ = greenServer.Close() })

	cfg := testConfig(t, greenPort)
	cfg.Slots[0].Name = "sub2api-blue"
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", greenPort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	job := &Job{
		ID:                   "second-deployment-0001",
		Status:               JobStatusRunning,
		OldSlot:              "sub2api-green",
		OldContainer:         "sub2api-green",
		OldContainerID:       fakeContainerID("sub2api-green"),
		OldRuntimeSlot:       "sub2api-green",
		OldSlotCaptured:      true,
		CandidateSlot:        "sub2api-blue",
		CandidateContainer:   "sub2api-blue",
		CandidateContainerID: fakeContainerID("sub2api-blue"),
		CandidatePort:        cfg.Slots[0].Port,
		CreatedAt:            now,
		StartedAt:            now,
		UpdatedAt:            now,
	}
	runner := &fakeRunner{candidate: true}
	manager := &Manager{
		cfg:        cfg,
		runner:     runner,
		httpClient: &http.Client{Timeout: time.Second},
		now:        time.Now,
		state: State{
			ActiveSlot:          "sub2api-green",
			ActiveContainer:     "sub2api-green",
			ActiveContainerID:   fakeContainerID("sub2api-green"),
			ActivePort:          greenPort,
			PreviousSlot:        "sub2api-blue",
			PreviousContainer:   cfg.InitialContainer,
			PreviousContainerID: fakeContainerID(cfg.InitialContainer),
			PreviousPort:        cfg.Slots[0].Port,
			Job:                 job,
		},
	}
	if err := manager.prepareInactiveSlot(context.Background(), job, cfg.Slots[0]); err != nil {
		t.Fatal(err)
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	legacyRemoval := strings.Index(commands, "docker rm -f "+fakeContainerID(cfg.InitialContainer))
	candidateRemoval := strings.Index(commands, "docker rm -f "+fakeContainerID("sub2api-blue"))
	if legacyRemoval < 0 || candidateRemoval < 0 || legacyRemoval > candidateRemoval {
		t.Fatalf("inactive slot cleanup did not remove legacy then named candidate:\n%s", commands)
	}
	if strings.Contains(commands, "docker rm -f "+fakeContainerID("sub2api-green")) {
		t.Fatalf("serving container was removed during inactive slot preparation:\n%s", commands)
	}
}

func newLivePortProxy(t *testing.T, livePort *atomic.Int64) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "http://127.0.0.1:"+strconv.FormatInt(livePort.Load(), 10)+r.URL.Path, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, response.Body)
	}))
	t.Cleanup(server.Close)
	return server
}
