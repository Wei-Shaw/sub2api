package upstreamstation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSub2APIConnectorReadsUserRatesAndRechargeMultiplier(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/settings/public", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"site_name": "Sub2"}})
	})
	mux.HandleFunc("/api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		require.Equal(t, "boss@example.com", body["email"])
		require.Equal(t, "secret", body["password"])
		writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{
			"access_token": "access", "refresh_token": "refresh", "expires_in": 3600,
		}})
	})
	mux.HandleFunc("/api/v1/auth/me", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer access", r.Header.Get("Authorization"))
		writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"balance": 12.5}})
	})
	mux.HandleFunc("/api/v1/groups/available", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"code": 0, "data": []map[string]any{{
			"id": 3, "name": "pro", "description": "Pro", "rate_multiplier": 1.2, "platform": "openai",
		}}})
	})
	mux.HandleFunc("/api/v1/groups/rates", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]float64{"3": 1.5}})
	})
	mux.HandleFunc("/api/v1/payment/checkout-info", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{"balance_recharge_multiplier": 2.0}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := NewSub2APIConnector(server.Client())
	detected, err := connector.Detect(context.Background(), server.URL)
	require.NoError(t, err)
	require.True(t, detected)
	session, err := connector.Authenticate(context.Background(), &Station{BaseURL: server.URL}, Credentials{
		Username: "boss@example.com", Password: "secret",
	})
	require.NoError(t, err)
	require.Equal(t, "access", session.AccessToken)

	balance, err := connector.GetBalance(context.Background(), server.URL, session)
	require.NoError(t, err)
	require.Equal(t, 12.5, balance)
	groups, err := connector.ListGroups(context.Background(), server.URL, session)
	require.NoError(t, err)
	require.Equal(t, []RemoteGroup{{Key: "3", Name: "pro", Description: "Pro", Rate: 1.5, Platform: "openai"}}, groups)
	multiplier, err := connector.GetRechargeMultiplier(context.Background(), server.URL, session)
	require.NoError(t, err)
	require.Equal(t, 2.0, multiplier)
}

func TestNewAPIConnectorSkipsAutomaticGroup(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 500000}})
	})
	mux.HandleFunc("/api/user/login", func(w http.ResponseWriter, _ *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "cookie"})
		writeTestJSON(t, w, map[string]any{"success": true, "data": map[string]any{"id": 7}})
	})
	mux.HandleFunc("/api/user/self", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "7", r.Header.Get("New-Api-User"))
		writeTestJSON(t, w, map[string]any{"success": true, "data": map[string]any{"quota": 1000000}})
	})
	mux.HandleFunc("/api/user/self/groups", func(w http.ResponseWriter, _ *http.Request) {
		writeTestJSON(t, w, map[string]any{"success": true, "data": map[string]any{
			"default": map[string]any{"ratio": 0.8, "desc": "Default"},
			"auto":    map[string]any{"ratio": "自动", "desc": "Auto"},
		}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := NewNewAPIConnector(server.Client())
	session, err := connector.Authenticate(context.Background(), &Station{BaseURL: server.URL}, Credentials{
		Username: "boss", Password: "secret",
	})
	require.NoError(t, err)
	require.Equal(t, "7", session.UserID)
	require.Contains(t, session.Cookie, "session=cookie")
	balance, err := connector.GetBalance(context.Background(), server.URL, session)
	require.NoError(t, err)
	require.Equal(t, 2.0, balance)
	groups, err := connector.ListGroups(context.Background(), server.URL, session)
	require.NoError(t, err)
	require.Equal(t, []RemoteGroup{{Key: "default", Name: "default", Description: "Default", Rate: 0.8, Platform: "openai"}}, groups)
}

func TestSub2APIConnectorCreatesManagedKeyAndListsModels(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{
				"items": []map[string]any{}, "total": 0, "page": 1, "page_size": 100,
			}})
		case http.MethodPost:
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "sub2api-auto-7-pro", body["name"])
			require.Equal(t, float64(3), body["group_id"])
			writeTestJSON(t, w, map[string]any{"code": 0, "data": map[string]any{
				"id": 8, "key": "sk-sub2", "name": body["name"], "group_id": 3, "status": "active",
			}})
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer sk-sub2", r.Header.Get("Authorization"))
		writeTestJSON(t, w, map[string]any{"data": []map[string]any{{"id": "gpt-5"}, {"id": "o3"}}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := NewSub2APIConnector(server.Client())
	created, err := connector.CreateAPIKey(context.Background(), server.URL, &Session{AccessToken: "access"}, "sub2api-auto-7-pro", RemoteGroup{Key: "3"})
	require.NoError(t, err)
	require.Equal(t, ManagedKey{ID: "8", Name: "sub2api-auto-7-pro", GroupKey: "3", Key: "sk-sub2", Status: "active"}, *created)
	models, err := connector.ListModels(context.Background(), server.URL, created.Key, "openai")
	require.NoError(t, err)
	require.Equal(t, []string{"gpt-5", "o3"}, models)
}

func TestNewAPIConnectorListsOnlyReturnedManagedKeys(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/token/", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "7", r.Header.Get("New-Api-User"))
		writeTestJSON(t, w, map[string]any{"success": true, "data": map[string]any{
			"items": []map[string]any{
				{"id": 10, "name": "personal", "group": "default", "status": 1, "key": "sk-mask"},
				{"id": 11, "name": "sub2api-auto-7-default", "group": "default", "status": 1, "key": "sk-managed"},
			},
		}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := NewNewAPIConnector(server.Client())
	keys, err := connector.ListAPIKeys(context.Background(), server.URL, &Session{Cookie: "session=x", UserID: "7"})
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Equal(t, "sub2api-auto-7-default", keys[1].Name)
}

func TestNewAPIConnectorCreatesManagedKeyFromEmptyCreateResponse(t *testing.T) {
	t.Parallel()

	const managedName = "sub2api-auto-7-vip"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/token/", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "access-token", r.Header.Get("Authorization"))
		require.Equal(t, "7", r.Header.Get("New-Api-User"))
		switch r.Method {
		case http.MethodPost:
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, managedName, body["name"])
			require.Equal(t, "vip", body["group"])
			// NewAPI's AddToken response contains success/message only, without data or id.
			writeTestJSON(t, w, map[string]any{"success": true, "message": ""})
		case http.MethodGet:
			require.Equal(t, "1", r.URL.Query().Get("p"))
			require.Equal(t, "100", r.URL.Query().Get("size"))
			require.Empty(t, r.URL.Query().Get("page_size"))
			writeTestJSON(t, w, map[string]any{"success": true, "data": map[string]any{
				"items": []map[string]any{{
					"id": 41, "name": managedName, "group": "vip", "status": 1, "key": "sk-masked********key",
				}},
			}})
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/token/41/key", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "access-token", r.Header.Get("Authorization"))
		require.Equal(t, "7", r.Header.Get("New-Api-User"))
		writeTestJSON(t, w, map[string]any{"success": true, "data": map[string]any{"key": "sk-full-key"}})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	connector := NewNewAPIConnector(server.Client())
	created, err := connector.CreateAPIKey(
		context.Background(),
		server.URL,
		&Session{AccessToken: "access-token", UserID: "7"},
		managedName,
		RemoteGroup{Key: "vip"},
	)
	require.NoError(t, err)
	require.Equal(t, ManagedKey{
		ID: "41", Name: managedName, GroupKey: "vip", Key: "sk-full-key", Status: "active",
	}, *created)
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(value))
}
