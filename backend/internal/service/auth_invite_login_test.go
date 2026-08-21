package service

import (
	"strings"
	"testing"
)

func TestValidateInviteLoginDeviceInputRequiresBoundDeviceHash(t *testing.T) {
	err := validateInviteLoginDeviceInput(InviteLoginInput{DeviceHash: strings.Repeat("a", 63)})
	if err != ErrDeviceHashInvalid {
		t.Fatalf("validateInviteLoginDeviceInput() error = %v, want %v", err, ErrDeviceHashInvalid)
	}
}
