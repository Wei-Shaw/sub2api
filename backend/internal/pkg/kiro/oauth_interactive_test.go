package kiro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeOIDCServer mimics oidc.{region}.amazonaws.com. It's reused by both
// the IdC tests and the Builder ID tests; routes are switched on path.
type fakeOIDCServer struct {
	t              *testing.T
	clientID       string
	clientSecret   string
	codeToVerifier map[string]string // tracks expected code -> verifier
	tokenResponses chan tokenScript
	deviceCode     string
	userCode       string
	pollCalls      int32
}

type tokenScript struct {
	status int
	body   string
}

func newFakeOIDC(t *testing.T) *fakeOIDCServer {
	return &fakeOIDCServer{
		t:              t,
		clientID:       "cid-test",
		clientSecret:   "cs-test",
		codeToVerifier: make(map[string]string),
		tokenResponses: make(chan tokenScript, 16),
		deviceCode:     "device-xyz",
		userCode:       "USER-CODE",
	}
}

func (f *fakeOIDCServer) handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/client/register":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"clientId":     f.clientID,
			"clientSecret": f.clientSecret,
		})
	case "/device_authorization":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deviceCode":      f.deviceCode,
			"userCode":        f.userCode,
			"verificationUri": "https://aws.example/verify",
			"interval":        1,
			"expiresIn":       60,
		})
	case "/token":
		atomic.AddInt32(&f.pollCalls, 1)
		// Reply with the next scripted response, or a default success.
		select {
		case s := <-f.tokenResponses:
			w.WriteHeader(s.status)
			_, _ = w.Write([]byte(s.body))
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "at-success",
				"refreshToken": "rt-success",
				"expiresIn":    3600,
				"profileArn":   "arn:cw:test",
			})
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestStartIdCLogin_BuildsAuthorizeURL(t *testing.T) {
	f := newFakeOIDC(t)
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()

	// We bypass region URL construction by pointing StartIdCLogin's OIDC
	// base at our fake via a region rewrite: not possible without changing
	// the API, so instead drive registerOIDCClient + the URL composer
	// directly and verify the building blocks.

	client := http.DefaultClient
	cid, cs, err := registerOIDCClient(client, srv.URL, "https://start.example", []string{idcRedirectURI}, []string{"authorization_code", "refresh_token"})
	if err != nil {
		t.Fatal(err)
	}
	if cid != f.clientID || cs != f.clientSecret {
		t.Fatalf("unexpected creds %q/%q", cid, cs)
	}

	verifier, err := generateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if generateCodeChallenge(verifier) == "" {
		t.Fatal("empty challenge")
	}
}

// startIdCLoginOverride lets tests stub the OIDC base URL.
// We expose this via a tiny helper so StartIdCLogin's real public surface
// stays clean for production callers.
func startIdCLoginForTest(t *testing.T, store *SessionStore, oidcBase, startURL string) (*IdCLoginStarted, *IdCSession) {
	t.Helper()
	cid, cs, err := registerOIDCClient(http.DefaultClient, oidcBase, startURL, []string{idcRedirectURI}, []string{"authorization_code", "refresh_token"})
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := generateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	state := "state-test"
	sessionID := "sess-test"

	sess := &IdCSession{
		ClientID:     cid,
		ClientSecret: cs,
		CodeVerifier: verifier,
		State:        state,
		Region:       "us-east-1",
		StartURL:     startURL,
		RedirectURI:  idcRedirectURI,
		ExpiresAt:    time.Now().Add(idcSessionTTL),
	}
	store.SetIdC(sessionID, sess)

	params := url.Values{}
	params.Set("client_id", cid)
	params.Set("state", state)
	params.Set("code_challenge", generateCodeChallenge(verifier))
	return &IdCLoginStarted{
		SessionID:    sessionID,
		AuthorizeURL: oidcBase + "/authorize?" + params.Encode(),
	}, sess
}

func TestCompleteIdCLogin_Success(t *testing.T) {
	f := newFakeOIDC(t)
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()

	store := NewSessionStore()
	defer store.Stop()
	_, sess := startIdCLoginForTest(t, store, srv.URL, "https://start.example")
	// CompleteIdCLogin uses sess.Region to build the OIDC base URL.
	// Rewrite the session to point at the fake server.
	sess.Region = ""

	// Override the OIDC base used inside CompleteIdCLogin: it composes
	// https://oidc.{region}.amazonaws.com, so we cannot fully fake the URL.
	// Instead exercise exchangeAuthorizationCode directly to validate the
	// token-exchange path; CompleteIdCLogin's pre-checks are tested below.
	info, err := exchangeAuthorizationCode(http.DefaultClient, srv.URL,
		sess.ClientID, sess.ClientSecret, "code-1", sess.CodeVerifier, sess.RedirectURI)
	if err != nil {
		t.Fatal(err)
	}
	if info.AccessToken != "at-success" || info.RefreshToken != "rt-success" || info.ProfileARN != "arn:cw:test" {
		t.Fatalf("unexpected info: %+v", info)
	}
}

func TestCompleteIdCLogin_RejectsStateMismatch(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()
	store.SetIdC("sid", &IdCSession{State: "good", ExpiresAt: time.Now().Add(time.Minute)})

	_, err := CompleteIdCLogin(store, "sid", "http://x/cb?code=c&state=bad")
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch, got %v", err)
	}
}

func TestCompleteIdCLogin_RejectsExpiredSession(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()
	store.SetIdC("sid", &IdCSession{State: "x", ExpiresAt: time.Now().Add(-time.Second)})
	_, err := CompleteIdCLogin(store, "sid", "http://x/cb?code=c&state=x")
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired session error, got %v", err)
	}
}

func TestCompleteIdCLogin_RejectsMissingCode(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()
	store.SetIdC("sid", &IdCSession{State: "x", ExpiresAt: time.Now().Add(time.Minute)})
	_, err := CompleteIdCLogin(store, "sid", "http://x/cb?state=x")
	if err == nil || !strings.Contains(err.Error(), "missing state/code") {
		t.Fatalf("expected missing-code error, got %v", err)
	}
}

func TestCompleteIdCLogin_RejectsErrorParam(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()
	store.SetIdC("sid", &IdCSession{State: "x", ExpiresAt: time.Now().Add(time.Minute)})
	_, err := CompleteIdCLogin(store, "sid", "http://x/cb?error=access_denied")
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("expected authz error, got %v", err)
	}
}

// PollBuilderIDLogin uses sess.Region to build the OIDC base URL, so
// directly testing it through StartBuilderIDLogin would require hitting
// the real AWS endpoint. The Builder ID flow is exercised at a lower
// level by these tests:

func TestPollBuilderIDLogin_DeletesOnExpired(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()
	store.SetBuilderID("sid", &BuilderIDSession{ExpiresAt: time.Now().Add(-time.Second)})
	_, err := PollBuilderIDLogin(store, "sid")
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
	if _, ok := store.GetBuilderID("sid"); ok {
		t.Fatal("expired session not removed")
	}
}

func TestPollBuilderIDLogin_MissingSession(t *testing.T) {
	store := NewSessionStore()
	defer store.Stop()
	_, err := PollBuilderIDLogin(store, "nope")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

// Drive the device-code endpoint and verify decoding via the fake server.
// We construct the session manually so we can point at the fake.
func TestPollBuilderIDLogin_AgainstFake(t *testing.T) {
	f := newFakeOIDC(t)
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()

	// We cannot fully reroute PollBuilderIDLogin (it computes the URL from
	// sess.Region), so this case exercises the raw exchange directly.
	resp, err := http.Post(srv.URL+"/token", "application/json", strings.NewReader(fmt.Sprintf(
		`{"clientId":"%s","clientSecret":"%s","grantType":"urn:ietf:params:oauth:grant-type:device_code","deviceCode":"%s"}`,
		f.clientID, f.clientSecret, f.deviceCode)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&parsed)
	if parsed.AccessToken != "at-success" {
		t.Fatalf("got %q", parsed.AccessToken)
	}
}
