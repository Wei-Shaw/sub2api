package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const feishuTokenURL = "https://open.feishu.cn/open-apis/authen/v2/oauth/token"
const feishuUserInfoURL = "https://open.feishu.cn/open-apis/authen/v1/user_info"

type feishuProfile struct {
	Name      string `json:"name"`
	TenantKey string `json:"tenant_key"`
	UnionID   string `json:"union_id"`
	OpenID    string `json:"open_id"`
	UserID    string `json:"user_id"`
}

// Neither provider response bodies nor credentials are included in errors.
// Fixed HTTPS endpoints avoid turning OAuth configuration into an SSRF surface.
func fetchFeishuProfile(ctx context.Context, client *http.Client, cfg config.FeishuConnectConfig, code string) (feishuProfile, error) {
	base := http.Client{Timeout: 15 * time.Second}
	if client != nil {
		base = *client
	}
	if base.Timeout <= 0 || base.Timeout > 15*time.Second {
		base.Timeout = 15 * time.Second
	}
	base.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	client = &base
	body, err := json.Marshal(map[string]string{"grant_type": "authorization_code", "client_id": cfg.ClientID, "client_secret": cfg.ClientSecret, "code": code, "redirect_uri": cfg.RedirectURL})
	if err != nil {
		return feishuProfile{}, errors.New("could not encode Feishu token request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, feishuTokenURL, bytes.NewReader(body))
	if err != nil {
		return feishuProfile{}, errors.New("could not create Feishu token request")
	}
	req.Header.Set("Content-Type", "application/json")
	var token struct {
		Code        *int   `json:"code"`
		AccessToken string `json:"access_token"`
	}
	if err := readFeishuJSON(client, req, &token); err != nil || (token.Code == nil || *token.Code != 0) || strings.TrimSpace(token.AccessToken) == "" {
		return feishuProfile{}, errors.New("feishu token exchange failed")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, feishuUserInfoURL, nil)
	if err != nil {
		return feishuProfile{}, errors.New("could not create Feishu userinfo request")
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	var profile struct {
		Code *int          `json:"code"`
		Data feishuProfile `json:"data"`
	}
	if err := readFeishuJSON(client, req, &profile); err != nil || (profile.Code == nil || *profile.Code != 0) {
		return feishuProfile{}, errors.New("feishu userinfo failed")
	}
	p := profile.Data
	p.TenantKey, p.UnionID, p.OpenID, p.UserID = strings.TrimSpace(p.TenantKey), strings.TrimSpace(p.UnionID), strings.TrimSpace(p.OpenID), strings.TrimSpace(p.UserID)
	if p.TenantKey == "" || (p.UnionID == "" && p.OpenID == "") || len(p.TenantKey) > 256 || len(p.UnionID) > 256 || len(p.OpenID) > 256 || len(p.UserID) > 256 {
		return feishuProfile{}, errors.New("feishu identity is missing a valid tenant or stable subject")
	}
	for _, tenant := range cfg.AllowedTenantKeys {
		if p.TenantKey == strings.TrimSpace(tenant) {
			return p, nil
		}
	}
	return feishuProfile{}, errors.New("feishu tenant is not allowed")
}

func readFeishuJSON(client *http.Client, req *http.Request, target any) error {
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("feishu request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errors.New("feishu returned an unsuccessful response")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil || len(body) > 1<<20 {
		return errors.New("feishu response is invalid")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("feishu response is invalid")
	}
	return nil
}

// Opaque subjects are tenant-scoped, avoiding exposure of provider identifiers.
// Keep both verified candidates so an existing open_id identity survives a
// later userinfo response that also supplies union_id. Conflicting owners fail.
func feishuSubjectCandidates(profile feishuProfile) []string {
	var subjects []string
	seen := make(map[string]bool)
	for _, value := range []string{profile.UnionID, profile.OpenID, profile.UserID} {
		if value == "" {
			continue
		}
		hash := sha256.Sum256([]byte(profile.TenantKey + "\x00" + value))
		subject := hex.EncodeToString(hash[:])
		if !seen[subject] {
			subjects = append(subjects, subject)
			seen[subject] = true
		}
	}
	return subjects
}
