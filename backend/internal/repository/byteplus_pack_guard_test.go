package repository

import "testing"

func TestPackGuardFactoryRejectsMissingSQLStore(t *testing.T) {
	upstream, err := NewPackGuardedHTTPUpstream(nil, nil)
	if err == nil || upstream != nil {
		t.Fatal("missing SQL wiring returned an unguarded upstream")
	}
}
