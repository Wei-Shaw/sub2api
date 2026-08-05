package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCreateAutomaticBackupWritesVerifiedManifest(t *testing.T) {
	cfg := testConfig(t, 19081)
	cfg.LoadedFrom = filepath.Join(cfg.ComposeWorkDir, "deployer-config.json")
	if err := os.WriteFile(cfg.LoadedFrom, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dockerConfig := filepath.Join(filepath.Dir(cfg.LoadedFrom), "docker", "config.json")
	if err := os.MkdirAll(filepath.Dir(dockerConfig), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dockerConfig, []byte("{\"auths\":{}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.BackupDockerConfigPath = dockerConfig
	now := time.Date(2026, 8, 5, 8, 0, 0, 123, time.UTC)
	state := State{
		ActiveVersion: "0.1.171-ts.1",
		ActiveImage:   managedTestImage('a'),
		UpdatedAt:     now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{
		cfg:       cfg,
		runner:    &fakeRunner{},
		now:       func() time.Time { return now },
		state:     state,
		buildInfo: BuildInfo{Version: "0.1.171-ts.1", Commit: "test", Date: "2026-08-05", Type: "release", Arch: "amd64"},
	}
	job := &Job{
		ID:            "backup-success-0001",
		Action:        "update",
		FromVersion:   state.ActiveVersion,
		FromImage:     state.ActiveImage,
		TargetVersion: "0.1.172-ts.1",
	}

	path, err := manager.createAutomaticBackup(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(cfg.BackupRootPath, "automatic") {
		t.Fatalf("backup path=%q", path)
	}
	if err := verifyBackupChecksums(path); err != nil {
		t.Fatalf("verify checksums: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest backupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Kind != "automatic-pre-update" || manifest.JobID != job.ID || manifest.TargetVersion != job.TargetVersion {
		t.Fatalf("manifest=%+v", manifest)
	}
	if manifest.Database.Format != "postgresql-custom" || len(manifest.Files) < 8 {
		t.Fatalf("backup did not contain the required files: %+v", manifest)
	}
	requiredNames := map[string]bool{"application-config": false, "deployer-docker-config": false}
	for _, record := range manifest.Files {
		if _, required := requiredNames[record.Name]; required {
			requiredNames[record.Name] = true
		}
	}
	for name, found := range requiredNames {
		if !found {
			t.Fatalf("backup is missing %s: %+v", name, manifest.Files)
		}
	}
	dump, err := os.ReadFile(filepath.Join(path, manifest.Database.File))
	if err != nil || !strings.HasPrefix(string(dump), "PGDMP") {
		t.Fatalf("database dump signature=%q err=%v", string(dump), err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("backup mode=%v err=%v", info.Mode(), err)
	}
}

func TestAutomaticBackupRecordsMissingOptionalConfig(t *testing.T) {
	cfg := testConfig(t, 19090)
	cfg.BackupApplicationConfigPath = filepath.Join(cfg.ComposeWorkDir, "missing", "config.yaml")
	cfg.BackupDockerConfigPath = filepath.Join(cfg.ComposeWorkDir, "missing", "docker-config.json")
	state := State{ActiveVersion: "0.1.171-ts.1", ActiveImage: managedTestImage('a'), UpdatedAt: time.Now().UTC()}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{cfg: cfg, runner: &fakeRunner{}, now: time.Now, state: state}
	path, err := manager.createAutomaticBackup(context.Background(), &Job{
		ID: "missing-optional-config-0001", FromVersion: state.ActiveVersion, FromImage: state.ActiveImage, TargetVersion: "0.1.172-ts.1",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest backupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	skipped := map[string]bool{}
	for _, record := range manifest.Skipped {
		skipped[record.Name] = record.Reason == "source file does not exist"
	}
	if !skipped["application-config"] || !skipped["deployer-docker-config"] {
		t.Fatalf("missing optional sources were not recorded: %+v", manifest.Skipped)
	}
}

type pgDumpFailureRunner struct {
	base *fakeRunner
}

func (r *pgDumpFailureRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	command := strings.Join(append([]string{name}, args...), " ")
	if strings.Contains(command, " pg_dump ") {
		r.base.mu.Lock()
		r.base.commands = append(r.base.commands, command)
		r.base.mu.Unlock()
		return "", errors.New("injected pg_dump failure")
	}
	return r.base.Run(ctx, env, name, args...)
}

func TestBackupFailureBlocksUpdateBeforeImagePull(t *testing.T) {
	cfg := testConfig(t, 19082)
	base := &fakeRunner{}
	manager, err := NewManager(cfg, &pgDumpFailureRunner{base: base})
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Start(DeployRequest{
		Action:                 "update",
		TargetVersion:          "0.1.172-ts.1",
		ExpectedCurrentVersion: "0.1.1-ts.1",
		RequestID:              "backup-failure-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForFinishedJob(t, manager, job.ID)
	if job.Status != JobStatusFailed || !strings.Contains(job.Error, "update was blocked") {
		t.Fatalf("job=%+v", job)
	}
	base.mu.Lock()
	commands := strings.Join(base.commands, "\n")
	base.mu.Unlock()
	if strings.Contains(commands, "docker pull ") || strings.Contains(commands, " compose ") && strings.Contains(commands, " run ") {
		t.Fatalf("update mutated images or containers after backup failure:\n%s", commands)
	}
	entries, readErr := os.ReadDir(filepath.Join(cfg.BackupRootPath, "automatic"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".pending-") {
			t.Fatalf("pending backup was not removed: %s", entry.Name())
		}
	}
}

func TestBackupSourceCopyFailureBlocksUpdateBeforeImagePull(t *testing.T) {
	cfg := testConfig(t, 19086)
	base := &fakeRunner{}
	manager, err := NewManager(cfg, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cfg.BackupDeployerBinaryPath); err != nil {
		t.Fatal(err)
	}
	job, err := manager.Start(DeployRequest{
		Action:                 "update",
		TargetVersion:          "0.1.172-ts.1",
		ExpectedCurrentVersion: "0.1.1-ts.1",
		RequestID:              "backup-copy-failure-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForFinishedJob(t, manager, job.ID)
	if job.Status != JobStatusFailed || !strings.Contains(job.Error, "update was blocked") || !strings.Contains(job.Error, "deployer-binary") {
		t.Fatalf("job=%+v", job)
	}
	base.mu.Lock()
	commands := strings.Join(base.commands, "\n")
	base.mu.Unlock()
	if strings.Contains(commands, "docker pull ") {
		t.Fatalf("image pull occurred after backup copy failure:\n%s", commands)
	}
}

type imagePullFailureRunner struct {
	base *fakeRunner
}

func (r *imagePullFailureRunner) Run(ctx context.Context, env map[string]string, name string, args ...string) (string, error) {
	command := strings.Join(append([]string{name}, args...), " ")
	if strings.Contains(command, "docker pull ") {
		r.base.mu.Lock()
		r.base.commands = append(r.base.commands, command)
		r.base.mu.Unlock()
		return "", errors.New("injected image pull failure")
	}
	return r.base.Run(ctx, env, name, args...)
}

func TestFailedDeploymentDoesNotPruneBackups(t *testing.T) {
	cfg := testConfig(t, 19087)
	automatic := filepath.Join(cfg.BackupRootPath, "automatic")
	if err := os.MkdirAll(automatic, 0700); err != nil {
		t.Fatal(err)
	}
	baseTime := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		writeCompletedBackupFixture(t, filepath.Join(automatic, fmt.Sprintf("existing-%d", index)), baseTime.Add(time.Duration(index)*time.Minute), "existing")
	}
	base := &fakeRunner{}
	manager, err := NewManager(cfg, &imagePullFailureRunner{base: base})
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Start(DeployRequest{
		Action:                 "update",
		TargetVersion:          "0.1.172-ts.1",
		ExpectedCurrentVersion: "0.1.1-ts.1",
		RequestID:              "pull-failure-no-cleanup-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForFinishedJob(t, manager, job.ID)
	if job.Status != JobStatusFailed || job.BackupPath == "" {
		t.Fatalf("job=%+v", job)
	}
	entries, err := os.ReadDir(automatic)
	if err != nil || len(entries) != 4 {
		t.Fatalf("failed deployment pruned backups: entries=%d err=%v", len(entries), err)
	}
}

func TestVerifyBackupChecksumsRejectsTampering(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeBackupChecksums(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyBackupChecksums(dir); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered backup passed verification: %v", err)
	}
}

func TestAutomaticBackupRetentionLeavesManualBackupsUntouched(t *testing.T) {
	cfg := testConfig(t, 19083)
	automatic := filepath.Join(cfg.BackupRootPath, "automatic")
	manual := filepath.Join(cfg.BackupRootPath, "manual")
	if err := os.MkdirAll(automatic, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(manual, 0700); err != nil {
		t.Fatal(err)
	}
	baseTime := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	for index := 0; index < 4; index++ {
		dir := filepath.Join(automatic, fmt.Sprintf("backup-%d", index))
		writeCompletedBackupFixture(t, dir, baseTime.Add(time.Duration(index)*time.Minute), strings.Repeat("x", index+1))
	}
	manualFile := filepath.Join(manual, "operator-backup.sql")
	if err := os.WriteFile(manualFile, []byte("manual"), 0600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{cfg: cfg}
	removed, reclaimed, err := manager.pruneAutomaticBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 || reclaimed <= 0 {
		t.Fatalf("removed=%v reclaimed=%d", removed, reclaimed)
	}
	entries, err := os.ReadDir(automatic)
	if err != nil || len(entries) != automaticBackupRetention {
		t.Fatalf("automatic entries=%d err=%v", len(entries), err)
	}
	if _, err := os.Stat(manualFile); err != nil {
		t.Fatalf("manual backup was touched: %v", err)
	}
}

func TestAutomaticBackupRetentionDoesNotDeleteUnrecognizedDirectory(t *testing.T) {
	cfg := testConfig(t, 19085)
	automatic := filepath.Join(cfg.BackupRootPath, "automatic")
	if err := os.MkdirAll(automatic, 0700); err != nil {
		t.Fatal(err)
	}
	baseTime := time.Date(2026, 8, 5, 8, 0, 0, 0, time.UTC)
	for index := 0; index < 3; index++ {
		writeCompletedBackupFixture(t, filepath.Join(automatic, fmt.Sprintf("valid-%d", index)), baseTime.Add(time.Duration(index)*time.Minute), "valid")
	}
	unknown := filepath.Join(automatic, "operator-note")
	if err := os.Mkdir(unknown, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unknown, "keep"), []byte("do not delete"), 0600); err != nil {
		t.Fatal(err)
	}

	manager := &Manager{cfg: cfg}
	removed, _, err := manager.pruneAutomaticBackups()
	if len(removed) != 1 || err == nil || !strings.Contains(err.Error(), "unrecognized") {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(unknown, "keep")); err != nil {
		t.Fatalf("unrecognized directory was touched: %v", err)
	}
}

func TestPostSuccessCleanupFailureReturnsWarningWithoutChangingResult(t *testing.T) {
	cfg := testConfig(t, 19088)
	automatic := filepath.Join(cfg.BackupRootPath, "automatic")
	if err := os.MkdirAll(filepath.Join(automatic, "unrecognized"), 0700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State{
		ActiveVersion: "0.1.172-ts.1",
		ActiveImage:   managedTestImage('b'),
		UpdatedAt:     now,
		Job: &Job{
			ID:         "cleanup-warning-0001",
			Action:     "update",
			Status:     JobStatusSucceeded,
			Stage:      StageCompleted,
			StartedAt:  now,
			UpdatedAt:  now,
			FinishedAt: &now,
		},
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{cfg: cfg, runner: &fakeRunner{}, now: time.Now, state: state}
	warning := manager.performPostSuccessMaintenance(state.Job.ID)
	job, err := manager.Job(state.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != JobStatusSucceeded || job.CleanupWarning != "" || !strings.Contains(warning, "retention cleanup failed") {
		t.Fatalf("cleanup changed deployment result incorrectly: %+v", job)
	}
}

func TestCreateAutomaticBackupRemovesPendingCrashResidue(t *testing.T) {
	cfg := testConfig(t, 19089)
	pending := filepath.Join(cfg.BackupRootPath, "automatic", ".pending-crash")
	if err := os.MkdirAll(pending, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pending, "partial"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	state := State{ActiveVersion: "0.1.171-ts.1", ActiveImage: managedTestImage('a'), UpdatedAt: time.Now().UTC()}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{cfg: cfg, runner: &fakeRunner{}, now: time.Now, state: state}
	if _, err := manager.createAutomaticBackup(context.Background(), &Job{ID: "pending-cleanup-0001", FromVersion: state.ActiveVersion, FromImage: state.ActiveImage, TargetVersion: "0.1.172-ts.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pending); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending crash residue still exists: %v", err)
	}
}

func writeCompletedBackupFixture(t *testing.T, dir string, createdAt time.Time, database string) {
	t.Helper()
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(dir, "database.dump")
	if err := os.WriteFile(databasePath, []byte("PGDMP"+database), 0600); err != nil {
		t.Fatal(err)
	}
	record, err := inspectBackupFile("postgresql-database", "database.dump", "compose:postgres", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := backupManifest{
		Schema:    backupManifestSchema,
		Kind:      "automatic-pre-update",
		CreatedAt: createdAt,
		Database: backupDatabaseRecord{
			Service: "postgres",
			Format:  "postgresql-custom",
			File:    "database.dump",
		},
		Files: []backupFileRecord{record},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeBackupChecksums(dir); err != nil {
		t.Fatal(err)
	}
}

func TestNextOlderReleaseHandlesForwardUpdateAndRollback(t *testing.T) {
	imageA := managedTestImage('a')
	imageB := managedTestImage('b')
	imageC := managedTestImage('c')
	tests := []struct {
		name                           string
		state                          State
		targetVersion, targetImage     string
		previousVersion, previousImage string
		wantVersion, wantImage         string
	}{
		{
			name:          "forward update keeps former previous as image-only generation",
			state:         State{PreviousVersion: "B", PreviousImage: imageB, OlderVersion: "C", OlderImage: imageC},
			targetVersion: "D", targetImage: managedTestImage('d'), previousVersion: "A", previousImage: imageA,
			wantVersion: "B", wantImage: imageB,
		},
		{
			name:          "rollback excludes the newly active former previous",
			state:         State{PreviousVersion: "B", PreviousImage: imageB, OlderVersion: "C", OlderImage: imageC},
			targetVersion: "B", targetImage: imageB, previousVersion: "A", previousImage: imageA,
			wantVersion: "C", wantImage: imageC,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version, image := nextOlderRelease(test.state, test.targetVersion, test.targetImage, test.previousVersion, test.previousImage)
			if version != test.wantVersion || image != test.wantImage {
				t.Fatalf("got=(%q,%q) want=(%q,%q)", version, image, test.wantVersion, test.wantImage)
			}
		})
	}
}

type imageRetentionRunner struct {
	mu       sync.Mutex
	images   map[string]dockerImageMetadata
	removed  []string
	listFail error
}

func (r *imageRetentionRunner) Run(_ context.Context, _ map[string]string, _ string, args ...string) (string, error) {
	command := strings.Join(args, " ")
	if strings.HasPrefix(command, "image ls ") {
		if r.listFail != nil {
			return "", r.listFail
		}
		ids := make([]string, 0, len(r.images))
		for id := range r.images {
			ids = append(ids, id)
		}
		return strings.Join(ids, "\n"), nil
	}
	if strings.HasPrefix(command, "image inspect ") {
		metadata, ok := r.images[args[len(args)-1]]
		if !ok {
			return "", errors.New("missing image")
		}
		data, _ := json.Marshal(metadata)
		return string(data), nil
	}
	if strings.HasPrefix(command, "image rm ") {
		r.mu.Lock()
		r.removed = append(r.removed, args[len(args)-1])
		r.mu.Unlock()
		return "removed", nil
	}
	return "", fmt.Errorf("unexpected command %s", command)
}

func TestManagedImageRetentionProtectsCurrentAndTwoOlderVersions(t *testing.T) {
	cfg := testConfig(t, 19084)
	images := map[string]dockerImageMetadata{}
	add := func(id string, digest rune, size int64, owned bool) {
		metadata := dockerImageMetadata{
			ID:          id,
			RepoTags:    []string{cfg.ImageRepository + ":version-" + id},
			RepoDigests: []string{managedTestImage(digest)},
			Size:        size,
		}
		metadata.Config.Labels = map[string]string{}
		if owned {
			for key, value := range cfg.RequiredImageLabels {
				metadata.Config.Labels[key] = value
			}
		}
		images[id] = metadata
	}
	add("current", 'a', 10, true)
	add("previous", 'b', 20, true)
	add("older", 'c', 30, true)
	add("stale", 'd', 40, true)
	add("foreign", 'e', 50, false)
	runner := &imageRetentionRunner{images: images}
	manager := &Manager{
		cfg:    cfg,
		runner: runner,
		state: State{
			ActiveImage:   managedTestImage('a'),
			PreviousImage: managedTestImage('b'),
			OlderImage:    managedTestImage('c'),
		},
	}
	removed, reclaimed, err := manager.pruneManagedImages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "stale" || reclaimed != 40 {
		t.Fatalf("removed=%v reclaimed=%d", removed, reclaimed)
	}
	if len(runner.removed) != 1 || runner.removed[0] != "stale" {
		t.Fatalf("docker removals=%v", runner.removed)
	}
}

func TestRollbackToOlderRetainedImageSkipsRegistryPull(t *testing.T) {
	activeListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	activePort := listenerTCPPort(t, activeListener)
	activeServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","deployment_runtime":{"state":"active","slot":"sub2api-green"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`))
	})}
	go func() { _ = activeServer.Serve(activeListener) }()
	t.Cleanup(func() { _ = activeServer.Close() })

	olderListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	olderPort := listenerTCPPort(t, olderListener)
	var runtimeState atomic.Value
	runtimeState.Store("standby")
	olderServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":"ok","deployment_runtime":{"state":%q,"slot":"blue"},"drain":{"supported":true,"active_requests":0,"hijacked_connections":0,"blockers":0}}`, atomicString(&runtimeState))
	})}
	go func() { _ = olderServer.Serve(olderListener) }()
	t.Cleanup(func() { _ = olderServer.Close() })

	cfg := testConfig(t, activePort)
	cfg.Slots[0].Port = olderPort
	if err := atomicWrite(cfg.NginxUpstreamPath, []byte(fmt.Sprintf("upstream sub2api_managed {\n server 127.0.0.1:%d;\n}\n", activePort)), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := State{
		ActiveSlot:        "sub2api-green",
		ActiveContainer:   "sub2api-green",
		ActivePort:        activePort,
		ActiveVersion:     "0.1.3-ts.1",
		ActiveImage:       managedTestImage('c'),
		PreviousSlot:      "blue",
		PreviousContainer: "blue",
		PreviousPort:      olderPort,
		PreviousVersion:   "0.1.2-ts.1",
		PreviousImage:     managedTestImage('b'),
		OlderVersion:      "0.1.1-ts.1",
		OlderImage:        managedTestImage('a'),
		UpdatedAt:         now,
	}
	if err := saveState(cfg.StatePath, state); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		candidate:    true,
		runtimeState: &runtimeState,
		versions: map[string]string{
			"blue":          "0.1.1-ts.1",
			"sub2api-green": "0.1.3-ts.1",
		},
	}
	manager, err := NewManager(cfg, runner)
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.Start(DeployRequest{Action: "rollback", TargetVersion: "0.1.1-ts.1", RequestID: "older-rollback-0001"})
	if err != nil {
		t.Fatal(err)
	}
	job = waitForFinishedJob(t, manager, job.ID)
	if job.Status != JobStatusSucceeded {
		t.Fatalf("status=%s error=%s rollback=%s", job.Status, job.Error, job.RollbackError)
	}
	if manager.state.ActiveVersion != "0.1.1-ts.1" || manager.state.PreviousVersion != "0.1.3-ts.1" || manager.state.OlderVersion != "0.1.2-ts.1" {
		t.Fatalf("retention state=%+v", manager.state)
	}
	runner.mu.Lock()
	commands := strings.Join(runner.commands, "\n")
	runner.mu.Unlock()
	if strings.Contains(commands, "docker pull ") || strings.Contains(commands, "pg_restore") {
		t.Fatalf("older rollback used registry or restored the database:\n%s", commands)
	}
}

func managedTestImage(digest rune) string {
	return "ghcr.io/ssharkkky/sub2api@sha256:" + strings.Repeat(string(digest), 64)
}
