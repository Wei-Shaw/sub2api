//go:build unit

package repository

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const fakePsqlExitCodeEnv = "SUB2API_FAKE_PSQL_EXIT_CODE"

// TestFakePsqlProcess is not a real test: the restore tests re-exec the test
// binary as a stand-in for psql so they run without sh or PostgreSQL.
func TestFakePsqlProcess(t *testing.T) {
	code := os.Getenv(fakePsqlExitCodeEnv)
	if code == "" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	if code != "0" {
		fmt.Fprint(os.Stderr, `psql:<stdin>:42: ERROR:  cannot drop table users because other objects depend on it`)
	}
	var exitCode int
	_, _ = fmt.Sscan(code, &exitCode)
	os.Exit(exitCode)
}

func fakePsql(t *testing.T, exitCode int, gotArgs *[]string) func(context.Context, string, ...string) *exec.Cmd {
	t.Helper()
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		require.Equal(t, "psql", name)
		*gotArgs = args
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFakePsqlProcess$")
		cmd.Env = append(os.Environ(), fmt.Sprintf("%s=%d", fakePsqlExitCodeEnv, exitCode))
		return cmd
	}
}

func TestPgDumperRestoreStopsOnFirstSQLError(t *testing.T) {
	var args []string
	dumper, _ := newTestPgDumper(t, fakePsql(t, 0, &args))

	require.NoError(t, dumper.Restore(context.Background(), strings.NewReader("SELECT 1;")))
	joined := strings.Join(args, " ")
	require.Contains(t, joined, "-v ON_ERROR_STOP=1",
		"without ON_ERROR_STOP psql exits 0 after the single transaction was rolled back")
	require.Contains(t, args, "--single-transaction")
}

func TestPgDumperRestoreReportsPsqlFailure(t *testing.T) {
	var args []string
	dumper, _ := newTestPgDumper(t, fakePsql(t, 3, &args))

	err := dumper.Restore(context.Background(), strings.NewReader("DROP TABLE users;"))
	require.ErrorContains(t, err, "exit status 3")
	require.ErrorContains(t, err, "cannot drop table users")
}
