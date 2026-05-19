package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/kiro"
)

type stubKiroAPI struct {
	refreshFn  func(rt, proxy string) (*kiro.TokenInfo, error)
	profileFn  func(at, proxy string) (*kiro.Profile, error)
	refreshHit int
	profileHit int
}

func (s *stubKiroAPI) RefreshSocial(rt, proxy string) (*kiro.TokenInfo, error) {
	s.refreshHit++
	return s.refreshFn(rt, proxy)
}

func (s *stubKiroAPI) FetchProfile(at, proxy string) (*kiro.Profile, error) {
	s.profileHit++
	return s.profileFn(at, proxy)
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
	_, err := svc.ValidateSocialRefreshToken(context.Background(), "  ", nil)
	if err == nil {
		t.Fatal("expected error on empty refresh token")
	}
}

func TestKiroOAuthService_BuildAccountCredentials(t *testing.T) {
	svc := newKiroOAuthServiceWithAPI(&stubKiroAPI{}, nil)
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
}

func TestKiroOAuthService_BuildAccountCredentials_Nil(t *testing.T) {
	svc := newKiroOAuthServiceWithAPI(&stubKiroAPI{}, nil)
	creds := svc.BuildAccountCredentials(nil)
	if len(creds) != 0 {
		t.Fatalf("expected empty map, got %+v", creds)
	}
}
