package repository

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestDevinCredentialBodyCannotFollowHTTPRedirect(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var forwarded atomic.Int64
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				forwarded.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(target.Close)
			redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != "synthetic-protobuf-session-token" || r.Method != http.MethodPost {
					t.Error("initial request did not carry the expected synthetic credential body")
				}
				http.Redirect(w, r, target.URL, status)
			}))
			t.Cleanup(redirector.Close)
			req, err := http.NewRequestWithContext(service.WithHTTPUpstreamRedirectsDisabled(t.Context()), http.MethodPost, redirector.URL, bytes.NewReader([]byte("synthetic-protobuf-session-token")))
			require.NoError(t, err)
			require.NotNil(t, req.GetBody, "must exercise a body that Go could replay on 307/308")
			req.Header.Set("Content-Type", "application/proto")
			resp, err := NewHTTPUpstream(nil).Do(req, "", 71, 1)
			require.NoError(t, err)
			require.Equal(t, status, resp.StatusCode)
			require.NoError(t, resp.Body.Close())
			require.Zero(t, forwarded.Load(), "Devin credentials must not reach the redirected host")
		})
	}
}
