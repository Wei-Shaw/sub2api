package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

const defaultSoraClientID = "app_LlGpXReQgckcGGUo2JrYvtJK"

// SoraTokenRefreshService handles Sora access token refresh.
type SoraTokenRefreshService struct {
	accountRepo     AccountRepository
	soraAccountRepo SoraAccountRepository
	settingService  *SettingService
	httpUpstream    HTTPUpstream
	cfg             *config.Config
	stopCh          chan struct{REDACTED
	stopOnce        sync.Once
REDACTED

func NewSoraTokenRefreshService(
	accountRepo AccountRepository,
	soraAccountRepo SoraAccountRepository,
	settingService *SettingService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
) *SoraTokenRefreshService {
	return &SoraTokenRefreshService{
		accountRepo:     accountRepo,
		soraAccountRepo: soraAccountRepo,
		settingService:  settingService,
		httpUpstream:    httpUpstream,
		cfg:             cfg,
		stopCh:          make(chan struct{REDACTED),
REDACTED
REDACTED

func (s *SoraTokenRefreshService) Start() {
	if s == nil {
		return
REDACTED
	go s.refreshLoop()
REDACTED

func (s *SoraTokenRefreshService) Stop() {
	if s == nil {
		return
REDACTED
	s.stopOnce.Do(func() {
		close(s.stopCh)
REDACTED)
REDACTED

func (s *SoraTokenRefreshService) refreshLoop() {
	for {
		wait := s.nextRunDelay()
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
			s.refreshOnce()
		case <-s.stopCh:
			timer.Stop()
			return
	REDACTED
REDACTED
REDACTED

func (s *SoraTokenRefreshService) refreshOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if !s.isEnabled(ctx) {
		log.Println("[SoraTokenRefresh] disabled by settings")
		return
REDACTED
	if s.accountRepo == nil || s.soraAccountRepo == nil {
		log.Println("[SoraTokenRefresh] repository not configured")
		return
REDACTED

	accounts, err := s.accountRepo.ListByPlatform(ctx, PlatformSora)
	if err != nil {
		log.Printf("[SoraTokenRefresh] list accounts failed: %v", err)
		return
REDACTED
	if len(accounts) == 0 {
		log.Println("[SoraTokenRefresh] no sora accounts")
		return
REDACTED
	ids := make([]int64, 0, len(accounts))
	accountMap := make(map[int64]*Account, len(accounts))
	for i := range accounts {
		acc := accounts[i]
		ids = append(ids, acc.ID)
		accountMap[acc.ID] = &acc
REDACTED
	accountExtras, err := s.soraAccountRepo.GetByAccountIDs(ctx, ids)
	if err != nil {
		log.Printf("[SoraTokenRefresh] load sora accounts failed: %v", err)
		return
REDACTED

	success := 0
	failed := 0
	skipped := 0
	for accountID, account := range accountMap {
		extra := accountExtras[accountID]
		if extra == nil {
			skipped++
			continue
	REDACTED
		result, err := s.refreshForAccount(ctx, account, extra)
		if err != nil {
			failed++
			log.Printf("[SoraTokenRefresh] account %d refresh failed: %v", accountID, err)
			continue
	REDACTED
		if result == nil {
			skipped++
			continue
	REDACTED

		updates := map[string]any{
			"access_token": result.AccessToken,
	REDACTED
		if result.RefreshToken != "" {
			updates["refresh_token"] = result.RefreshToken
	REDACTED
		if result.Email != "" {
			updates["email"] = result.Email
	REDACTED
		if err := s.soraAccountRepo.Upsert(ctx, accountID, updates); err != nil {
			failed++
			log.Printf("[SoraTokenRefresh] account %d update failed: %v", accountID, err)
			continue
	REDACTED
		success++
REDACTED
	log.Printf("[SoraTokenRefresh] done: success=%d failed=%d skipped=%d", success, failed, skipped)
REDACTED

func (s *SoraTokenRefreshService) refreshForAccount(ctx context.Context, account *Account, extra *SoraAccount) (*soraRefreshResult, error) {
	if extra == nil {
		return nil, nil
REDACTED
	if strings.TrimSpace(extra.SessionToken) == "" && strings.TrimSpace(extra.RefreshToken) == "" {
		return nil, nil
REDACTED

	if extra.SessionToken != "" {
		result, err := s.refreshWithSessionToken(ctx, account, extra.SessionToken)
		if err == nil && result != nil && result.AccessToken != "" {
			return result, nil
	REDACTED
		if strings.TrimSpace(extra.RefreshToken) == "" {
			return nil, err
	REDACTED
REDACTED

	clientID := strings.TrimSpace(extra.ClientID)
	if clientID == "" {
		clientID = defaultSoraClientID
REDACTED
	return s.refreshWithRefreshToken(ctx, account, extra.RefreshToken, clientID)
REDACTED

type soraRefreshResult struct {
	AccessToken  string
	RefreshToken string
	Email        string
REDACTED

type soraSessionResponse struct {
	AccessToken string `json:"accessToken"`
	User        struct {
		Email string `json:"email"`
REDACTED `json:"user"`
REDACTED

type soraRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
REDACTED

func (s *SoraTokenRefreshService) refreshWithSessionToken(ctx context.Context, account *Account, sessionToken string) (*soraRefreshResult, error) {
	if s.httpUpstream == nil {
		return nil, fmt.Errorf("upstream not configured")
REDACTED
	req, err := http.NewRequestWithContext(ctx, "GET", "https://sora.chatgpt.com/api/auth/session", nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Cookie", "__Secure-next-auth.session-token="+sessionToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Origin", "https://sora.chatgpt.com")
	req.Header.Set("Referer", "https://sora.chatgpt.com/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	enableTLS := false
	if s.cfg != nil {
		enableTLS = s.cfg.Gateway.TLSFingerprint.Enabled
REDACTED
	proxyURL := ""
	accountConcurrency := 0
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
		accountConcurrency = account.Concurrency
		if account.Proxy != nil {
			proxyURL = account.Proxy.URL()
	REDACTED
REDACTED
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, accountID, accountConcurrency, enableTLS)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("session refresh failed: %d", resp.StatusCode)
REDACTED
	var payload soraSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
REDACTED
	if payload.AccessToken == "" {
		return nil, errors.New("session refresh missing access token")
REDACTED
	return &soraRefreshResult{AccessToken: payload.AccessToken, Email: payload.User.EmailREDACTED, nil
REDACTED

func (s *SoraTokenRefreshService) refreshWithRefreshToken(ctx context.Context, account *Account, refreshToken, clientID string) (*soraRefreshResult, error) {
	if s.httpUpstream == nil {
		return nil, fmt.Errorf("upstream not configured")
REDACTED
	payload := map[string]any{
		"client_id":     clientID,
		"grant_type":    "refresh_token",
		"redirect_uri":  "com.openai.chat://auth0.openai.com/ios/com.openai.chat/callback",
		"refresh_token": refreshToken,
REDACTED
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
REDACTED
	req, err := http.NewRequestWithContext(ctx, "POST", "https://auth.openai.com/oauth/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")

	enableTLS := false
	if s.cfg != nil {
		enableTLS = s.cfg.Gateway.TLSFingerprint.Enabled
REDACTED
	proxyURL := ""
	accountConcurrency := 0
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
		accountConcurrency = account.Concurrency
		if account.Proxy != nil {
			proxyURL = account.Proxy.URL()
	REDACTED
REDACTED
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, accountID, accountConcurrency, enableTLS)
	if err != nil {
		return nil, err
REDACTED
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("refresh token failed: %d", resp.StatusCode)
REDACTED
	var payloadResp soraRefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		return nil, err
REDACTED
	if payloadResp.AccessToken == "" {
		return nil, errors.New("refresh token missing access token")
REDACTED
	return &soraRefreshResult{AccessToken: payloadResp.AccessToken, RefreshToken: payloadResp.RefreshTokenREDACTED, nil
REDACTED

func (s *SoraTokenRefreshService) nextRunDelay() time.Duration {
	location := time.Local
	if s.cfg != nil && strings.TrimSpace(s.cfg.Timezone) != "" {
		if tz, err := time.LoadLocation(strings.TrimSpace(s.cfg.Timezone)); err == nil {
			location = tz
	REDACTED
REDACTED
	now := time.Now().In(location)
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location).Add(24 * time.Hour)
	return time.Until(next)
REDACTED

func (s *SoraTokenRefreshService) isEnabled(ctx context.Context) bool {
	if s.settingService == nil {
		return s.cfg != nil && s.cfg.Sora.TokenRefresh.Enabled
REDACTED
	cfg := s.settingService.GetSoraConfig(ctx)
	return cfg.TokenRefresh.Enabled
REDACTED
