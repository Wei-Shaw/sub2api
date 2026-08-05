package deployer

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	automaticBackupRetention = 2
	backupManifestSchema     = 1
)

type backupManifest struct {
	Schema        int                   `json:"schema"`
	Kind          string                `json:"kind"`
	CreatedAt     time.Time             `json:"created_at"`
	JobID         string                `json:"job_id"`
	FromVersion   string                `json:"from_version"`
	FromImage     string                `json:"from_image"`
	TargetVersion string                `json:"target_version"`
	Database      backupDatabaseRecord  `json:"database"`
	Deployer      BuildInfo             `json:"deployer"`
	Files         []backupFileRecord    `json:"files"`
	Skipped       []backupSkippedRecord `json:"skipped,omitempty"`
}

type backupDatabaseRecord struct {
	Service string `json:"service"`
	Format  string `json:"format"`
	File    string `json:"file"`
}

type backupFileRecord struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Source string `json:"source"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type backupSkippedRecord struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Reason string `json:"reason"`
}

type backupSource struct {
	name     string
	path     string
	optional bool
}

func (m *Manager) createAutomaticBackup(ctx context.Context, job *Job) (string, error) {
	backupCtx, cancel := context.WithTimeout(ctx, m.cfg.BackupTimeout.Duration)
	defer cancel()

	automaticDir := filepath.Join(m.cfg.BackupRootPath, "automatic")
	manualDir := filepath.Join(m.cfg.BackupRootPath, "manual")
	for _, dir := range []string{m.cfg.BackupRootPath, automaticDir, manualDir} {
		if err := ensurePrivateDirectory(dir); err != nil {
			return "", fmt.Errorf("prepare backup directory %s: %w", dir, err)
		}
	}
	if err := removePendingBackups(automaticDir); err != nil {
		return "", fmt.Errorf("remove stale pending backups: %w", err)
	}

	tempDir, err := os.MkdirTemp(automaticDir, ".pending-")
	if err != nil {
		return "", fmt.Errorf("create pending backup: %w", err)
	}
	if err := os.Chmod(tempDir, 0700); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", fmt.Errorf("protect pending backup: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(tempDir)
		}
	}()

	createdAt := m.now().UTC()
	manifest := backupManifest{
		Schema:        backupManifestSchema,
		Kind:          "automatic-pre-update",
		CreatedAt:     createdAt,
		JobID:         job.ID,
		FromVersion:   job.FromVersion,
		FromImage:     job.FromImage,
		TargetVersion: job.TargetVersion,
		Database: backupDatabaseRecord{
			Service: m.cfg.BackupDatabaseService,
			Format:  "postgresql-custom",
			File:    "database.dump",
		},
		Deployer: m.buildInfo,
	}

	databasePath := filepath.Join(tempDir, manifest.Database.File)
	if err := m.dumpPostgreSQL(backupCtx, databasePath); err != nil {
		return "", fmt.Errorf("dump PostgreSQL database: %w", err)
	}
	databaseRecord, err := inspectBackupFile("postgresql-database", manifest.Database.File, "compose:"+m.cfg.BackupDatabaseService, databasePath)
	if err != nil {
		return "", err
	}
	manifest.Files = append(manifest.Files, databaseRecord)

	for _, source := range m.backupSources() {
		destinationName := source.name
		destinationPath := filepath.Join(tempDir, destinationName)
		if err := copyBackupFile(source.path, destinationPath); err != nil {
			if source.optional && errors.Is(err, os.ErrNotExist) {
				manifest.Skipped = append(manifest.Skipped, backupSkippedRecord{
					Name: source.name, Source: source.path, Reason: "source file does not exist",
				})
				continue
			}
			return "", fmt.Errorf("copy %s from %s: %w", source.name, source.path, err)
		}
		record, err := inspectBackupFile(source.name, destinationName, source.path, destinationPath)
		if err != nil {
			return "", err
		}
		manifest.Files = append(manifest.Files, record)
	}

	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode backup manifest: %w", err)
	}
	if err := atomicWrite(filepath.Join(tempDir, "manifest.json"), append(manifestData, '\n'), 0600); err != nil {
		return "", fmt.Errorf("write backup manifest: %w", err)
	}
	if err := writeBackupChecksums(tempDir); err != nil {
		return "", err
	}
	if err := verifyBackupChecksums(tempDir); err != nil {
		return "", fmt.Errorf("verify completed backup: %w", err)
	}
	if err := syncDirectory(tempDir); err != nil {
		return "", fmt.Errorf("sync completed backup: %w", err)
	}

	finalName := createdAt.Format("20060102T150405.000000000Z") + "-" + shortBackupID(job.ID)
	finalPath := filepath.Join(automaticDir, finalName)
	if err := os.Rename(tempDir, finalPath); err != nil {
		return "", fmt.Errorf("commit completed backup: %w", err)
	}
	committed = true
	if err := syncDirectory(automaticDir); err != nil {
		return "", fmt.Errorf("sync automatic backup directory: %w", err)
	}
	log.Printf("sub2api-deployer job_id=%q backup_created=%q files=%d", job.ID, finalPath, len(manifest.Files))
	return finalPath, nil
}

func (m *Manager) dumpPostgreSQL(ctx context.Context, destination string) error {
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	closeWithError := func(runErr error) error {
		if syncErr := file.Sync(); runErr == nil && syncErr != nil {
			runErr = syncErr
		}
		if closeErr := file.Close(); runErr == nil && closeErr != nil {
			runErr = closeErr
		}
		return runErr
	}

	args := m.composeArgs()
	args = append(args,
		"exec", "-T", m.cfg.BackupDatabaseService,
		"sh", "-eu", "-c",
		`exec pg_dump --format=custom --no-owner --no-privileges --username="$POSTGRES_USER" --dbname="$POSTGRES_DB"`,
	)
	env := map[string]string{
		m.cfg.ImageEnvironment:        m.currentImage(),
		"SUB2API_DEPLOYER_SOCKET_GID": strconv.Itoa(m.cfg.SocketGID),
	}
	if streaming, ok := m.runner.(StreamingCommandRunner); ok {
		if err := closeWithError(streaming.RunTo(ctx, env, file, m.cfg.DockerBinary, args...)); err != nil {
			return err
		}
	} else {
		output, err := m.runner.Run(ctx, env, m.cfg.DockerBinary, args...)
		if err == nil {
			_, err = io.WriteString(file, output)
		}
		if err := closeWithError(err); err != nil {
			return err
		}
	}

	opened, err := os.Open(destination)
	if err != nil {
		return err
	}
	defer func() { _ = opened.Close() }()
	magic := make([]byte, 5)
	if _, err := io.ReadFull(opened, magic); err != nil {
		return fmt.Errorf("database dump is incomplete: %w", err)
	}
	if string(magic) != "PGDMP" {
		return errors.New("database dump does not have the PostgreSQL custom-format signature")
	}
	return nil
}

func (m *Manager) composeArgs() []string {
	args := []string{"compose", "--project-name", m.cfg.ComposeProject, "--project-directory", m.cfg.ComposeWorkDir}
	for _, file := range m.cfg.ComposeEnvFiles {
		args = append(args, "--env-file", file)
	}
	for _, file := range m.cfg.ComposeFiles {
		args = append(args, "-f", file)
	}
	return args
}

func (m *Manager) currentImage() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.ActiveImage
}

func (m *Manager) backupSources() []backupSource {
	sources := make([]backupSource, 0, len(m.cfg.ComposeEnvFiles)+len(m.cfg.ComposeFiles)+7)
	appendSource := func(name, path string) {
		if strings.TrimSpace(path) != "" {
			sources = append(sources, backupSource{name: name, path: filepath.Clean(path)})
		}
	}
	appendOptionalSource := func(name, path string) {
		if strings.TrimSpace(path) != "" {
			sources = append(sources, backupSource{name: name, path: filepath.Clean(path), optional: true})
		}
	}
	for index, path := range m.cfg.ComposeEnvFiles {
		appendSource(fmt.Sprintf("compose-env-%02d", index+1), path)
	}
	for index, path := range m.cfg.ComposeFiles {
		appendSource(fmt.Sprintf("compose-file-%02d", index+1), path)
	}
	appendOptionalSource("application-config", m.cfg.BackupApplicationConfigPath)
	appendSource("deployer-config", m.cfg.LoadedFrom)
	appendOptionalSource("deployer-docker-config", m.cfg.BackupDockerConfigPath)
	appendSource("deployer-binary", m.cfg.BackupDeployerBinaryPath)
	appendSource("deployer-state", m.cfg.StatePath)
	appendSource("nginx-site", m.cfg.NginxSitePath)
	appendSource("nginx-managed-upstream", m.cfg.NginxUpstreamPath)

	seen := make(map[string]struct{}, len(sources))
	unique := sources[:0]
	for _, source := range sources {
		if _, exists := seen[source.path]; exists {
			continue
		}
		seen[source.path] = struct{}{}
		unique = append(unique, source)
	}
	return unique
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("path is not a real directory")
	}
	return os.Chmod(path, 0700)
}

func removePendingBackups(automaticDir string) error {
	entries, err := os.ReadDir(automaticDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".pending-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(automaticDir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyBackupFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("source is not a regular file")
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return err
	}
	return output.Close()
}

func inspectBackupFile(name, filename, source, path string) (backupFileRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return backupFileRecord{}, err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return backupFileRecord{}, copyErr
	}
	if closeErr != nil {
		return backupFileRecord{}, closeErr
	}
	return backupFileRecord{
		Name:   name,
		File:   filename,
		Source: source,
		SHA256: hex.EncodeToString(hash.Sum(nil)),
		Size:   size,
	}, nil
}

func writeBackupChecksums(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "checksums.sha256" {
			continue
		}
		record, err := inspectBackupFile(entry.Name(), entry.Name(), "", filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		lines = append(lines, record.SHA256+"  "+entry.Name())
	}
	sort.Strings(lines)
	return atomicWrite(filepath.Join(dir, "checksums.sha256"), []byte(strings.Join(lines, "\n")+"\n"), 0600)
}

func verifyBackupChecksums(dir string) error {
	file, err := os.Open(filepath.Join(dir, "checksums.sha256"))
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	count := 0
	manifestCovered := false
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "  ", 2)
		if len(parts) != 2 || len(parts[0]) != sha256.Size*2 || filepath.Base(parts[1]) != parts[1] {
			return fmt.Errorf("invalid checksum line %q", scanner.Text())
		}
		record, err := inspectBackupFile(parts[1], parts[1], "", filepath.Join(dir, parts[1]))
		if err != nil {
			return err
		}
		if record.SHA256 != parts[0] {
			return fmt.Errorf("checksum mismatch for %s", parts[1])
		}
		if parts[1] == "manifest.json" {
			manifestCovered = true
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if count == 0 {
		return errors.New("backup checksum file is empty")
	}
	if !manifestCovered {
		return errors.New("backup checksums do not cover manifest.json")
	}
	return nil
}

func shortBackupID(jobID string) string {
	digest := sha256.Sum256([]byte(jobID))
	return hex.EncodeToString(digest[:6])
}

func (m *Manager) pruneAutomaticBackups() ([]string, int64, error) {
	automaticDir := filepath.Join(m.cfg.BackupRootPath, "automatic")
	entries, err := os.ReadDir(automaticDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	type candidate struct {
		name      string
		createdAt time.Time
		size      int64
	}
	var candidates []candidate
	var failures []error
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".pending-") {
			continue
		}
		path := filepath.Join(automaticDir, entry.Name())
		manifest, err := inspectCompletedAutomaticBackup(path)
		if err != nil {
			failures = append(failures, fmt.Errorf("leave unrecognized automatic backup %s untouched: %w", path, err))
			continue
		}
		size, err := directorySize(path)
		if err != nil {
			failures = append(failures, fmt.Errorf("measure automatic backup %s: %w", path, err))
			continue
		}
		candidates = append(candidates, candidate{name: entry.Name(), createdAt: manifest.CreatedAt, size: size})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].createdAt.Equal(candidates[j].createdAt) {
			return candidates[i].name > candidates[j].name
		}
		return candidates[i].createdAt.After(candidates[j].createdAt)
	})
	if len(candidates) <= automaticBackupRetention {
		return nil, 0, errors.Join(failures...)
	}
	var removed []string
	var reclaimed int64
	for _, candidate := range candidates[automaticBackupRetention:] {
		path := filepath.Join(automaticDir, candidate.name)
		if err := os.RemoveAll(path); err != nil {
			failures = append(failures, err)
			continue
		}
		removed = append(removed, path)
		reclaimed += candidate.size
	}
	if len(removed) > 0 {
		if err := syncDirectory(automaticDir); err != nil {
			failures = append(failures, err)
		}
	}
	return removed, reclaimed, errors.Join(failures...)
}

func inspectCompletedAutomaticBackup(path string) (backupManifest, error) {
	if err := verifyBackupChecksums(path); err != nil {
		return backupManifest{}, fmt.Errorf("verify checksums: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "manifest.json"))
	if err != nil {
		return backupManifest{}, err
	}
	var manifest backupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return backupManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.Schema != backupManifestSchema || manifest.Kind != "automatic-pre-update" || manifest.CreatedAt.IsZero() {
		return backupManifest{}, errors.New("manifest identity is invalid")
	}
	if manifest.Database.Format != "postgresql-custom" || filepath.Base(manifest.Database.File) != manifest.Database.File {
		return backupManifest{}, errors.New("manifest database record is invalid")
	}

	databaseFound := false
	seen := make(map[string]struct{}, len(manifest.Files))
	for _, expected := range manifest.Files {
		if expected.File == "" || filepath.Base(expected.File) != expected.File {
			return backupManifest{}, fmt.Errorf("manifest contains unsafe file %q", expected.File)
		}
		if _, exists := seen[expected.File]; exists {
			return backupManifest{}, fmt.Errorf("manifest contains duplicate file %q", expected.File)
		}
		seen[expected.File] = struct{}{}
		filePath := filepath.Join(path, expected.File)
		info, err := os.Lstat(filePath)
		if err != nil || !info.Mode().IsRegular() {
			if err == nil {
				err = errors.New("backup entry is not a regular file")
			}
			return backupManifest{}, fmt.Errorf("inspect %s: %w", expected.File, err)
		}
		actual, err := inspectBackupFile(expected.Name, expected.File, expected.Source, filePath)
		if err != nil {
			return backupManifest{}, err
		}
		if actual.SHA256 != expected.SHA256 || actual.Size != expected.Size {
			return backupManifest{}, fmt.Errorf("manifest metadata mismatch for %s", expected.File)
		}
		if expected.File == manifest.Database.File {
			databaseFound = true
		}
	}
	if !databaseFound {
		return backupManifest{}, errors.New("manifest does not include the database dump")
	}
	return manifest, nil
}

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}
