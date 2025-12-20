package service

import (
	"context"
	"errors"
	"fmt"
	"log"
)

var (
	ErrTurnstileVerificationFailed = errors.New("turnstile verification failed")
	ErrTurnstileNotConfigured      = errors.New("turnstile not configured")
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// TurnstileVerifier 验证 Turnstile token 的接口
type TurnstileVerifier interface {
	VerifyToken(ctx context.Context, secretKey, token, remoteIP string) (*TurnstileVerifyResponse, error)
REDACTED

// TurnstileService Turnstile 验证服务
type TurnstileService struct {
	settingService *SettingService
	verifier       TurnstileVerifier
REDACTED

// TurnstileVerifyResponse Cloudflare Turnstile 验证响应
type TurnstileVerifyResponse struct {
	Success     bool     `json:"success"`
	ChallengeTS string   `json:"challenge_ts"`
	Hostname    string   `json:"hostname"`
	ErrorCodes  []string `json:"error-codes"`
	Action      string   `json:"action"`
	CData       string   `json:"cdata"`
REDACTED

// NewTurnstileService 创建 Turnstile 服务实例
func NewTurnstileService(settingService *SettingService, verifier TurnstileVerifier) *TurnstileService {
	return &TurnstileService{
		settingService: settingService,
		verifier:       verifier,
REDACTED
REDACTED

// VerifyToken 验证 Turnstile token
func (s *TurnstileService) VerifyToken(ctx context.Context, token string, remoteIP string) error {
	// 检查是否启用 Turnstile
	if !s.settingService.IsTurnstileEnabled(ctx) {
		log.Println("[Turnstile] Disabled, skipping verification")
		return nil
REDACTED

	// 获取 Secret Key
	secretKey := s.settingService.GetTurnstileSecretKey(ctx)
	if secretKey == "" {
		log.Println("[Turnstile] Secret key not configured")
		return ErrTurnstileNotConfigured
REDACTED

	// 如果 token 为空，返回错误
	if token == "" {
		log.Println("[Turnstile] Token is empty")
		return ErrTurnstileVerificationFailed
REDACTED

	log.Printf("[Turnstile] Verifying token for IP: %s", remoteIP)
	result, err := s.verifier.VerifyToken(ctx, secretKey, token, remoteIP)
	if err != nil {
		log.Printf("[Turnstile] Request failed: %v", err)
		return fmt.Errorf("send request: %w", err)
REDACTED

	if !result.Success {
		log.Printf("[Turnstile] Verification failed, error codes: %v", result.ErrorCodes)
		return ErrTurnstileVerificationFailed
REDACTED

	log.Println("[Turnstile] Verification successful")
	return nil
REDACTED

// IsEnabled 检查 Turnstile 是否启用
func (s *TurnstileService) IsEnabled(ctx context.Context) bool {
	return s.settingService.IsTurnstileEnabled(ctx)
REDACTED
