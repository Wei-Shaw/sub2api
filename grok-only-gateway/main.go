package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

type App struct {
	store      *Store
	httpClient *HTTPClientFactory
	config     Config
	mux        *http.ServeMux
	sessions   *OAuthSessionStore
}

type Config struct {
	Addr             string
	DataPath         string
	AdminToken       string
	RequireAdminAuth bool
	DefaultAPIKey    string
	MaxBodyBytes     int64
	RequestTimeout   time.Duration
}

func main() {
	cfg := loadConfig()

	store, err := OpenStore(cfg.DataPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	if cfg.DefaultAPIKey != "" && !store.HasAPIKeyValue(cfg.DefaultAPIKey) {
		_, _ = store.CreateAPIKey("default", cfg.DefaultAPIKey)
	}

	app := &App{
		store:      store,
		httpClient: &HTTPClientFactory{timeout: cfg.RequestTimeout},
		config:     cfg,
		mux:        http.NewServeMux(),
		sessions:   NewOAuthSessionStore(),
	}
	app.registerRoutes()

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           requestLogger(app.mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Fatalf("listen %s: %v", cfg.Addr, err)
	}

	log.Printf("Grok-only gateway listening on http://%s", listener.Addr().String())
	if cfg.RequireAdminAuth {
		log.Printf("Admin UI token is required. Set it in the UI as X-Admin-Token.")
	} else {
		log.Printf("Admin UI is open because GROK_ADMIN_TOKEN is empty. Bind address defaults to 127.0.0.1.")
	}
	log.Printf("State file: %s", cfg.DataPath)

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func loadConfig() Config {
	addr := firstNonEmpty(os.Getenv("GROK_ADDR"), "127.0.0.1:8088")
	dataPath := firstNonEmpty(os.Getenv("GROK_DATA_PATH"), filepath.Join("data", "state.json"))
	adminToken := strings.TrimSpace(os.Getenv("GROK_ADMIN_TOKEN"))
	defaultAPIKey := strings.TrimSpace(os.Getenv("GROK_API_KEY"))
	maxBodyBytes := envInt64("GROK_MAX_BODY_BYTES", 64<<20)
	timeoutSeconds := envInt("GROK_REQUEST_TIMEOUT_SECONDS", 180)
	if timeoutSeconds <= 0 {
		timeoutSeconds = 180
	}
	return Config{
		Addr:             addr,
		DataPath:         dataPath,
		AdminToken:       adminToken,
		RequireAdminAuth: adminToken != "",
		DefaultAPIKey:    defaultAPIKey,
		MaxBodyBytes:     maxBodyBytes,
		RequestTimeout:   time.Duration(timeoutSeconds) * time.Second,
	}
}

func (a *App) registerRoutes() {
	staticRoot, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(staticRoot))
	a.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") ||
			strings.HasPrefix(r.URL.Path, "/v1/") ||
			strings.HasPrefix(r.URL.Path, "/responses") ||
			strings.HasPrefix(r.URL.Path, "/chat/") ||
			strings.HasPrefix(r.URL.Path, "/images/") ||
			strings.HasPrefix(r.URL.Path, "/videos") ||
			strings.HasPrefix(r.URL.Path, "/backend-api/") {
			writeJSONError(w, http.StatusNotFound, "not_found_error", "route not found")
			return
		}
		if r.URL.Path == "/" {
			data, err := staticFiles.ReadFile("static/index.html")
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(data)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	a.mux.HandleFunc("GET /api/admin/status", a.admin(a.handleAdminStatus))
	a.mux.HandleFunc("POST /api/admin/api-keys", a.admin(a.handleCreateAPIKey))
	a.mux.HandleFunc("DELETE /api/admin/api-keys/{id}", a.admin(a.handleDeleteAPIKey))
	a.mux.HandleFunc("POST /api/admin/accounts", a.admin(a.handleCreateAccount))
	a.mux.HandleFunc("PATCH /api/admin/accounts/{id}", a.admin(a.handleUpdateAccount))
	a.mux.HandleFunc("DELETE /api/admin/accounts/{id}", a.admin(a.handleDeleteAccount))
	a.mux.HandleFunc("POST /api/admin/accounts/{id}/refresh", a.admin(a.handleRefreshAccount))
	a.mux.HandleFunc("POST /api/admin/accounts/{id}/probe", a.admin(a.handleProbeAccount))
	a.mux.HandleFunc("POST /api/admin/grok/auth-url", a.admin(a.handleGrokAuthURL))
	a.mux.HandleFunc("POST /api/admin/grok/exchange-code", a.admin(a.handleGrokExchangeCode))
	a.mux.HandleFunc("POST /api/admin/grok/refresh-token", a.admin(a.handleGrokRefreshToken))
	a.mux.HandleFunc("GET /api/admin/runtime", a.admin(a.handleRuntime))

	a.mux.HandleFunc("GET /v1/models", a.user(a.handleModels))
	a.mux.HandleFunc("POST /v1/responses", a.user(a.handleResponses))
	a.mux.HandleFunc("POST /responses", a.user(a.handleResponses))
	a.mux.HandleFunc("POST /backend-api/codex/responses", a.user(a.handleResponses))
	a.mux.HandleFunc("POST /v1/chat/completions", a.user(a.handleChatCompletions))
	a.mux.HandleFunc("POST /chat/completions", a.user(a.handleChatCompletions))
	a.mux.HandleFunc("POST /v1/messages", a.user(a.handleMessages))
	a.mux.HandleFunc("POST /v1/images/generations", a.user(a.handleImagesGenerations))
	a.mux.HandleFunc("POST /images/generations", a.user(a.handleImagesGenerations))
	a.mux.HandleFunc("POST /v1/images/edits", a.user(a.handleImagesEdits))
	a.mux.HandleFunc("POST /images/edits", a.user(a.handleImagesEdits))
	a.mux.HandleFunc("POST /v1/videos/generations", a.user(a.handleVideoGeneration))
	a.mux.HandleFunc("POST /videos/generations", a.user(a.handleVideoGeneration))
	a.mux.HandleFunc("POST /v1/videos", a.user(a.handleVideoGeneration))
	a.mux.HandleFunc("POST /videos", a.user(a.handleVideoGeneration))
	a.mux.HandleFunc("GET /v1/videos", a.user(a.handleVideoStatus))
	a.mux.HandleFunc("GET /videos", a.user(a.handleVideoStatus))
	a.mux.HandleFunc("GET /v1/videos/{request_id}", a.user(a.handleVideoStatus))
	a.mux.HandleFunc("GET /videos/{request_id}", a.user(a.handleVideoStatus))
}

func (a *App) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.config.RequireAdminAuth {
			token := bearerToken(r)
			if token == "" {
				token = strings.TrimSpace(r.Header.Get("X-Admin-Token"))
			}
			if token != a.config.AdminToken {
				writeJSONError(w, http.StatusUnauthorized, "authentication_error", "invalid admin token")
				return
			}
		}
		next(w, r)
	}
}

func (a *App) user(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := bearerToken(r)
		if key == "" {
			key = strings.TrimSpace(r.Header.Get("X-API-Key"))
		}
		apiKey, ok := a.store.GetAPIKeyByValue(key)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "authentication_error", "invalid api key")
			return
		}
		ctx := context.WithValue(r.Context(), contextKeyAPIKey{}, apiKey)
		next(w, r.WithContext(ctx))
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

type contextKeyAPIKey struct{}

func currentAPIKey(r *http.Request) *APIKey {
	v, _ := r.Context().Value(contextKeyAPIKey{}).(*APIKey)
	return v
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, typ, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"type":    typ,
			"message": message,
		},
	})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	return dec.Decode(dst)
}

func bearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	parts := strings.Fields(auth)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil {
		return fallback
	}
	return value
}

func envInt64(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var value int64
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil {
		return fallback
	}
	return value
}
