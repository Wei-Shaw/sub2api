package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sub2api/internal/service"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type turnstileVerifier struct {
	httpClient *http.Client
REDACTED

func NewTurnstileVerifier() service.TurnstileVerifier {
	return &turnstileVerifier{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
	REDACTED,
REDACTED
REDACTED

func (v *turnstileVerifier) VerifyToken(ctx context.Context, secretKey, token, remoteIP string) (*service.TurnstileVerifyResponse, error) {
	formData := url.Values{REDACTED
	formData.Set("secret", secretKey)
	formData.Set("response", token)
	if remoteIP != "" {
		formData.Set("remoteip", remoteIP)
REDACTED

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
REDACTED
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()

	var result service.TurnstileVerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
REDACTED

	return &result, nil
REDACTED
