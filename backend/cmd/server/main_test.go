package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGracefulShutdownTimeout(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "")
	require.Equal(t, 30*time.Minute, gracefulShutdownTimeout())

	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "120")
	require.Equal(t, 120*time.Second, gracefulShutdownTimeout())

	t.Setenv("SHUTDOWN_TIMEOUT_SECONDS", "invalid")
	require.Equal(t, 30*time.Minute, gracefulShutdownTimeout())

}
