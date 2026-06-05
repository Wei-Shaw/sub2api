//go:build unit

package plugin

import (
	"strings"
	"testing"
)

func TestMatchSegments(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    int
	}{
		// exact match
		{"/admin/monitors", "/admin/monitors", 3},
		// prefix match (path longer than pattern)
		{"/admin/monitors/42", "/admin/monitors", 3},
		// param segment matches any value
		{"/admin/monitors/42", "/admin/monitors/:id", 4},
		// param + fixed sub-resource
		{"/admin/monitors/42/run", "/admin/monitors/:id/run", 5},
		{"/admin/monitors/42/history", "/admin/monitors/:id/history", 5},
		// param mismatch: fixed segment after param
		{"/admin/monitors/42/history", "/admin/monitors/:id/run", 0},
		// path too short for pattern
		{"/admin/monitors", "/admin/monitors/:id", 0},
		// param must match non-empty segment
		{"/admin/monitors/", "/admin/monitors/:id", 0},
		// trailing slash treated as prefix (trailing empty segment dropped)
		{"/api/v1/plugin/demo/foo", "/api/v1/plugin/demo/", 5},
		{"/api/v1/plugin/demo/foo/bar", "/api/v1/plugin/demo/", 5},
		// no params
		{"/v1/chat/completions", "/v1/chat/completions", 4},
		// wildcard param
		{"/api/v1/plugin/foo/bar", "/api/v1/plugin/foo/*rest", 6},
		// root
		{"/", "/", 1},
	}
	for _, tt := range tests {
		pathSegs := strings.Split(tt.path, "/")
		got := matchSegments(pathSegs, tt.pattern)
		if got != tt.want {
			t.Errorf("matchSegments(%q, %q) = %d, want %d", tt.path, tt.pattern, got, tt.want)
		}
	}
}

func TestRouteTableMatchWithParams(t *testing.T) {
	rt := NewRouteTable()
	rt = rt.AddPlugin("monitor", []RouteEntry{
		{Method: "GET", PathPrefix: "/api/v1/plugin/cm/admin/monitors"},
		{Method: "POST", PathPrefix: "/api/v1/plugin/cm/admin/monitors"},
		{Method: "GET", PathPrefix: "/api/v1/plugin/cm/admin/monitors/:id"},
		{Method: "PUT", PathPrefix: "/api/v1/plugin/cm/admin/monitors/:id"},
		{Method: "DELETE", PathPrefix: "/api/v1/plugin/cm/admin/monitors/:id"},
		{Method: "POST", PathPrefix: "/api/v1/plugin/cm/admin/monitors/:id/run"},
		{Method: "GET", PathPrefix: "/api/v1/plugin/cm/admin/monitors/:id/history"},
	})

	tests := []struct {
		method string
		path   string
		match  bool
	}{
		{"GET", "/api/v1/plugin/cm/admin/monitors", true},
		{"POST", "/api/v1/plugin/cm/admin/monitors", true},
		{"PUT", "/api/v1/plugin/cm/admin/monitors/42", true},
		{"DELETE", "/api/v1/plugin/cm/admin/monitors/42", true},
		{"GET", "/api/v1/plugin/cm/admin/monitors/42", true},
		{"POST", "/api/v1/plugin/cm/admin/monitors/42/run", true},
		{"GET", "/api/v1/plugin/cm/admin/monitors/42/history", true},
		// no match
		{"PATCH", "/api/v1/plugin/cm/admin/monitors/42", false},
		{"PUT", "/api/v1/plugin/cm/admin/monitors", false},
		{"GET", "/api/v1/plugin/cm/admin/other", false},
	}
	for _, tt := range tests {
		entry, ok := rt.Match(tt.method, tt.path)
		if ok != tt.match {
			t.Errorf("Match(%s, %s) ok=%v, want %v", tt.method, tt.path, ok, tt.match)
		}
		if ok && entry == nil {
			t.Errorf("Match(%s, %s) returned nil entry", tt.method, tt.path)
		}
	}
}
