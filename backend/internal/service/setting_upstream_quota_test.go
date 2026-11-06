//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeUpstreamQuotaSettings(t *testing.T) {
	for _, ids := range [][]int64{{0}, {-1}, make([]int64, 201)} {
		if _, err := normalizeUpstreamQuotaSettings(&UpstreamQuotaSettings{AccountIDs: ids}); err == nil {
			t.Fatal("invalid allowlist accepted")
		}
	}
	got, err := normalizeUpstreamQuotaSettings(&UpstreamQuotaSettings{Enabled: true, AccountIDs: []int64{2, 1, 2}})
	if err != nil || len(got.AccountIDs) != 2 || got.AccountIDs[0] != 1 {
		t.Fatalf("not canonical: %+v %v", got, err)
	}
}

func TestUpstreamQuotaSettingsFailClosed(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{}}
	svc := NewSettingService(repo, nil)
	got, err := svc.GetUpstreamQuotaSettings(context.Background())
	if err != nil || got.Enabled || len(got.AccountIDs) != 0 {
		t.Fatalf("missing settings must disable sharing: %#v %v", got, err)
	}
	for _, value := range []string{`invalid`, `{"enabled":true,"account_ids":[-1]}`} {
		repo.values[SettingKeyUpstreamQuota] = value
		if _, err = svc.GetUpstreamQuotaSettings(context.Background()); err == nil {
			t.Fatal("invalid stored config accepted")
		}
	}
	repo.getValueErr = errors.New("database unavailable")
	if _, err = svc.GetUpstreamQuotaSettings(context.Background()); err == nil {
		t.Fatal("database error ignored")
	}
	repo.getValueErr = nil
	want := &UpstreamQuotaSettings{Enabled: true, AccountIDs: []int64{2, 1, 2}}
	if err = svc.SetUpstreamQuotaSettings(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err = svc.GetUpstreamQuotaSettings(context.Background())
	if err != nil || !got.Enabled || len(got.AccountIDs) != 2 || got.AccountIDs[0] != 1 {
		t.Fatalf("roundtrip: %#v %v", got, err)
	}
}
