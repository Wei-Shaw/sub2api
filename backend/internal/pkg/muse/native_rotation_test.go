//go:build unit

package muse

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/flynn/noise"
	"github.com/stretchr/testify/require"
)

// This is a synthetic server confined to memory; it contacts no remote endpoint.
type rotationSocket struct {
	hs            *noise.HandshakeState
	static        []byte
	writes        int
	reply         []byte
	receive, send *noise.CipherState
}

func newRotationSocket(t *testing.T) *rotationSocket {
	suite := noise.NewCipherSuite(noise.DH25519, noise.CipherAESGCM, noise.HashSHA256)
	key, err := suite.GenerateKeypair(rand.Reader)
	require.NoError(t, err)
	hs, err := noise.NewHandshakeState(noise.Config{CipherSuite: suite, Pattern: noise.HandshakeXX, StaticKeypair: key})
	require.NoError(t, err)
	return &rotationSocket{hs: hs, static: key.Public}
}
func (s *rotationSocket) Write(ctx context.Context, body []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.writes++
	if s.writes == 1 {
		first, _, _, err := s.hs.ReadMessage(nil, body)
		if err != nil {
			return err
		}
		statement := noiseBytes(noiseBytes(nil, 1, s.static), 2, first[2:])
		s.reply, _, _, err = s.hs.WriteMessage(nil, noiseBytes(nil, 2, statement))
		return err
	}
	if s.writes == 2 {
		_, s.receive, s.send, _ = s.hs.ReadMessage(nil, body)
		return nil
	}
	if _, err := s.receive.Decrypt(nil, nil, body); err != nil {
		return err
	}
	response := noiseUint(nil, 1, 200)
	response = noiseBytes(response, 3, []byte(`{"available_models":[{"id":"auto/auto"}]}`))
	response = noiseUint(response, 4, 1)
	envelope := noiseBytes(noiseUint(nil, 1, 1), 3, response)
	chunk := noiseBytes(noiseUint(noiseUint(noiseUint(nil, 1, 17), 2, 0), 3, 1), 4, noiseBytes(nil, 1, envelope))
	var err error
	s.reply, err = s.send.Encrypt(nil, nil, chunk)
	return err
}
func (s *rotationSocket) Read(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.reply, nil
}

func TestNativeVerifyPersistsGatewayRotation(t *testing.T) {
	session := Session{Document: map[string]any{"cookies": map[string]any{"hatch_sess": "synthetic-session", "hatch_gw": "old-gateway", "hatch_native_auth_device": "synthetic-device"}}}
	current := "old-gateway"
	calls := 0
	provider := NewNativeProvider(func(req *http.Request, _ Session) (*http.Response, error) {
		calls++
		gateway, err := req.Cookie("hatch_gw")
		require.NoError(t, err)
		if gateway.Value != current {
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		}
		h := http.Header{}
		body := ""
		switch req.URL.String() {
		case SessionEndpoint:
			target := testGatewayTarget()
			body = `{"status":"assigned","vm_id":"` + target.VMID + `","vm_name":"` + target.Name + `","endpoint_url":"` + target.URL + `","vm_state":"RUNNING","vms":[{"vm_id":"` + target.VMID + `","vm_type":"hatch_vm"}]}`
		case authCheckEndpoint:
			body = `{"outcome":"validated","viewer_id":"synthetic-viewer","session_binding_id":"synthetic-binding"}`
		case GatewayTokenEndpoint:
			current = "rotated-gateway"
			h.Set("Set-Cookie", "hatch_gw=rotated-gateway; Path=/; Secure; HttpOnly; Max-Age=7200")
			body = `{"token":"synthetic-token"}`
		default:
			t.Fatal("unexpected endpoint")
		}
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	provider.dial = func(context.Context, Session, string) (NoiseSocket, func(), error) {
		return newRotationSocket(t), func() {}, nil
	}
	session.WithCredentials = func(ctx context.Context, use func(Session) error) error {
		latest := session
		latest.WithCredentials = nil
		latest.SaveCredentials = func(_ context.Context, doc map[string]any) error { session.Document = doc; return nil }
		return use(latest)
	}
	observation, err := provider.Verify(context.Background(), session)
	require.NoError(t, err)
	require.True(t, observation.InferenceAllowed)
	require.Equal(t, 3, calls)
	cookies, err := ParseCookieSession(session.Document)
	require.NoError(t, err)
	require.Equal(t, "rotated-gateway", cookies.Cookies["hatch_gw"])
	_, err = provider.Verify(context.Background(), session)
	require.NoError(t, err, "a second bootstrap must use the persisted rotation")
	require.Equal(t, 6, calls)
}

func TestRotationSurvivesInvalidResponseAndHandshakeFailure(t *testing.T) {
	for _, failure := range []string{"session", "auth", "token", "handshake"} {
		t.Run(failure, func(t *testing.T) {
			session := Session{Document: map[string]any{"cookies": map[string]any{"hatch_sess": "synthetic-session", "hatch_gw": "old-gateway", "hatch_native_auth_device": "synthetic-device"}}}
			saved := ""
			session.SaveCredentials = func(_ context.Context, doc map[string]any) error {
				cookies, err := ParseCookieSession(doc)
				require.NoError(t, err)
				saved = cookies.Cookies["hatch_gw"]
				return nil
			}
			provider := NewNativeProvider(func(req *http.Request, _ Session) (*http.Response, error) {
				header := http.Header{}
				body := "{}"
				switch req.URL.String() {
				case SessionEndpoint:
					target := testGatewayTarget()
					body = `{"status":"assigned","vm_id":"` + target.VMID + `","vm_name":"` + target.Name + `","endpoint_url":"` + target.URL + `","vm_state":"RUNNING","vms":[{"vm_id":"` + target.VMID + `","vm_type":"hatch_vm"}]}`
					if failure == "session" {
						header.Set("Set-Cookie", "hatch_gw=rotated-session; Path=/")
						body = "invalid"
					}
				case authCheckEndpoint:
					body = `{"outcome":"validated","viewer_id":"viewer","session_binding_id":"binding"}`
					if failure == "auth" {
						header.Set("Set-Cookie", "hatch_gw=rotated-auth; Path=/")
						body = "invalid"
					}
				case GatewayTokenEndpoint:
					header.Set("Set-Cookie", "hatch_gw=rotated-"+failure+"; Path=/")
					body = `{"token":"synthetic-token"}`
					if failure == "token" {
						body = "invalid"
					}
				}
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			provider.dial = func(context.Context, Session, string) (NoiseSocket, func(), error) { return nil, nil, ErrNoiseProtocol }
			_, err := provider.Verify(context.Background(), session)
			require.Error(t, err)
			require.Equal(t, "rotated-"+failure, saved)
		})
	}
}

func TestNativeBootstrapBoundsCredentialLockAndNetworkBudget(t *testing.T) {
	provider := NewNativeProvider(func(*http.Request, Session) (*http.Response, error) {
		t.Fatal("unexpected network call")
		return nil, nil
	})
	session := Session{WithCredentials: func(ctx context.Context, _ func(Session) error) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok, "Execute and recovery callers must have a bootstrap deadline")
		require.LessOrEqual(t, time.Until(deadline), 30*time.Second)
		return ErrSessionResponse
	}}
	_, err := provider.bootstrap(context.Background(), session)
	require.ErrorIs(t, err, ErrSessionResponse)
}
