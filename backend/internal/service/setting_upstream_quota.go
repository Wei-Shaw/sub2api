package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const SettingKeyUpstreamQuota = "upstream_quota_settings"
const MaxSharedQuotaAccounts = 200

// UpstreamQuotaSettings is opt-in: only explicitly selected accounts are shared.
type UpstreamQuotaSettings struct {
	Enabled    bool    `json:"enabled"`
	AccountIDs []int64 `json:"account_ids"`
}

func normalizeUpstreamQuotaSettings(in *UpstreamQuotaSettings) (*UpstreamQuotaSettings, error) {
	if in == nil || len(in.AccountIDs) > MaxSharedQuotaAccounts {
		return nil, fmt.Errorf("select at most %d accounts", MaxSharedQuotaAccounts)
	}
	out := &UpstreamQuotaSettings{Enabled: in.Enabled, AccountIDs: make([]int64, 0, len(in.AccountIDs))}
	seen := make(map[int64]bool)
	for _, id := range in.AccountIDs {
		if id <= 0 {
			return nil, errors.New("account IDs must be positive")
		}
		if !seen[id] {
			out.AccountIDs = append(out.AccountIDs, id)
			seen[id] = true
		}
	}
	sort.Slice(out.AccountIDs, func(i, j int) bool { return out.AccountIDs[i] < out.AccountIDs[j] })
	return out, nil
}

func (s *SettingService) GetUpstreamQuotaSettings(ctx context.Context) (*UpstreamQuotaSettings, error) {
	value, err := s.settingRepo.GetValue(ctx, SettingKeyUpstreamQuota)
	if errors.Is(err, ErrSettingNotFound) || (err == nil && strings.TrimSpace(value) == "") {
		return &UpstreamQuotaSettings{AccountIDs: []int64{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var settings UpstreamQuotaSettings
	if err = json.Unmarshal([]byte(value), &settings); err != nil {
		return nil, err
	}
	return normalizeUpstreamQuotaSettings(&settings)
}

func (s *SettingService) SetUpstreamQuotaSettings(ctx context.Context, in *UpstreamQuotaSettings) error {
	settings, err := normalizeUpstreamQuotaSettings(in)
	if err != nil {
		return err
	}
	value, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.settingRepo.Set(ctx, SettingKeyUpstreamQuota, string(value))
}
