package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

// SoraGDriveOAuthService 处理 Google Drive OAuth2 授权流程。
type SoraGDriveOAuthService struct{}

// NewSoraGDriveOAuthService 创建 GDrive OAuth 服务。
func NewSoraGDriveOAuthService(_ *SettingService) *SoraGDriveOAuthService {
	return &SoraGDriveOAuthService{}
}

// GenerateAuthURL 生成 Google OAuth 授权 URL。
func (s *SoraGDriveOAuthService) GenerateAuthURL(clientID, clientSecret, redirectURI string) (authURL, state string, err error) {
	if clientID == "" || clientSecret == "" || redirectURI == "" {
		return "", "", fmt.Errorf("client_id, client_secret, redirect_uri are required")
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveFileScope},
		RedirectURL:  redirectURI,
	}

	// 生成随机 state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("generate state: %w", err)
	}
	state = hex.EncodeToString(stateBytes)

	authURL = config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	return authURL, state, nil
}

// ExchangeCode 用授权码换取 refresh_token。
func (s *SoraGDriveOAuthService) ExchangeCode(ctx context.Context, clientID, clientSecret, redirectURI, code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("authorization code is required")
	}

	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveFileScope},
		RedirectURL:  redirectURI,
	}

	token, err := config.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("exchange code: %w", err)
	}

	if token.RefreshToken == "" {
		return "", fmt.Errorf("no refresh_token received, please revoke app access and try again")
	}

	return token.RefreshToken, nil
}
