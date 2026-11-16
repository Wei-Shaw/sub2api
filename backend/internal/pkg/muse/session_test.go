package muse

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func cookieFixture() map[string]any {
	return map[string]any{"cookies": map[string]any{"hatch_sess": "fixture-sess", "hatch_gw": "fixture-gw", "hatch_vml": "fixture-vml", "hatch_native_auth_device": "fixture-device"}}
}

func TestMuseSessionImportReferenceExportsAndHttpOnlyCookies(t *testing.T) {
	for _, key := range []string{"expires", "cookies_exp", "cookie_expires"} {
		doc := cookieFixture()
		doc[key] = map[string]any{"hatch_vml": time.Now().Add(time.Hour).Unix()}
		parsed, err := ParseCookieSession(doc)
		if err != nil || len(parsed.Cookies) != 4 || parsed.Expires["hatch_vml"] == 0 {
			t.Fatalf("%s import failed: %v", key, err)
		}
	}
	var cookies []any
	for _, name := range SessionCookieNames {
		cookies = append(cookies, map[string]any{"name": name, "value": "fixture", "domain": ".muse.ai", "path": "/", "httpOnly": true, "secure": true, "expires": -1})
	}
	doc := map[string]any{"cookies": cookies, "origins": []any{}}
	parsed, err := ParseCookieSession(doc)
	if err != nil || len(parsed.Expires) != 0 {
		t.Fatalf("session-cookie expiry must stay unknown: %v", err)
	}
	first, ok := cookies[0].(map[string]any)
	if !ok {
		t.Fatal("fixture cookie type")
	}
	first["domain"] = "evil-muse.ai"
	if _, err = ParseCookieSession(doc); !errors.Is(err, ErrSessionCredentials) {
		t.Fatalf("unrelated domain accepted: %v", err)
	}
	doc = cookieFixture()
	values, ok := doc["cookies"].(map[string]any)
	if !ok {
		t.Fatal("fixture cookie type")
	}
	values["hatch_sess"] = "fixture\r\nInjected: value"
	if _, err = ParseCookieSession(doc); !errors.Is(err, ErrSessionCredentials) {
		t.Fatal("header injection accepted")
	}
	doc = cookieFixture()
	values, ok = doc["cookies"].(map[string]any)
	if !ok {
		t.Fatal("fixture cookie type")
	}
	delete(values, "hatch_native_auth_device")
	if _, err = ParseCookieSession(doc); !errors.Is(err, ErrSessionCredentials) {
		t.Fatal("incomplete auth accepted")
	}
}

func TestMuseSessionRefreshUsesAppCookiesAndReturnedExpiryOnly(t *testing.T) {
	doc := cookieFixture()
	doc["opaque_binding"] = map[string]any{"reference": "fixture-local-handle"}
	expires := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	calls := 0
	client := &SessionClient{Do: func(req *http.Request, s Session) (*http.Response, error) {
		calls++
		if req.Method != "GET" || req.URL.String() != SessionEndpoint {
			t.Fatal("unexpected endpoint or VM wake")
		}
		if req.Header.Get("Authorization") != "" || req.Header.Get("Origin") != "https://muse.ai" {
			t.Fatal("wrong app authentication")
		}
		for _, name := range SessionCookieNames {
			if cookie, err := req.Cookie(name); err != nil || cookie.Value == "" {
				t.Fatal("missing HttpOnly session cookie")
			}
		}
		headers := http.Header{}
		headers.Add("Set-Cookie", (&http.Cookie{Name: "hatch_vml", Value: "rotated-fixture", Path: "/", Domain: ".muse.ai", HttpOnly: true, Secure: true, Expires: expires}).String())
		return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(`{"status":"assigned","vm_id":"fixture-vm","vm_state":"RUNNING"}`))}, nil
	}}
	refresh, err := client.Refresh(context.Background(), Session{Document: doc})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !refresh.Check.Authenticated || refresh.Check.ExpiresAt == nil || !refresh.Check.ExpiresAt.Equal(expires) {
		t.Fatal("incorrect session observation")
	}
	parsed, err := ParseCookieSession(refresh.Document)
	if err != nil || parsed.Cookies["hatch_vml"] != "rotated-fixture" {
		t.Fatal("rotated cookie missing")
	}
	originalCookies, ok := doc["cookies"].(map[string]any)
	if !ok {
		t.Fatal("input cookies have an unexpected type")
	}
	if originalCookies["hatch_vml"] != "fixture-vml" {
		t.Fatal("input credential map was mutated")
	}
	if refresh.Document["opaque_binding"] == nil {
		t.Fatal("opaque app binding lost")
	}
}

func TestMuseSessionRefreshRejectsLoginRedirectAndMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{{401, `{}`, ErrSessionExpired}, {403, `{}`, ErrSessionResponse}, {302, `{}`, ErrSessionResponse}, {500, `{}`, ErrSessionResponse}, {200, `<html>login</html>`, ErrSessionResponse}, {200, `{"status":"signed_out"}`, ErrSessionResponse}, {200, `{"status":"assigned"}`, ErrSessionResponse}} {
		client := &SessionClient{Do: func(*http.Request, Session) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
		}}
		if _, err := client.Refresh(context.Background(), Session{Document: cookieFixture()}); !errors.Is(err, tc.want) {
			t.Fatalf("status %d: %v", tc.status, err)
		}
	}
	client := &SessionClient{Do: func(*http.Request, Session) (*http.Response, error) {
		return nil, errors.New("synthetic-secret-must-not-escape")
	}}
	_, err := client.Refresh(context.Background(), Session{Document: cookieFixture()})
	if err == nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal("transport error leaked credential-bearing context")
	}
}

func TestMuseSessionRefreshDoesNotInventExpiryAndHonorsCookieDeletion(t *testing.T) {
	for _, deleteCookie := range []bool{false, true} {
		client := &SessionClient{Do: func(*http.Request, Session) (*http.Response, error) {
			headers := http.Header{}
			if deleteCookie {
				headers.Set("Set-Cookie", "hatch_sess=; Path=/; Max-Age=0")
			}
			return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(`{"status":"assigned","vm_id":"fixture-vm"}`))}, nil
		}}
		refresh, err := client.Refresh(context.Background(), Session{Document: cookieFixture()})
		if deleteCookie {
			if !errors.Is(err, ErrSessionExpired) {
				t.Fatalf("deleted cookie: %v", err)
			}
		} else if err != nil || refresh.Check.ExpiresAt != nil {
			t.Fatal("unknown expiry was fabricated")
		}
	}
}

func TestMuseSessionRefreshMaxAgeOverridesExpires(t *testing.T) {
	client := &SessionClient{Do: func(*http.Request, Session) (*http.Response, error) {
		headers := http.Header{}
		headers.Set("Set-Cookie", "hatch_vml=rotated-fixture; Path=/; Max-Age=3600; Expires=Wed, 01 Jan 2020 00:00:00 GMT")
		return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(`{"status":"assigned","vm_id":"fixture-vm"}`))}, nil
	}}
	started := time.Now()
	refresh, err := client.Refresh(context.Background(), Session{Document: cookieFixture()})
	if err != nil {
		t.Fatal(err)
	}
	if refresh.Check.ExpiresAt == nil || refresh.Check.ExpiresAt.Before(started.Add(59*time.Minute)) || refresh.Check.ExpiresAt.After(time.Now().Add(time.Hour)) {
		t.Fatal("Max-Age did not take precedence over Expires")
	}
}

func TestMuseSessionBootstrapObtainsMissingVmLeaseCookie(t *testing.T) {
	doc := cookieFixture()
	values, ok := doc["cookies"].(map[string]any)
	if !ok {
		t.Fatal("fixture cookie type")
	}
	delete(values, "hatch_vml")
	client := &SessionClient{Do: func(req *http.Request, _ Session) (*http.Response, error) {
		if _, err := req.Cookie("hatch_vml"); err == nil {
			t.Fatal("bootstrap must not send an empty VM lease")
		}
		headers := http.Header{"Set-Cookie": []string{"hatch_vml=lease-fixture; Path=/; Secure; HttpOnly; Max-Age=7200"}}
		return &http.Response{StatusCode: 200, Header: headers, Body: io.NopCloser(strings.NewReader(`{"status":"assigned","vm_id":"fixture-vm","vm_state":"RUNNING"}`))}, nil
	}}
	refresh, err := client.Refresh(context.Background(), Session{Document: doc})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCookieSession(refresh.Document)
	if err != nil || parsed.Cookies["hatch_vml"] != "lease-fixture" || !refresh.Check.Authenticated {
		t.Fatal("bootstrap did not preserve the returned VM lease")
	}
}

func TestMuseSessionCookieVariantCleanupKeepsReplacementLease(t *testing.T) {
	for _, deletionFirst := range []bool{true, false} {
		client := &SessionClient{Do: func(*http.Request, Session) (*http.Response, error) {
			cleanup := "hatch_vml=; Path=/; Max-Age=0"
			replacement := "hatch_vml=lease-fixture; Domain=.muse.ai; Path=/; HttpOnly; Secure; Max-Age=172800"
			cookies := []string{cleanup, replacement}
			if !deletionFirst {
				cookies = []string{replacement, cleanup}
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": cookies}, Body: io.NopCloser(strings.NewReader(`{"status":"assigned","vm_id":"fixture-vm","vm_state":"RUNNING"}`))}, nil
		}}
		refresh, err := client.Refresh(context.Background(), Session{Document: cookieFixture()})
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := ParseCookieSession(refresh.Document)
		if err != nil || parsed.Cookies["hatch_vml"] != "lease-fixture" {
			t.Fatal("host-only cookie cleanup discarded the valid domain lease")
		}
	}
}
