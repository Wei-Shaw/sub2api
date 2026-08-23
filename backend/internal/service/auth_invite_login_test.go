package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestValidateInviteLoginDeviceInputRequiresBoundDeviceHash(t *testing.T) {
	err := validateInviteLoginDeviceInput(InviteLoginInput{DeviceHash: strings.Repeat("a", 63)})
	if err != ErrDeviceHashInvalid {
		t.Fatalf("validateInviteLoginDeviceInput() error = %v, want %v", err, ErrDeviceHashInvalid)
	}
}

type inviteLoginDeviceRepoProbe struct {
	device               *UserDevice
	getByDeviceCodeCalls int
	lastLoginCalls       int
	lastLoginID          int64
}

func (p *inviteLoginDeviceRepoProbe) GetByDeviceHash(context.Context, string) (*UserDevice, error) {
	panic("unexpected GetByDeviceHash call")
}

func (p *inviteLoginDeviceRepoProbe) GetByDeviceCode(_ context.Context, code string) (*UserDevice, error) {
	p.getByDeviceCodeCalls++
	if code != "DLG-ABCD-EFGH-JKLM" {
		return nil, ErrUserDeviceNotFound
	}
	return p.device, nil
}

func (p *inviteLoginDeviceRepoProbe) GetByLoginRedeemCodeID(context.Context, int64) (*UserDevice, error) {
	panic("unexpected GetByLoginRedeemCodeID call")
}

func (p *inviteLoginDeviceRepoProbe) GetByClaimRedeemCodeID(context.Context, int64) (*UserDevice, error) {
	panic("unexpected GetByClaimRedeemCodeID call")
}

func (p *inviteLoginDeviceRepoProbe) Create(context.Context, *UserDevice) error {
	panic("unexpected Create call")
}

func (p *inviteLoginDeviceRepoProbe) UpdateLastClaimedAt(context.Context, int64, time.Time) error {
	panic("unexpected UpdateLastClaimedAt call")
}

func (p *inviteLoginDeviceRepoProbe) UpdateLastLoginAt(_ context.Context, id int64, _ time.Time) error {
	p.lastLoginCalls++
	p.lastLoginID = id
	return nil
}

func TestResolveDirectInviteDeviceCodeNormalizesCodeAndUsesDeviceRepository(t *testing.T) {
	probe := &inviteLoginDeviceRepoProbe{device: &UserDevice{ID: 17, UserID: 23, Status: UserDeviceStatusActive}}
	svc := &AuthService{inviteLoginDeviceRepo: probe}

	device, err := svc.resolveDirectInviteDeviceCode(context.Background(), " dlg-abcd-efgh-jklm ")
	if err != nil {
		t.Fatalf("resolveDirectInviteDeviceCode() error = %v", err)
	}
	if device == nil || device.ID != 17 || device.UserID != 23 {
		t.Fatalf("resolveDirectInviteDeviceCode() = %#v, want device 17 owned by user 23", device)
	}
	if probe.getByDeviceCodeCalls != 1 {
		t.Fatalf("GetByDeviceCode calls = %d, want 1", probe.getByDeviceCodeCalls)
	}
}

func TestInviteLoginDirectDeviceCodeAllowsWebWithoutDeviceHash(t *testing.T) {
	probe := &inviteLoginDeviceRepoProbe{device: &UserDevice{ID: 17, UserID: 23, Status: UserDeviceStatusActive}}
	svc := &AuthService{inviteLoginDeviceRepo: probe}

	_, err := svc.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: "dlg-abcd-efgh-jklm",
		ClientKind:     "web",
	})
	if err != ErrServiceUnavailable {
		t.Fatalf("InviteLogin() error = %v, want %v after direct device resolution", err, ErrServiceUnavailable)
	}
}

func TestInviteLoginDirectDeviceCodeRequiresDeviceHashForNonWebClient(t *testing.T) {
	probe := &inviteLoginDeviceRepoProbe{device: &UserDevice{ID: 17, UserID: 23, Status: UserDeviceStatusActive}}
	svc := &AuthService{inviteLoginDeviceRepo: probe}

	_, err := svc.InviteLogin(context.Background(), InviteLoginInput{
		InvitationCode: "DLG-ABCD-EFGH-JKLM",
		ClientKind:     "desktop",
	})
	if err != ErrDeviceHashRequired {
		t.Fatalf("InviteLogin() error = %v, want %v", err, ErrDeviceHashRequired)
	}
}

func TestValidateInviteLoginDeviceInputForClientAllowsWebWithoutDeviceHashOnly(t *testing.T) {
	if _, err := validateInviteLoginDeviceInputForClient(InviteLoginInput{ClientKind: "web"}); err != nil {
		t.Fatalf("web device input without hash error = %v, want nil", err)
	}
	if _, err := validateInviteLoginDeviceInputForClient(InviteLoginInput{ClientKind: "desktop"}); err != ErrDeviceHashRequired {
		t.Fatalf("non-web device input without hash error = %v, want %v", err, ErrDeviceHashRequired)
	}
}
