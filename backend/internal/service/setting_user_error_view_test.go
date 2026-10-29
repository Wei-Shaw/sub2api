package service

import "testing"

func TestSettingKeyAllowUserViewErrorRequests_Constant(t *testing.T) {
	if SettingKeyAllowUserViewErrorRequests != "allow_user_view_error_requests" {
		t.Fatalf("unexpected key: %s", SettingKeyAllowUserViewErrorRequests)
	}
}
