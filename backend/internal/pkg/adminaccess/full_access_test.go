//go:build unit

package adminaccess

import (
	"encoding/json"
	"testing"
)

func TestAdminFullAccessRouteBoundary(t *testing.T) {
	var policy Policy
	if err := json.Unmarshal([]byte(`{"full_access":true}`), &policy); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path    string
		allowed bool
	}{
		{"/api/v1/admin/future-operation", true},
		{"/api/v1/admin/accounts/:id", true},
		{"/api/v1/admin/settings/admin-api-keys", false},
		{"/api/v1/admin/settings/admin-api-keys/:id/rotate", false},
		{"/api/v1/admin/settings/admin-api-key/regenerate", false},
		{"/api/v1/user/profile", false},
		{"/v1/messages", false},
		{"", false},
	} {
		if got := RouteAllowed(policy, "POST", tc.path); got != tc.allowed {
			t.Errorf("%s: got %v want %v", tc.path, got, tc.allowed)
		}
	}
}
