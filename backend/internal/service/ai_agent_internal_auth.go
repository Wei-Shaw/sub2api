package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const AgentInternalAuthHeader = "X-Sub2API-Agent-Authorization"

type AgentInternalIdentity struct {
	UserID      int64  `json:"user_id"`
	Concurrency int    `json:"concurrency"`
	Email       string `json:"email,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	ExpiresAt   int64  `json:"expires_at"`
	Method      string `json:"method"`
	RequestURI  string `json:"request_uri"`
}

type AgentInternalAuth struct {
	key []byte
}

func NewAgentInternalAuth() (*AgentInternalAuth, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate ai agent internal auth key: %w", err)
	}
	return &AgentInternalAuth{key: key}, nil
}

func (a *AgentInternalAuth) Sign(identity AgentInternalIdentity, method, requestURI string) (string, error) {
	if a == nil || len(a.key) == 0 || identity.UserID <= 0 {
		return "", errors.New("ai agent internal auth is unavailable")
	}
	identity.ExpiresAt = time.Now().Add(30 * time.Second).Unix()
	identity.Method = strings.ToUpper(method)
	identity.RequestURI = requestURI
	payload, err := json.Marshal(identity)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, a.key)
	_, _ = mac.Write(payload)
	signature := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (a *AgentInternalAuth) Validate(r *http.Request) (AgentInternalIdentity, error) {
	if a == nil || r == nil || !isLoopbackRequest(r.RemoteAddr) {
		return AgentInternalIdentity{}, errors.New("invalid internal request origin")
	}
	parts := strings.Split(r.Header.Get(AgentInternalAuthHeader), ".")
	if len(parts) != 2 {
		return AgentInternalIdentity{}, errors.New("invalid internal auth token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return AgentInternalIdentity{}, errors.New("invalid internal auth payload")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AgentInternalIdentity{}, errors.New("invalid internal auth signature")
	}
	mac := hmac.New(sha256.New, a.key)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return AgentInternalIdentity{}, errors.New("internal auth signature mismatch")
	}
	var identity AgentInternalIdentity
	if err := json.Unmarshal(payload, &identity); err != nil {
		return AgentInternalIdentity{}, errors.New("invalid internal auth claims")
	}
	if identity.UserID <= 0 || identity.ExpiresAt < time.Now().Unix() || identity.Method != r.Method || identity.RequestURI != r.URL.RequestURI() {
		return AgentInternalIdentity{}, errors.New("internal auth claims mismatch")
	}
	return identity, nil
}

func isLoopbackRequest(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
