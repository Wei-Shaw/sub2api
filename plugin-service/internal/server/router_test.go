package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/config"
	hostprincipal "github.com/Wei-Shaw/sub2api/plugin-service/internal/host/principal"
	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
)

func TestRouterPluginPageAllowsEmbeddingFromForwardedHost(t *testing.T) {
	router := NewRouter(config.Config{ListenAddr: ":0"})

	req := httptest.NewRequest(http.MethodGet, "/plugins/image-generation", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "app.example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("plugin page status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'self' https://app.example.com" {
		t.Fatalf("plugin page CSP = %q, want %q", got, "frame-ancestors 'self' https://app.example.com")
	}
}

func TestRouterPluginPageAllowsEmbeddingFromRefererFallback(t *testing.T) {
	router := NewRouter(config.Config{ListenAddr: ":0"})

	req := httptest.NewRequest(http.MethodGet, "/plugins/image-generation", nil)
	req.Header.Set("Referer", "https://app.example.com/user/custom-page/plugin")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("plugin page status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'self' https://app.example.com" {
		t.Fatalf("plugin page CSP = %q, want %q", got, "frame-ancestors 'self' https://app.example.com")
	}
}

func TestRouterPluginPageAlwaysAllowsSameOriginEmbedding(t *testing.T) {
	router := NewRouter(config.Config{ListenAddr: ":0"})

	req := httptest.NewRequest(http.MethodGet, "/plugins/image-generation", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("plugin page status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'self'" {
		t.Fatalf("plugin page CSP = %q, want %q", got, "frame-ancestors 'self'")
	}
}

func TestRouterSharedAuthGenerateAndListHistory(t *testing.T) {
	mainSite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer launch-token" {
			t.Fatalf("authorization = %q, want %q", got, "Bearer launch-token")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":42,"email":"user@example.com","username":"launch-user","role":"user"}}`))
	}))
	defer mainSite.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/keys/7" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":7,"user_id":42,"key":"provider-secret","status":"active","group":{"allow_image_generation":true}}}`))
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer provider-secret" {
			t.Fatalf("provider authorization = %q, want %q", got, "Bearer provider-secret")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"imgbatch_history","status":"queued","model":"gemini-2.5-flash-image"}`))
	}))
	defer upstream.Close()

	restoreMainSiteResolver(t, mainSite.URL)
	router := NewRouter(config.Config{ListenAddr: ":0"})

	body := bytes.NewBufferString(`{"prompt":"make a poster","api_key_id":7,"model":"gemini-2.5-flash-image","inputs":{"conversation_id":"conversation-live-test"}}`)
	generateReq := httptest.NewRequest(http.MethodPost, "/plugins/image-generation/api/generate", body)
	generateReq.Header.Set("Authorization", "Bearer launch-token")
	addForwardedProviderOrigin(generateReq, upstream.URL)
	generateRec := httptest.NewRecorder()
	router.ServeHTTP(generateRec, generateReq)
	if generateRec.Code != http.StatusCreated {
		t.Fatalf("generate status = %d, want %d; body=%s", generateRec.Code, http.StatusCreated, generateRec.Body.String())
	}
	secondBody := bytes.NewBufferString(`{"prompt":"make another poster","api_key_id":7,"model":"gemini-2.5-flash-image","inputs":{"conversation_id":"conversation-live-test"}}`)
	secondGenerateReq := httptest.NewRequest(http.MethodPost, "/plugins/image-generation/api/generate", secondBody)
	secondGenerateReq.Header.Set("Authorization", "Bearer launch-token")
	addForwardedProviderOrigin(secondGenerateReq, upstream.URL)
	secondGenerateRec := httptest.NewRecorder()
	router.ServeHTTP(secondGenerateRec, secondGenerateReq)
	if secondGenerateRec.Code != http.StatusCreated {
		t.Fatalf("second generate status = %d, want %d; body=%s", secondGenerateRec.Code, http.StatusCreated, secondGenerateRec.Body.String())
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/plugins/image-generation/api/conversations", nil)
	historyReq.Header.Set("Authorization", "Bearer launch-token")
	historyRec := httptest.NewRecorder()
	router.ServeHTTP(historyRec, historyReq)
	if historyRec.Code != http.StatusOK {
		t.Fatalf("history status = %d, want %d; body=%s", historyRec.Code, http.StatusOK, historyRec.Body.String())
	}

	var conversations struct {
		Items []model.ConversationSummary `json:"items"`
	}
	if err := json.NewDecoder(historyRec.Body).Decode(&conversations); err != nil {
		t.Fatal(err)
	}
	if len(conversations.Items) != 1 || conversations.Items[0].ID != "conversation-live-test" {
		t.Fatalf("conversations = %#v", conversations.Items)
	}
	messagesReq := httptest.NewRequest(http.MethodGet, "/plugins/image-generation/api/conversations/conversation-live-test/messages", nil)
	messagesReq.Header.Set("Authorization", "Bearer launch-token")
	messagesRec := httptest.NewRecorder()
	router.ServeHTTP(messagesRec, messagesReq)
	var payload struct {
		Items []model.HistoryRecord `json:"items"`
	}
	if err := json.NewDecoder(messagesRec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("message item count = %d, want 2", len(payload.Items))
	}
	for _, item := range payload.Items {
		if item.UserID != 42 {
			t.Fatalf("history user_id = %d, want 42", item.UserID)
		}
		if item.ConversationID != "conversation-live-test" {
			t.Fatalf("history conversation_id = %q, want %q", item.ConversationID, "conversation-live-test")
		}
		if _, ok := item.Request["provider_api_key"]; ok {
			t.Fatal("history exposed provider_api_key")
		}
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/plugins/image-generation/api/history/"+payload.Items[0].ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer launch-token")
	deleteRec := httptest.NewRecorder()
	router.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete history status = %d, want %d; body=%s", deleteRec.Code, http.StatusNoContent, deleteRec.Body.String())
	}

	historyAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/plugins/image-generation/api/conversations/conversation-live-test/messages", nil)
	historyAfterDeleteReq.Header.Set("Authorization", "Bearer launch-token")
	historyAfterDeleteRec := httptest.NewRecorder()
	router.ServeHTTP(historyAfterDeleteRec, historyAfterDeleteReq)
	if historyAfterDeleteRec.Code != http.StatusOK {
		t.Fatalf("history after delete status = %d, want %d; body=%s", historyAfterDeleteRec.Code, http.StatusOK, historyAfterDeleteRec.Body.String())
	}

	var afterDeletePayload struct {
		Items []model.HistoryRecord `json:"items"`
	}
	if err := json.NewDecoder(historyAfterDeleteRec.Body).Decode(&afterDeletePayload); err != nil {
		t.Fatal(err)
	}
	if len(afterDeletePayload.Items) != 1 {
		t.Fatalf("history item count after delete = %d, want 1", len(afterDeletePayload.Items))
	}
}

func TestRouterGenerateUsesConfiguredMainServiceBaseURL(t *testing.T) {
	mainSite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/keys/7" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":7,"user_id":42,"key":"provider-secret","status":"active","group":{"allow_image_generation":true}}}`))
			return
		}
		if r.URL.Path != "/v1/images/batches" {
			t.Fatalf("main service path = %s, want /v1/images/batches", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer provider-secret" {
			t.Fatalf("provider authorization = %q, want %q", got, "Bearer provider-secret")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"imgbatch_configured","status":"queued","model":"gemini-2.5-flash-image"}`))
	}))
	defer mainSite.Close()

	restoreMainSiteResolver(t, mainSite.URL)
	t.Setenv("PLUGIN_MAIN_SERVICE_BASE_URL", mainSite.URL)
	router := NewRouter(config.Config{ListenAddr: ":0"})

	body := bytes.NewBufferString(`{"prompt":"make a poster","api_key_id":7,"model":"gemini-2.5-flash-image"}`)
	req := httptest.NewRequest(http.MethodPost, "/plugins/image-generation/api/generate", body)
	req.Header.Set("X-Sub2api-User-Id", "42")
	req.Header.Set("X-Sub2api-User-Role", "user")
	req.Header.Set("X-Forwarded-Proto", "http")
	req.Header.Set("X-Forwarded-Host", "192.168.0.230:8080")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("generate status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestRouterImageTaskStatusAndCancel(t *testing.T) {
	customID := ""
	batchServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/keys/7" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":7,"user_id":42,"key":"batch-api-key","status":"active","group":{"allow_image_generation":true}}}`))
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer batch-api-key" {
			t.Fatalf("batch authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches":
			var payload struct {
				Items []struct {
					CustomID string `json:"custom_id"`
				} `json:"items"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			customID = payload.Items[0].CustomID
			_, _ = w.Write([]byte(`{"id":"imgbatch_router","status":"queued","model":"gemini-2.5-flash-image"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_router":
			_, _ = w.Write([]byte(`{"id":"imgbatch_router","status":"running","model":"gemini-2.5-flash-image"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/images/batches/imgbatch_router/items":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/images/batches/imgbatch_router/cancel":
			_, _ = w.Write([]byte(`{"id":"imgbatch_router","status":"cancelled","model":"gemini-2.5-flash-image"}`))
		default:
			t.Fatalf("unexpected batch request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer batchServer.Close()

	t.Setenv("PLUGIN_MAIN_SERVICE_BASE_URL", batchServer.URL)
	router := NewRouter(config.Config{ListenAddr: ":0"})
	body := bytes.NewBufferString(`{"prompt":"draw a cat","api_key_id":7,"model":"gemini-2.5-flash-image"}`)
	generateReq := httptest.NewRequest(http.MethodPost, "/plugins/image-generation/api/generate", body)
	generateReq.Header.Set("X-Sub2api-User-Id", "42")
	generateReq.Header.Set("X-Sub2api-User-Role", "user")
	generateRec := httptest.NewRecorder()
	router.ServeHTTP(generateRec, generateReq)
	if generateRec.Code != http.StatusCreated {
		t.Fatalf("generate status = %d; body=%s", generateRec.Code, generateRec.Body.String())
	}
	var generated struct {
		JobID  string `json:"job_id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(generateRec.Body).Decode(&generated); err != nil {
		t.Fatal(err)
	}
	if generated.Status != model.HistoryStatusPending || customID != "plugin-image-"+generated.JobID {
		t.Fatalf("generated = %#v, custom_id=%q", generated, customID)
	}

	callHistoryAction := func(method, action string) struct {
		JobID        string         `json:"job_id"`
		Status       string         `json:"status"`
		Result       map[string]any `json:"result"`
		ErrorMessage string         `json:"error_message"`
	} {
		t.Helper()
		req := httptest.NewRequest(method, "/plugins/image-generation/api/history/"+generated.JobID+"/"+action, nil)
		req.Header.Set("X-Sub2api-User-Id", "42")
		req.Header.Set("X-Sub2api-User-Role", "user")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d; body=%s", action, rec.Code, rec.Body.String())
		}
		var record struct {
			JobID        string         `json:"job_id"`
			Status       string         `json:"status"`
			Result       map[string]any `json:"result"`
			ErrorMessage string         `json:"error_message"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&record); err != nil {
			t.Fatal(err)
		}
		return record
	}
	if status := callHistoryAction(http.MethodGet, "status"); status.Status != model.HistoryStatusPending || status.Result != nil {
		t.Fatalf("status record = %#v", status)
	}
	if canceled := callHistoryAction(http.MethodPost, "cancel"); canceled.Status != model.HistoryStatusCanceled {
		t.Fatalf("cancel record = %#v", canceled)
	}
	for _, action := range []string{"pause", "resume"} {
		req := httptest.NewRequest(http.MethodPost, "/plugins/image-generation/api/history/"+generated.JobID+"/"+action, nil)
		req.Header.Set("X-Sub2api-User-Id", "42")
		req.Header.Set("X-Sub2api-User-Role", "user")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("removed %s route status = %d, want 404", action, rec.Code)
		}
	}
}

func restoreMainSiteResolver(t *testing.T, baseURL string) {
	t.Helper()
	previous := hostprincipal.ResolveMainSiteBaseCandidates
	hostprincipal.ResolveMainSiteBaseCandidates = func(_ *http.Request) []string {
		return []string{baseURL}
	}
	t.Cleanup(func() {
		hostprincipal.ResolveMainSiteBaseCandidates = previous
	})
}

func addForwardedProviderOrigin(req *http.Request, upstreamURL string) {
	parsed, err := url.Parse(upstreamURL)
	if err != nil {
		panic(err)
	}
	req.Header.Set("X-Forwarded-Proto", parsed.Scheme)
	req.Header.Set("X-Forwarded-Host", parsed.Host)
}
