package register

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/tools/warp-gateway/internal/store"
	"golang.org/x/crypto/curve25519"
)

const (
	defaultRegAPI = "https://api.cloudflareclient.com/v0a1922/reg"
	defaultPeerPK = "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="
	defaultEngage = "engage.cloudflareclient.com:2408"
)

// Result is a newly registered free WARP WireGuard profile.
type Result struct {
	Profile    store.Profile `json:"profile"`
	DeviceID   string        `json:"device_id,omitempty"`
	AccountID  string        `json:"account_id,omitempty"`
	LicenseKey string        `json:"license_key,omitempty"`
}

// RegisterFree creates a free Cloudflare WARP device and returns a WireGuard profile.
func RegisterFree(ctx context.Context) (*Result, error) {
	priv, pub, err := generateKeyPair()
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"key":        pub,
		"install_id": "",
		"fcm_token":  "",
		"tos":        time.Now().UTC().Format(time.RFC3339),
		"model":      "PC",
		"type":       "Android",
		"locale":     "en_US",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, defaultRegAPI, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", "a-6.30-3596")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("warp register: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("warp register HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var parsed regResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode warp register: %w", err)
	}

	addrs := []string{}
	if v4 := strings.TrimSpace(parsed.Config.Interface.Addresses.V4); v4 != "" {
		if !strings.Contains(v4, "/") {
			v4 += "/32"
		}
		addrs = append(addrs, v4)
	}
	if v6 := strings.TrimSpace(parsed.Config.Interface.Addresses.V6); v6 != "" {
		if !strings.Contains(v6, "/") {
			v6 += "/128"
		}
		addrs = append(addrs, v6)
	}
	if len(addrs) == 0 {
		addrs = []string{"172.16.0.2/32"}
	}

	peerPK := defaultPeerPK
	endpoint := defaultEngage
	if len(parsed.Config.Peers) > 0 {
		if pk := strings.TrimSpace(parsed.Config.Peers[0].PublicKey); pk != "" {
			peerPK = pk
		}
		ep := parsed.Config.Peers[0].Endpoint
		if host := strings.TrimSpace(ep.Host); host != "" {
			endpoint = host + ":2408"
		} else if v4 := strings.TrimSpace(ep.V4); v4 != "" {
			endpoint = v4 + ":2408"
		}
	}

	license := strings.TrimSpace(parsed.Account.License)
	return &Result{
		Profile: store.Profile{
			PrivateKey: priv,
			Address:    addrs,
			DNS:        []string{"1.1.1.1", "1.0.0.1"},
			MTU:        1280,
			LicenseKey: license,
			Peers: []store.PeerConfig{{
				PublicKey:  peerPK,
				Endpoint:   endpoint,
				AllowedIPs: []string{"0.0.0.0/0", "::/0"},
			}},
		},
		DeviceID:   parsed.ID,
		AccountID:  parsed.Account.ID,
		LicenseKey: license,
	}, nil
}

// RegisterMany registers n free WARP profiles (sequential to reduce rate limits).
func RegisterMany(ctx context.Context, n int) ([]Result, error) {
	if n <= 0 {
		return nil, fmt.Errorf("count must be > 0")
	}
	if n > 20 {
		return nil, fmt.Errorf("count too large (max 20 per call)")
	}
	out := make([]Result, 0, n)
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		r, err := RegisterFree(ctx)
		if err != nil {
			return out, fmt.Errorf("register %d/%d: %w", i+1, n, err)
		}
		out = append(out, *r)
		if i+1 < n {
			time.Sleep(800 * time.Millisecond)
		}
	}
	return out, nil
}

type regResponse struct {
	ID      string `json:"id"`
	Account struct {
		ID      string `json:"id"`
		License string `json:"license"`
	} `json:"account"`
	Config struct {
		Interface struct {
			Addresses struct {
				V4 string `json:"v4"`
				V6 string `json:"v6"`
			} `json:"addresses"`
		} `json:"interface"`
		Peers []struct {
			PublicKey string `json:"public_key"`
			Endpoint  struct {
				Host string `json:"host"`
				V4   string `json:"v4"`
				V6   string `json:"v6"`
			} `json:"endpoint"`
		} `json:"peers"`
	} `json:"config"`
}

func generateKeyPair() (privateB64, publicB64 string, err error) {
	var priv [32]byte
	if _, err = io.ReadFull(rand.Reader, priv[:]); err != nil {
		return "", "", err
	}
	// curve25519 clamp
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	var pub [32]byte
	curve25519.ScalarBaseMult(&pub, &priv)
	return base64.StdEncoding.EncodeToString(priv[:]), base64.StdEncoding.EncodeToString(pub[:]), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
