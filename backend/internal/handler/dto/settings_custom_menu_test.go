package dto

import (
	"encoding/json"
	"testing"
)

func TestCustomMenuAuthenticationForwarding(t *testing.T) {
	// Missing fields in old settings must be safe without a migration.
	legacy := ParseCustomMenuItems(`[{"id":"docs","url":"https://example.com/docs","visibility":"user"}]`)
	if len(legacy) != 1 || legacy[0].PassUserID || legacy[0].PassAuthToken {
		t.Fatalf("legacy menus must not forward authentication context: %+v", legacy)
	}

	raw := `[
		{"id":"id-only","visibility":"user","pass_user_id":true},
		{"id":"token-only","visibility":"user","pass_auth_token":true},
		{"id":"admin","visibility":"admin","pass_user_id":true,"pass_auth_token":true}
	]`
	items := ParseCustomMenuItems(raw)
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	// The settings API uses this DTO to save and return the JSON setting.
	visible := ParseUserVisibleMenuItems(string(encoded))
	if len(visible) != 2 {
		t.Fatalf("expected only user-visible menus, got %+v", visible)
	}
	if !visible[0].PassUserID || visible[0].PassAuthToken {
		t.Fatalf("user ID option did not survive round trip: %+v", visible[0])
	}
	if visible[1].PassUserID || !visible[1].PassAuthToken {
		t.Fatalf("token option did not survive round trip: %+v", visible[1])
	}
	if len(items) != 3 || !items[2].PassUserID || !items[2].PassAuthToken {
		t.Fatalf("admin menu options were not parsed: %+v", items)
	}
}
