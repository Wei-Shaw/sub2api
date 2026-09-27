//go:build unit

package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShutdownServer_DrainsRequestsBeforeStoppingOpsErrorLogWorkers(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	started := make(chan struct{})
	release := make(chan struct{})
	events := make(chan string, 2)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = io.WriteString(w, "ok")
		events <- "request"
	})}
	go func() { _ = srv.Serve(ln) }()

	status := make(chan int, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			status <- 0
			return
		}
		_ = resp.Body.Close()
		status <- resp.StatusCode
	}()
	<-started

	done := make(chan struct{})
	go func() {
		shutdownServer(srv, 5*time.Second, func() bool {
			events <- "workers"
			return true
		})
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("shutdownServer returned while a request was still in flight")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdownServer did not return after the request finished")
	}
	require.Equal(t, "request", <-events)
	require.Equal(t, "workers", <-events)
	require.Equal(t, http.StatusOK, <-status)
}

func TestGracefulShutdownFitsComposeStopGracePeriod(t *testing.T) {
	compose, err := os.ReadFile("../../../deploy/docker-compose.yml")
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^    stop_grace_period: (\d+)s$`).FindSubmatch(compose)
	require.NotNil(t, m, "sub2api service must set stop_grace_period")
	secs, err := strconv.Atoi(string(m[1]))
	require.NoError(t, err)

	// After the HTTP drain come the ops error log drain (up to 10s) and
	// app.Cleanup (nominally ~10s, not hard-capped); all of it should finish
	// before Docker SIGKILLs.
	require.LessOrEqual(t, gracefulShutdownTimeout+20*time.Second, time.Duration(secs)*time.Second)
}

// docker-compose.yml forwards these keys, so a value in .env.example would
// override config.yaml and the backend default for anyone who copies it.
func TestEnvExampleLeavesBehaviorChangingOverridesEmpty(t *testing.T) {
	sample, err := os.ReadFile("../../../deploy/.env.example")
	require.NoError(t, err)
	for _, key := range []string{"JWT_ACCESS_TOKEN_EXPIRE_MINUTES", "LOG_FORMAT", "SERVER_H2C_ENABLED", "REDIS_MAXCLIENTS"} {
		lines := regexp.MustCompile(`(?m)^`+key+`=[^\r\n]*`).FindAllString(string(sample), -1)
		require.Equal(t, []string{key + "="}, lines, key)
	}
}
