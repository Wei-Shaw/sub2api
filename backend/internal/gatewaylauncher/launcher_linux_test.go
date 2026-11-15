//go:build linux

package gatewaylauncher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const fixtureUID uint32 = 65534
const fixtureGID uint32 = 65534

const syntheticEngineDiagnosticLine = "{\"event\":\"gateway_native_response_failure\",\"phase\":\"http_status\",\"http_status\":401}\n"

type fixture struct {
	root   string
	config Config
}

// Qualification uses only a new /run fixture, a COPY of this test executable,
// and dedicated synthetic UID/GID. Production ownership checks are unchanged.
// No test mode, same-UID bypass or failure-injection field exists in production.
func newFixture(t *testing.T) fixture {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("NOT_RUN: disposable root Linux qualification required")
	}
	root, err := os.MkdirTemp("/run", "gatewaylauncher-fixture-")
	if err != nil {
		t.Skip("NOT_RUN: cannot create isolated /run fixture")
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	if err = os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(root, "protected")
	if err = os.Mkdir(meta, 0700); err != nil {
		t.Fatal(err)
	}
	exchange := filepath.Join(root, "exchange")
	if err = os.Mkdir(exchange, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Chown(exchange, int(fixtureUID), int(fixtureGID)); err != nil {
		t.Skip("NOT_RUN: separate synthetic uid/gid unavailable")
	}
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	image := filepath.Join(root, "synthetic-engine")
	w, err := os.OpenFile(image, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(w, r)
	ce := w.Close()
	if err != nil || ce != nil {
		t.Fatal("copy synthetic image failed")
	}
	// Confirm actual synthetic non-root exec before interpreting qualification.
	q := exec.Command(image, "-test.run=^TestSyntheticProcess$", "--", "qualification", root, "credentials")
	q.Env = []string{}
	q.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: fixtureUID, Gid: fixtureGID, Groups: []uint32{}}}
	if err = q.Start(); err != nil {
		t.Skip("NOT_RUN: isolated synthetic credential exec unavailable")
	}
	if err = q.Wait(); err != nil {
		t.Fatal("synthetic credential preflight failed")
	}
	c := fixtureConfig(root, "hold")
	return fixture{root, c}
}

func fixtureConfig(root, mode string) Config {
	return Config{
		Authority:  AuthorityConfig{Directory: filepath.Join(root, "protected"), OriginRef: "opaque-disposable-origin"},
		EnginePath: filepath.Join(root, "synthetic-engine"),
		Args:       []string{"-test.run=^TestSyntheticProcess$", "--", "engine", root, mode},
		EngineUID:  fixtureUID, EngineGID: fixtureGID,
	}
}

func deadline(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func trackLauncher(t *testing.T, l *Launcher) {
	t.Helper()
	if l == nil {
		return
	}
	t.Cleanup(func() {
		// Test cleanup only, after positive exact-child exit observation.
		select {
		case <-l.done:
		default:
			_ = l.cmd.Process.Kill()
		}
		ctx, cancel := deadline(t)
		defer cancel()
		select {
		case <-l.done:
		case <-ctx.Done():
			t.Error("synthetic child cleanup unobserved")
			return
		}
		_, _ = l.Wait(ctx)
		l.mu.Lock()
		l.a.close()
		l.mu.Unlock()
	})
}

func startFixture(t *testing.T, c Config) *Launcher {
	t.Helper()
	l, err := Start(c)
	trackLauncher(t, l)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := deadline(t)
	defer cancel()
	if err = l.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	return l
}

func stopEngine(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "exchange", "stop"), []byte("stop"), 0644); err != nil {
		t.Fatal(err)
	}
}

func waitFile(t *testing.T, path string) []byte {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		b, err := os.ReadFile(path)
		if err == nil && json.Valid(b) {
			return b
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("synthetic result not observed: %s", filepath.Base(path))
	return nil
}

func probeLock(t *testing.T, c Config, busy bool) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(c.Authority.Directory, lockName), os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	// Process-state observation alone does not prove this independent flock free.
	// Require the actual flock itself, bounded by the fixture deadline.
	until := time.Now().Add(5 * time.Second)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if busy || err == nil || (err != syscall.EWOULDBLOCK && err != syscall.EAGAIN) || !time.Now().Before(until) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if busy {
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
			t.Fatalf("independent flock: want occupied, got %v", err)
		}
	} else if err != nil {
		t.Fatalf("independent flock: want reacquired, got %v", err)
	}
	// Closing this descriptor, never explicit LOCK_UN, releases our test probe.
}

type engineReport struct {
	Device          uint64  `json:"device"`
	Inode           uint64  `json:"inode"`
	FDFlags         uintptr `json:"fdFlags"`
	AccessMode      uintptr `json:"accessMode"`
	SharedFlock     bool    `json:"sharedFlock"`
	MetadataDenied  bool    `json:"metadataDenied"`
	UID             int     `json:"uid"`
	GID             int     `json:"gid"`
	Groups          []int   `json:"groups"`
	SyntheticMarker string  `json:"syntheticMarker"`
	Incarnation     string  `json:"incarnation"`
}

type supervisorReport struct {
	Status  string  `json:"status"`
	Binding Binding `json:"binding"`
}

func writeSynthetic(path string, value any) {
	b, err := json.Marshal(value)
	if err != nil || os.WriteFile(path, b, 0600) != nil {
		os.Exit(70)
	}
}

// Regression: all subprocesses are explicit new synthetic fixture processes;
// this helper is compiled ONLY into the Go test executable, never production.
func TestSyntheticProcess(t *testing.T) {
	t.Log("Regression: synthetic helper cannot run without explicit disposable fixture arguments")
	marker := -1
	for i, s := range os.Args {
		if s == "--" {
			marker = i
			break
		}
	}
	if marker < 0 {
		return
	}
	args := os.Args[marker+1:]
	if len(args) < 3 {
		os.Exit(70)
	}
	kind, root, mode := args[0], args[1], args[2]
	if filepath.Dir(root) != "/run" || !strings.HasPrefix(filepath.Base(root), "gatewaylauncher-fixture-") {
		os.Exit(70)
	}
	if kind == "root-check" {
		// Enter only the parent's OWN fixture after exec, so the same fixture
		// also works with the dynamically linked race test executable.
		if err := syscall.Chroot(root); err != nil {
			if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
				os.Exit(78)
			}
			os.Exit(70)
		}
		if os.Chdir("/") != nil {
			os.Exit(70)
		}
		a, authorityErr := openAuthority("/protected")
		if a != nil {
			a.close()
		}
		engineErr := checkEngine("/synthetic-engine")
		if mode == "unsafe" {
			if !errors.Is(authorityErr, ErrUnsafe) || !errors.Is(engineErr, ErrUnsafe) {
				os.Exit(70)
			}
		} else if authorityErr != nil || engineErr != nil {
			os.Exit(70)
		}
		os.Exit(0)
	}
	if kind == "pid-reuse" {
		qualifyFixturePIDReuse(t, root)
		os.Exit(0)
	}
	if kind == "witness" {
		writeSynthetic(filepath.Join(root, "exchange", "witness.json"), map[string]int{"pid": os.Getpid()})
		for {
			if _, err := os.Stat(filepath.Join(root, "exchange", "stop-witness")); err == nil {
				os.Exit(0)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if kind == "qualification" {
		if os.Geteuid() != int(fixtureUID) || os.Getegid() != int(fixtureGID) {
			os.Exit(70)
		}
		os.Exit(0)
	}
	if kind == "supervisor" {
		if os.Geteuid() != 0 {
			os.Exit(70)
		}
		if mode == "race" {
			for {
				if _, err := os.Stat(filepath.Join(root, "barrier")); err == nil {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		if len(args) != 4 {
			os.Exit(70)
		}
		result := filepath.Join(root, args[3])
		l, err := Start(fixtureConfig(root, "hold"))
		if l == nil {
			status := "denied"
			if errors.Is(err, ErrBusy) {
				status = "busy"
			}
			writeSynthetic(result, supervisorReport{Status: status})
			os.Exit(0)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err != nil || l.AwaitReady(ctx) != nil {
			cancel()
			_ = l.cmd.Process.Kill()
			os.Exit(70)
		}
		cancel()
		writeSynthetic(result, supervisorReport{Status: "started", Binding: l.Binding()})
		if _, err = l.Wait(context.Background()); err != nil {
			os.Exit(70)
		}
		os.Exit(0)
	}
	if kind == "retainer" {
		if os.Geteuid() != int(fixtureUID) || os.Getegid() != int(fixtureGID) {
			os.Exit(70)
		}
		var st syscall.Stat_t
		if syscall.Fstat(3, &st) != nil {
			os.Exit(70)
		}
		writeSynthetic(filepath.Join(root, "exchange", "retainer.json"), map[string]bool{"alive": true})
		for {
			if _, err := os.Stat(filepath.Join(root, "exchange", "stop-retainer")); err == nil {
				os.Exit(0)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if kind != "engine" || os.Geteuid() != int(fixtureUID) || os.Getegid() != int(fixtureGID) {
		os.Exit(70)
	}
	if mode == "stubborn" {
		signal.Ignore(syscall.SIGTERM)
	}
	var st syscall.Stat_t
	if syscall.Fstat(3, &st) != nil {
		os.Exit(70)
	}
	fdFlags, _, e := syscall.Syscall(syscall.SYS_FCNTL, 3, syscall.F_GETFD, 0)
	if e != 0 {
		os.Exit(70)
	}
	accessMode, _, e := syscall.Syscall(syscall.SYS_FCNTL, 3, syscall.F_GETFL, 0)
	if e != 0 {
		os.Exit(70)
	}
	groups, err := os.Getgroups()
	if err != nil {
		os.Exit(70)
	}
	shared := syscall.Flock(3, syscall.LOCK_EX|syscall.LOCK_NB) == nil
	// Access is genuinely denied under separate credentials, not simulated.
	metadataErr := os.WriteFile(filepath.Join(root, "protected", journalName), []byte("forged"), 0600)
	gate := os.NewFile(5, "synthetic-gate")
	var g [1]byte
	n, err := gate.Read(g[:])
	_ = gate.Close()
	if err != nil || n != 1 || g[0] != 'G' {
		os.Exit(70)
	}
	if mode == "diagnostic" {
		if _, err = io.WriteString(os.Stderr, syntheticEngineDiagnosticLine); err != nil {
			os.Exit(70)
		}
		if _, err = io.WriteString(os.Stdout, "synthetic-engine-stdout-sentinel\n"); err != nil {
			os.Exit(70)
		}
	}
	if mode == "bootstrap" {
		// Parent closes its original and replaces the pathname after Start,
		// before this real child reads the captured description.
		for {
			if _, err := os.Stat(filepath.Join(root, "exchange", "read-bootstrap")); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		file := os.NewFile(6, "synthetic-bootstrap")
		var st syscall.Stat_t
		flags, flagErr := unix.FcntlInt(6, unix.F_GETFL, 0)
		fdflags, fdErr := unix.FcntlInt(6, unix.F_GETFD, 0)
		if syscall.Fstat(6, &st) != nil || flagErr != nil || fdErr != nil || flags&unix.O_ACCMODE != unix.O_RDONLY || fdflags&unix.FD_CLOEXEC != 0 || st.Uid != 0 || st.Mode&0777 != 0600 {
			os.Exit(70)
		}
		data, err := io.ReadAll(io.NewSectionReader(file, 0, 65537))
		if err != nil || len(data) > 65536 {
			os.Exit(70)
		}
		_, writeErr := file.Write([]byte("fixture-write-denied"))
		_ = file.Close()
		hash := sha256.Sum256(data)
		writeSynthetic(filepath.Join(root, "exchange", "bootstrap.json"), map[string]any{"sha256": hex.EncodeToString(hash[:]), "readonly": writeErr != nil, "nonCloexec": fdflags == 0})
	}
	writeSynthetic(filepath.Join(root, "exchange", "engine.json"), engineReport{
		uint64(st.Dev), st.Ino, fdFlags, accessMode & syscall.O_ACCMODE, shared,
		os.IsPermission(metadataErr), os.Geteuid(), os.Getegid(), groups, os.Getenv("SYNTHETIC_VALUE"), os.Getenv("GATEWAY_LAUNCHER_ENGINE_INCARNATION"),
	})
	ready := os.NewFile(4, "synthetic-ready")
	if _, err = ready.Write([]byte("R")); err != nil {
		os.Exit(70)
	}
	_ = ready.Close()
	for {
		if _, err = os.Stat(filepath.Join(root, "exchange", "stop")); err == nil {
			os.Exit(0)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// Red without EngineStderr wiring: the real child's finite diagnostic goes to
// /dev/null. Exercise actual FD 2 delivery without mixing engine stdout into it.
func TestEngineStderrDescriptorForRealChild(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			f := newFixture(t)
			stderr, err := os.CreateTemp(f.root, "engine-stderr-")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stderr.Close() }()
			c := fixtureConfig(f.root, "diagnostic")
			if enabled {
				c.EngineStderr = stderr
			}
			l := startFixture(t, c) // readiness retains its separate FD handshake.
			binding := l.Binding()
			probeLock(t, c, true)
			stopEngine(t, f.root)
			ctx, cancel := deadline(t)
			defer cancel()
			receipt, err := l.Wait(ctx)
			if err != nil || receipt.Binding != binding || receipt.Scope != LocalTeardownScope || receipt.Method != "observed-child-exit" {
				t.Fatalf("exact child retirement failed: %v", err)
			}
			if _, err = stderr.Stat(); err != nil {
				t.Fatal("launcher closed caller-owned stderr", err)
			}
			data, err := os.ReadFile(stderr.Name())
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			if enabled {
				want = syntheticEngineDiagnosticLine
			}
			if string(data) != want {
				t.Fatalf("engine stderr delivery differs: got %q want %q", data, want)
			}
			probeLock(t, c, false)
		})
	}
}

func TestBootstrapDescriptorCapturedForRealChild(t *testing.T) {
	f := newFixture(t)
	c := fixtureConfig(f.root, "bootstrap")
	path := filepath.Join(f.root, "protected", "bootstrap.json")
	original := []byte(`{"fixture":"synthetic-config-never-in-receipt"}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	c.BootstrapFile = file
	l, err := Start(c)
	trackLauncher(t, l)
	if err != nil || l == nil {
		t.Fatal("bootstrap exec failed", err)
	}
	// The captured file, rather than the caller's numeric FD/path/offset, is
	// authoritative. A replaced path has different synthetic configuration.
	if _, err = file.Seek(int64(len(original)), io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(`{"fixture":"replacement"}`), 0600); err != nil {
		t.Fatal(err)
	}
	replacement, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = replacement.Close() }()
	if err = os.WriteFile(filepath.Join(f.root, "exchange", "read-bootstrap"), []byte("go"), 0644); err != nil {
		t.Fatal(err)
	}
	var observed struct {
		SHA256     string `json:"sha256"`
		Readonly   bool   `json:"readonly"`
		NonCloexec bool   `json:"nonCloexec"`
	}
	if json.Unmarshal(waitFile(t, filepath.Join(f.root, "exchange", "bootstrap.json")), &observed) != nil {
		t.Fatal("missing child config read")
	}
	hash := sha256.Sum256(original)
	if observed.SHA256 != hex.EncodeToString(hash[:]) || !observed.Readonly || !observed.NonCloexec {
		t.Fatal("captured readonly FD 6 changed")
	}
	ctx, cancel := deadline(t)
	defer cancel()
	if l.AwaitReady(ctx) != nil {
		t.Fatal("bootstrap readiness failed")
	}
	probeLock(t, c, true)
	stopEngine(t, f.root)
	receipt, err := l.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(receipt)
	journal, err := os.ReadFile(filepath.Join(c.Authority.Directory, journalName))
	if err != nil || strings.Contains(string(encoded), "synthetic-config-never-in-receipt") || strings.Contains(string(journal), "synthetic-config-never-in-receipt") {
		t.Fatal("bootstrap configuration escaped into durable evidence")
	}
	probeLock(t, c, false)
}

func TestBootstrapUnsafeDescriptorDeniedBeforeExec(t *testing.T) {
	for _, kind := range []string{"writable", "unowned", "permissions", "overcap", "empty", "closed"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			path := filepath.Join(f.root, "protected", "bootstrap.json")
			data := []byte(`{"fixture":true}`)
			if kind == "empty" {
				data = nil
			}
			if kind == "overcap" {
				data = make([]byte, 65537)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if kind == "unowned" {
				if err := os.Chown(path, int(fixtureUID), int(fixtureGID)); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "permissions" {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			}
			flags := os.O_RDONLY
			if kind == "writable" {
				flags = os.O_RDWR
			}
			file, err := os.OpenFile(path, flags, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()
			if kind == "closed" {
				_ = file.Close()
			}
			c := f.config
			c.BootstrapFile = file
			l, err := Start(c)
			trackLauncher(t, l)
			if l != nil || !errors.Is(err, ErrConfig) {
				t.Fatal("unsafe bootstrap reached exec", err)
			}
			if _, err := os.Stat(filepath.Join(f.root, "exchange", "engine.json")); !os.IsNotExist(err) {
				t.Fatal("unsafe bootstrap child ran")
			}
			if kind != "closed" {
				if _, err := file.Stat(); err != nil {
					t.Fatal("Start closed caller-owned file")
				}
			}
		})
	}
}

// Regression: exec must retain the SAME open lock description with CLOEXEC
// cleared, while separate engine credentials cannot modify protected metadata.
func TestInheritedDescriptionAndAuthority(t *testing.T) {
	t.Log("Regression: inherited FD survives exec, shares flock and cannot write supervisor metadata")
	f := newFixture(t)
	l := startFixture(t, f.config)
	var report engineReport
	if err := json.Unmarshal(waitFile(t, filepath.Join(f.root, "exchange", "engine.json")), &report); err != nil {
		t.Fatal(err)
	}
	b := l.Binding()
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(b.Incarnation) || report.Incarnation != b.Incarnation {
		t.Fatal("kernel canonical UUID differs from inherited engine identity")
	}
	if report.Device != b.LockDevice || report.Inode != b.LockInode || report.FDFlags&syscall.FD_CLOEXEC != 0 || report.AccessMode != syscall.O_RDONLY || !report.SharedFlock || !report.MetadataDenied || report.UID != int(fixtureUID) || report.GID != int(fixtureGID) {
		t.Fatalf("inherited descriptor/authority violation: %+v", report)
	}
	for _, g := range report.Groups {
		if g == 0 {
			t.Fatal("root supplementary group inherited")
		}
	}
	probeLock(t, f.config, true)
	stopEngine(t, f.root)
	ctx, cancel := deadline(t)
	defer cancel()
	r, err := l.Wait(ctx)
	if err != nil || r.Binding != b || r.Scope != LocalTeardownScope || r.Method != "observed-child-exit" {
		t.Fatalf("exact exit receipt failed: %v", err)
	}
	probeLock(t, f.config, false)
	read, err := ReadReceipt(f.config.Authority, b)
	if err != nil || read != r {
		t.Fatal("protected exact readback failed")
	}
}

func runSupervisor(t *testing.T, f fixture, mode, result string) *exec.Cmd {
	t.Helper()
	p := exec.Command(f.config.EnginePath, "-test.run=^TestSyntheticProcess$", "--", "supervisor", f.root, mode, result)
	p.Env = []string{}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p.ProcessState == nil {
			_ = p.Process.Kill()
			_ = p.Wait()
		}
	})
	return p
}

func exactBinding(t *testing.T, path string) supervisorReport {
	t.Helper()
	var r supervisorReport
	if err := json.Unmarshal(waitFile(t, path), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// Regression: killing a supervisor must not drop the inherited flock or allow
// rollover/old-child cancellation. Recovery must preserve exact old evidence.
func TestSupervisorLossAndExactRecovery(t *testing.T) {
	t.Log("Regression: surviving child denies rollover after supervisor death, then exact recovery retains the previous receipt")
	f := newFixture(t)
	p := runSupervisor(t, f, "hold", "supervisor.json")
	old := exactBinding(t, filepath.Join(f.root, "supervisor.json"))
	if old.Status != "started" {
		t.Fatal("synthetic supervisor failed")
	}
	// Cleanup targets this synthetic child only, using its observed birth.
	t.Cleanup(func() { cleanupFixtureProcess(t, old.Binding) })
	if err := p.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err == nil {
		t.Fatal("supervisor was not killed")
	}
	probeLock(t, f.config, true)
	l, startErr := Start(f.config)
	trackLauncher(t, l)
	if l != nil || !errors.Is(startErr, ErrBusy) {
		t.Fatalf("surviving child rollover: %v", startErr)
	}
	n, state, err := birth(old.Binding.PID)
	if err != nil || n != old.Binding.BirthTicks || state == 'Z' {
		t.Fatal("second launcher affected the surviving old child")
	}
	if _, err = ReadReceipt(f.config.Authority, old.Binding); err == nil {
		t.Fatal("live child certified retired")
	}
	assertRetirementLookup(t, f, old.Binding, []RetirementSelector{{old.Binding.OriginRef, old.Binding.Incarnation}}, nil)
	stopEngine(t, f.root)
	until := time.Now().Add(5 * time.Second)
	for {
		n, state, err = birth(old.Binding.PID)
		if os.IsNotExist(err) || (err == nil && n == old.Binding.BirthTicks && state == 'Z') {
			break
		}
		if time.Now().After(until) {
			t.Fatal("synthetic old child exit not observed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	probeLock(t, f.config, false)
	if err = os.Remove(filepath.Join(f.root, "exchange", "stop")); err != nil {
		t.Fatal(err)
	}
	p = runSupervisor(t, f, "hold", "supervisor-b.json")
	b := exactBinding(t, filepath.Join(f.root, "supervisor-b.json"))
	if b.Status != "started" || b.Binding.Incarnation == old.Binding.Incarnation {
		t.Fatal("incarnation reused")
	}
	t.Cleanup(func() { cleanupFixtureProcess(t, b.Binding) })
	r, err := ReadReceipt(f.config.Authority, old.Binding)
	if err != nil || r.Binding != old.Binding || r.Method != "reacquired-after-supervisor-loss" {
		t.Fatalf("old recovery evidence overwritten: %v", err)
	}
	wrong := old.Binding
	wrong.Incarnation = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	if _, err = ReadReceipt(f.config.Authority, wrong); !errors.Is(err, ErrBinding) {
		t.Fatal("wrong incarnation accepted")
	}
	assertRetirementLookup(t, f, b.Binding, []RetirementSelector{{old.Binding.OriginRef, old.Binding.Incarnation}}, []Binding{old.Binding})
	// Lose B after Current overwrite and before any A cleanup/publication.
	// The supervisor helper publishes only its own binding, never priorReceipt.
	if err = p.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = p.Wait()
	assertRetirementLookup(t, f, b.Binding, []RetirementSelector{{b.Binding.OriginRef, b.Binding.Incarnation}}, nil)
	probeLock(t, f.config, true)
	stopEngine(t, f.root)
	waitFixtureExit(t, b.Binding)
	if err = os.Remove(filepath.Join(f.root, "exchange", "stop")); err != nil {
		t.Fatal(err)
	}
	c := startFixture(t, f.config)
	selectors := []RetirementSelector{{old.Binding.OriginRef, old.Binding.Incarnation}, {b.Binding.OriginRef, b.Binding.Incarnation}}
	assertRetirementLookup(t, f, c.Binding(), selectors, []Binding{old.Binding, b.Binding})
	testRetirementDenials(t, f, c.Binding(), selectors[0])
	stopEngine(t, f.root)
	ctx, cancel := deadline(t)
	defer cancel()
	if _, err = c.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(f.root, "exchange", "stop")); err != nil {
		t.Fatal(err)
	}
	// An actual failed exec overwrites Current with start-failed, but cannot
	// erase A; the next successful current can still select the durable receipt.
	bad := filepath.Join(f.root, "failed-retirement-exec")
	if err = os.WriteFile(bad, []byte("\x7fELFsynthetic-invalid-image"), 0755); err != nil {
		t.Fatal(err)
	}
	failed := f.config
	failed.EnginePath = bad
	failedLauncher, startErr := Start(failed)
	trackLauncher(t, failedLauncher)
	if failedLauncher != nil || !errors.Is(startErr, ErrStart) {
		t.Fatal("expected actual failed exec", startErr)
	}
	d := startFixture(t, f.config)
	assertRetirementLookup(t, f, d.Binding(), selectors[:1], []Binding{old.Binding})
}

func waitFixtureExit(t *testing.T, b Binding) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		n, state, err := birth(b.PID)
		if os.IsNotExist(err) || (err == nil && n == b.BirthTicks && (state == 'Z' || state == 'X')) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("exact disposable engine exit absent")
}

func assertRetirementLookup(t *testing.T, f fixture, current Binding, selectors []RetirementSelector, want []Binding) {
	t.Helper()
	path := filepath.Join(f.config.Authority.Directory, journalName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var j journal
	if json.Unmarshal(before, &j) != nil {
		t.Fatal("fixture journal malformed")
	}
	n, state, birthErr := birth(j.Current.Binding.PID)
	receipts, err := ReadRetirements(f.config.Authority, current, selectors)
	if want == nil {
		if err == nil || receipts != nil {
			t.Fatal("denied lookup returned positive evidence")
		}
	} else {
		if err != nil || len(receipts) != len(want) {
			t.Fatal("durable selected evidence absent", err)
		}
		for i, r := range receipts {
			if r.Binding != want[i] || r.Scope != LocalTeardownScope || (r.Method != "observed-child-exit" && r.Method != "reacquired-after-supervisor-loss") {
				t.Fatal("selected evidence changed identity/order/scope")
			}
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("readonly lookup changed source journal")
	}
	if _, err = os.Stat(filepath.Join(f.config.Authority.Directory, stagingName)); !os.IsNotExist(err) {
		t.Fatal("readonly lookup created publication staging")
	}
	if birthErr == nil && state != 'Z' && state != 'X' {
		got, s, e := birth(j.Current.Binding.PID)
		if e != nil || got != n || s == 'Z' || s == 'X' {
			t.Fatal("readonly lookup affected exact live engine")
		}
		probeLock(t, f.config, true)
	}
}

func testRetirementDenials(t *testing.T, f fixture, current Binding, old RetirementSelector) {
	t.Helper()
	for _, field := range []string{"origin", "boot", "incarnation", "pid", "birth", "device", "inode"} {
		wrong := current
		switch field {
		case "origin":
			wrong.OriginRef += "-foreign"
		case "boot":
			wrong.BootID = "00000000-0000-0000-0000-000000000000"
		case "incarnation":
			wrong.Incarnation = old.EngineIncarnation
		case "pid":
			wrong.PID = os.Getpid() // live different process, never a signal target
		case "birth":
			wrong.BirthTicks++ // same PID with a reused/different birth identity
		case "device":
			wrong.LockDevice++
		case "inode":
			wrong.LockInode++
		}
		assertRetirementLookup(t, f, wrong, []RetirementSelector{old}, nil)
	}
	for _, selectors := range [][]RetirementSelector{
		nil, {old, old}, {old, old, old},
		{{old.OriginRef + "-foreign", old.EngineIncarnation}},
		{{current.OriginRef, current.Incarnation}},
		{{current.OriginRef, "00000000-0000-0000-0000-000000000000"}},
		{old, {current.OriginRef, "00000000-0000-0000-0000-000000000000"}},
		{{current.OriginRef, strings.ReplaceAll(old.EngineIncarnation, "-", "")}},
	} {
		assertRetirementLookup(t, f, current, selectors, nil)
	}
	// Validate the ENTIRE journal, including unselected records and ambiguity.
	path := filepath.Join(f.config.Authority.Directory, journalName)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.WriteFile(path, original, 0600); err != nil {
			t.Error(err)
		}
	}()
	for _, mode := range []string{"ambiguous", "unselected-corrupt"} {
		var j journal
		if json.Unmarshal(original, &j) != nil {
			t.Fatal("fixture journal malformed")
		}
		if mode == "ambiguous" {
			j.Receipts = append(j.Receipts, j.Receipts[0])
		} else {
			j.Receipts[len(j.Receipts)-1].Scope = "invalid-local-scope"
		}
		data, e := json.Marshal(j)
		if e != nil || os.WriteFile(path, data, 0600) != nil {
			t.Fatal("cannot publish owned corrupt fixture")
		}
		assertRetirementLookup(t, f, current, []RetirementSelector{old}, nil)
	}
}

// Regression: actual competing root supervisor processes must have one winner;
// the losing startup must not kill the winner or replace its lock inode.
func TestRacingSupervisors(t *testing.T) {
	t.Log("Regression: two racing supervisor processes have one winner and one fail-closed flock denial")
	f := newFixture(t)
	p := runSupervisor(t, f, "race", "race-a.json")
	q := runSupervisor(t, f, "race", "race-b.json")
	if err := os.WriteFile(filepath.Join(f.root, "barrier"), []byte("go"), 0600); err != nil {
		t.Fatal(err)
	}
	a := exactBinding(t, filepath.Join(f.root, "race-a.json"))
	b := exactBinding(t, filepath.Join(f.root, "race-b.json"))
	if (a.Status != "started" || b.Status != "busy") && (b.Status != "started" || a.Status != "busy") {
		t.Fatalf("race outcomes: %s/%s", a.Status, b.Status)
	}
	winner := a
	if b.Status == "started" {
		winner = b
	}
	t.Cleanup(func() { cleanupFixtureProcess(t, winner.Binding) })
	probeLock(t, f.config, true)
	stopEngine(t, f.root)
	if err := p.Wait(); err != nil {
		t.Fatal("race supervisor a failed")
	}
	if err := q.Wait(); err != nil {
		t.Fatal("race supervisor b failed")
	}
	if _, err := ReadReceipt(f.config.Authority, winner.Binding); err != nil {
		t.Fatal("race winner receipt missing")
	}
}

// Regression: unsafe paths, owners, permissions, symlinks and hardlinks cannot
// become supervisor authority or an alternate fixed lock inode.
func TestUnsafeAuthority(t *testing.T) {
	t.Log("Regression: unsafe protected filesystem authority is rejected before starting any child")
	cases := []string{"directory-mode", "directory-owner", "directory-symlink", "lock-symlink", "lock-mode", "lock-owner", "lock-hardlink", "metadata-symlink", "metadata-mode", "staging-symlink", "ancestor-mode", "engine-symlink", "engine-mode", "root-engine-uid", "sentinel-engine-uid", "sentinel-engine-gid"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Log("Regression: reject " + name)
			f := newFixture(t)
			lock := filepath.Join(f.config.Authority.Directory, lockName)
			metadata := filepath.Join(f.config.Authority.Directory, journalName)
			if err := os.WriteFile(lock, nil, 0600); err != nil {
				t.Fatal(err)
			}
			switch name {
			case "directory-mode":
				if err := os.Chmod(f.config.Authority.Directory, 0755); err != nil {
					t.Fatal(err)
				}
			case "directory-owner":
				if err := os.Chown(f.config.Authority.Directory, int(fixtureUID), int(fixtureGID)); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				old := f.config.Authority.Directory + "-real"
				if err := os.Rename(f.config.Authority.Directory, old); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(old, f.config.Authority.Directory); err != nil {
					t.Fatal(err)
				}
			case "lock-symlink":
				if err := os.Remove(lock); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/dev/null", lock); err != nil {
					t.Fatal(err)
				}
			case "lock-mode":
				if err := os.Chmod(lock, 0666); err != nil {
					t.Fatal(err)
				}
			case "lock-owner":
				if err := os.Chown(lock, int(fixtureUID), int(fixtureGID)); err != nil {
					t.Fatal(err)
				}
			case "lock-hardlink":
				if err := os.Link(lock, filepath.Join(f.config.Authority.Directory, "second-link")); err != nil {
					t.Fatal(err)
				}
			case "metadata-symlink":
				if err := os.Symlink("/dev/null", metadata); err != nil {
					t.Fatal(err)
				}
			case "metadata-mode":
				if err := os.WriteFile(metadata, []byte("{}"), 0644); err != nil {
					t.Fatal(err)
				}
			case "staging-symlink":
				if err := os.Symlink("/dev/null", filepath.Join(f.config.Authority.Directory, stagingName)); err != nil {
					t.Fatal(err)
				}
			case "ancestor-mode":
				if err := os.Chmod(f.root, 0777); err != nil {
					t.Fatal(err)
				}
			case "engine-symlink":
				if err := os.Rename(f.config.EnginePath, f.config.EnginePath+"-real"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(f.config.EnginePath+"-real", f.config.EnginePath); err != nil {
					t.Fatal(err)
				}
			case "engine-mode":
				if err := os.Chmod(f.config.EnginePath, 0777); err != nil {
					t.Fatal(err)
				}
			case "root-engine-uid":
				f.config.EngineUID = 0
			case "sentinel-engine-uid":
				f.config.EngineUID = ^uint32(0)
			case "sentinel-engine-gid":
				f.config.EngineGID = ^uint32(0)
			}
			l, err := Start(f.config)
			trackLauncher(t, l)
			if l != nil {
				t.Fatal("unsafe authority started a child")
			}
			if err == nil {
				t.Fatal("unsafe authority accepted")
			}
		})
	}
}

// Regression: a replaced live lock path cannot certify retirement or create
// new authority while the original child still holds its inherited old inode.
func TestLockInodeReplacement(t *testing.T) {
	t.Log("Regression: replacing the lock pathname never rolls authority or certifies old retirement")
	f := newFixture(t)
	l := startFixture(t, f.config)
	lock := filepath.Join(f.config.Authority.Directory, lockName)
	if err := os.Rename(lock, lock+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	next, startErr := Start(f.config)
	trackLauncher(t, next)
	if next != nil || !errors.Is(startErr, ErrBinding) {
		t.Fatalf("replacement accepted: %v", startErr)
	}
	stopEngine(t, f.root)
	ctx, cancel := deadline(t)
	defer cancel()
	if _, err := l.Wait(ctx); !errors.Is(err, ErrBinding) {
		t.Fatalf("replaced inode certified: %v", err)
	}
	if _, err := ReadReceipt(f.config.Authority, l.Binding()); !errors.Is(err, ErrBinding) {
		t.Fatal("replaced inode read accepted")
	}
	// Restore the exact original inode under root fixture control, then clean.
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(lock+"-original", lock); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

// Regression: an actual exec failure cannot produce ready or retirement proof;
// its durable no-child start-failed fact permits a later configured startup.
func TestFailedExec(t *testing.T) {
	t.Log("Regression: failed exec records no child and never produces a retirement receipt")
	f := newFixture(t)
	bad := filepath.Join(f.root, "not-an-executable-image")
	if err := os.WriteFile(bad, []byte("\x7fELFsynthetic-invalid-image"), 0755); err != nil {
		t.Fatal(err)
	}
	c := f.config
	c.EnginePath = bad
	l, startErr := Start(c)
	trackLauncher(t, l)
	if l != nil || !errors.Is(startErr, ErrStart) {
		t.Fatalf("failed exec result: %v", startErr)
	}
	a, err := openAuthority(c.Authority.Directory)
	if err != nil {
		t.Fatal(err)
	}
	j, exists, err := a.read()
	a.close()
	if err != nil || !exists || j.Current.Phase != "start-failed" || j.Current.Binding.PID != 0 || len(j.Receipts) != 0 {
		t.Fatal("failed exec lied about lifecycle")
	}
	startFixture(t, f.config)
}

// Regression: actual Linux directory fsync failure cannot report ready or a
// durable receipt. O_PATH permits openat/Fstat but fsync returns EBADF: no mock.
func TestFailedDirectoryFsync(t *testing.T) {
	t.Log("Regression: an actual failed directory fsync denies readiness and durable retirement")
	f := newFixture(t)
	l, err := Start(f.config)
	if l == nil || err != nil {
		t.Fatalf("synthetic start: %v", err)
	}
	trackLauncher(t, l)
	j, exists, err := l.a.read()
	if err != nil || !exists {
		t.Fatal("starting journal unavailable")
	}
	original := l.a.dir
	fd, err := syscall.Open(f.config.Authority.Directory, 0x200000|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0) // Linux O_PATH
	if err != nil {
		t.Fatal(err)
	}
	opath := os.NewFile(uintptr(fd), "synthetic-opath-directory")
	defer func() {
		if l.a.dir == opath {
			l.a.dir = original
		}
		_ = opath.Close()
	}()
	if e := opath.Sync(); !errors.Is(e, syscall.EBADF) {
		_ = opath.Close()
		t.Fatalf("actual fsync failure not established: %v", e)
	}
	l.a.dir = opath
	ctx, cancel := deadline(t)
	defer cancel()
	select {
	case <-l.readyDone:
	case <-ctx.Done():
		t.Fatal("synthetic readiness signal missing")
	}
	if !l.readyObserved {
		t.Fatal("synthetic child did not actually signal readiness")
	}
	j.Current.Phase = "ready"
	if err = l.a.write(j); !errors.Is(err, ErrStorage) {
		t.Fatalf("actual publication fsync failure ignored: %v", err)
	}
	if err = l.AwaitReady(ctx); !errors.Is(err, ErrStorage) {
		t.Fatalf("failed fsync reported ready: %v", err)
	}
	l.a.dir = original
	if err = l.AwaitReady(ctx); err != nil {
		t.Fatal("durability restoration did not recover actual readiness")
	}
	l.a.dir = opath
	stopEngine(t, f.root)
	if _, err = l.Wait(ctx); !errors.Is(err, ErrStorage) {
		t.Fatalf("failed fsync certified retirement: %v", err)
	}
	l.a.dir = original
	_ = opath.Close()
	if _, err = l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

// Regression: exact receipt readback rejects each altered original binding,
// including a reused PID identity, host boot, incarnation and receipt scope.
func TestExactReceiptBindings(t *testing.T) {
	t.Log("Regression: original scope and every binding field are mandatory for retirement readback")
	f := newFixture(t)
	l := startFixture(t, f.config)
	stopEngine(t, f.root)
	ctx, cancel := deadline(t)
	defer cancel()
	if _, err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	original := l.Binding()
	for _, field := range []string{"origin", "boot", "pid", "birth", "incarnation", "device", "inode"} {
		t.Run(field, func(t *testing.T) {
			t.Log("Regression: reject altered original " + field)
			b := original
			switch field {
			case "origin":
				b.OriginRef = "another-origin"
			case "boot":
				b.BootID = "00000000-0000-0000-0000-000000000000"
			case "pid":
				b.PID++
			case "birth":
				b.BirthTicks++
			case "incarnation":
				b.Incarnation = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
			case "device":
				b.LockDevice++
			case "inode":
				b.LockInode++
			}
			if _, err := ReadReceipt(f.config.Authority, b); !errors.Is(err, ErrBinding) {
				t.Fatal("altered binding accepted")
			}
		})
	}
	a, err := openAuthority(f.config.Authority.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	if err = a.acquire(false); err != nil {
		t.Fatal(err)
	}
	j, _, err := a.read()
	if err != nil {
		t.Fatal(err)
	}
	j.Receipts[0].Scope = "provider-no-effect"
	if err = a.write(j); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadReceipt(f.config.Authority, original); !errors.Is(err, ErrBinding) {
		t.Fatal("wrong receipt scope accepted")
	}
}

// Regression: a free lock plus wrong protected boot/process identity must not
// turn PID reuse, a fresh boot, or a live process into old retirement proof.
func TestRecoveryIdentityDenials(t *testing.T) {
	t.Log("Regression: a free lock cannot certify wrong boot or reused/live process identity")
	for _, mode := range []string{"boot", "pid-reuse", "live-exact", "reserved", "legacy-incarnation", "uppercase-incarnation", "unhyphenated-incarnation", "legacy-receipt"} {
		t.Run(mode, func(t *testing.T) {
			t.Log("Regression: recovery rejects " + mode)
			f := newFixture(t)
			l := startFixture(t, f.config)
			stopEngine(t, f.root)
			ctx, cancel := deadline(t)
			defer cancel()
			if _, err := l.Wait(ctx); err != nil {
				t.Fatal(err)
			}
			a, err := openAuthority(f.config.Authority.Directory)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.acquire(false); err != nil {
				a.close()
				t.Fatal(err)
			}
			j, _, err := a.read()
			if err != nil {
				a.close()
				t.Fatal(err)
			}
			j.Receipts = nil
			j.Current.Phase = "starting"
			switch mode {
			case "boot":
				j.Current.Binding.BootID = "00000000-0000-0000-0000-000000000000"
			case "pid-reuse":
				j.Current.Binding.PID = os.Getpid()
				n, _, e := birth(os.Getpid())
				if e != nil {
					t.Fatal(e)
				}
				j.Current.Binding.BirthTicks = n + 1
			case "live-exact":
				j.Current.Binding.PID = os.Getpid()
				n, _, e := birth(os.Getpid())
				if e != nil {
					t.Fatal(e)
				}
				j.Current.Binding.BirthTicks = n
			case "legacy-incarnation":
				j.Current.Binding.Incarnation = strings.Repeat("a", 64)
			case "uppercase-incarnation":
				j.Current.Binding.Incarnation = "ABCDEFAB-CDEF-4ABC-8DEF-ABCDEFABCDEF"
			case "unhyphenated-incarnation":
				j.Current.Binding.Incarnation = "abcdefabcdef4abc8defabcdefabcdef"
			case "legacy-receipt":
				legacy := j.Current.Binding
				legacy.Incarnation = strings.Repeat("b", 64)
				j.Receipts = []Receipt{{legacy, LocalTeardownScope, "observed-child-exit"}}
				j.Current.Phase = "start-failed"
				j.Current.Binding.PID = 0
				j.Current.Binding.BirthTicks = 0
			case "reserved":
				j.Current.Phase = "reserved"
				j.Current.Binding.PID = 0
				j.Current.Binding.BirthTicks = 0
			}
			if err = a.write(j); err != nil {
				a.close()
				t.Fatal(err)
			}
			a.close()
			before, err := os.ReadFile(filepath.Join(f.config.Authority.Directory, journalName))
			if err != nil {
				t.Fatal(err)
			}
			probeLock(t, f.config, false)
			next, startErr := Start(f.config)
			trackLauncher(t, next)
			if next != nil || startErr == nil {
				t.Fatal("identity mismatch or unresolved reservation recovered")
			}
			after, err := os.ReadFile(filepath.Join(f.config.Authority.Directory, journalName))
			if err != nil || string(before) != string(after) {
				t.Fatal("denied identity was guessed, migrated or discarded")
			}
			if _, err := ReadReceipt(f.config.Authority, l.Binding()); err == nil {
				t.Fatal("denial manufactured receipt")
			}
		})
	}
}

// Regression: bounded cancellation of a stubborn child must return pending,
// retain real lock ownership, deny a second start, and avoid false retirement.
func TestBoundedStubbornShutdown(t *testing.T) {
	t.Log("Regression: shutdown timeout is pending, not child exit or retirement")
	f := newFixture(t)
	c := fixtureConfig(f.root, "stubborn")
	l := startFixture(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := l.Shutdown(ctx); !errors.Is(err, ErrPending) {
		t.Fatalf("stubborn shutdown: %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("shutdown exceeded its bound")
	}
	n, state, err := birth(l.Binding().PID)
	if err != nil || n != l.Binding().BirthTicks || state == 'Z' {
		t.Fatal("timeout was not a live exact child")
	}
	probeLock(t, c, true)
	next, startErr := Start(c)
	trackLauncher(t, next)
	if next != nil || !errors.Is(startErr, ErrBusy) {
		t.Fatal("pending child lost lock authority")
	}
	if _, err := ReadReceipt(c.Authority, l.Binding()); err == nil {
		t.Fatal("timeout certified retirement")
	}
	stopEngine(t, f.root)
	finish, done := deadline(t)
	defer done()
	if _, err = l.Wait(finish); err != nil {
		t.Fatal(err)
	}
}

// Regression: observing child exit is insufficient while another real helper
// retains the inherited description. All holders must exit before a receipt.
func TestRetainedHolderAfterChildExit(t *testing.T) {
	t.Log("Regression: an inherited description retained beyond child exit prevents retirement and new startup")
	f := newFixture(t)
	l := startFixture(t, f.config)
	p := exec.Command(f.config.EnginePath, "-test.run=^TestSyntheticProcess$", "--", "retainer", f.root, "hold")
	p.Env = []string{}
	p.ExtraFiles = []*os.File{l.a.lock}
	p.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: fixtureUID, Gid: fixtureGID, Groups: []uint32{}}}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if p.ProcessState == nil {
			_ = p.Process.Kill()
			_ = p.Wait()
		}
	})
	waitFile(t, filepath.Join(f.root, "exchange", "retainer.json"))
	stopEngine(t, f.root)
	ctx, cancel := deadline(t)
	defer cancel()
	if _, err := l.Wait(ctx); !errors.Is(err, ErrBusy) {
		t.Fatalf("retained description certified retirement: %v", err)
	}
	probeLock(t, f.config, true)
	next, startErr := Start(f.config)
	trackLauncher(t, next)
	if next != nil || !errors.Is(startErr, ErrBusy) {
		t.Fatal("retained holder allowed startup")
	}
	if _, err := ReadReceipt(f.config.Authority, l.Binding()); err == nil {
		t.Fatal("retained holder got a receipt")
	}
	if err := os.WriteFile(filepath.Join(f.root, "exchange", "stop-retainer"), []byte("stop"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := p.Wait(); err != nil {
		t.Fatal("synthetic retainer failed")
	}
	if _, err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

// Restart history is bounded by bytes, not an arbitrary number of restarts.
func TestRetirementHistoryBeyond64(t *testing.T) {
	f := newFixture(t)
	var bindings []Binding
	for i := 0; i < 129; i++ {
		if i > 0 {
			if err := os.Remove(filepath.Join(f.root, "exchange", "stop")); err != nil {
				t.Fatal(err)
			}
		}
		if i == 64 || i == 128 {
			t.Logf("starting restart %d", i+1)
		}
		l := startFixture(t, f.config)
		bindings = append(bindings, l.Binding())
		stopEngine(t, f.root)
		ctx, cancel := deadline(t)
		_, err := l.Wait(ctx)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if i == 64 || i == 128 {
			for _, binding := range bindings {
				if _, err := ReadReceipt(f.config.Authority, binding); err != nil {
					t.Fatalf("history lost after %d restarts: %v", i+1, err)
				}
			}
		}
	}
}

func TestJournalByteBoundPreservesEvidence(t *testing.T) {
	f := newFixture(t)
	l := startFixture(t, f.config)
	binding := l.Binding()
	stopEngine(t, f.root)
	ctx, cancel := deadline(t)
	receipt, err := l.Wait(ctx)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.config.Authority.Directory, journalName)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := openAuthority(f.config.Authority.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	if err = a.acquire(false); err != nil {
		t.Fatal(err)
	}
	j, _, err := a.read()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	j.Receipts = make([]Receipt, maxJournalBytes/len(encoded)+1)
	for i := range j.Receipts {
		j.Receipts[i] = receipt
	}
	if err = a.write(j); !errors.Is(err, ErrStorage) {
		t.Fatalf("oversize write accepted: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("oversize write changed existing evidence")
	}
	a.close()
	if _, err = ReadReceipt(f.config.Authority, binding); err != nil {
		t.Fatal("existing receipt lost:", err)
	}
	for _, tc := range []struct{ name, raw string }{
		{"duplicate", strings.Replace(string(before), `"version":1`, `"version":1,"version":1`, 1)},
		{"malformed", "{"},
		{"oversize", strings.Repeat(" ", maxJournalBytes+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.raw), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadReceipt(f.config.Authority, binding); err == nil {
				t.Fatal("invalid journal accepted")
			}
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
		})
	}
	// A valid near-full journal must refuse launch before a new reservation
	// can consume the space needed for its eventual exact retirement receipt.
	if err = json.Unmarshal(before, &j); err != nil {
		t.Fatal(err)
	}
	extra := (maxJournalBytes - len(before)) / (len(encoded) + 1)
	for i := 0; i < extra; i++ {
		r := receipt
		r.Binding.Incarnation = fmt.Sprintf("00000000-0000-4000-8000-%012x", i+1)
		j.Receipts = append(j.Receipts, r)
	}
	a, err = openAuthority(f.config.Authority.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.acquire(false); err != nil {
		a.close()
		t.Fatal(err)
	}
	if err = a.write(j); err != nil {
		a.close()
		t.Fatal(err)
	}
	a.close()
	nearFull, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	next, startErr := Start(f.config)
	trackLauncher(t, next)
	if next != nil || !errors.Is(startErr, ErrSaturated) {
		t.Fatalf("near-full journal started child: %v", startErr)
	}
	after, err = os.ReadFile(path)
	if err != nil || string(nearFull) != string(after) {
		t.Fatal("capacity refusal changed existing journal")
	}
}

// Regression: mutating caller-owned slices after startup must not modify the
// child's frozen environment/arguments or generated process authority.
func TestConfigurationSnapshot(t *testing.T) {
	t.Log("Regression: configuration and generated identity remain immutable for the launch")
	f := newFixture(t)
	c := f.config
	c.Env = []string{"SYNTHETIC_VALUE=original"}
	l := startFixture(t, c)
	b := l.Binding()
	c.Args[0] = "changed"
	c.Env[0] = "SYNTHETIC_VALUE=changed"
	c.Authority.OriginRef = "changed"
	data, err := os.ReadFile("/proc/" + strconv.Itoa(b.PID) + "/cmdline")
	if err != nil || !strings.Contains(string(data), "-test.run=^TestSyntheticProcess$") || strings.Contains(string(data), "changed") {
		t.Fatal("argv snapshot mutated")
	}
	if l.Binding() != b || b.OriginRef != "opaque-disposable-origin" {
		t.Fatal("immutable launch authority mutated")
	}
	var report engineReport
	if err = json.Unmarshal(waitFile(t, filepath.Join(f.root, "exchange", "engine.json")), &report); err != nil || report.SyntheticMarker != "original" {
		t.Fatal("synthetic environment snapshot mutated")
	}
}

// Regression: a crash-abandoned staging slot must not be overwritten or
// discarded, and a fresh startup must preserve already durable old evidence.
func TestBoundedAbandonedStaging(t *testing.T) {
	t.Log("Regression: abandoned staging denies new publication without overwriting unresolved or previous evidence")
	f := newFixture(t)
	l := startFixture(t, f.config)
	stopEngine(t, f.root)
	ctx, cancel := deadline(t)
	defer cancel()
	if _, err := l.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	slot := filepath.Join(f.config.Authority.Directory, stagingName)
	content := []byte("synthetic unresolved staging bytes")
	if err := os.WriteFile(slot, content, 0600); err != nil {
		t.Fatal(err)
	}
	prior, err := os.ReadFile(filepath.Join(f.config.Authority.Directory, journalName))
	if err != nil {
		t.Fatal(err)
	}
	next, startErr := Start(f.config)
	trackLauncher(t, next)
	if next != nil || startErr == nil {
		t.Fatal("abandoned staging slot allowed publication")
	}
	after, err := os.ReadFile(slot)
	if err != nil || string(after) != string(content) {
		t.Fatal("unresolved staging bytes discarded")
	}
	journalAfter, err := os.ReadFile(filepath.Join(f.config.Authority.Directory, journalName))
	if err != nil || string(journalAfter) != string(prior) {
		t.Fatal("old journal overwritten on staging denial")
	}
	if _, err = ReadReceipt(f.config.Authority, l.Binding()); err != nil {
		t.Fatal("previous exact receipt lost")
	}
}

// Pin BEFORE reading birth. A subsequent exit or numeric PID reuse cannot
// retarget this descriptor. There is deliberately no numeric-signal fallback.
func pinFixtureProcess(b Binding) (int, error) {
	fd, err := unix.PidfdOpen(b.PID, 0)
	if err != nil {
		return -1, err
	}
	n, state, err := birth(b.PID)
	if err != nil || n != b.BirthTicks || state == 'Z' || state == 'X' {
		_ = unix.Close(fd)
		if os.IsNotExist(err) || state == 'Z' || state == 'X' {
			return -1, unix.ESRCH
		}
		return -1, ErrBinding
	}
	return fd, nil
}

func cleanupFixtureProcess(t *testing.T, b Binding) {
	t.Helper()
	fd, err := pinFixtureProcess(b)
	if errors.Is(err, unix.ESRCH) || errors.Is(err, ErrBinding) {
		return // absent original or replaced numeric identity: never signal it
	}
	if err != nil {
		t.Errorf("exact synthetic cleanup cannot acquire pidfd: %v", err)
		return
	}
	defer func() { _ = unix.Close(fd) }()
	if err = unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0); err != nil && !errors.Is(err, unix.ESRCH) {
		t.Errorf("exact synthetic pidfd cleanup failed: %v", err)
	}
}

// This uses only helpers present on e206: an old-source run compiles, reaches
// actual R/EOF or exit, then fails behaviorally when its mutex wait overruns.
func TestPublicationDeadlinesAfterObservation(t *testing.T) {
	t.Log("Regression: observed readiness/exit cannot bypass caller deadlines during serialized durability")
	for _, mode := range []string{"ready", "wait", "shutdown-with-background-wait"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			l, err := Start(f.config)
			trackLauncher(t, l)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := deadline(t)
			defer cancel()
			select {
			case <-l.readyDone:
			case <-ctx.Done():
				t.Fatal("actual readiness handshake absent")
			}
			if mode != "ready" {
				if err = l.AwaitReady(ctx); err != nil {
					t.Fatal(err)
				}
				stopEngine(t, f.root)
				select {
				case <-l.done:
				case <-ctx.Done():
					t.Fatal("actual child exit absent")
				}
			}
			l.mu.Lock()
			locked := true
			defer func() {
				if locked {
					l.mu.Unlock()
				}
			}()
			baselineWorkers := runtime.NumGoroutine()
			background := make(chan error, 1)
			if mode == "shutdown-with-background-wait" {
				go func() { _, e := l.Wait(context.Background()); background <- e }()
			}
			// Many retries must time out without creating publication workers,
			// losing the inherited lock, or falsely returning a durable fact.
			for retry := 0; retry < 8; retry++ {
				short, stop := context.WithTimeout(context.Background(), 25*time.Millisecond)
				result := make(chan error, 1)
				go func() {
					switch mode {
					case "ready":
						result <- l.AwaitReady(short)
					case "wait":
						_, e := l.Wait(short)
						result <- e
					default:
						_, e := l.Shutdown(short)
						result <- e
					}
				}()
				select {
				case e := <-result:
					stop()
					want := ErrPending
					if mode == "ready" {
						want = ErrNotReady
					}
					if !errors.Is(e, want) {
						t.Fatalf("deadline returned %v, want %v", e, want)
					}
				case <-time.After(time.Second):
					stop()
					l.mu.Unlock()
					locked = false
					<-result // release the old implementation before reporting red
					t.Fatal("observed lifecycle blocked beyond deadline until mutex release")
				}
			}
			if runtime.NumGoroutine() > baselineWorkers+3 {
				t.Fatal("deadline retries accumulated blocked publication workers")
			}
			probeLock(t, f.config, true)
			l.mu.Unlock()
			locked = false
			switch mode {
			case "ready":
				if err = l.AwaitReady(ctx); err != nil {
					t.Fatal("late durable readiness unavailable", err)
				}
				stopEngine(t, f.root)
			case "shutdown-with-background-wait":
				select {
				case err = <-background:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal("original publication did not finish")
				}
			}
			r, err := l.Wait(ctx)
			if err != nil || r.Binding != l.Binding() || r.Scope != LocalTeardownScope {
				t.Fatal("late exact durable receipt unavailable", err)
			}
			read, err := ReadReceipt(f.config.Authority, l.Binding())
			if err != nil || read != r {
				t.Fatal("cancellation lost exact durable readback", err)
			}
		})
	}
}

func TestRootDirectoryAuthority(t *testing.T) {
	t.Log("Regression: both walkers validate the opened filesystem root before traversing")
	f := newFixture(t)
	for _, mode := range []string{"unsafe", "safe"} {
		t.Run(mode, func(t *testing.T) {
			perm := os.FileMode(0777)
			if mode == "safe" {
				perm = 0755
			}
			// Only this test's own disposable directory is chmod'ed. Host / is
			// never modified; the child sees the directory as / through chroot.
			if err := os.Chmod(f.root, perm); err != nil {
				t.Fatal(err)
			}
			p := exec.Command(f.config.EnginePath, "-test.run=^TestSyntheticProcess$", "--", "root-check", f.root, mode)
			p.Env = []string{}
			if err := p.Run(); err != nil {
				var exit *exec.ExitError
				if errors.As(err, &exit) && exit.ExitCode() == 78 {
					t.Skip("NOT_RUN: owned chroot capability unavailable")
				}
				t.Fatalf("actual chroot root %04o rejected expected walker outcome: %v", perm, err)
			}
		})
	}
}

func TestPinnedOrphanCleanupPIDReuse(t *testing.T) {
	t.Log("Regression: exited orphan's pinned pidfd cannot signal an actual same-PID replacement")
	f := newFixture(t)
	p := exec.Command(f.config.EnginePath, "-test.run=^TestSyntheticProcess$", "--", "pid-reuse", f.root, "controlled")
	p.Env = []string{}
	p.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWPID | syscall.CLONE_NEWNS}
	output, err := p.CombinedOutput()
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skip("NOT_RUN: controlled private PID/mount namespace unavailable")
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 78 {
		t.Skip("NOT_RUN: namespace-local proc/pidfd/PID reuse authority unavailable")
	}
	if err != nil {
		t.Fatalf("actual controlled PID reuse fixture failed: %v\n%s", err, output)
	}
}

func qualifyFixturePIDReuse(t *testing.T, root string) {
	t.Helper()
	// Every numeric PID below belongs to this NEW namespace, never the host.
	// A private proc mount also makes birth reads and ns_last_pid local.
	if os.Getpid() != 1 {
		t.Fatal("PID-reuse fixture is not its own namespace init")
	}
	if unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, "") != nil || unix.Mount("proc", "/proc", "proc", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "") != nil {
		os.Exit(78)
	}
	f := fixture{root: root, config: fixtureConfig(root, "hold")}
	p := runSupervisor(t, f, "hold", "pin-supervisor.json")
	old := exactBinding(t, filepath.Join(root, "pin-supervisor.json"))
	if old.Status != "started" {
		t.Fatal("owned supervisor did not start")
	}
	fd, err := pinFixtureProcess(old.Binding)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EPERM) {
		os.Exit(78)
	}
	if err != nil {
		t.Fatal("cannot pin original owned child", err)
	}
	defer func() { _ = unix.Close(fd) }()
	if err = p.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = p.Wait()
	// This is an actual orphan; as namespace init we adopt and reap it.
	stopEngine(t, root)
	var status unix.WaitStatus
	if reaped, e := unix.Wait4(old.Binding.PID, &status, 0, nil); e != nil || reaped != old.Binding.PID {
		t.Fatal("original orphan was not reaped", e)
	}
	time.Sleep(30 * time.Millisecond) // force a distinct kernel birth tick
	// Reexec the copied fixture image actually running this controller.
	image, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var replacement *exec.Cmd
	for attempt := 0; attempt < 32; attempt++ {
		if err = os.Remove(filepath.Join(root, "exchange", "witness.json")); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err = os.WriteFile("/proc/sys/kernel/ns_last_pid", []byte(strconv.Itoa(old.Binding.PID-1)), 0600); err != nil {
			os.Exit(78)
		}
		// G702: reexec our own copied test ELF inside the private PID namespace;
		// root is owned fixture data passed as argv, with no shell/interpreter.
		q := exec.Command(image, "-test.run=^TestSyntheticProcess$", "--", "witness", root, "hold") //nolint:gosec
		q.Env = []string{}
		q.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: fixtureUID, Gid: fixtureGID, Groups: []uint32{}}}
		if err = q.Start(); err != nil {
			t.Fatal(err)
		}
		if q.Process.Pid == old.Binding.PID {
			replacement = q
			break
		}
		_ = q.Process.Kill() // exact owned exec handle, no numeric fallback
		_ = q.Wait()
	}
	if replacement == nil {
		t.Fatal("controlled fixture did not actually reuse the original PID")
	}
	defer func() { _ = replacement.Process.Kill(); _ = replacement.Wait() }()
	var witness map[string]int
	if err = json.Unmarshal(waitFile(t, filepath.Join(root, "exchange", "witness.json")), &witness); err != nil || witness["pid"] != old.Binding.PID {
		t.Fatal("replacement did not acknowledge its actual reused PID", err)
	}
	newBirth, state, err := birth(old.Binding.PID)
	if err != nil || newBirth == old.Binding.BirthTicks || state == 'Z' || state == 'X' {
		t.Fatal("replacement is not a distinct live kernel process", err)
	}
	if err = unix.PidfdSendSignal(fd, unix.SIGKILL, nil, 0); !errors.Is(err, unix.ESRCH) {
		t.Fatal("old pinned signal did not report original task gone", err)
	}
	if n, state, e := birth(old.Binding.PID); e != nil || n != newBirth || state == 'Z' || state == 'X' {
		t.Fatal("same-PID replacement was affected by orphan cleanup", e)
	}
	retarget, err := pinFixtureProcess(old.Binding)
	if retarget >= 0 {
		_ = unix.Close(retarget)
	}
	if !errors.Is(err, ErrBinding) {
		t.Fatal("cleanup accepted replacement's different birth", err)
	}
	if err = os.WriteFile(filepath.Join(root, "exchange", "stop-witness"), []byte("stop"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = replacement.Wait(); err != nil {
		t.Fatal("replacement did not survive to its own normal exit", err)
	}
}
