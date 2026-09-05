package backend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	hostprincipal "github.com/Wei-Shaw/sub2api/plugin-service/internal/host/principal"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
)

const (
	defaultRoutePageSize = 20
	maxRoutePageSize     = 100
)

type routeListPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type RelayHandler struct {
	routes    RouteRepository
	platforms *PlatformRegistry
	clients   RelayClientProvider
}

func RegisterRoutes(mux *http.ServeMux, auth *hostprincipal.Middleware, handler *RelayHandler) {
	if mux == nil || handler == nil {
		return
	}
	mux.HandleFunc("/plugins/ai-relay/", handler.ProxyPlatformPath)
	if auth == nil {
		return
	}
	mux.HandleFunc("GET /plugins/ai-relay/api/routes", auth.RequirePlugin("ai-relay", handler.ListRoutes))
	mux.HandleFunc("GET /plugins/ai-relay/api/platforms", auth.RequirePlugin("ai-relay", handler.ListPlatforms))
	mux.HandleFunc("GET /plugins/ai-relay/api/runtime", auth.RequirePlugin("ai-relay", handler.Runtime))
	mux.HandleFunc("POST /plugins/ai-relay/api/routes", auth.RequirePlugin("ai-relay", handler.CreateRoute))
	mux.HandleFunc("PUT /plugins/ai-relay/api/routes/{platform}/{slug}", auth.RequirePlugin("ai-relay", handler.UpsertRoute))
	mux.HandleFunc("DELETE /plugins/ai-relay/api/routes/{platform}/{slug}", auth.RequirePlugin("ai-relay", handler.DeleteRoute))
	mux.HandleFunc("DELETE /plugins/ai-relay/api/routes", auth.RequirePlugin("ai-relay", handler.DeleteRoutes))
}

func (h *RelayHandler) ListPlatforms(w http.ResponseWriter, _ *http.Request, principal model.CurrentPrincipal) {
	if !isAdmin(principal) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access is required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": h.platforms.Platforms()})
}

func (h *RelayHandler) Runtime(w http.ResponseWriter, _ *http.Request, principal model.CurrentPrincipal) {
	if !isAdmin(principal) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access is required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"base_url": resolvePublicBaseURL()})
}

func NewRelayHandler(routes RouteRepository, platforms *PlatformRegistry, client *http.Client) *RelayHandler {
	if client == nil {
		client = http.DefaultClient
	}
	return NewRelayHandlerWithClientProvider(routes, platforms, NewProxyClientProvider(client, unavailableProxyResolver{}))
}

func NewRelayHandlerWithClientProvider(routes RouteRepository, platforms *PlatformRegistry, clients RelayClientProvider) *RelayHandler {
	if platforms == nil {
		platforms = NewDefaultPlatformRegistry()
	}
	if clients == nil {
		clients = NewProxyClientProvider(http.DefaultClient, unavailableProxyResolver{})
	}
	return &RelayHandler{routes: routes, platforms: platforms, clients: clients}
}

func (h *RelayHandler) Relay(w http.ResponseWriter, r *http.Request) {
	if r == nil || r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "only POST is supported")
		return
	}
	platform, slug, ok := relayRouteParts(r.URL.Path)
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "not_found_error", "relay route not found")
		return
	}
	r.SetPathValue("platform", platform)
	r.SetPathValue("slug", slug)
	h.dispatch(w, r, "images/generations")
}

type routeConfigInput struct {
	Platform     string            `json:"platform"`
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	BaseURL      string            `json:"base_url"`
	PathMappings map[string]string `json:"path_mappings"`
}

func (h *RelayHandler) CreateRoute(w http.ResponseWriter, r *http.Request, principal model.CurrentPrincipal) {
	if !isAdmin(principal) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access is required"})
		return
	}
	input, err := decodeRouteConfigInput(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route configuration"})
		return
	}
	if _, ok := h.platforms.Get(input.Platform); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported relay platform"})
		return
	}
	config, err := h.routes.Upsert(r.Context(), routeConfigFromInput(input.Platform, input.Slug, input))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, config)
}

func (h *RelayHandler) ListRoutes(w http.ResponseWriter, r *http.Request, principal model.CurrentPrincipal) {
	if !isAdmin(principal) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access is required"})
		return
	}
	query := RouteQuery{Platform: r.URL.Query().Get("platform"), Search: r.URL.Query().Get("search")}
	routes, err := h.routes.List(r.Context(), query)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list relay routes"})
		return
	}
	page, pageSize := routeListPage(r)
	total := len(routes)
	totalPages := (total + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}
	start := (page - 1) * pageSize
	if start >= total {
		routes = []RouteConfig{}
	} else {
		end := start + pageSize
		if end > total {
			end = total
		}
		routes = routes[start:end]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": routes,
		"pagination": routeListPagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

func routeListPage(r *http.Request) (int, int) {
	page := parsePositiveQueryInt(r, "page", 1, 1_000_000)
	pageSize := parsePositiveQueryInt(r, "page_size", defaultRoutePageSize, maxRoutePageSize)
	return page, pageSize
}

func parsePositiveQueryInt(r *http.Request, key string, fallback, maximum int) int {
	value, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get(key)))
	if err != nil || value < 1 {
		return fallback
	}
	if value > maximum {
		return maximum
	}
	return value
}

func (h *RelayHandler) UpsertRoute(w http.ResponseWriter, r *http.Request, principal model.CurrentPrincipal) {
	if !isAdmin(principal) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access is required"})
		return
	}
	input, err := decodeRouteConfigInput(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route configuration"})
		return
	}
	if _, ok := h.platforms.Get(r.PathValue("platform")); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported relay platform"})
		return
	}
	config, err := h.routes.Upsert(r.Context(), routeConfigFromInput(r.PathValue("platform"), r.PathValue("slug"), input))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, config)
}

func decodeRouteConfigInput(body io.Reader) (routeConfigInput, error) {
	var input routeConfigInput
	decoder := json.NewDecoder(io.LimitReader(body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return routeConfigInput{}, err
	}
	return input, nil
}

func routeConfigFromInput(platform, slug string, input routeConfigInput) RouteConfig {
	return RouteConfig{
		Platform: platform, Slug: slug, Name: input.Name, BaseURL: input.BaseURL,
		PathMappings: input.PathMappings,
	}
}

func (h *RelayHandler) DeleteRoute(w http.ResponseWriter, r *http.Request, principal model.CurrentPrincipal) {
	if !isAdmin(principal) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access is required"})
		return
	}
	if err := h.routes.Delete(r.Context(), r.PathValue("platform"), r.PathValue("slug")); err != nil {
		status := http.StatusInternalServerError
		if err == ErrRouteNotFound {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *RelayHandler) DeleteRoutes(w http.ResponseWriter, r *http.Request, principal model.CurrentPrincipal) {
	if !isAdmin(principal) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access is required"})
		return
	}
	var input struct {
		Items []RouteReference `json:"items"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route configuration"})
		return
	}
	if len(input.Items) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "at least one relay route is required"})
		return
	}
	if err := h.routes.DeleteMany(r.Context(), input.Items); err != nil {
		status := http.StatusInternalServerError
		if err == ErrInvalidRouteConfig || err == ErrRouteNotFound {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": len(input.Items)})
}

func isAdmin(principal model.CurrentPrincipal) bool {
	return strings.EqualFold(strings.TrimSpace(principal.Role), model.RoleAdmin)
}

func relayRouteParts(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 && len(parts) != 7 || len(parts) >= 2 && (parts[0] != "plugins" || parts[1] != "ai-relay") {
		return "", "", false
	}
	if len(parts) == 7 && (parts[4] != "v1" || parts[5] != "images" || parts[6] != "generations") {
		return "", "", false
	}
	return parts[2], parts[3], true
}

func relayPlatform(r *http.Request) string {
	if platform := strings.TrimSpace(r.PathValue("platform")); platform != "" {
		return platform
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "plugins" && parts[1] == "ai-relay" {
		return parts[2]
	}
	return ""
}

func (h *RelayHandler) ProxyOpenAIPath(w http.ResponseWriter, r *http.Request) {
	h.ProxyPlatformPath(w, r)
}

func (h *RelayHandler) ProxyPlatformPath(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) == 4 && parts[0] == "plugins" && parts[1] == "ai-relay" {
		h.Relay(w, r)
		return
	}
	if len(parts) < 6 || parts[0] != "plugins" || parts[1] != "ai-relay" || parts[4] != "v1" {
		writeOpenAIError(w, http.StatusNotFound, "not_found_error", "relay endpoint not found")
		return
	}
	endpoint := strings.Join(parts[5:], "/")
	r.SetPathValue("platform", parts[2])
	r.SetPathValue("slug", parts[3])
	h.dispatch(w, r, endpoint)
}

func (h *RelayHandler) dispatch(w http.ResponseWriter, r *http.Request, endpoint string) {
	if r == nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "request is required")
		return
	}
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(authorization), "bearer ") || strings.TrimSpace(authorization[7:]) == "" {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", "Bearer authorization is required")
		return
	}
	if h == nil || h.routes == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "server_error", "relay routes are unavailable")
		return
	}
	config, found, err := h.routes.Get(r.Context(), relayPlatform(r), r.PathValue("slug"))
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "server_error", "failed to load relay route")
		return
	}
	if !found {
		writeOpenAIError(w, http.StatusNotFound, "not_found_error", "relay route not found")
		return
	}
	platform, found := h.platforms.Get(config.Platform)
	if !found {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "unsupported relay platform")
		return
	}
	client, err := h.clientForRequest(r)
	if err != nil {
		writeProxyClientError(w, err)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
		return
	}
	response, err := platform.Handle(r.Context(), PlatformRequest{Route: config, Endpoint: endpoint, Method: r.Method, Query: r.URL.RawQuery, Headers: r.Header.Clone(), Body: body, Client: client})
	if err != nil {
		status := response.StatusCode
		if status == 0 {
			status = http.StatusBadGateway
		}
		writeOpenAIError(w, status, "upstream_error", err.Error())
		return
	}
	copyEndToEndHeaders(w.Header(), response.Headers)
	if response.Stream != nil {
		w.Header().Del("Content-Length")
		if response.StatusCode == 0 {
			response.StatusCode = http.StatusOK
		}
		w.WriteHeader(response.StatusCode)
		if err := response.Stream(w); err != nil {
			log.Printf("ai-relay stream terminated: %v", err)
		}
		return
	}
	if response.StatusCode == 0 {
		response.StatusCode = http.StatusOK
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(response.Body)
}

var hopByHopHeaders = map[string]struct{}{
	"Connection": {}, "Proxy-Connection": {}, "Keep-Alive": {}, "Transfer-Encoding": {},
	"TE": {}, "Trailer": {}, "Upgrade": {},
}

func copyEndToEndHeaders(destination, source http.Header) {
	for key, values := range source {
		canonicalKey := http.CanonicalHeaderKey(key)
		if _, excluded := hopByHopHeaders[canonicalKey]; excluded || strings.EqualFold(canonicalKey, ProxyIDHeader) {
			continue
		}
		destination.Del(canonicalKey)
		for _, value := range values {
			destination.Add(canonicalKey, value)
		}
	}
}

func (h *RelayHandler) clientForRequest(r *http.Request) (*http.Client, error) {
	if h == nil || h.clients == nil {
		return nil, ErrProxyStorageUnavailable
	}
	proxyID := ""
	ctx := context.Background()
	if r != nil {
		proxyID = r.Header.Get(ProxyIDHeader)
		ctx = r.Context()
	}
	return h.clients.ClientFor(ctx, proxyID)
}

func writeProxyClientError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrInvalidProxyID) {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid account proxy context")
		return
	}
	writeOpenAIError(w, http.StatusBadGateway, "upstream_error", "account proxy is unavailable")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOpenAIError(w http.ResponseWriter, status int, kind, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{
		"message": message,
		"type":    kind,
		"code":    status,
	}})
}
