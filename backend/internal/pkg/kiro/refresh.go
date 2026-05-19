package kiro

import "fmt"

// socialRefresher is a package-level seam so tests can inject a fake
// without standing up an httptest.Server for every dispatch test.
var socialRefresher = RefreshSocial

func withSocialRefresher(fn func(rt, proxy string) (*TokenInfo, error), body func()) {
	prev := socialRefresher
	socialRefresher = fn
	defer func() { socialRefresher = prev }()
	body()
}

// RefreshToken refreshes the access token for an account according to its
// auth_method. Phase 2 only implements Social; Phase 3 extends this switch
// with IdC and Builder ID via the existing oidc.{region}.amazonaws.com/token
// endpoint.
func RefreshToken(a *RefreshableAccount) (*TokenInfo, error) {
	if a == nil {
		return nil, fmt.Errorf("kiro refresh: nil account")
	}
	switch a.AuthMethod {
	case AuthMethodSocial:
		return socialRefresher(a.RefreshToken, a.ProxyURL)
	case AuthMethodIdC, AuthMethodBuilderID:
		return nil, fmt.Errorf("kiro refresh: %q not implemented yet (Phase 3)", a.AuthMethod)
	default:
		return nil, fmt.Errorf("kiro refresh: unknown auth method %q", a.AuthMethod)
	}
}
