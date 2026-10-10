package muse

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// applySessionCookies merges the complete response before honoring deletions.
// It is shared by bootstrap and gateway renewal; neither invents expiry.
func applySessionCookies(session Session, response *http.Response, cookies *CookieSession) (map[string]any, *time.Time, error) {
	now := time.Now()
	deleted := map[string]bool{}
	replaced := map[string]bool{}
	for _, cookie := range response.Cookies() {
		if !isSessionCookie(cookie.Name) || (cookie.Domain != "" && !sessionCookieDomain(cookie.Domain)) || (cookie.Path != "" && cookie.Path != "/") {
			continue
		}
		if cookie.Valid() != nil {
			return nil, nil, ErrSessionResponse
		}
		if cookie.MaxAge < 0 || cookie.Value == "" || (cookie.MaxAge == 0 && !cookie.Expires.IsZero() && !cookie.Expires.After(now)) {
			deleted[cookie.Name] = true
			continue
		}
		replaced[cookie.Name] = true
		cookies.Cookies[cookie.Name] = cookie.Value
		switch {
		case cookie.MaxAge > 0:
			if cookie.MaxAge > 10*365*24*60*60 {
				return nil, nil, ErrSessionResponse
			}
			cookies.Expires[cookie.Name] = now.Add(time.Duration(cookie.MaxAge) * time.Second).Unix()
		case !cookie.Expires.IsZero():
			cookies.Expires[cookie.Name] = cookie.Expires.Unix()
		default:
			delete(cookies.Expires, cookie.Name) // A session cookie has unknown expiry.
		}
	}
	for name := range deleted {
		// The app clears legacy host-only cookies while issuing a replacement
		// for .muse.ai. Evaluate the complete response before declaring expiry.
		if !replaced[name] {
			return nil, nil, ErrSessionExpired
		}
	}
	check := SessionCheck{}
	for _, name := range SessionCookieNames {
		if expiry, ok := cookies.Expires[name]; ok {
			at := time.Unix(expiry, 0)
			if !at.After(now) {
				return nil, nil, ErrSessionExpired
			}
			if check.ExpiresAt == nil || at.Before(*check.ExpiresAt) {
				check.ExpiresAt = &at
			}
		}
	}
	// Preserve opaque app session extensions without sharing mutable input maps.
	encoded, _ := json.Marshal(session.Document)
	document := map[string]any{}
	if json.Unmarshal(encoded, &document) != nil {
		return nil, nil, ErrSessionCredentials
	}
	for _, name := range SessionCookieNames {
		delete(document, name)
	}
	document["cookies"] = cookies.Cookies
	document["expires"] = cookies.Expires
	delete(document, "cookies_exp")
	delete(document, "cookie_expires")
	return document, check.ExpiresAt, nil
}

// Persist before interpreting the rest of the response: a rotated cookie can
// invalidate its predecessor even if the response body or next step fails.
func persistSessionCookies(ctx context.Context, session Session, response *http.Response, cookies *CookieSession) (map[string]any, *time.Time, error) {
	document, expires, err := applySessionCookies(session, response, cookies)
	if err != nil {
		return nil, nil, err
	}
	if session.SaveCredentials != nil {
		if err := session.SaveCredentials(ctx, document); err != nil {
			return nil, nil, err
		}
	}
	return document, expires, nil
}
