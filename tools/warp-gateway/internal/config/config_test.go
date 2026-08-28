package config

import "testing"

func TestValidateRequiresAuthForNonLoopbackListen(t *testing.T) {
	for _, listen := range []string{"0.0.0.0:19798", "[::]:19798", ":19798", "192.0.2.1:19798"} {
		cfg := Default()
		cfg.Listen = listen
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded without authentication", listen)
		}
	}

	cfg := Default()
	cfg.Listen = "0.0.0.0:19798"
	cfg.Token = "secret"
	cfg.ProfileKey = "stable-profile-key"
	if err := cfg.Validate(); err == nil {
		t.Fatal("non-loopback token-authenticated config succeeded without TLS")
	}
	cfg.TLSCertFile = "server.crt"
	cfg.TLSKeyFile = "server.key"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("TLS token-authenticated config rejected: %v", err)
	}

	cfg.Token = ""
	cfg.ClientCAFile = "clients.crt"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("mTLS config rejected: %v", err)
	}
}

func TestValidateRejectsLoopbackWithoutAuth(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:19798", "[::1]:19798", "localhost:19798"} {
		cfg := Default()
		cfg.Listen = listen
		if err := cfg.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded without authentication", listen)
		}
	}
}

func TestProfileSecretNeverFallsBackToBearerToken(t *testing.T) {
	cfg := Default()
	cfg.Token = "rotating-bearer-token"
	if got := cfg.ProfileSecret(); got != "" {
		t.Fatalf("ProfileSecret fell back to bearer token: %q", got)
	}
	cfg.ProfileKey = " stable-profile-key "
	if got := cfg.ProfileSecret(); got != "stable-profile-key" {
		t.Fatalf("ProfileSecret=%q", got)
	}
}

func TestValidateRequiresIndependentProfileKeyForMTLSOnly(t *testing.T) {
	cfg := Default()
	cfg.Listen = "0.0.0.0:19798"
	cfg.TLSCertFile = "server.crt"
	cfg.TLSKeyFile = "server.key"
	cfg.ClientCAFile = "clients.crt"
	if err := cfg.Validate(); err == nil {
		t.Fatal("mTLS-only config succeeded without profile encryption key")
	}
	cfg.ProfileKey = "stable-profile-key"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("mTLS-only config with profile key rejected: %v", err)
	}
}
