package service

import (
	"os"
	"os/exec"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// runAllocationTestInSubprocess isolates process-wide AllocsPerRun counters
// from background workers started by other service tests. The child runs the
// original assertions; the parent returns after checking the child's result.
func runAllocationTestInSubprocess(t *testing.T) bool {
	t.Helper()
	const isolatedEnv = "SUB2API_TEST_ISOLATED_ALLOCATION"
	if os.Getenv(isolatedEnv) == t.Name() {
		return false
	}
	binary, err := os.Executable()
	require.NoError(t, err)
	command := exec.CommandContext(t.Context(), binary,
		"-test.run=^"+regexp.QuoteMeta(t.Name())+"$",
		"-test.count=1", "-test.timeout=30s", "-test.v")
	command.Env = append(os.Environ(), isolatedEnv+"="+t.Name())
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Contains(t, string(output), "--- PASS: "+t.Name()+" ", "allocation test must execute in the child")
	return true
}
