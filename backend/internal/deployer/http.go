package deployer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type HTTPServer struct {
	cfg      Config
	manager  *Manager
	server   *http.Server
	listener net.Listener
}

func NewHTTPServer(cfg Config, manager *Manager) *HTTPServer {
	mux := http.NewServeMux()
	s := &HTTPServer{cfg: cfg, manager: manager}
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/jobs/current", s.handleCurrentJob)
	mux.HandleFunc("GET /v1/jobs/{id}", s.handleJob)
	mux.HandleFunc("POST /v1/deployments", s.handleDeployment)
	s.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	return s
}

func (s *HTTPServer) ListenAndServe() error {
	if err := os.MkdirAll(filepath.Dir(s.cfg.SocketPath), 0750); err != nil {
		return err
	}
	if info, err := os.Lstat(s.cfg.SocketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("refusing to replace non-socket path %s", s.cfg.SocketPath)
		}
		connection, dialErr := net.DialTimeout("unix", s.cfg.SocketPath, 250*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return fmt.Errorf("deployer daemon is already accepting connections on %s", s.cfg.SocketPath)
		}
		if !isConnectionRefused(dialErr) && !errors.Is(dialErr, os.ErrNotExist) {
			return fmt.Errorf("cannot prove existing deployer socket is stale: %w", dialErr)
		}
		if err := os.Remove(s.cfg.SocketPath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	listener, err := net.Listen("unix", s.cfg.SocketPath)
	if err != nil {
		return err
	}
	s.listener = listener
	if err := os.Chmod(s.cfg.SocketPath, os.FileMode(s.cfg.SocketMode)); err != nil {
		_ = listener.Close()
		return err
	}
	if err := os.Chown(s.cfg.SocketPath, 0, s.cfg.SocketGID); err != nil {
		_ = listener.Close()
		return err
	}
	err = s.server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	err := s.server.Shutdown(ctx)
	_ = os.Remove(s.cfg.SocketPath)
	return err
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	health := s.manager.Health()
	status := http.StatusOK
	if health.Degraded {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, health)
}

func (s *HTTPServer) handleCurrentJob(w http.ResponseWriter, _ *http.Request) {
	job, err := s.manager.Job("")
	if err != nil {
		writeProblem(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *HTTPServer) handleJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.manager.Job(strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeProblem(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *HTTPServer) handleDeployment(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req DeployRequest
	if err := decoder.Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, http.StatusBadRequest, "invalid request: trailing JSON data")
		return
	}
	job, err := s.manager.Start(req)
	if err != nil {
		switch {
		case errors.Is(err, ErrDeployerDegraded):
			writeProblem(w, http.StatusServiceUnavailable, err.Error())
		case errors.Is(err, ErrJobRunning), errors.Is(err, ErrRequestConflict), errors.Is(err, ErrVersionConflict):
			writeProblem(w, http.StatusConflict, err.Error())
		default:
			writeProblem(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
