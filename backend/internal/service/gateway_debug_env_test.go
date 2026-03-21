package service

import "testing"

func TestDebugGatewayBodyLoggingEnabled(t *testing.T) {
	t.Run("default disabled", func(t *testing.T) {
		t.Setenv(debugGatewayBodyEnv, "")
		if debugGatewayBodyLoggingEnabled() {
			t.Fatalf("expected debug gateway body logging to be disabled by default")
		}
	})

	t.Run("enabled with true-like values", func(t *testing.T) {
		for _, value := range []string{"1", "true", "TRUE", "yes", "on"} {
			t.Run(value, func(t *testing.T) {
				t.Setenv(debugGatewayBodyEnv, value)
				if !debugGatewayBodyLoggingEnabled() {
					t.Fatalf("expected debug gateway body logging to be enabled for %q", value)
				}
			})
		}
	})

	t.Run("disabled with other values", func(t *testing.T) {
		for _, value := range []string{"0", "false", "off", "debug"} {
			t.Run(value, func(t *testing.T) {
				t.Setenv(debugGatewayBodyEnv, value)
				if debugGatewayBodyLoggingEnabled() {
					t.Fatalf("expected debug gateway body logging to be disabled for %q", value)
				}
			})
		}
	})
}
