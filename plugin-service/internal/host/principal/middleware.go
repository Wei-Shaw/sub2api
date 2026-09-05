package principal

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/host/httpx"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
)

var (
	// ResolveMainSiteBaseCandidates is overrideable in tests so the router can point
	// auth verification at an httptest server without adding user-facing config.
	ResolveMainSiteBaseCandidates = defaultResolveMainSiteBaseCandidates
	MainSiteHTTPClient            = http.DefaultClient
)

type Middleware struct{}

func NewMiddleware() *Middleware {
	return &Middleware{}
}

func (m *Middleware) Require(next func(http.ResponseWriter, *http.Request, model.CurrentPrincipal)) http.HandlerFunc {
	return m.requireForPlugin("", next)
}

func (m *Middleware) RequirePlugin(pluginKey string, next func(http.ResponseWriter, *http.Request, model.CurrentPrincipal)) http.HandlerFunc {
	return m.requireForPlugin(strings.TrimSpace(pluginKey), next)
}

func (m *Middleware) requireForPlugin(pluginKey string, next func(http.ResponseWriter, *http.Request, model.CurrentPrincipal)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, err := LoadCurrentPrincipal(r, pluginKey)
		if err != nil {
			httpx.WriteError(w, http.StatusUnauthorized, err.Error())
			return
		}
		next(w, r, principal)
	}
}

func LoadCurrentPrincipal(r *http.Request, pluginKey string) (model.CurrentPrincipal, error) {
	if principal, ok := loadForwardedPrincipal(r, pluginKey); ok {
		return principal, nil
	}

	profile, err := loadMainSiteProfile(r)
	if err != nil {
		return model.CurrentPrincipal{}, err
	}

	role := model.RoleUser
	if strings.EqualFold(strings.TrimSpace(profile.Role), model.RoleAdmin) {
		role = model.RoleAdmin
	}

	return model.CurrentPrincipal{
		UserID:   profile.ID,
		Role:     role,
		Email:    strings.TrimSpace(profile.Email),
		Username: strings.TrimSpace(profile.Username),
		Plugin:   strings.TrimSpace(pluginKey),
	}, nil
}

func loadForwardedPrincipal(r *http.Request, pluginKey string) (model.CurrentPrincipal, bool) {
	if r == nil {
		return model.CurrentPrincipal{}, false
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get("X-Sub2api-User-Id")), 10, 64)
	if err != nil || userID <= 0 {
		return model.CurrentPrincipal{}, false
	}
	role := model.RoleUser
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Sub2api-User-Role")), model.RoleAdmin) {
		role = model.RoleAdmin
	}
	return model.CurrentPrincipal{
		UserID:   userID,
		Role:     role,
		Email:    strings.TrimSpace(r.Header.Get("X-Sub2api-User-Email")),
		Username: strings.TrimSpace(r.Header.Get("X-Sub2api-User-Name")),
		Plugin:   strings.TrimSpace(pluginKey),
	}, true
}

type mainSiteProfileEnvelope struct {
	Code int                 `json:"code"`
	Data mainSiteProfileData `json:"data"`
}

type mainSiteProfileData struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func loadMainSiteProfile(r *http.Request) (*mainSiteProfileData, error) {
	var lastErr error
	for _, baseURL := range ResolveMainSiteBaseCandidates(r) {
		profile, err := loadMainSiteProfileFromBaseURL(r, strings.TrimRight(strings.TrimSpace(baseURL), "/"))
		if err == nil {
			return profile, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errUnauthorized("failed to resolve main site")
}

func loadMainSiteProfileFromBaseURL(r *http.Request, baseURL string) (*mainSiteProfileData, error) {
	if baseURL == "" {
		return nil, errUnauthorized("failed to resolve main site")
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, baseURL+"/api/v1/auth/me", nil)
	if err != nil {
		return nil, errUnauthorized("failed to build main site request")
	}
	copyMainSiteCredentials(req, r)

	resp, err := MainSiteHTTPClient.Do(req)
	if err != nil {
		return nil, errUnauthorized("failed to load current user from main site")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errUnauthorized("failed to read current user from main site")
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errUnauthorized("main site authentication required")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errUnauthorized("failed to load current user from main site")
	}

	var envelope mainSiteProfileEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, errUnauthorized("failed to parse current user from main site")
	}
	if envelope.Code != 0 || envelope.Data.ID <= 0 {
		return nil, errUnauthorized("main site authentication required")
	}
	return &envelope.Data, nil
}

func copyMainSiteCredentials(dst *http.Request, src *http.Request) {
	if dst == nil || src == nil {
		return
	}
	if token := firstNonEmptyQuery(src, "token", "session"); token != "" {
		dst.Header.Set("Authorization", "Bearer "+token)
	}
	if authHeader := strings.TrimSpace(src.Header.Get("Authorization")); authHeader != "" && dst.Header.Get("Authorization") == "" {
		dst.Header.Set("Authorization", authHeader)
	}
	if rawCookie := strings.TrimSpace(src.Header.Get("Cookie")); rawCookie != "" {
		dst.Header.Set("Cookie", rawCookie)
	}
	if forwardedFor := strings.TrimSpace(src.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		dst.Header.Set("X-Forwarded-For", forwardedFor)
	}
	if userAgent := strings.TrimSpace(src.Header.Get("User-Agent")); userAgent != "" {
		dst.Header.Set("User-Agent", userAgent)
	}
	if forwardedCookie := strings.TrimSpace(src.URL.Query().Get("cookie")); forwardedCookie != "" && dst.Header.Get("Cookie") == "" {
		dst.Header.Set("Cookie", forwardedCookie)
	}
	if acceptLanguage := strings.TrimSpace(src.Header.Get("Accept-Language")); acceptLanguage != "" {
		dst.Header.Set("Accept-Language", acceptLanguage)
	}
}

// CopyMainSiteCredentials forwards the current user's main-site credentials.
func CopyMainSiteCredentials(dst *http.Request, src *http.Request) {
	copyMainSiteCredentials(dst, src)
}

func firstNonEmptyQuery(r *http.Request, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(r.URL.Query().Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func defaultResolveMainSiteBaseCandidates(_ *http.Request) []string {
	return []string{
		"http://sub2api:8080",
		"http://localhost:8080",
		"http://127.0.0.1:8080",
	}
}

type unauthorizedError struct {
	message string
}

func (e unauthorizedError) Error() string {
	return e.message
}

func errUnauthorized(message string) error {
	return unauthorizedError{message: message}
}
