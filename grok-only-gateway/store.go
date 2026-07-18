package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Store struct {
	mu    sync.Mutex
	path  string
	state storeState
}

type storeState struct {
	NextAccountID int64                    `json:"next_account_id"`
	Accounts      map[int64]*Account       `json:"accounts"`
	NextAPIKeyID  int64                    `json:"next_api_key_id"`
	APIKeys       map[int64]*APIKey        `json:"api_keys"`
	VideoBindings map[string]*VideoBinding `json:"video_bindings"`
	Logs          []RequestLog             `json:"logs"`
}

type AccountInput struct {
	Name         string     `json:"name"`
	Enabled      *bool      `json:"enabled"`
	AccessToken  string     `json:"access_token"`
	RefreshToken string     `json:"refresh_token"`
	IDToken      string     `json:"id_token"`
	TokenType    string     `json:"token_type"`
	ExpiresAt    *time.Time `json:"expires_at"`
	ExpiresIn    int64      `json:"expires_in"`
	ClientID     string     `json:"client_id"`
	Scope        string     `json:"scope"`
	Email        string     `json:"email"`
	BaseURL      string     `json:"base_url"`
	ProxyURL     string     `json:"proxy_url"`
	Concurrency  int        `json:"concurrency"`
	Priority     int        `json:"priority"`
}

type AccountPatch struct {
	Name          *string    `json:"name"`
	Enabled       *bool      `json:"enabled"`
	AccessToken   *string    `json:"access_token"`
	RefreshToken  *string    `json:"refresh_token"`
	BaseURL       *string    `json:"base_url"`
	ProxyURL      *string    `json:"proxy_url"`
	Concurrency   *int       `json:"concurrency"`
	Priority      *int       `json:"priority"`
	ExpiresAt     *time.Time `json:"expires_at"`
	ClearCooldown bool       `json:"clear_cooldown"`
}

func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	s.state = storeState{
		NextAccountID: 1,
		Accounts:      map[int64]*Account{},
		NextAPIKeyID:  1,
		APIKeys:       map[int64]*APIKey{},
		VideoBindings: map[string]*VideoBinding{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		return s, s.saveLocked()
	}
	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &s.state); err != nil {
			return nil, err
		}
	}
	s.ensureDefaultsLocked()
	return s, nil
}

func (s *Store) ensureDefaultsLocked() {
	if s.state.NextAccountID <= 0 {
		s.state.NextAccountID = 1
	}
	if s.state.Accounts == nil {
		s.state.Accounts = map[int64]*Account{}
	}
	if s.state.NextAPIKeyID <= 0 {
		s.state.NextAPIKeyID = 1
	}
	if s.state.APIKeys == nil {
		s.state.APIKeys = map[int64]*APIKey{}
	}
	if s.state.VideoBindings == nil {
		s.state.VideoBindings = map[string]*VideoBinding{}
	}
	for _, account := range s.state.Accounts {
		if account.BaseURL == "" {
			account.BaseURL = DefaultBaseURL
		}
		if account.Concurrency <= 0 {
			account.Concurrency = 1
		}
	}
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) HasAPIKeyValue(value string) bool {
	_, ok := s.GetAPIKeyByValue(value)
	return ok
}

func (s *Store) GetAPIKeyByValue(value string) (*APIKey, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range s.state.APIKeys {
		if key.Key == value {
			cp := *key
			return &cp, true
		}
	}
	return nil, false
}

func (s *Store) CreateAPIKey(name, value string) (*APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value = strings.TrimSpace(value)
	if value == "" {
		value = "grok_" + randomHex(24)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	for _, existing := range s.state.APIKeys {
		if existing.Key == value {
			cp := *existing
			return &cp, nil
		}
	}
	now := time.Now().UTC()
	key := &APIKey{
		ID:        s.state.NextAPIKeyID,
		Name:      name,
		Key:       value,
		Prefix:    keyPrefix(value),
		CreatedAt: now,
	}
	s.state.NextAPIKeyID++
	s.state.APIKeys[key.ID] = key
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	cp := *key
	return &cp, nil
}

func (s *Store) DeleteAPIKey(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.APIKeys, id)
	return s.saveLocked()
}

func (s *Store) ListAPIKeys(includeSecret bool) []APIKey {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]APIKey, 0, len(s.state.APIKeys))
	for _, key := range s.state.APIKeys {
		cp := *key
		if !includeSecret {
			cp.Key = ""
		}
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Store) CreateAccount(input AccountInput) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	expiresAt := input.ExpiresAt
	if expiresAt == nil && input.ExpiresIn > 0 {
		t := now.Add(time.Duration(input.ExpiresIn) * time.Second)
		expiresAt = &t
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = firstNonEmpty(input.Email, "Grok Account")
	}
	baseURL := normalizeBaseURL(input.BaseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	concurrency := input.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	account := &Account{
		ID:           s.state.NextAccountID,
		Name:         name,
		Enabled:      enabled,
		AccessToken:  strings.TrimSpace(input.AccessToken),
		RefreshToken: strings.TrimSpace(input.RefreshToken),
		IDToken:      strings.TrimSpace(input.IDToken),
		TokenType:    firstNonEmpty(input.TokenType, "Bearer"),
		ExpiresAt:    expiresAt,
		ClientID:     firstNonEmpty(input.ClientID, DefaultClientID),
		Scope:        strings.TrimSpace(input.Scope),
		Email:        strings.TrimSpace(input.Email),
		BaseURL:      baseURL,
		ProxyURL:     strings.TrimSpace(input.ProxyURL),
		Concurrency:  concurrency,
		Priority:     input.Priority,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.state.NextAccountID++
	s.state.Accounts[account.ID] = account
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return cloneAccount(account), nil
}

func (s *Store) UpdateAccount(id int64, patch AccountPatch) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.state.Accounts[id]
	if account == nil {
		return nil, errors.New("account not found")
	}
	if patch.Name != nil {
		account.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Enabled != nil {
		account.Enabled = *patch.Enabled
	}
	if patch.AccessToken != nil {
		account.AccessToken = strings.TrimSpace(*patch.AccessToken)
	}
	if patch.RefreshToken != nil {
		account.RefreshToken = strings.TrimSpace(*patch.RefreshToken)
	}
	if patch.BaseURL != nil {
		if baseURL := normalizeBaseURL(*patch.BaseURL); baseURL != "" {
			account.BaseURL = baseURL
		}
	}
	if patch.ProxyURL != nil {
		account.ProxyURL = strings.TrimSpace(*patch.ProxyURL)
	}
	if patch.Concurrency != nil {
		account.Concurrency = *patch.Concurrency
		if account.Concurrency <= 0 {
			account.Concurrency = 1
		}
	}
	if patch.Priority != nil {
		account.Priority = *patch.Priority
	}
	if patch.ExpiresAt != nil {
		account.ExpiresAt = patch.ExpiresAt
	}
	if patch.ClearCooldown {
		account.CooldownUntil = nil
		account.LastError = ""
	}
	account.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return cloneAccount(account), nil
}

func (s *Store) DeleteAccount(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.state.Accounts, id)
	for requestID, binding := range s.state.VideoBindings {
		if binding.AccountID == id {
			delete(s.state.VideoBindings, requestID)
		}
	}
	return s.saveLocked()
}

func (s *Store) GetAccount(id int64) (*Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.state.Accounts[id]
	if account == nil {
		return nil, false
	}
	return cloneAccount(account), true
}

func (s *Store) ListAccounts() []PublicAccount {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PublicAccount, 0, len(s.state.Accounts))
	for _, account := range s.state.Accounts {
		out = append(out, account.Public())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Store) SelectAccount(preferID int64, failed map[int64]bool) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.cleanupExpiredVideoBindingsLocked(now)

	if preferID > 0 {
		if account := s.state.Accounts[preferID]; account != nil && accountSelectable(account, now, failed) {
			account.InFlight++
			t := now
			account.LastUsedAt = &t
			account.UpdatedAt = now
			_ = s.saveLocked()
			return cloneAccount(account), nil
		}
	}

	var candidates []*Account
	for _, account := range s.state.Accounts {
		if accountSelectable(account, now, failed) {
			candidates = append(candidates, account)
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("no available Grok account")
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.InFlight != b.InFlight {
			return a.InFlight < b.InFlight
		}
		at, bt := time.Time{}, time.Time{}
		if a.LastUsedAt != nil {
			at = *a.LastUsedAt
		}
		if b.LastUsedAt != nil {
			bt = *b.LastUsedAt
		}
		if !at.Equal(bt) {
			return at.Before(bt)
		}
		return a.ID < b.ID
	})
	selected := candidates[0]
	selected.InFlight++
	t := now
	selected.LastUsedAt = &t
	selected.UpdatedAt = now
	_ = s.saveLocked()
	return cloneAccount(selected), nil
}

func accountSelectable(account *Account, now time.Time, failed map[int64]bool) bool {
	if account == nil || !account.Enabled {
		return false
	}
	if failed != nil && failed[account.ID] {
		return false
	}
	if account.CooldownUntil != nil && account.CooldownUntil.After(now) {
		return false
	}
	if account.Concurrency > 0 && account.InFlight >= account.Concurrency {
		return false
	}
	return true
}

func (s *Store) ReleaseAccount(id int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if account := s.state.Accounts[id]; account != nil {
		if account.InFlight > 0 {
			account.InFlight--
		}
		account.UpdatedAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) UpdateAccountToken(id int64, info *TokenInfo) (*Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	account := s.state.Accounts[id]
	if account == nil {
		return nil, errors.New("account not found")
	}
	applyTokenInfo(account, info)
	account.CooldownUntil = nil
	account.LastError = ""
	account.UpdatedAt = time.Now().UTC()
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return cloneAccount(account), nil
}

func (s *Store) MarkAccountCooldown(id int64, until time.Time, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if account := s.state.Accounts[id]; account != nil {
		account.CooldownUntil = &until
		account.LastError = reason
		account.UpdatedAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) MarkAccountError(id int64, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if account := s.state.Accounts[id]; account != nil {
		account.LastError = reason
		account.UpdatedAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) UpdateQuota(id int64, quota any) {
	if quota == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if account := s.state.Accounts[id]; account != nil {
		account.LastQuota = quota
		account.UpdatedAt = time.Now().UTC()
		_ = s.saveLocked()
	}
}

func (s *Store) BindVideo(requestID string, accountID int64) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || accountID <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.state.VideoBindings[requestID] = &VideoBinding{
		RequestID: requestID,
		AccountID: accountID,
		CreatedAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	_ = s.saveLocked()
}

func (s *Store) VideoAccountID(requestID string) int64 {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.cleanupExpiredVideoBindingsLocked(now)
	if binding := s.state.VideoBindings[requestID]; binding != nil {
		return binding.AccountID
	}
	return 0
}

func (s *Store) cleanupExpiredVideoBindingsLocked(now time.Time) {
	for requestID, binding := range s.state.VideoBindings {
		if binding == nil || binding.ExpiresAt.Before(now) {
			delete(s.state.VideoBindings, requestID)
		}
	}
}

func (s *Store) AddLog(entry RequestLog) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	s.state.Logs = append(s.state.Logs, entry)
	if len(s.state.Logs) > 200 {
		s.state.Logs = s.state.Logs[len(s.state.Logs)-200:]
	}
	_ = s.saveLocked()
}

func (s *Store) Logs() []RequestLog {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]RequestLog, len(s.state.Logs))
	copy(out, s.state.Logs)
	sort.Slice(out, func(i, j int) bool { return out[i].Time.After(out[j].Time) })
	return out
}

func applyTokenInfo(account *Account, info *TokenInfo) {
	if account == nil || info == nil {
		return
	}
	if info.AccessToken != "" {
		account.AccessToken = info.AccessToken
	}
	if info.RefreshToken != "" {
		account.RefreshToken = info.RefreshToken
	}
	if info.IDToken != "" {
		account.IDToken = info.IDToken
	}
	if info.TokenType != "" {
		account.TokenType = info.TokenType
	}
	if info.ClientID != "" {
		account.ClientID = info.ClientID
	}
	if info.Scope != "" {
		account.Scope = info.Scope
	}
	if info.Email != "" {
		account.Email = info.Email
	}
	if info.ExpiresAt > 0 {
		t := time.Unix(info.ExpiresAt, 0).UTC()
		account.ExpiresAt = &t
	}
}

func cloneAccount(account *Account) *Account {
	if account == nil {
		return nil
	}
	cp := *account
	return &cp
}

func keyPrefix(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 10 {
		return value
	}
	return value[:10]
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b)
}
