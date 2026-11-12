//go:build unit

package meta

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartDeviceFlowSendsClientIDForm(t *testing.T) {
	var gotForm map[string][]string
	var gotUA, gotAccept, gotContentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		gotUA = r.Header.Get("User-Agent")
		require.NoError(t, r.ParseForm())
		gotForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"device_code":"dev-1","user_code":"ABCD-EFGH","verification_uri":"https://auth.meta.com/device","verification_uri_complete":"https://auth.meta.com/device?user_code=ABCD-EFGH","expires_in":900,"interval":5}`))
	}))
	defer server.Close()

	client := NewClient(server.Client()).WithEndpoints(server.URL, server.URL+"/token", server.URL+"/mint")
	code, err := client.StartDeviceFlow(context.Background())
	require.NoError(t, err)
	require.Equal(t, "dev-1", code.DeviceCode)
	require.Equal(t, "ABCD-EFGH", code.UserCode)
	require.Equal(t, 900, code.ExpiresIn)
	require.Equal(t, []string{ClientID}, gotForm["client_id"])
	require.Equal(t, ClientUserAgent, gotUA)
	require.Equal(t, "application/json", gotAccept)
	require.Equal(t, "application/x-www-form-urlencoded", gotContentType)
}

func TestStartDeviceFlowRejectsIncompleteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"device_code":"dev-1"}`))
	}))
	defer server.Close()

	client := NewClient(server.Client()).WithEndpoints(server.URL, server.URL, server.URL)
	_, err := client.StartDeviceFlow(context.Background())
	require.ErrorContains(t, err, "missing device_code or user_code")
}

func TestStartDeviceFlowReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"slow_down"}`))
	}))
	defer server.Close()

	client := NewClient(server.Client()).WithEndpoints(server.URL, server.URL, server.URL)
	_, err := client.StartDeviceFlow(context.Background())
	require.ErrorContains(t, err, "HTTP 429")
}

func TestPollDeviceCodeStates(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
		want       PollState
		wantErrf   bool
	}{
		{name: "authorized", statusCode: http.StatusOK, body: `{"access_token":"dca:abc","token_type":"Bearer","expires_in":3600}`, want: PollAuthorized},
		{name: "pending", statusCode: http.StatusBadRequest, body: `{"error":"authorization_pending"}`, want: PollPending},
		{name: "slow_down", statusCode: http.StatusBadRequest, body: `{"error":"slow_down"}`, want: PollSlowDown},
		{name: "denied", statusCode: http.StatusBadRequest, body: `{"error":"access_denied","error_description":"user refused"}`, want: PollDenied},
		{name: "expired", statusCode: http.StatusBadRequest, body: `{"error":"expired_token"}`, want: PollExpired},
		{name: "unexpected", statusCode: http.StatusBadRequest, body: `{"error":"invalid_grant"}`, wantErrf: true},
		{name: "ok without token", statusCode: http.StatusOK, body: `{"token_type":"Bearer"}`, wantErrf: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotGrant string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, r.ParseForm())
				gotGrant = r.PostForm.Get("grant_type")
				require.Equal(t, "dev-1", r.PostForm.Get("device_code"))
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			client := NewClient(server.Client()).WithEndpoints(server.URL, server.URL, server.URL)
			result, err := client.PollDeviceCode(context.Background(), "dev-1", DefaultPollInterval)
			if tc.wantErrf {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, result.State)
			require.Equal(t, DeviceCodeGrantType, gotGrant)
			if tc.want == PollAuthorized {
				require.Equal(t, "dca:abc", result.Token.AccessToken)
			}
			if tc.want == PollSlowDown {
				require.Equal(t, DefaultPollInterval+SlowDownStep, result.Interval)
			}
			if tc.want == PollDenied {
				require.Equal(t, "user refused", result.Message)
			}
		})
	}
}

func TestPollDeviceCodeRequiresDeviceCode(t *testing.T) {
	client := NewClient(nil)
	_, err := client.PollDeviceCode(context.Background(), "  ", DefaultPollInterval)
	require.ErrorContains(t, err, "missing device code")
}

func TestMintAPIKeySendsBearerAndParsesSubscription(t *testing.T) {
	var gotAuth, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		raw, errRead := io.ReadAll(r.Body)
		require.NoError(t, errRead)
		gotBody = string(raw)
		require.Equal(t, ClientUserAgent, r.Header.Get("User-Agent"))
		_, _ = w.Write([]byte(`{"api_key":"sk-muse","base_url":"https://api.meta.ai/v1","user_email":"a@b.c","subs_tier_name":"Pro","subs_tier_id":"t1","is_subs_active":true,"has_payment_method":true}`))
	}))
	defer server.Close()

	client := NewClient(server.Client()).WithEndpoints(server.URL, server.URL, server.URL+"/key")
	minted, err := client.MintAPIKey(context.Background(), "dca:xyz")
	require.NoError(t, err)
	require.Equal(t, "sk-muse", minted.APIKey)
	require.Equal(t, "a@b.c", minted.UserEmail)
	require.True(t, minted.IsSubsActive)
	require.Equal(t, "Bearer dca:xyz", gotAuth)
	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(gotBody), &payload))
	require.Equal(t, "dca:xyz", payload["dca_token"])
}

func TestMintAPIKeyErrors(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
		}))
		defer server.Close()
		client := NewClient(server.Client()).WithEndpoints(server.URL, server.URL, server.URL)
		_, err := client.MintAPIKey(context.Background(), "dca:xyz")
		require.ErrorContains(t, err, "HTTP 401")
	})

	t.Run("missing api key", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"user_email":"a@b.c"}`))
		}))
		defer server.Close()
		client := NewClient(server.Client()).WithEndpoints(server.URL, server.URL, server.URL)
		_, err := client.MintAPIKey(context.Background(), "dca:xyz")
		require.ErrorContains(t, err, "missing api_key")
	})

	t.Run("missing dca token", func(t *testing.T) {
		client := NewClient(nil)
		_, err := client.MintAPIKey(context.Background(), "")
		require.ErrorContains(t, err, "missing dca token")
	})
}
