package tlsfingerprint

import "testing"

func TestSupportsHTTP2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		profile *Profile
		want    bool
	}{
		{name: "nil profile uses http1 default", profile: nil, want: false},
		{name: "empty profile uses http1 default", profile: &Profile{}, want: false},
		{name: "explicit h2", profile: &Profile{ALPNProtocols: []string{"h2", "http/1.1"}}, want: true},
		{name: "missing alpn extension", profile: &Profile{ALPNProtocols: []string{"h2"}, Extensions: []uint16{0, 10, 43}}, want: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := SupportsHTTP2(tc.profile); got != tc.want {
				t.Fatalf("SupportsHTTP2() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHTTP1OnlyProfileRemovesH2ALPN(t *testing.T) {
	t.Parallel()

	profile := &Profile{Name: "h2", ALPNProtocols: []string{"h2", "http/1.1"}}
	got := HTTP1OnlyProfile(profile)
	if got == profile {
		t.Fatal("expected cloned profile")
	}
	if SupportsHTTP2(got) {
		t.Fatal("HTTP1OnlyProfile should remove h2")
	}
	if len(got.ALPNProtocols) != 1 || got.ALPNProtocols[0] != "http/1.1" {
		t.Fatalf("ALPNProtocols = %#v, want [http/1.1]", got.ALPNProtocols)
	}
	if len(profile.ALPNProtocols) != 2 {
		t.Fatal("original profile should not be mutated")
	}
}
