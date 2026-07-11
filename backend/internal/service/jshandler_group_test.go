package service

import "testing"

func TestJShandlerScriptIDsFromGroup(t *testing.T) {
	t.Parallel()
	if got := JShandlerScriptIDsFromGroup(nil); got != nil {
		t.Fatalf("nil group: got %v", got)
	}
	g := &Group{JSHandlerScriptIDs: []string{" a ", "b", "a", ""}}
	got := JShandlerScriptIDsFromGroup(g)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected ids: %v", got)
	}
	if got := JShandlerScriptIDsFromAPIKeyGroup(&APIKey{Group: g}); len(got) != 2 {
		t.Fatalf("from api key: %v", got)
	}
	if got := JShandlerScriptIDsFromAPIKeyGroup(&APIKey{}); got != nil {
		t.Fatalf("no group: %v", got)
	}
}
