package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGatewayService_AcceptEncodingOnWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, protocol := range []struct {
		name  string
		major int
	}{
		{name: "HTTP1", major: 1},
		{name: "HTTP2", major: 2},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			type receivedRequest struct {
				encodings []string
				proto     int
			}
			received := make(chan receivedRequest, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- receivedRequest{encodings: r.Header.Values("Accept-Encoding"), proto: r.ProtoMajor}
				w.WriteHeader(http.StatusNoContent)
			}))
			server.EnableHTTP2 = protocol.major == 2
			server.StartTLS()
			defer server.Close()
			client := server.Client()
			client.Timeout = 5 * time.Second
			upstreamURL, err := url.Parse(server.URL)
			require.NoError(t, err)

			for _, tc := range []struct {
				name     string
				encoding string
				want     string
			}{
				{name: "gzip", encoding: "gzip", want: "gzip"},
				{name: "multiple encodings", encoding: "gzip, deflate, br", want: "gzip, deflate, br"},
				{name: "identity", encoding: "identity", want: "identity"},
				{name: "omitted", want: "gzip"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
					if tc.encoding != "" {
						c.Request.Header.Set("Accept-Encoding", tc.encoding)
					}
					svc := &GatewayService{cfg: &config.Config{}}
					account := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
					req, _, err := svc.buildUpstreamRequest(context.Background(), c, account,
						[]byte(`{"model":"claude-sonnet-4-6","messages":[]}`), "test-token", "oauth", "claude-sonnet-4-6", false, false)
					require.NoError(t, err)
					req.URL.Scheme = upstreamURL.Scheme
					req.URL.Host = upstreamURL.Host
					req.Host = ""

					resp, err := client.Do(req)
					require.NoError(t, err)
					_, err = io.Copy(io.Discard, resp.Body)
					require.NoError(t, err)
					require.NoError(t, resp.Body.Close())
					got := <-received
					require.Equal(t, protocol.major, got.proto)
					require.Equal(t, []string{tc.want}, got.encodings)
				})
			}
		})
	}
}
