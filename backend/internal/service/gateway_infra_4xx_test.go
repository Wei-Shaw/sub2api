package service

import (
	"net/http"
	"testing"
)

func TestIsInfraLevelUpstream4xxResponse(t *testing.T) {
	cases := []struct {
		name    string
		ct      string
		body    string
		want    bool
		comment string
	}{
		{
			name:    "nginx html 400 page (no content-type)",
			ct:      "",
			body:    "<html>\n<head><title>400 Bad Request</title></head>\n<body>\n<center><h1>400 Bad Request</h1></center>\n<hr><center>nginx/1.24.0 (Ubuntu)</center>\n</body>\n</html>\n",
			want:    true,
			comment: "screenshot scenario: nginx html 400 from upstream",
		},
		{
			name: "nginx html with text/html content-type",
			ct:   "text/html; charset=utf-8",
			body: "<html><body>502 Bad Gateway</body></html>",
			want: true,
		},
		{
			name: "doctype html",
			ct:   "",
			body: "<!DOCTYPE html>\n<html><body>blocked</body></html>",
			want: true,
		},
		{
			name: "empty body",
			ct:   "",
			body: "",
			want: true,
		},
		{
			name: "whitespace-only body",
			ct:   "",
			body: "   \n\t  ",
			want: true,
		},
		{
			name: "plain text body",
			ct:   "text/plain",
			body: "service unavailable",
			want: true,
		},
		{
			name:    "real anthropic invalid_request_error json (must NOT failover)",
			ct:      "application/json",
			body:    `{"type":"error","error":{"type":"invalid_request_error","message":"max_tokens must be positive"}}`,
			want:    false,
			comment: "legitimate client error — failover would be wrong",
		},
		{
			name: "json with leading whitespace",
			ct:   "application/json",
			body: "  \n  {\"error\":{\"message\":\"bad model\"}}  ",
			want: false,
		},
		{
			name:    "json with text/html content-type (defensive: trust content-type)",
			ct:      "text/html",
			body:    `{"error":{"message":"x"}}`,
			want:    true,
			comment: "broken upstream returning JSON with wrong content-type still treated as infra",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.ct != "" {
				h.Set("Content-Type", tc.ct)
			}
			got := isInfraLevelUpstream4xxResponse(h, []byte(tc.body))
			if got != tc.want {
				t.Fatalf("isInfraLevelUpstream4xxResponse: got=%v want=%v (%s)", got, tc.want, tc.comment)
			}
		})
	}
}
