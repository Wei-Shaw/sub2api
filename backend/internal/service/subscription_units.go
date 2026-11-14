package service

import (
	"context"
	"encoding/json"
	"fmt"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

var quotaUnitIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,19}$`)

func defaultSubscriptionUnits(name string) []SubscriptionUnitConfig {
	return []SubscriptionUnitConfig{{ID: "points", Name: normalizeSubscriptionUnitName(name), UnitsPerUSD: 1}}
}

// Read existing single-name settings without losing the site's previous label.
func parseSubscriptionUnits(raw, legacyName string) []SubscriptionUnitConfig {
	var units []SubscriptionUnitConfig
	if json.Unmarshal([]byte(raw), &units) != nil || units == nil {
		return defaultSubscriptionUnits(legacyName)
	}
	normalized, err := ValidateSubscriptionUnits(units)
	if err != nil {
		return defaultSubscriptionUnits(legacyName)
	}
	return normalized
}

func ValidateSubscriptionUnits(units []SubscriptionUnitConfig) ([]SubscriptionUnitConfig, error) {
	if len(units) > 50 {
		return nil, infraerrors.BadRequest("INVALID_QUOTA_UNITS", "最多可配置 50 个额度单位")
	}
	out := make([]SubscriptionUnitConfig, 0, len(units))
	ids := map[string]bool{}
	names := map[string]bool{}
	for _, unit := range units {
		unit.Name = strings.TrimSpace(unit.Name)
		if !quotaUnitIDPattern.MatchString(unit.ID) || unit.ID == "usd" {
			return nil, infraerrors.BadRequest("INVALID_QUOTA_UNIT_ID", "额度单位标识无效或使用了保留标识 usd")
		}
		if ids[unit.ID] || names[strings.ToLower(unit.Name)] {
			return nil, infraerrors.BadRequest("DUPLICATE_QUOTA_UNIT", "额度单位名称或标识不能重复")
		}
		if unit.Name == "" || utf8.RuneCountInString(unit.Name) > 20 {
			return nil, infraerrors.BadRequest("INVALID_QUOTA_UNIT_NAME", "额度单位名称须为 1 到 20 个字符")
		}
		if math.IsNaN(unit.UnitsPerUSD) || math.IsInf(unit.UnitsPerUSD, 0) || unit.UnitsPerUSD <= 0 || unit.UnitsPerUSD > 1e12 {
			return nil, infraerrors.BadRequest("INVALID_QUOTA_UNIT_RATE", "换算比例必须大于 0 且不超过 1000000000000")
		}
		ids[unit.ID] = true
		names[strings.ToLower(unit.Name)] = true
		out = append(out, unit)
	}
	return out, nil
}

func (s *SettingService) GetSubscriptionUnits(ctx context.Context) ([]SubscriptionUnitConfig, error) {
	vals, err := s.settingRepo.GetMultiple(ctx, []string{SettingKeySubscriptionUnits, SettingKeySubscriptionUnitName})
	if err != nil {
		return nil, err
	}
	return parseSubscriptionUnits(vals[SettingKeySubscriptionUnits], vals[SettingKeySubscriptionUnitName]), nil
}

func (s *adminServiceImpl) validateGroupPricingUnit(ctx context.Context, id string) error {
	if id == "" || id == "usd" {
		return nil
	}
	if s.settingService == nil {
		if id == "points" {
			return nil
		}
		return infraerrors.BadRequest("INVALID_QUOTA_UNIT", "请选择已配置的额度单位")
	}
	units, err := s.settingService.GetSubscriptionUnits(ctx)
	if err != nil {
		return err
	}
	for _, u := range units {
		if u.ID == id {
			return nil
		}
	}
	return infraerrors.BadRequest("INVALID_QUOTA_UNIT", "额度单位不存在，请刷新后重新选择")
}

type quotaUnitUsageReader interface {
	CountGroupsUsingPricingUnits(context.Context, []string) (int, error)
}

func (s *SettingService) validateUnitRemoval(ctx context.Context, units []SubscriptionUnitConfig) error {
	old, err := s.GetSubscriptionUnits(ctx)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, u := range units {
		keep[u.ID] = true
	}
	removed := []string{}
	for _, u := range old {
		if !keep[u.ID] {
			removed = append(removed, u.ID)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	reader, ok := s.defaultSubGroupReader.(quotaUnitUsageReader)
	if !ok {
		if s.defaultSubGroupReader != nil {
			return fmt.Errorf("quota unit usage reader unavailable")
		}
		return nil
	}
	count, err := reader.CountGroupsUsingPricingUnits(ctx, removed)
	if err != nil {
		return err
	}
	if count > 0 {
		return infraerrors.BadRequest("QUOTA_UNIT_IN_USE", "该额度单位仍被分组使用，请先修改相关分组后再删除")
	}
	return nil
}

func normalizeSubscriptionUsageDisplay(value string) string {
	if value == "percentage" {
		return value
	}
	return "amount"
}

func validateSubscriptionUsageDisplay(value string) error {
	if value != "" && value != "amount" && value != "percentage" {
		return infraerrors.BadRequest("INVALID_SUBSCRIPTION_USAGE_DISPLAY", "订阅用量展示方式无效")
	}
	return nil
}
