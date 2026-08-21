package service

import (
	"strings"
	"testing"
)

func TestValidateVClawClaimRequestRejectsInvalidDeviceHash(t *testing.T) {
	err := validateVClawClaimRequest(VClawClaimRequest{
		Device: VClawDeviceInput{
			DeviceHash:         strings.Repeat("a", 63),
			FingerprintVersion: 1,
			Platform:           "linux",
			Arch:               "amd64",
		},
	})
	if err != ErrDeviceHashInvalid {
		t.Fatalf("validateVClawClaimRequest() error = %v, want %v", err, ErrDeviceHashInvalid)
	}
}

func TestGenerateDeviceLoginCodeUsesDLGFormat(t *testing.T) {
	code, err := generateDeviceLoginCode()
	if err != nil {
		t.Fatalf("generateDeviceLoginCode() error = %v", err)
	}
	parts := strings.Split(code, "-")
	if len(parts) != 4 || parts[0] != "DLG" {
		t.Fatalf("generateDeviceLoginCode() = %q, want DLG-XXXX-XXXX-XXXX", code)
	}
	for _, part := range parts[1:] {
		if len(part) != 4 {
			t.Fatalf("generateDeviceLoginCode() part %q has length %d, want 4", part, len(part))
		}
	}
}
