package repository

import (
	"strings"
	"testing"
)

func integrationDockerUnavailablePolicy(ci, allowSkip string) (int, string) {
	if strings.TrimSpace(ci) == "" && strings.TrimSpace(allowSkip) == "1" {
		return 0, "INTEGRATION_SKIPPED_DOCKER_UNAVAILABLE"
	}
	return 1, "INTEGRATION_REQUIRED_DOCKER_UNAVAILABLE"
}

func TestIntegrationDockerUnavailablePolicy(t *testing.T) {
	tests := []struct {
		name      string
		ci        string
		allowSkip string
		wantExit  int
		wantEvent string
	}{
		{name: "local defaults fail closed", wantExit: 1, wantEvent: "INTEGRATION_REQUIRED_DOCKER_UNAVAILABLE"},
		{name: "CI cannot skip", ci: "true", allowSkip: "1", wantExit: 1, wantEvent: "INTEGRATION_REQUIRED_DOCKER_UNAVAILABLE"},
		{name: "explicit local diagnostic skip", allowSkip: "1", wantExit: 0, wantEvent: "INTEGRATION_SKIPPED_DOCKER_UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exitCode, event := integrationDockerUnavailablePolicy(tt.ci, tt.allowSkip)
			if exitCode != tt.wantExit || event != tt.wantEvent {
				t.Fatalf("policy(%q,%q) = %d/%q, want %d/%q", tt.ci, tt.allowSkip, exitCode, event, tt.wantExit, tt.wantEvent)
			}
		})
	}
}
