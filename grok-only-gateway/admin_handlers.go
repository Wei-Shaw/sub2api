package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts":          a.store.ListAccounts(),
		"api_keys":          a.store.ListAPIKeys(false),
		"logs":              a.store.Logs(),
		"models":            DefaultModels(),
		"admin_auth":        a.config.RequireAdminAuth,
		"max_body_bytes":    a.config.MaxBodyBytes,
		"request_timeout_s": int(a.config.RequestTimeout.Seconds()),
	})
}

func (a *App) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	key, err := a.store.CreateAPIKey(req.Name, req.Key)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, key)
}

func (a *App) handleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r, "id")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "invalid api key id")
		return
	}
	if err := a.store.DeleteAPIKey(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req AccountInput
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	account, err := a.store.CreateAccount(req)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, account.Public())
}

func (a *App) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r, "id")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "invalid account id")
		return
	}
	var patch AccountPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	account, err := a.store.UpdateAccount(id, patch)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not_found_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, account.Public())
}

func (a *App) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r, "id")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "invalid account id")
		return
	}
	if err := a.store.DeleteAccount(id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleRefreshAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r, "id")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "invalid account id")
		return
	}
	account, ok := a.store.GetAccount(id)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "not_found_error", "account not found")
		return
	}
	info, err := a.refreshOAuthToken(r.Context(), account.RefreshToken, account.ProxyURL, account.ClientID)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	updated, err := a.store.UpdateAccountToken(id, info)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated.Public())
}

func (a *App) handleProbeAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r, "id")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "invalid account id")
		return
	}
	account, ok := a.store.GetAccount(id)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "not_found_error", "account not found")
		return
	}
	token, account, err := a.accessTokenForAccount(r.Context(), account)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	body := []byte(`{"model":"grok-4.3","input":"ping","max_output_tokens":1,"store":false}`)
	resp, respBody, err := a.doUpstream(r.Context(), account, http.MethodPost, endpointURL(account.BaseURL, "/responses"), body, "application/json", "application/json", token)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	quota := quotaFromHeaders(resp.Header, resp.StatusCode, "active_probe")
	a.store.UpdateQuota(account.ID, quota)
	if resp.StatusCode >= 400 {
		a.markAccountFromStatus(account.ID, resp.StatusCode, resp.Header, sanitizeLogBody(respBody))
		writeJSONError(w, http.StatusBadGateway, "upstream_error", sanitizeLogBody(respBody))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"status_code": resp.StatusCode,
		"quota":       quota,
	})
}

func (a *App) handleGrokAuthURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RedirectURI string `json:"redirect_uri"`
		ProxyURL    string `json:"proxy_url"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	result, err := a.generateAuthURL(req.RedirectURI, req.ProxyURL)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) handleGrokExchangeCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID     string `json:"session_id"`
		Code          string `json:"code"`
		State         string `json:"state"`
		RedirectURI   string `json:"redirect_uri"`
		ProxyURL      string `json:"proxy_url"`
		Name          string `json:"name"`
		CreateAccount bool   `json:"create_account"`
		Concurrency   int    `json:"concurrency"`
		Priority      int    `json:"priority"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	info, err := a.exchangeOAuthCode(r.Context(), req.SessionID, req.Code, req.State, req.RedirectURI, req.ProxyURL)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	resp := map[string]any{"token_info": info}
	if req.CreateAccount {
		account, err := a.store.CreateAccount(accountInputFromTokenInfo(info, req.Name, req.ProxyURL, req.Concurrency, req.Priority))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
			return
		}
		resp["account"] = account.Public()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleGrokRefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken  string `json:"refresh_token"`
		RT            string `json:"rt"`
		ClientID      string `json:"client_id"`
		ProxyURL      string `json:"proxy_url"`
		Name          string `json:"name"`
		CreateAccount bool   `json:"create_account"`
		Concurrency   int    `json:"concurrency"`
		Priority      int    `json:"priority"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	refreshToken := firstNonEmpty(req.RefreshToken, req.RT)
	info, err := a.refreshOAuthToken(r.Context(), refreshToken, req.ProxyURL, req.ClientID)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	resp := map[string]any{"token_info": info}
	if req.CreateAccount {
		account, err := a.store.CreateAccount(accountInputFromTokenInfo(info, req.Name, req.ProxyURL, req.Concurrency, req.Priority))
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "api_error", err.Error())
			return
		}
		resp["account"] = account.Public()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) handleRuntime(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"base_url":            normalizeBaseURL(""),
		"oauth_authorize_url": effectiveAuthorizeURL(),
		"oauth_token_url":     effectiveTokenURL(),
		"oauth_redirect_uri":  effectiveRedirectURI(""),
		"oauth_client_id":     effectiveClientID(""),
		"oauth_scope":         effectiveScope(),
		"data_path":           a.config.DataPath,
		"admin_auth_required": a.config.RequireAdminAuth,
		"supported_endpoints": []string{"/v1/responses", "/v1/chat/completions", "/v1/messages", "/v1/images/generations", "/v1/images/edits", "/v1/videos/generations", "/v1/videos/{request_id}"},
		"generated_at":        time.Now().UTC().Format(time.RFC3339),
	})
}

func accountInputFromTokenInfo(info *TokenInfo, name, proxyURL string, concurrency, priority int) AccountInput {
	var expiresAt *time.Time
	if info != nil && info.ExpiresAt > 0 {
		t := time.Unix(info.ExpiresAt, 0).UTC()
		expiresAt = &t
	}
	if name == "" && info != nil {
		name = info.Email
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	input := AccountInput{
		Name:         name,
		AccessToken:  info.AccessToken,
		RefreshToken: info.RefreshToken,
		IDToken:      info.IDToken,
		TokenType:    info.TokenType,
		ExpiresAt:    expiresAt,
		ClientID:     info.ClientID,
		Scope:        info.Scope,
		Email:        info.Email,
		BaseURL:      DefaultBaseURL,
		ProxyURL:     strings.TrimSpace(proxyURL),
		Concurrency:  concurrency,
		Priority:     priority,
	}
	return input
}

func parsePathID(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(r.PathValue(name)), 10, 64)
}
