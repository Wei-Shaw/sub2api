package backend

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/plugin-service/internal/model"
)

func TestMainServiceAPIKeyResolverResolvesOwnedActiveImageKey(t *testing.T) {
	var authorization, cookie, forwardedFor, userAgent, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		cookie = r.Header.Get("Cookie")
		forwardedFor = r.Header.Get("X-Forwarded-For")
		userAgent = r.Header.Get("User-Agent")
		path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":0,"data":{"id":42,"user_id":7,"key":"provider-secret","status":"active","group":{"allow_image_generation":true,"models_list_config":{"enabled":true,"models":["gpt-image-1"]}}}}`)
	}))
	defer server.Close()

	request := httptest.NewRequest(http.MethodPost, "/plugins/image-generation/api/generate", nil)
	request.Header.Set("Authorization", "Bearer user-token")
	request.Header.Set("Cookie", "session=user-session")
	request.Header.Set("X-Forwarded-For", "203.0.113.10, 192.0.2.1")
	request.Header.Set("User-Agent", "Sub2API integration test")

	resolver := NewMainServiceAPIKeyResolver(server.Client())
	secret, err := resolver.Resolve(context.Background(), request, model.CurrentPrincipal{UserID: 7}, server.URL, 42, "gpt-image-1")
	if err != nil {
		t.Fatal(err)
	}
	if secret != "provider-secret" {
		t.Fatalf("Resolve() secret = %q, want provider-secret", secret)
	}
	if path != "/api/v1/keys/42" || authorization != "Bearer user-token" || cookie != "session=user-session" || forwardedFor != "203.0.113.10, 192.0.2.1" || userAgent != "Sub2API integration test" {
		t.Fatalf("request path=%q authorization=%q cookie=%q forwarded_for=%q user_agent=%q", path, authorization, cookie, forwardedFor, userAgent)
	}
}

func TestMainServiceAPIKeyResolverRejectsInvalidKeys(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "wrong owner", data: `{"id":42,"user_id":8,"key":"secret","status":"active","group":{"allow_image_generation":true}}`},
		{name: "disabled", data: `{"id":42,"user_id":7,"key":"secret","status":"disabled","group":{"allow_image_generation":true}}`},
		{name: "image permission disabled", data: `{"id":42,"user_id":7,"key":"secret","status":"active","group":{"allow_image_generation":false}}`},
		{name: "model excluded", data: `{"id":42,"user_id":7,"key":"secret","status":"active","group":{"allow_image_generation":true,"models_list_config":{"enabled":true,"models":["gpt-image-2"]}}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"code":0,"data":%s}`, test.data)
			}))
			defer server.Close()

			resolver := NewMainServiceAPIKeyResolver(server.Client())
			_, err := resolver.Resolve(context.Background(), httptest.NewRequest(http.MethodGet, "/", nil), model.CurrentPrincipal{UserID: 7}, server.URL, 42, "gpt-image-1")
			if err == nil {
				t.Fatal("Resolve() error = nil, want rejection")
			}
		})
	}
}
