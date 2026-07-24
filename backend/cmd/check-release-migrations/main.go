package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const migrationPathspec = ":(top,glob)backend/migrations/*.sql"

var objectIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)

type migrationChange struct {
	status string
	paths  []string
}

type migrationStatement struct {
	path      string
	statement sqlStatement
}

type gitRepository struct {
	root string
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: check-release-migrations <schema-baseline-ref> [target-ref]")
		os.Exit(2)
	}
	target := "HEAD"
	if len(os.Args) == 3 {
		target = os.Args[2]
	}
	if err := runCheck(os.Args[1], target); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func runCheck(baseRef, targetRef string) error {
	repo, err := discoverRepository()
	if err != nil {
		return err
	}
	base, err := repo.resolveCommit("schema baseline", baseRef)
	if err != nil {
		return err
	}
	target, err := repo.resolveCommit("target", targetRef)
	if err != nil {
		return err
	}
	if err := repo.requireAncestor(base, target); err != nil {
		return err
	}

	changes, err := repo.migrationChanges(base, target)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Printf("No database migration changes between %s and %s\n", baseRef, targetRef)
		return nil
	}

	var violations []string
	var addedPaths []string
	lastReleasedMigration, err := repo.lastMigrationPathAt(base)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if change.status != "A" {
			violations = append(violations, fmt.Sprintf("released migration was modified, removed, copied, or renamed: %s %s", change.status, strings.Join(change.paths, " -> ")))
			continue
		}
		if lastReleasedMigration != "" && change.paths[0] <= lastReleasedMigration {
			violations = append(violations, fmt.Sprintf(
				"new migration does not append after the last released migration: %s <= %s",
				change.paths[0], lastReleasedMigration,
			))
			continue
		}
		addedPaths = append(addedPaths, change.paths[0])
	}
	sort.Strings(addedPaths)

	baselineTables, err := repo.tablesAt(base)
	if err != nil {
		return fmt.Errorf("inspect schema baseline: %w", err)
	}
	statements, parseViolations := repo.readTargetStatements(target, addedPaths)
	violations = append(violations, parseViolations...)
	releasedStatements, err := repo.statementFingerprintsAt(base)
	if err != nil {
		return fmt.Errorf("fingerprint released migrations: %w", err)
	}
	for _, migration := range statements {
		if releasedPath, exists := releasedStatements[statementFingerprint(migration.statement)]; exists {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: statement replays released migration content from %s",
				migration.path,
				migration.statement.startLine,
				releasedPath,
			))
		}
	}
	newTables := make(map[relationName]struct{})

	for _, migration := range statements {
		decision := evaluateStatement(migration.statement, newTables, migration.path)
		if !decision.allowed {
			violations = append(violations, fmt.Sprintf(
				"%s:%d: %s: %s",
				migration.path,
				migration.statement.startLine,
				decision.description,
				decision.reason,
			))
			continue
		}
		// A conditional CREATE can be a no-op against an out-of-band table, so
		// only an unconditional earlier CREATE proves that later blocking index
		// work targets a table introduced by this release.
		recordNewTableProof(decision, baselineTables, newTables)
	}

	if len(violations) > 0 {
		for _, violation := range violations {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", violation)
		}
		return errors.New("migration set is not eligible for automatic image rollback; use an expand/contract release or the documented reviewed-compatible process")
	}

	fmt.Printf("Migration changes from %s to %s are rollback-compatible for managed deployment\n", baseRef, targetRef)
	return nil
}

func (repo *gitRepository) statementFingerprintsAt(commit string) (map[string]string, error) {
	output, err := repo.git("ls-tree", "-r", "-z", "--name-only", commit, "--", ":(top)backend/migrations")
	if err != nil {
		return nil, fmt.Errorf("list released migrations: %w", err)
	}
	fingerprints := make(map[string]string)
	for _, path := range splitNUL(output) {
		if !strings.HasPrefix(path, "backend/migrations/") || !strings.HasSuffix(path, ".sql") {
			continue
		}
		src, err := repo.readBlob(commit, path)
		if err != nil {
			return nil, err
		}
		statements, err := scanSQLStatements(src)
		if err != nil {
			return nil, fmt.Errorf("parse %s at baseline: %w", path, err)
		}
		for _, statement := range statements {
			fingerprint := statementFingerprint(statement)
			if _, exists := fingerprints[fingerprint]; !exists {
				fingerprints[fingerprint] = path
			}
		}
	}
	return fingerprints, nil
}

func statementFingerprint(statement sqlStatement) string {
	hash := sha256.New()
	var length [8]byte
	for _, token := range statement.tokens {
		value := token.text
		switch token.kind {
		case tokenWord:
			value = strings.ToLower(value)
		case tokenString:
			value = token.raw
		}
		_, _ = hash.Write([]byte{byte(token.kind)})
		binary.LittleEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write([]byte(value))
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func (repo *gitRepository) lastMigrationPathAt(commit string) (string, error) {
	output, err := repo.git("ls-tree", "-r", "-z", "--name-only", commit, "--", ":(top)backend/migrations")
	if err != nil {
		return "", fmt.Errorf("list released migrations: %w", err)
	}
	last := ""
	for _, path := range splitNUL(output) {
		if !strings.HasPrefix(path, "backend/migrations/") || !strings.HasSuffix(path, ".sql") {
			continue
		}
		if path > last {
			last = path
		}
	}
	return last, nil
}

func discoverRepository() (*gitRepository, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("locate Git repository: %w: %s", err, strings.TrimSpace(string(output)))
	}
	root := strings.TrimSpace(string(output))
	if root == "" || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("locate Git repository: git returned invalid root %q", root)
	}
	return &gitRepository{root: root}, nil
}

func (repo *gitRepository) resolveCommit(label, ref string) (string, error) {
	if ref == "" || strings.IndexByte(ref, 0) >= 0 {
		return "", fmt.Errorf("resolve %s: empty or invalid ref", label)
	}
	output, err := repo.git("rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve %s ref %q: %w", label, ref, err)
	}
	commit := strings.TrimSpace(string(output))
	if !objectIDPattern.MatchString(commit) {
		return "", fmt.Errorf("resolve %s ref %q: git returned invalid object ID %q", label, ref, commit)
	}
	return strings.ToLower(commit), nil
}

func (repo *gitRepository) requireAncestor(base, target string) error {
	cmd := exec.Command("git", "-C", repo.root, "merge-base", "--is-ancestor", base, target)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return fmt.Errorf("schema baseline %s is not an ancestor of target %s", base, target)
	}
	return fmt.Errorf("verify schema-baseline ancestry: %w: %s", err, strings.TrimSpace(string(output)))
}

func (repo *gitRepository) migrationChanges(base, target string) ([]migrationChange, error) {
	output, err := repo.git(
		"diff", "--name-status", "-z",
		"--find-renames", "--find-copies=100%", "--find-copies-harder",
		base, target, "--", migrationPathspec,
	)
	if err != nil {
		return nil, fmt.Errorf("list migration changes: %w", err)
	}
	fields := splitNUL(output)
	changes := make([]migrationChange, 0, len(fields)/2)
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		pathCount := 1
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			pathCount = 2
		}
		if status == "" || i+pathCount > len(fields) {
			return nil, fmt.Errorf("list migration changes: malformed git --name-status output")
		}
		paths := append([]string(nil), fields[i:i+pathCount]...)
		i += pathCount
		changes = append(changes, migrationChange{status: status, paths: paths})
	}
	return changes, nil
}

func (repo *gitRepository) tablesAt(commit string) (map[relationName]struct{}, error) {
	output, err := repo.git("ls-tree", "-r", "-z", "--name-only", commit, "--", ":(top)backend/migrations")
	if err != nil {
		return nil, fmt.Errorf("list baseline migrations: %w", err)
	}
	tables := make(map[relationName]struct{})
	for _, path := range splitNUL(output) {
		if !strings.HasPrefix(path, "backend/migrations/") || !strings.HasSuffix(path, ".sql") {
			continue
		}
		src, err := repo.readBlob(commit, path)
		if err != nil {
			return nil, err
		}
		statements, err := scanSQLStatements(src)
		if err != nil {
			return nil, fmt.Errorf("parse %s at baseline: %w", path, err)
		}
		for _, statement := range statements {
			decision := evaluateCreateOnly(statement.tokens)
			if decision.createdTable != "" {
				tables[decision.createdTable] = struct{}{}
			}
		}
	}
	return tables, nil
}

func (repo *gitRepository) readTargetStatements(commit string, paths []string) ([]migrationStatement, []string) {
	var statements []migrationStatement
	var violations []string
	for _, path := range paths {
		src, err := repo.readBlob(commit, path)
		if err != nil {
			violations = append(violations, err.Error())
			continue
		}
		parsed, err := scanSQLStatements(src)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		for _, statement := range parsed {
			statements = append(statements, migrationStatement{path: path, statement: statement})
		}
	}
	return statements, violations
}

func (repo *gitRepository) readBlob(commit, path string) ([]byte, error) {
	output, err := repo.git("cat-file", "blob", commit+":"+path)
	if err != nil {
		return nil, fmt.Errorf("read target blob %s: %w", path, err)
	}
	return output, nil
}

func (repo *gitRepository) git(args ...string) ([]byte, error) {
	cmdArgs := append([]string{"-C", repo.root}, args...)
	cmd := exec.Command("git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

func splitNUL(output []byte) []string {
	if len(output) == 0 {
		return nil
	}
	parts := bytes.Split(output, []byte{0})
	if len(parts) > 0 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	result := make([]string, len(parts))
	for i := range parts {
		result[i] = string(parts[i])
	}
	return result
}
