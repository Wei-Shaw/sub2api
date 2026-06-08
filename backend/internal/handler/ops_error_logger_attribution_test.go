package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLooksLikeSystemKey(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"sk-abcdef0123456789", true},
		{"ABCdef_-0123456789", true},
		{"short", false},
		{"with space xxxxxxxxxx", false},
		{"汉字key1234567890", false},
		{"", false},
	}
	for _, c := range cases {
		if got := looksLikeSystemKey(c.in); got != c.want {
			t.Errorf("looksLikeSystemKey(%q)=%v want %v", c.in, got, c.want)
		}
	}
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	if looksLikeSystemKey(string(long)) {
		t.Errorf("129-char key should be rejected")
	}
}

func TestKeyPrefix(t *testing.T) {
	if got := keyPrefix("sk-3f2a9c7e", 8); got != "sk-3f2a9" {
		t.Errorf("keyPrefix=%q want %q", got, "sk-3f2a9")
	}
	if got := keyPrefix("abc", 8); got != "abc" {
		t.Errorf("short key should be returned as-is, got %q", got)
	}
}

func TestExtractAttemptedKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "Bearer in Authorization",
			headers: map[string]string{"Authorization": "Bearer sk-testkey0123456789"},
			want:    "sk-testkey0123456789",
		},
		{
			name:    "Bearer case-insensitive",
			headers: map[string]string{"Authorization": "BEARER sk-testkey0123456789"},
			want:    "sk-testkey0123456789",
		},
		{
			name:    "x-api-key header",
			headers: map[string]string{"x-api-key": "sk-xapikey0123456789"},
			want:    "sk-xapikey0123456789",
		},
		{
			name:    "x-goog-api-key header",
			headers: map[string]string{"x-goog-api-key": "sk-goog0123456789"},
			want:    "sk-goog0123456789",
		},
		{
			name:    "Authorization takes priority over x-api-key",
			headers: map[string]string{"Authorization": "Bearer sk-auth0123456789", "x-api-key": "sk-xapi0123456789"},
			want:    "sk-auth0123456789",
		},
		{
			name:    "x-api-key takes priority over x-goog-api-key",
			headers: map[string]string{"x-api-key": "sk-xapi0123456789", "x-goog-api-key": "sk-goog0123456789"},
			want:    "sk-xapi0123456789",
		},
		{
			name:    "no key headers",
			headers: map[string]string{},
			want:    "",
		},
		{
			name:    "Bearer with leading/trailing spaces trimmed",
			headers: map[string]string{"Authorization": "Bearer   sk-trimmed0123456789  "},
			want:    "sk-trimmed0123456789",
		},
		{
			// 非 Bearer Authorization 应被忽略,继续 fall-through 到 x-api-key(与认证中间件一致)
			name:    "non-Bearer Authorization falls through to x-api-key",
			headers: map[string]string{"Authorization": "junk-not-bearer", "x-api-key": "sk-realkey1234567"},
			want:    "sk-realkey1234567",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			c.Request = req

			got := extractAttemptedKey(c)
			if got != tc.want {
				t.Errorf("extractAttemptedKey(%v) = %q, want %q", tc.headers, got, tc.want)
			}
		})
	}
}
