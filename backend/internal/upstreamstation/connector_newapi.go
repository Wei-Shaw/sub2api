package upstreamstation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

type NewAPIConnector struct {
	http *http.Client
}

func NewNewAPIConnector(client *http.Client) *NewAPIConnector {
	return &NewAPIConnector{http: connectorHTTPClient(client)}
}

func (c *NewAPIConnector) Type() string { return SiteTypeNewAPI }

func (c *NewAPIConnector) Detect(ctx context.Context, baseURL string) (bool, error) {
	_, err := c.get(ctx, normalizeBaseURL(baseURL)+"/api/status", nil)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func (c *NewAPIConnector) Authenticate(ctx context.Context, station *Station, credentials Credentials) (*Session, error) {
	if strings.TrimSpace(credentials.Cookie) != "" || strings.TrimSpace(credentials.AccessToken) != "" {
		if strings.TrimSpace(credentials.UserID) == "" {
			return nil, errors.New("newapi user_id is required for token credentials")
		}
		return &Session{Cookie: credentials.Cookie, AccessToken: credentials.AccessToken, UserID: credentials.UserID}, nil
	}
	if station == nil {
		return nil, errors.New("upstream station is required")
	}
	body, err := json.Marshal(map[string]string{"username": credentials.Username, "password": credentials.Password})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeBaseURL(station.BaseURL)+"/api/user/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("newapi login returned HTTP %d", resp.StatusCode)
	}
	data, err := decodeNewAPIResponse(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	var result struct {
		ID         int64 `json:"id"`
		Require2FA bool  `json:"require_2fa"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result.Require2FA {
		return nil, errors.New("newapi account requires 2FA")
	}
	if result.ID <= 0 {
		return nil, errors.New("newapi login returned an invalid user id")
	}
	parts := make([]string, 0, len(resp.Cookies()))
	for _, cookie := range resp.Cookies() {
		parts = append(parts, cookie.Name+"="+cookie.Value)
	}
	if len(parts) == 0 {
		return nil, errors.New("newapi login returned no session cookie")
	}
	return &Session{Cookie: strings.Join(parts, "; "), UserID: strconv.FormatInt(result.ID, 10)}, nil
}

func (c *NewAPIConnector) GetBalance(ctx context.Context, baseURL string, session *Session) (float64, error) {
	baseURL = normalizeBaseURL(baseURL)
	statusData, err := c.get(ctx, baseURL+"/api/status", nil)
	if err != nil {
		return 0, err
	}
	var status struct {
		QuotaPerUnit float64 `json:"quota_per_unit"`
	}
	if err := json.Unmarshal(statusData, &status); err != nil {
		return 0, err
	}
	if status.QuotaPerUnit <= 0 {
		status.QuotaPerUnit = 500000
	}
	selfData, err := c.get(ctx, baseURL+"/api/user/self", session)
	if err != nil {
		return 0, err
	}
	var self struct {
		Quota float64 `json:"quota"`
	}
	if err := json.Unmarshal(selfData, &self); err != nil {
		return 0, err
	}
	return self.Quota / status.QuotaPerUnit, nil
}

func (c *NewAPIConnector) ListGroups(ctx context.Context, baseURL string, session *Session) ([]RemoteGroup, error) {
	data, err := c.get(ctx, normalizeBaseURL(baseURL)+"/api/user/self/groups", session)
	if err != nil {
		return nil, err
	}
	raw := map[string]struct {
		Ratio json.RawMessage `json:"ratio"`
		Desc  string          `json:"desc"`
	}{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	groups := make([]RemoteGroup, 0, len(raw))
	for name, item := range raw {
		var rate float64
		if err := json.Unmarshal(item.Ratio, &rate); err != nil {
			continue
		}
		groups = append(groups, RemoteGroup{Key: name, Name: name, Description: item.Desc, Rate: rate, Platform: "openai"})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Key < groups[j].Key })
	return groups, nil
}

func (c *NewAPIConnector) GetRechargeMultiplier(context.Context, string, *Session) (float64, error) {
	return 0, nil
}

func (c *NewAPIConnector) get(ctx context.Context, url string, session *Session) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	applyNewAPIAuth(req, session)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	return decodeNewAPIResponse(io.LimitReader(resp.Body, 8<<20))
}

func applyNewAPIAuth(req *http.Request, session *Session) {
	if session == nil {
		return
	}
	if strings.TrimSpace(session.Cookie) != "" {
		req.Header.Set("Cookie", session.Cookie)
	} else if strings.TrimSpace(session.AccessToken) != "" {
		req.Header.Set("Authorization", session.AccessToken)
	}
	if strings.TrimSpace(session.UserID) != "" {
		req.Header.Set("New-Api-User", session.UserID)
	}
}

func decodeNewAPIResponse(reader io.Reader) ([]byte, error) {
	var wrapped struct {
		Success bool            `json:"success"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(reader).Decode(&wrapped); err != nil {
		return nil, err
	}
	if !wrapped.Success {
		return nil, errors.New(strings.TrimSpace(wrapped.Message))
	}
	return wrapped.Data, nil
}
