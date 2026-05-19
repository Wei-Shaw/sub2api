package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

type stubKiroAPI struct {
	refreshFn      func(rt, proxy string) (*kiro.TokenInfo, error)
	profileFn      func(at, proxy string) (*kiro.Profile, error)
	startIdCFn     func(store *kiro.SessionStore, startURL, region, proxy string) (*kiro.IdCLoginStarted, *kiro.IdCSession, error)
	completeIdCFn  func(store *kiro.SessionStore, sessionID, callbackURL string) (*kiro.TokenInfo, error)
	startBuilderFn func(store *kiro.SessionStore, region, proxy string) (*kiro.BuilderIDLoginStarted, error)
	pollBuilderFn  func(store *kiro.SessionStore, sessionID string) (*kiro.BuilderIDPollResult, error)
	refreshHit     int
	profileHit     int
}

func (s *stubKiroAPI) RefreshSocial(rt, proxy string) (*kiro.TokenInfo, error) {
	s.refreshHit++
	return s.refreshFn(rt, proxy)
}

func (s *stubKiroAPI) FetchProfile(at, proxy string) (*kiro.Profile, error) {
	s.profileHit++
	return s.profileFn(at, proxy)
}

func (s *stubKiroAPI) StartIdCLogin(store *kiro.SessionStore, startURL, region, proxy string) (*kiro.IdCLoginStarted, *kiro.IdCSession, error) {
	if s.startIdCFn == nil {
		return nil, nil, errors.New("stub: StartIdCLogin not set")
	}
	return s.startIdCFn(store, startURL, region, proxy)
}

func (s *stubKiroAPI) CompleteIdCLogin(store *kiro.SessionStore, sessionID, callbackURL string) (*kiro.TokenInfo, error) {
	if s.completeIdCFn == nil {
		return nil, errors.New("stub: CompleteIdCLogin not set")
	}
	return s.completeIdCFn(store, sessionID, callbackURL)
}

func (s *stubKiroAPI) StartBuilderIDLogin(store *kiro.SessionStore, region, proxy string) (*kiro.BuilderIDLoginStarted, error) {
	if s.startBuilderFn == nil {
		return nil, errors.New("stub: StartBuilderIDLogin not set")
	}
	return s.startBuilderFn(store, region, proxy)
}

func (s *stubKiroAPI) PollBuilderIDLogin(store *kiro.SessionStore, sessionID string) (*kiro.BuilderIDPollResult, error) {
	if s.pollBuilderFn == nil {
		return nil, errors.New("stub: PollBuilderIDLogin not set")
	}
	return s.pollBuilderFn(store, sessionID)
}

func TestKiroOAuthService_ValidateSocialRefreshToken_HappyPath(t *testing.T) {
	api := &stubKiroAPI{
		refreshFn: func(rt, _ string) (*kiro.TokenInfo, error) {
			if rt != "rt-paste" {
				t.Fatalf("got rt=%q", rt)
			}
			return &kiro.TokenInfo{
				AccessToken:  "at",
				RefreshToken: "rt2",
				ExpiresAt:    999,
				ProfileARN:   "arn",
				AuthMethod:   kiro.AuthMethodSocial,
			}, nil
		},
		profileFn: func(at, _ string) (*kiro.Profile, error) {
			if at != "at" {
				t.Fatalf("profile called with wrong AT %q", at)
			}
			return &kiro.Profile{Email: "u@e.com", UserID: "u-1"}, nil
		},
	}
	svc := newKiroOAuthServiceWithAPI(api, nil)
	defer svc.Stop()

	info, err := svc.ValidateSocialRefreshToken(context.Background(), "rt-paste", nil)
	if err != nil {
		t.Fatal(err)
	}
	if info.AccessToken != "at" ||
		info.RefreshToken != "rt2" ||
		info.ExpiresAt != 999 ||
		info.ProfileARN != "arn" ||
		info.AuthMethod != "social" ||
		info.Email != "u@e.com" ||
		info.UserID != "u-1" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if api.refreshHit != 1 || api.profileHit != 1 {
		t.Fatalf("call counts: refresh=%d profile=%d", api.refreshHit, api.profileHit)
	}
}

func TestKiroOAuthService_ValidateSocialRefreshToken_RefreshFails(t *testing.T) {
	api := &stubKiroAPI{
		refreshFn: func(_, _ string) (*kiro.TokenInfo, error) { return nil, errors.New("upstream 400") },
		profileFn: func(_, _ string) (*kiro.Profile, error) {
			t.Fatal("profile should not be called when refresh fails")
			return nil, nil
		},
	}
	svc := newKiroOAuthServiceWithAPI(api, nil)
	defer svc.Stop()
	_, err := svc.ValidateSocialRefreshToken(context.Background(), "rt", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestKiroOAuthService_ValidateSocialRefreshToken_ProfileFailureNonFatal(t *testing.T) {
	api := &stubKiroAPI{
		refreshFn: func(_, _ string) (*kiro.TokenInfo, error) {
			return &kiro.TokenInfo{AccessToken: "at", AuthMethod: kiro.AuthMethodSocial}, nil
		},
		profileFn: func(_, _ string) (*kiro.Profile, error) { return nil, errors.New("404") },
	}
	svc := newKiroOAuthServiceWithAPI(api, nil)
	defer svc.Stop()
	info, err := svc.ValidateSocialRefreshToken(context.Background(), "rt", nil)
	if err != nil {
		t.Fatalf("expected success despite profile failure, got %v", err)
	}
	if info.Email != "" || info.UserID != "" {
		t.Fatalf("expected empty email/userID on profile failure, got %+v", info)
	}
	if info.AccessToken != "at" {
		t.Fatalf("expected token preserved, got %+v", info)
	}
}

func TestKiroOAuthService_ValidateSocialRefreshToken_EmptyInput(t *testing.T) {
	svc := newKiroOAuthServiceWithAPI(&stubKiroAPI{}, nil)
	defer svc.Stop()
	_, err := svc.ValidateSocialRefreshToken(context.Background(), "  ", nil)
	if err == nil {
		t.Fatal("expected error on empty refresh token")
	}
}

func TestKiroOAuthService_StartIdCLogin_HappyPath(t *testing.T) {
	api := &stubKiroAPI{
		startIdCFn: func(_ *kiro.SessionStore, startURL, region, _ string) (*kiro.IdCLoginStarted, *kiro.IdCSession, error) {
			if startURL != "https://start.aws" || region != "us-west-2" {
				t.Fatalf("got startURL=%q region=%q", startURL, region)
			}
			return &kiro.IdCLoginStarted{
				SessionID:     "sid-1",
				AuthorizeURL:  "https://oidc.example/authorize?x=1",
				ExpiresInSecs: 600,
			}, nil, nil
		},
	}
	svc := newKiroOAuthServiceWithAPI(api, nil)
	defer svc.Stop()

	result, err := svc.StartIdCLogin(context.Background(), "https://start.aws", "us-west-2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "sid-1" || result.AuthURL == "" || result.ExpiresIn != 600 {
		t.Fatalf("unexpected: %+v", result)
	}
}

func TestKiroOAuthService_StartIdCLogin_EmptyStartURL(t *testing.T) {
	svc := newKiroOAuthServiceWithAPI(&stubKiroAPI{}, nil)
	defer svc.Stop()
	_, err := svc.StartIdCLogin(context.Background(), "  ", "us-east-1", nil)
	if err == nil {
		t.Fatal("expected start_url required error")
	}
}

func TestKiroOAuthService_CompleteIdCLogin_HappyPath(t *testing.T) {
	api := &stubKiroAPI{
		completeIdCFn: func(_ *kiro.SessionStore, sessionID, callbackURL string) (*kiro.TokenInfo, error) {
			if sessionID != "sid-1" {
				t.Fatalf("sessionID=%q", sessionID)
			}
			if callbackURL == "" {
				t.Fatal("empty callbackURL")
			}
			return &kiro.TokenInfo{
				AccessToken:  "at",
				RefreshToken: "rt",
				ExpiresAt:    111,
				AuthMethod:   kiro.AuthMethodIdC,
				ClientID:     "cid",
				ClientSecret: "cs",
				Region:       "us-east-1",
				StartURL:     "https://start",
			}, nil
		},
		profileFn: func(_, _ string) (*kiro.Profile, error) {
			return &kiro.Profile{Email: "u@e", UserID: "u-1"}, nil
		},
	}
	svc := newKiroOAuthServiceWithAPI(api, nil)
	defer svc.Stop()

	info, err := svc.CompleteIdCLogin(context.Background(), "sid-1", "http://x/cb?code=c&state=s")
	if err != nil {
		t.Fatal(err)
	}
	if info.AuthMethod != "idc" || info.ClientID != "cid" || info.ClientSecret != "cs" ||
		info.Region != "us-east-1" || info.StartURL != "https://start" || info.Email != "u@e" {
		t.Fatalf("unexpected: %+v", info)
	}
}

func TestKiroOAuthService_StartBuilderIDLogin_HappyPath(t *testing.T) {
	api := &stubKiroAPI{
		startBuilderFn: func(_ *kiro.SessionStore, region, _ string) (*kiro.BuilderIDLoginStarted, error) {
			if region != "us-east-1" {
				t.Fatalf("region=%q", region)
			}
			return &kiro.BuilderIDLoginStarted{
				SessionID:       "bid-1",
				UserCode:        "ABCD-WXYZ",
				VerificationURI: "https://aws.example/verify",
				Interval:        5,
				ExpiresAtUnix:   999,
			}, nil
		},
	}
	svc := newKiroOAuthServiceWithAPI(api, nil)
	defer svc.Stop()

	result, err := svc.StartBuilderIDLogin(context.Background(), "us-east-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID != "bid-1" || result.UserCode != "ABCD-WXYZ" {
		t.Fatalf("unexpected: %+v", result)
	}
}

func TestKiroOAuthService_PollBuilderIDLogin_Pending(t *testing.T) {
	api := &stubKiroAPI{
		pollBuilderFn: func(_ *kiro.SessionStore, _ string) (*kiro.BuilderIDPollResult, error) {
			return &kiro.BuilderIDPollResult{Status: kiro.BuilderIDPollPending}, nil
		},
	}
	svc := newKiroOAuthServiceWithAPI(api, nil)
	defer svc.Stop()
	r, err := svc.PollBuilderIDLogin(context.Background(), "bid-1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pending" || r.TokenInfo != nil {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestKiroOAuthService_PollBuilderIDLogin_Completed(t *testing.T) {
	api := &stubKiroAPI{
		pollBuilderFn: func(_ *kiro.SessionStore, _ string) (*kiro.BuilderIDPollResult, error) {
			return &kiro.BuilderIDPollResult{
				Status: kiro.BuilderIDPollCompleted,
				Token: &kiro.TokenInfo{
					AccessToken:  "at",
					RefreshToken: "rt",
					AuthMethod:   kiro.AuthMethodBuilderID,
					ClientID:     "cid",
					ClientSecret: "cs",
					Region:       "us-east-1",
				},
			}, nil
		},
		profileFn: func(_, _ string) (*kiro.Profile, error) {
			return &kiro.Profile{Email: "u@e", UserID: "u-1"}, nil
		},
	}
	svc := newKiroOAuthServiceWithAPI(api, nil)
	defer svc.Stop()
	r, err := svc.PollBuilderIDLogin(context.Background(), "bid-1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "completed" || r.TokenInfo == nil {
		t.Fatalf("unexpected: %+v", r)
	}
	if r.TokenInfo.ClientID != "cid" || r.TokenInfo.Email != "u@e" {
		t.Fatalf("token info: %+v", r.TokenInfo)
	}
}

func TestKiroOAuthService_BuildAccountCredentials_Social(t *testing.T) {
	svc := newKiroOAuthServiceWithAPI(&stubKiroAPI{}, nil)
	defer svc.Stop()
	creds := svc.BuildAccountCredentials(&KiroTokenInfo{
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresAt:    1700000000,
		AuthMethod:   "social",
	})
	if creds["access_token"] != "at" ||
		creds["refresh_token"] != "rt" ||
		creds["expires_at"].(int64) != 1700000000 ||
		creds["auth_method"] != "social" {
		t.Fatalf("unexpected creds: %+v", creds)
	}
	if _, ok := creds["client_id"]; ok {
		t.Fatal("social should not persist client_id")
	}
}

func TestKiroOAuthService_BuildAccountCredentials_IdC(t *testing.T) {
	svc := newKiroOAuthServiceWithAPI(&stubKiroAPI{}, nil)
	defer svc.Stop()
	creds := svc.BuildAccountCredentials(&KiroTokenInfo{
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresAt:    1700000000,
		AuthMethod:   "idc",
		ClientID:     "cid",
		ClientSecret: "cs",
		Region:       "us-west-2",
		StartURL:     "https://start.aws",
	})
	if creds["client_id"] != "cid" || creds["client_secret"] != "cs" ||
		creds["region"] != "us-west-2" || creds["start_url"] != "https://start.aws" {
		t.Fatalf("unexpected idc creds: %+v", creds)
	}
}

func TestKiroOAuthService_BuildAccountCredentials_Nil(t *testing.T) {
	svc := newKiroOAuthServiceWithAPI(&stubKiroAPI{}, nil)
	defer svc.Stop()
	creds := svc.BuildAccountCredentials(nil)
	if len(creds) != 0 {
		t.Fatalf("expected empty map, got %+v", creds)
	}
}
