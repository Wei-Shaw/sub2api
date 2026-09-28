package repository

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestHTTPUpstreamClientGeoPrivacy(t *testing.T) {
	const environment = "<environment_context>\n<cwd>/work/Asia/Shanghai</cwd>\n<timezone>Asia/Shanghai</timezone>\n</environment_context>"
	for _, tls := range []bool{false, true} {
		t.Run(fmt.Sprintf("tls=%v", tls), func(t *testing.T) {
			type received struct {
				body   []byte
				header http.Header
				length int64
				err    error
			}
			seen := make(chan received, 1)
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				seen <- received{body, r.Header.Clone(), r.ContentLength, err}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"ok":true}`)
			}))
			if tls {
				srv.StartTLS()
			} else {
				srv.Start()
			}
			defer srv.Close()
			cfg := &config.Config{Gateway: config.GatewayConfig{RedactClientGeoMetadata: true}}
			upstream, ok := NewHTTPUpstream(cfg).(*httpUpstreamService)
			require.True(t, ok)
			profile := &tlsfingerprint.Profile{Name: "privacy-test"}
			if tls {
				entry, err := upstream.getClientEntryWithTLS("", 1, 1, profile, service.HTTPUpstreamProfileDefault, false, true)
				require.NoError(t, err)
				entry.client = srv.Client()
			}
			do := func(req *http.Request) (*http.Response, error) {
				if tls {
					return upstream.DoWithTLS(req, "", 1, 1, profile)
				}
				return upstream.Do(req, "", 1, 1)
			}
			raw, err := json.Marshal(map[string]any{"model": "gpt-6-sol", "input": environment, "instructions": "Task: use Asia/Shanghai", "metadata": map[string]any{"timezone": "Asia/Shanghai", "user_id": "kept"}})
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/responses", strings.NewReader(string(raw)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Forwarded-For", "192.0.2.10")
			req.Header.Set("Accept-Language", "zh-CN")
			resp, err := do(req)
			require.NoError(t, err)
			_, err = io.Copy(io.Discard, resp.Body)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			got := <-seen
			require.NoError(t, got.err)
			require.Equal(t, int64(len(got.body)), got.length)
			require.NotContains(t, gjson.GetBytes(got.body, "input").String(), "<timezone>")
			require.Contains(t, gjson.GetBytes(got.body, "input").String(), "/work/Asia/Shanghai")
			require.Equal(t, "Task: use Asia/Shanghai", gjson.GetBytes(got.body, "instructions").String())
			require.False(t, gjson.GetBytes(got.body, "metadata.timezone").Exists())
			require.Equal(t, "kept", gjson.GetBytes(got.body, "metadata.user_id").String())
			require.Empty(t, got.header.Get("X-Forwarded-For"))
			require.Empty(t, got.header.Get("Accept-Language"))
			requireNoUpstreamInFlight(t, upstream)
		})
	}
}
