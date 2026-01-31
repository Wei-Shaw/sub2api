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

// Sora2APIModel represents a model entry returned by sora2api.
type Sora2APIModel struct {
	ID          string `json:"id"`
	Object      string `json:"object"`
	OwnedBy     string `json:"owned_by,omitempty"`
	Description string `json:"description,omitempty"`
REDACTED

// Sora2APIModelList represents /v1/models response.
type Sora2APIModelList struct {
	Object string          `json:"object"`
	Data   []Sora2APIModel `json:"data"`
REDACTED

// Sora2APIImportTokenItem mirrors sora2api ImportTokenItem.
type Sora2APIImportTokenItem struct {
	Email            string `json:"email"`
	AccessToken      string `json:"access_token,omitempty"`
	SessionToken     string `json:"session_token,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	ClientID         string `json:"client_id,omitempty"`
	ProxyURL         string `json:"proxy_url,omitempty"`
	Remark           string `json:"remark,omitempty"`
	IsActive         bool   `json:"is_active"`
	ImageEnabled     bool   `json:"image_enabled"`
	VideoEnabled     bool   `json:"video_enabled"`
	ImageConcurrency int    `json:"image_concurrency"`
	VideoConcurrency int    `json:"video_concurrency"`
REDACTED

// Sora2APIToken represents minimal fields for admin list.
type Sora2APIToken struct {
	ID     int64  `json:"id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
	Remark string `json:"remark"`
REDACTED

// Sora2APIService provides access to sora2api endpoints.
type Sora2APIService struct {
	cfg *config.Config

	baseURL         string
	apiKey          string
	adminUsername   string
	adminPassword   string
	adminTokenTTL   time.Duration
	adminTimeout    time.Duration
	tokenImportMode string

	client      *http.Client
	adminClient *http.Client

	adminToken   string
	adminTokenAt time.Time
	adminMu      sync.Mutex

	modelCache   []Sora2APIModel
	modelCacheAt time.Time
	modelMu      sync.RWMutex
REDACTED

func NewSora2APIService(cfg *config.Config) *Sora2APIService {
	if cfg == nil {
		return &Sora2APIService{REDACTED
REDACTED
	adminTTL := time.Duration(cfg.Sora2API.AdminTokenTTLSeconds) * time.Second
	if adminTTL <= 0 {
		adminTTL = 15 * time.Minute
REDACTED
	adminTimeout := time.Duration(cfg.Sora2API.AdminTimeoutSeconds) * time.Second
	if adminTimeout <= 0 {
		adminTimeout = 10 * time.Second
REDACTED
	return &Sora2APIService{
		cfg:             cfg,
		baseURL:         strings.TrimRight(strings.TrimSpace(cfg.Sora2API.BaseURL), "/"),
		apiKey:          strings.TrimSpace(cfg.Sora2API.APIKey),
		adminUsername:   strings.TrimSpace(cfg.Sora2API.AdminUsername),
		adminPassword:   strings.TrimSpace(cfg.Sora2API.AdminPassword),
		adminTokenTTL:   adminTTL,
		adminTimeout:    adminTimeout,
		tokenImportMode: strings.ToLower(strings.TrimSpace(cfg.Sora2API.TokenImportMode)),
		client:          &http.Client{REDACTED,
		adminClient:     &http.Client{Timeout: adminTimeoutREDACTED,
REDACTED
REDACTED

func (s *Sora2APIService) Enabled() bool {
	return s != nil && s.baseURL != "" && s.apiKey != ""
REDACTED

func (s *Sora2APIService) AdminEnabled() bool {
	return s != nil && s.baseURL != "" && s.adminUsername != "" && s.adminPassword != ""
REDACTED

func (s *Sora2APIService) buildURL(path string) string {
	if s.baseURL == "" {
		return path
REDACTED
	if strings.HasPrefix(path, "/") {
		return s.baseURL + path
REDACTED
	return s.baseURL + "/" + path
REDACTED

// BuildURL 返回完整的 sora2api URL（用于代理媒体）
func (s *Sora2APIService) BuildURL(path string) string {
	return s.buildURL(path)
REDACTED

func (s *Sora2APIService) NewAPIRequest(ctx context.Context, method string, path string, body []byte) (*http.Request, error) {
	if !s.Enabled() {
		return nil, errors.New("sora2api not configured")
REDACTED
	req, err := http.NewRequestWithContext(ctx, method, s.buildURL(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
REDACTED

func (s *Sora2APIService) ListModels(ctx context.Context) ([]Sora2APIModel, error) {
	if !s.Enabled() {
		return nil, errors.New("sora2api not configured")
REDACTED
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.buildURL("/v1/models"), nil)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return s.cachedModelsOnError(err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode != http.StatusOK {
		return s.cachedModelsOnError(fmt.Errorf("sora2api models status: %d", resp.StatusCode))
REDACTED

	var payload Sora2APIModelList
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return s.cachedModelsOnError(err)
REDACTED
	models := payload.Data
	if s.cfg != nil && s.cfg.Gateway.SoraModelFilters.HidePromptEnhance {
		filtered := make([]Sora2APIModel, 0, len(models))
		for _, m := range models {
			if strings.HasPrefix(strings.ToLower(m.ID), "prompt-enhance") {
				continue
		REDACTED
			filtered = append(filtered, m)
	REDACTED
		models = filtered
REDACTED

	s.modelMu.Lock()
	s.modelCache = models
	s.modelCacheAt = time.Now()
	s.modelMu.Unlock()

	return models, nil
REDACTED

func (s *Sora2APIService) cachedModelsOnError(err error) ([]Sora2APIModel, error) {
	s.modelMu.RLock()
	cached := append([]Sora2APIModel(nil), s.modelCache...)
	s.modelMu.RUnlock()
	if len(cached) > 0 {
		log.Printf("[Sora2API] 模型列表拉取失败，回退缓存: %v", err)
		return cached, nil
REDACTED
	return nil, err
REDACTED

func (s *Sora2APIService) ImportTokens(ctx context.Context, items []Sora2APIImportTokenItem) error {
	if !s.AdminEnabled() {
		return errors.New("sora2api admin not configured")
REDACTED
	mode := s.tokenImportMode
	if mode == "" {
		mode = "at"
REDACTED
	payload := map[string]any{
		"tokens": items,
		"mode":   mode,
REDACTED
	_, err := s.doAdminRequest(ctx, http.MethodPost, "/api/tokens/import", payload, nil)
	return err
REDACTED

func (s *Sora2APIService) ListTokens(ctx context.Context) ([]Sora2APIToken, error) {
	if !s.AdminEnabled() {
		return nil, errors.New("sora2api admin not configured")
REDACTED
	var tokens []Sora2APIToken
	_, err := s.doAdminRequest(ctx, http.MethodGet, "/api/tokens", nil, &tokens)
	return tokens, err
REDACTED

func (s *Sora2APIService) DisableToken(ctx context.Context, tokenID int64) error {
	if !s.AdminEnabled() {
		return errors.New("sora2api admin not configured")
REDACTED
	path := fmt.Sprintf("/api/tokens/%d/disable", tokenID)
	_, err := s.doAdminRequest(ctx, http.MethodPost, path, nil, nil)
	return err
REDACTED

func (s *Sora2APIService) DeleteToken(ctx context.Context, tokenID int64) error {
	if !s.AdminEnabled() {
		return errors.New("sora2api admin not configured")
REDACTED
	path := fmt.Sprintf("/api/tokens/%d", tokenID)
	_, err := s.doAdminRequest(ctx, http.MethodDelete, path, nil, nil)
	return err
REDACTED

func (s *Sora2APIService) doAdminRequest(ctx context.Context, method string, path string, body any, out any) (*http.Response, error) {
	if !s.AdminEnabled() {
		return nil, errors.New("sora2api admin not configured")
REDACTED
	token, err := s.getAdminToken(ctx)
	if err != nil {
		return nil, err
REDACTED
	resp, err := s.doAdminRequestWithToken(ctx, method, path, token, body, out)
	if err == nil && resp != nil && resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
REDACTED
	if resp != nil && resp.StatusCode == http.StatusUnauthorized {
		s.invalidateAdminToken()
		token, err = s.getAdminToken(ctx)
		if err != nil {
			return resp, err
	REDACTED
		return s.doAdminRequestWithToken(ctx, method, path, token, body, out)
REDACTED
	return resp, err
REDACTED

func (s *Sora2APIService) doAdminRequestWithToken(ctx context.Context, method string, path string, token string, body any, out any) (*http.Response, error) {
	var reader *bytes.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
	REDACTED
		reader = bytes.NewReader(buf)
REDACTED else {
		reader = bytes.NewReader(nil)
REDACTED
	req, err := http.NewRequestWithContext(ctx, method, s.buildURL(path), reader)
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
REDACTED
	resp, err := s.adminClient.Do(req)
	if err != nil {
		return resp, err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, fmt.Errorf("sora2api admin status: %d", resp.StatusCode)
REDACTED
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, err
	REDACTED
REDACTED
	return resp, nil
REDACTED

func (s *Sora2APIService) getAdminToken(ctx context.Context) (string, error) {
	s.adminMu.Lock()
	defer s.adminMu.Unlock()

	if s.adminToken != "" && time.Since(s.adminTokenAt) < s.adminTokenTTL {
		return s.adminToken, nil
REDACTED

	if !s.AdminEnabled() {
		return "", errors.New("sora2api admin not configured")
REDACTED

	payload := map[string]string{
		"username": s.adminUsername,
		"password": s.adminPassword,
REDACTED
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", err
REDACTED
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.buildURL("/api/login"), bytes.NewReader(buf))
	if err != nil {
		return "", err
REDACTED
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.adminClient.Do(req)
	if err != nil {
		return "", err
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("sora2api login failed: %d", resp.StatusCode)
REDACTED
	var result struct {
		Success bool   `json:"success"`
		Token   string `json:"token"`
		Message string `json:"message"`
REDACTED
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
REDACTED
	if !result.Success || result.Token == "" {
		if result.Message == "" {
			result.Message = "sora2api login failed"
	REDACTED
		return "", errors.New(result.Message)
REDACTED
	s.adminToken = result.Token
	s.adminTokenAt = time.Now()
	return result.Token, nil
REDACTED

func (s *Sora2APIService) invalidateAdminToken() {
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	s.adminToken = ""
	s.adminTokenAt = time.Time{REDACTED
REDACTED
