package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/service"
	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
)

type Server struct {
	mgr   *service.Manager
	token string
	mux   *http.ServeMux
}

func NewServer(mgr *service.Manager, token string) *Server {
	s := &Server{mgr: mgr, token: token, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.authMiddleware(s.mux)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Liveness stays open for systemd/k8s probes.
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if s.token != "" {
			auth := r.Header.Get("Authorization")
			const p = "Bearer "
			if !strings.HasPrefix(auth, p) || strings.TrimPrefix(auth, p) != s.token {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /metrics", s.handleMetrics)
	s.mux.HandleFunc("GET /v1/instances", s.handleList)
	s.mux.HandleFunc("POST /v1/instances", s.handleCreate)
	s.mux.HandleFunc("POST /v1/pools", s.handleCreatePool)
	s.mux.HandleFunc("GET /v1/pools/snapshot", s.handlePoolSnapshot)
	s.mux.HandleFunc("POST /v1/profiles/register", s.handleRegisterProfiles)
	s.mux.HandleFunc("GET /v1/instances/{id}", s.handleGet)
	s.mux.HandleFunc("POST /v1/instances/{id}/start", s.handleStart)
	s.mux.HandleFunc("POST /v1/instances/{id}/stop", s.handleStop)
	s.mux.HandleFunc("POST /v1/instances/{id}/restart", s.handleRestart)
	s.mux.HandleFunc("POST /v1/instances/{id}/rotate", s.handleRotate)
	s.mux.HandleFunc("POST /v1/instances/{id}/health", s.handleInstanceHealth)
	s.mux.HandleFunc("DELETE /v1/instances/{id}", s.handleDelete)
	s.mux.HandleFunc("GET /v1/alerts/duplicate-exit-ips", s.handleDupIPs)
	s.mux.HandleFunc("POST /v1/health/all", s.handleHealthAll)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "time": time.Now().UTC()})
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	list := s.mgr.List()
	var up, unhealthy int
	for _, i := range list {
		if i.Status == store.StatusRunning {
			up++
		}
		if i.Status == store.StatusUnhealthy {
			unhealthy++
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = fmt.Fprintf(w,
		"# HELP warp_instances_total Total registered instances\n"+
			"# TYPE warp_instances_total gauge\n"+
			"warp_instances_total %d\n"+
			"# HELP warp_instances_up Running instances\n"+
			"# TYPE warp_instances_up gauge\n"+
			"warp_instances_up %d\n"+
			"# HELP warp_instances_unhealthy Unhealthy instances\n"+
			"# TYPE warp_instances_unhealthy gauge\n"+
			"warp_instances_unhealthy %d\n",
		len(list), up, unhealthy,
	)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"instances": s.mgr.List()})
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req service.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	inst, err := s.mgr.Create(r.Context(), req)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 201, inst)
}

func (s *Server) handleCreatePool(w http.ResponseWriter, r *http.Request) {
	var req service.CreatePoolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	list, err := s.mgr.CreatePool(r.Context(), req)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": err.Error(), "created": list})
		return
	}
	writeJSON(w, 201, map[string]any{"instances": list, "count": len(list)})
}

// handleRegisterProfiles POST /v1/profiles/register { "count": 3 }
// Registers free WARP accounts; private keys are NOT returned (use POST /v1/pools with register=true).
func (s *Server) handleRegisterProfiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if body.Count <= 0 {
		body.Count = 1
	}
	profiles, err := s.mgr.RegisterProfiles(r.Context(), body.Count)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	// Redact secrets — registration is for readiness check / count verification.
	type safeProfile struct {
		HasPrivateKey bool     `json:"has_private_key"`
		Address       []string `json:"address"`
		PeerCount     int      `json:"peer_count"`
		LicenseKey    string   `json:"license_key,omitempty"`
	}
	out := make([]safeProfile, 0, len(profiles))
	for _, p := range profiles {
		lic := p.LicenseKey
		if lic != "" {
			lic = "***"
		}
		out = append(out, safeProfile{
			HasPrivateKey: p.PrivateKey != "",
			Address:       p.Address,
			PeerCount:     len(p.Peers),
			LicenseKey:    lic,
		})
	}
	writeJSON(w, 201, map[string]any{"count": len(out), "profiles": out})
}

func (s *Server) handlePoolSnapshot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.mgr.PoolSnapshot())
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	inst, err := s.mgr.Get(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, inst)
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mgr.Start(r.Context(), id); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	inst, _ := s.mgr.Get(id)
	writeJSON(w, 200, inst)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mgr.Stop(r.Context(), id); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	inst, _ := s.mgr.Get(id)
	writeJSON(w, 200, inst)
}

func (s *Server) handleRestart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mgr.Restart(r.Context(), id); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	inst, _ := s.mgr.Get(id)
	writeJSON(w, 200, inst)
}

func (s *Server) handleRotate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Profile *store.Profile `json:"profile"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	inst, err := s.mgr.Rotate(r.Context(), id, body.Profile)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, inst)
}

func (s *Server) handleInstanceHealth(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.mgr.HealthCheck(r.Context(), id); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	inst, _ := s.mgr.Get(id)
	writeJSON(w, 200, inst)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// ?deregister_cloudflare=false to skip CF API (local-only delete)
	opts := service.DeleteOptions{}
	if v := r.URL.Query().Get("deregister_cloudflare"); v != "" {
		b := v != "0" && !strings.EqualFold(v, "false") && !strings.EqualFold(v, "no")
		opts.DeregisterCloudflare = &b
	}
	// optional JSON body override
	var body struct {
		DeregisterCloudflare *bool `json:"deregister_cloudflare"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.DeregisterCloudflare != nil {
		opts.DeregisterCloudflare = body.DeregisterCloudflare
	}
	if err := s.mgr.DeleteWithOptions(r.Context(), id, opts); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"status":                "deleted",
		"deregister_cloudflare": opts.DeregisterCloudflare == nil || (opts.DeregisterCloudflare != nil && *opts.DeregisterCloudflare),
	})
}

func (s *Server) handleDupIPs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"duplicate_exit_ips": s.mgr.ExitIPDuplicates()})
}

func (s *Server) handleHealthAll(w http.ResponseWriter, r *http.Request) {
	unhealthy, err := s.mgr.HealthAll(r.Context())
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"unhealthy_ids":      unhealthy,
		"duplicate_exit_ips": s.mgr.ExitIPDuplicates(),
		"snapshot":           s.mgr.PoolSnapshot(),
	})
}
