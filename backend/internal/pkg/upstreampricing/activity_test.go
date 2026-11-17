package upstreampricing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestActivityWindowDayClamp(t *testing.T) {
	now := time.Date(2026, 11, 17, 0, 3, 0, 0, stationTimezone)
	day := time.Date(2026, 11, 17, 0, 0, 0, 0, stationTimezone)
	for _, last := range []*time.Time{nil, ptrTime(day.Add(-time.Hour)), ptrTime(now.Add(time.Hour))} {
		start, end := activityWindow(now, last, true)
		require.Equal(t, day, start)
		require.Equal(t, now, end)
	}
	last := now.Add(-time.Minute)
	start, _ := activityWindow(now, &last, true)
	require.Equal(t, last, start)
	start, _ = activityWindow(now, &last, false)
	require.Equal(t, day, start)
}
func ptrTime(v time.Time) *time.Time { return &v }

func TestActivityWalletValidation(t *testing.T) {
	for _, body := range []string{`{"data":{"balance":0}}`, `{"data":{"balance":-2.125}}`, `{"data":{"balance":"12.340"}}`,
		`{"data":{}}`, `{"data":{"balance":"NaN"}}`, `{"data":{"balance":"1e99999999"}}`, `{"data":{"balance":"1/2"}}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/auth/me", r.URL.Path)
				require.Equal(t, "Asia/Shanghai", r.URL.Query().Get("timezone"))
				require.Equal(t, "1", r.Header.Get("X-User-UI-Request"))
				require.Equal(t, "Bearer synthetic-panel", r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			value, err := NewRemote("").wallet(context.Background(), Config{Provider: "sub2api", BaseURL: server.URL, Token: "synthetic-panel"})
			if body == `{"data":{"balance":0}}` {
				require.NoError(t, err)
				require.Equal(t, "0", value)
			} else if body == `{"data":{"balance":-2.125}}` {
				require.NoError(t, err)
				require.Equal(t, "-2.125", value)
			} else if body == `{"data":{"balance":"12.340"}}` {
				require.NoError(t, err)
				require.Equal(t, "12.340", value)
			} else {
				require.Error(t, err)
				require.Empty(t, value)
				require.NotContains(t, err.Error(), "synthetic-panel")
			}
		})
	}
}

func TestActivityIdentityIgnoresPriceRevision(t *testing.T) {
	src := &Source{Revision: 1, AccountFingerprint: "fp", Credential: "encrypted", Config: Config{Provider: "sub2api", BaseURL: "https://station.example", Group: "g", GroupID: "1"}}
	first := activityIdentity(src)
	src.Revision++
	require.Equal(t, first, activityIdentity(src))
	src.Config.Group = "other"
	require.NotEqual(t, first, activityIdentity(src))
	src.Config.Group = "g"
	src.Credential = "rotated"
	require.NotEqual(t, first, activityIdentity(src))
}
