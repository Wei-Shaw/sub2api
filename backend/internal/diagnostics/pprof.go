// Package diagnostics provides opt-in runtime diagnostics on a separate listener.
package diagnostics

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof" // #nosec G108 -- Explicit opt-in diagnostic server; never mounted on the business router.
	"os"
	"strconv"
	"time"
)

// StartFromEnv starts a separate listener, never the business HTTP router.
// The caller must close the returned server on shutdown when it is non-nil.
func StartFromEnv() (*http.Server, error) {
	return start(os.Getenv)
}

func start(getenv func(string) string) (*http.Server, error) {
	value := getenv("SUB2API_PPROF_ENABLED")
	if value == "" {
		return nil, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("SUB2API_PPROF_ENABLED must be a boolean")
	}
	if !enabled {
		return nil, nil
	}
	port := 6060
	if value := getenv("SUB2API_PPROF_PORT"); value != "" {
		port, err = strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("SUB2API_PPROF_PORT must be between 1 and 65535")
		}
	}
	return listen(port)
}

func listen(port int) (*http.Server, error) {
	// Remote access is intentional when enabled; deployment controls exposure.
	listener, err := net.Listen("tcp4", net.JoinHostPort("0.0.0.0", strconv.Itoa(port))) // #nosec G102 -- Remote diagnostics are intentional and disabled by default.
	if err != nil {
		return nil, fmt.Errorf("start pprof listener: %w", err)
	}
	server := &http.Server{
		Addr:              listener.Addr().String(),
		Handler:           handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("pprof listener stopped: %v", err)
		}
	}()
	return server, nil
}

func handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)
	return mux
}
