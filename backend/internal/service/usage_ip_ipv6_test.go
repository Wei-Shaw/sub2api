package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type usageIPSettingRepoStub struct {
	values map[string]string
}

func (s *usageIPSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *usageIPSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (s *usageIPSettingRepoStub) Set(context.Context, string, string) error {
	panic("unexpected Set call")
}

func (s *usageIPSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple call")
}

func (s *usageIPSettingRepoStub) SetMultiple(_ context.Context, settings map[string]string) error {
	for key, value := range settings {
		s.values[key] = value
	}
	return nil
}

func (s *usageIPSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	panic("unexpected GetAll call")
}

func (s *usageIPSettingRepoStub) Delete(context.Context, string) error {
	panic("unexpected Delete call")
}

func TestNormalizeUsageIPStatsKeyUsesIPv6Slash64(t *testing.T) {
	first := normalizeUsageIPStatsKey("240e:387:92f:8410:60e5:4277:d6c8:99fa")
	second := normalizeUsageIPStatsKey("240e:387:92f:8410:987d:e27d:9557:8ee0")
	if first != "240e:387:92f:8410::/64" {
		t.Fatalf("first key = %q, want IPv6 /64", first)
	}
	if second != first {
		t.Fatalf("second key = %q, want same /64 as first %q", second, first)
	}
}

func TestNormalizeUsageIPProbeTargetSupportsIPv6CIDR(t *testing.T) {
	target, ok := normalizeUsageIPProbeTarget("240e:387:92f:8410:60e5:4277:d6c8:99fa/64")
	if !ok {
		t.Fatal("expected valid IPv6 CIDR target")
	}
	if target.Key != "240e:387:92f:8410::/64" {
		t.Fatalf("key = %q, want canonical /64", target.Key)
	}
	if target.ProbeIP != "240e:387:92f:8410::" {
		t.Fatalf("probe IP = %q, want network representative", target.ProbeIP)
	}
}

func TestUsageIPPatternInBlocklistMatchesIPv6Subnet(t *testing.T) {
	blocklist := []string{"240e:387:92f:8410::/64"}
	if !usageIPPatternInBlocklist("240e:387:92f:8410::/64", blocklist) {
		t.Fatal("expected identical /64 to match blocklist")
	}
	if !usageIPPatternInBlocklist("240e:387:92f:8410:987d:e27d:9557:8ee0", blocklist) {
		t.Fatal("expected IP inside /64 to match blocklist")
	}
	if usageIPPatternInBlocklist("240e:387:92f:8411::/64", blocklist) {
		t.Fatal("did not expect adjacent /64 to match blocklist")
	}
}

func TestNormalizeAPIRequestIPBlockCandidateUsesIPv6Slash64(t *testing.T) {
	got := normalizeAPIRequestIPBlockCandidate("240e:387:92f:8410:60e5:4277:d6c8:99fa")
	if got != "240e:387:92f:8410::/64" {
		t.Fatalf("candidate = %q, want IPv6 /64", got)
	}
}

func TestRemoveAPIRequestIPBlocklistIPsUsesNormalizedCandidates(t *testing.T) {
	repo := &usageIPSettingRepoStub{values: map[string]string{}}
	initial := []string{"240e:387:92f:8410::/64", "1.2.3.4", "5.6.7.8"}
	payload, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	repo.values[SettingKeyAPIRequestIPBlocklist] = string(payload)
	cfg := &config.Config{}
	svc := NewSettingService(repo, cfg)

	removed, err := svc.RemoveAPIRequestIPBlocklistIPs(context.Background(), []string{
		"240e:387:92f:8410:60e5:4277:d6c8:99fa",
		"1.2.3.4",
	})
	if err != nil {
		t.Fatalf("remove blocklist IPs failed: %v", err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}

	var remaining []string
	if err := json.Unmarshal([]byte(repo.values[SettingKeyAPIRequestIPBlocklist]), &remaining); err != nil {
		t.Fatalf("unmarshal remaining blocklist: %v", err)
	}
	if len(remaining) != 1 || remaining[0] != "5.6.7.8" {
		t.Fatalf("remaining = %#v, want only 5.6.7.8", remaining)
	}
	if got := cfg.Security.APIRequestIPBlocklist; len(got) != 1 || got[0] != "5.6.7.8" {
		t.Fatalf("config blocklist = %#v, want only 5.6.7.8", got)
	}
}

func TestRemoveAPIRequestIPBlocklistIPsRemovesContainingCIDR(t *testing.T) {
	repo := &usageIPSettingRepoStub{values: map[string]string{}}
	initial := []string{
		"240e:387:92f:8410::/63",
		"240e:387:92f:8420::/64",
		"5.6.7.8",
	}
	payload, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	repo.values[SettingKeyAPIRequestIPBlocklist] = string(payload)
	cfg := &config.Config{}
	svc := NewSettingService(repo, cfg)

	removed, err := svc.RemoveAPIRequestIPBlocklistIPs(context.Background(), []string{
		"240e:387:92f:8410:60e5:4277:d6c8:99fa",
	})
	if err != nil {
		t.Fatalf("remove blocklist IPs failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}

	var remaining []string
	if err := json.Unmarshal([]byte(repo.values[SettingKeyAPIRequestIPBlocklist]), &remaining); err != nil {
		t.Fatalf("unmarshal remaining blocklist: %v", err)
	}
	want := []string{"240e:387:92f:8420::/64", "5.6.7.8"}
	if len(remaining) != len(want) {
		t.Fatalf("remaining = %#v, want %#v", remaining, want)
	}
	for i := range want {
		if remaining[i] != want[i] {
			t.Fatalf("remaining = %#v, want %#v", remaining, want)
		}
	}
	if got := cfg.Security.APIRequestIPBlocklist; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("config blocklist = %#v, want %#v", got, want)
	}
}
