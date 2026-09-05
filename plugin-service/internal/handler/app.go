package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/config"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/host/httpx"
)

type AppDeps struct {
	Config config.Config
}

type App struct {
	cfg config.Config
}

func NewApp(deps AppDeps) *App {
	return &App{cfg: deps.Config}
}

func (a *App) WithCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		csp := "frame-ancestors 'self'"
		if origin := httpx.ResolveFrameAncestorOrigin(r); origin != "" && origin != "null" {
			csp += " " + origin
		}
		w.Header().Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

func (a *App) Health(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}
