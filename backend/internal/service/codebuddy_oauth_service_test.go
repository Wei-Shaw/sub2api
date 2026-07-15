package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/codebuddy"
)

type fakeCodeBuddyClient struct{}

func (f *fakeCodeBuddyClient) FetchState(ctx context.Context, proxyURL string) (*codebuddy.StateResult, error) {
	return &codebuddy.StateResult{State: "st", AuthURL: "https://copilot.tencent.com/login?state=st"}, nil
}

func (f *fakeCodeBuddyClient) FetchToken(ctx context.Context, state, proxyURL string) (*codebuddy.TokenResponse, error) {
	return &codebuddy.TokenResponse{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 100, TokenType: "Bearer"}, nil
}

func (f *fakeCodeBuddyClient) GetAccountInfo(ctx context.Context, accessToken, state, proxyURL string) (*codebuddy.AccountInfo, error) {
	return &codebuddy.AccountInfo{UID: "u1", Nickname: "nick"}, nil
}

func (f *fakeCodeBuddyClient) GetConfig(ctx context.Context, accessToken, uid, proxyURL string) ([]byte, error) {
	return []byte(`{"agents":[{"name":"cli","models":["auto","hy3","glm-5.2","hy3"]},{"name":"craft","models":["glm-5.1"]}]}`), nil
}

func (f *fakeCodeBuddyClient) RefreshToken(ctx context.Context, refreshToken, proxyURL string) (*codebuddy.TokenResponse, error) {
	return &codebuddy.TokenResponse{AccessToken: "at2", RefreshToken: "rt2", ExpiresIn: 100}, nil
}

func newTestCodeBuddyService() *CodeBuddyOAuthService {
	return NewCodeBuddyOAuthService(nil, &fakeCodeBuddyClient{})
}

func TestCodeBuddyExchangeState(t *testing.T) {
	svc := newTestCodeBuddyService()
	info, err := svc.ExchangeState(context.Background(), &CodeBuddyExchangeStateInput{State: "st"})
	if err != nil {
		t.Fatalf("ExchangeState err = %v", err)
	}
	if info.AccessToken != "at" {
		t.Fatalf("AccessToken = %q", info.AccessToken)
	}
	if info.UID != "u1" || info.Nickname != "nick" {
		t.Fatalf("account info not parsed: %+v", info)
	}
	if len(info.EnabledModels) != 3 {
		t.Fatalf("EnabledModels = %v (want 3 after dedup/auto-filter)", info.EnabledModels)
	}
}

func TestCodeBuddyBuildAccountCredentials(t *testing.T) {
	svc := newTestCodeBuddyService()
	info, err := svc.ExchangeState(context.Background(), &CodeBuddyExchangeStateInput{State: "st"})
	if err != nil {
		t.Fatalf("ExchangeState err = %v", err)
	}
	creds := svc.BuildAccountCredentials(info)
	if creds["access_token"] != "at" {
		t.Fatalf("access_token = %v", creds["access_token"])
	}
	if creds["refresh_token"] != "rt" {
		t.Fatalf("refresh_token = %v", creds["refresh_token"])
	}
	models, ok := creds["models"].([]string)
	if !ok || len(models) != 3 {
		t.Fatalf("models = %v", creds["models"])
	}
	mapping, ok := creds["model_mapping"].(map[string]string)
	if !ok {
		t.Fatalf("model_mapping type = %T", creds["model_mapping"])
	}
	for _, m := range models {
		if mapping[m] != m {
			t.Fatalf("model_mapping[%s] = %q, want identity", m, mapping[m])
		}
	}
}

func TestCodeBuddyRefreshToken(t *testing.T) {
	svc := newTestCodeBuddyService()
	info, err := svc.RefreshToken(context.Background(), "rt", "")
	if err != nil {
		t.Fatalf("RefreshToken err = %v", err)
	}
	if info.AccessToken != "at2" || info.RefreshToken != "rt2" {
		t.Fatalf("refresh response = %+v", info)
	}
}

func TestCodeBuddyTokenRefresherCanRefresh(t *testing.T) {
	svc := newTestCodeBuddyService()
	refresher := NewCodeBuddyTokenRefresher(svc)

	cbAccount := &Account{
		ID:          1,
		Platform:    PlatformCodeBuddy,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"refresh_token": "rt", "access_token": "at"},
	}
	if !refresher.CanRefresh(cbAccount) {
		t.Fatal("CanRefresh should be true for codebuddy oauth account")
	}
	if !refresher.NeedsRefresh(cbAccount, 0) {
		t.Fatal("NeedsRefresh should be true when expires_at is missing")
	}

	other := &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	if refresher.CanRefresh(other) {
		t.Fatal("CanRefresh should be false for non-codebuddy account")
	}
}
