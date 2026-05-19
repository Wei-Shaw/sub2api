package kiro

import (
	"errors"
	"strings"
	"testing"
)

func TestRefreshToken_DispatchesSocial(t *testing.T) {
	called := false
	stub := func(rt, _ string) (*TokenInfo, error) {
		called = true
		if rt != "rt-1" {
			t.Fatalf("rt=%q", rt)
		}
		return &TokenInfo{AccessToken: "at", AuthMethod: AuthMethodSocial}, nil
	}
	withSocialRefresher(stub, func() {
		info, err := RefreshToken(&RefreshableAccount{
			AuthMethod:   AuthMethodSocial,
			RefreshToken: "rt-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("social refresher not called")
		}
		if info.AuthMethod != AuthMethodSocial {
			t.Fatalf("authMethod=%v", info.AuthMethod)
		}
	})
}

func TestRefreshToken_PropagatesSocialError(t *testing.T) {
	stub := func(_, _ string) (*TokenInfo, error) {
		return nil, errors.New("upstream 400")
	}
	withSocialRefresher(stub, func() {
		_, err := RefreshToken(&RefreshableAccount{AuthMethod: AuthMethodSocial})
		if err == nil || !strings.Contains(err.Error(), "upstream 400") {
			t.Fatalf("expected propagated error, got %v", err)
		}
	})
}

func TestRefreshToken_DispatchesIdC(t *testing.T) {
	called := false
	stub := func(a *RefreshableAccount) (*TokenInfo, error) {
		called = true
		if a.AuthMethod != AuthMethodIdC {
			t.Fatalf("dispatch got method=%v", a.AuthMethod)
		}
		return &TokenInfo{AccessToken: "at", AuthMethod: AuthMethodIdC}, nil
	}
	withOIDCRefresher(stub, func() {
		info, err := RefreshToken(&RefreshableAccount{
			AuthMethod:   AuthMethodIdC,
			RefreshToken: "rt",
			ClientID:     "cid",
			ClientSecret: "cs",
			Region:       "us-east-1",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !called {
			t.Fatal("oidc refresher not called")
		}
		if info.AuthMethod != AuthMethodIdC {
			t.Fatalf("authMethod=%v", info.AuthMethod)
		}
	})
}

func TestRefreshToken_DispatchesBuilderID(t *testing.T) {
	stub := func(a *RefreshableAccount) (*TokenInfo, error) {
		if a.AuthMethod != AuthMethodBuilderID {
			t.Fatalf("dispatch got method=%v", a.AuthMethod)
		}
		return &TokenInfo{AccessToken: "at", AuthMethod: AuthMethodBuilderID}, nil
	}
	withOIDCRefresher(stub, func() {
		_, err := RefreshToken(&RefreshableAccount{
			AuthMethod:   AuthMethodBuilderID,
			RefreshToken: "rt",
			ClientID:     "cid",
			ClientSecret: "cs",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestRefreshOIDC_MissingClientCreds(t *testing.T) {
	_, err := RefreshOIDC(&RefreshableAccount{AuthMethod: AuthMethodIdC, RefreshToken: "rt"})
	if err == nil || !strings.Contains(err.Error(), "client_id/client_secret") {
		t.Fatalf("expected missing-creds error, got %v", err)
	}
}

func TestRefreshToken_NilAccount(t *testing.T) {
	_, err := RefreshToken(nil)
	if err == nil {
		t.Fatal("expected nil-account error")
	}
}

func TestRefreshToken_UnknownMethod(t *testing.T) {
	_, err := RefreshToken(&RefreshableAccount{AuthMethod: "bogus"})
	if err == nil || !strings.Contains(err.Error(), "unknown auth method") {
		t.Fatalf("expected unknown-method error, got %v", err)
	}
}
