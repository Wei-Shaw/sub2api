package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// GroupUpstreamPlanOption 分组可选上游订阅档位（系统设置可配置）。
type GroupUpstreamPlanOption struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

// DefaultGroupUpstreamPlansSeed 内置种子（K1）：首次空配置写回 settings。
func DefaultGroupUpstreamPlansSeed() map[string][]GroupUpstreamPlanOption {
	return map[string][]GroupUpstreamPlanOption{
		PlatformOpenAI: {
			{Code: "free", Label: "Free"},
			{Code: "plus", Label: "Plus"},
			{Code: "team", Label: "Team"},
			{Code: "pro", Label: "Pro"},
		},
		PlatformGrok: {
			{Code: "free", Label: "Grok Free"},
			{Code: "basic", Label: "Basic"},
			{Code: "supergrok", Label: "SuperGrok"},
			{Code: "supergrokheavy", Label: "SuperGrok Heavy"},
		},
		PlatformAntigravity: {
			{Code: "free-tier", Label: "Free"},
			{Code: "g1-pro-tier", Label: "Pro"},
			{Code: "g1-ultra-tier", Label: "Ultra"},
		},
		PlatformAnthropic: {},
		PlatformGemini:    {},
	}
}

// NormalizeUpstreamPlanCode 规范化档位 code（trim + lower）。
func NormalizeUpstreamPlanCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

// NormalizeGroupUpstreamPlans 规范化并校验配置 map（code 非空、同平台唯一）。
func NormalizeGroupUpstreamPlans(in map[string][]GroupUpstreamPlanOption) (map[string][]GroupUpstreamPlanOption, error) {
	if in == nil {
		return map[string][]GroupUpstreamPlanOption{}, nil
	}
	out := make(map[string][]GroupUpstreamPlanOption, len(in))
	for platform, opts := range in {
		platform = strings.TrimSpace(strings.ToLower(platform))
		if platform == "" {
			continue
		}
		if platform == PlatformComposite {
			// composite 不提供档位配置
			continue
		}
		seen := make(map[string]struct{}, len(opts))
		normalized := make([]GroupUpstreamPlanOption, 0, len(opts))
		for _, o := range opts {
			code := NormalizeUpstreamPlanCode(o.Code)
			if code == "" {
				return nil, infraerrors.New(http.StatusBadRequest, "INVALID_GROUP_UPSTREAM_PLANS", "upstream plan code must not be empty")
			}
			if _, dup := seen[code]; dup {
				return nil, infraerrors.Newf(http.StatusBadRequest, "INVALID_GROUP_UPSTREAM_PLANS", "duplicate upstream plan code %q for platform %s", code, platform)
			}
			seen[code] = struct{}{}
			label := strings.TrimSpace(o.Label)
			if label == "" {
				label = code
			}
			normalized = append(normalized, GroupUpstreamPlanOption{Code: code, Label: label})
		}
		out[platform] = normalized
	}
	// 确保五平台 key 存在（缺失补空切片，避免前端 undefined）
	for _, p := range []string{PlatformAnthropic, PlatformOpenAI, PlatformGemini, PlatformAntigravity, PlatformGrok} {
		if _, ok := out[p]; !ok {
			out[p] = []GroupUpstreamPlanOption{}
		}
	}
	return out, nil
}

// ParseGroupUpstreamPlansJSON 解析设置 JSON；空串返回 empty map。
func ParseGroupUpstreamPlansJSON(raw string) (map[string][]GroupUpstreamPlanOption, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string][]GroupUpstreamPlanOption{}, nil
	}
	var m map[string][]GroupUpstreamPlanOption
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, fmt.Errorf("parse group_upstream_plans: %w", err)
	}
	return NormalizeGroupUpstreamPlans(m)
}

// MarshalGroupUpstreamPlansJSON 序列化配置。
func MarshalGroupUpstreamPlansJSON(plans map[string][]GroupUpstreamPlanOption) (string, error) {
	normalized, err := NormalizeGroupUpstreamPlans(plans)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("marshal group_upstream_plans: %w", err)
	}
	return string(b), nil
}

// GetGroupUpstreamPlans 读取配置；raw 为空或不存在时用种子写回并返回。
func (s *SettingService) GetGroupUpstreamPlans(ctx context.Context) (map[string][]GroupUpstreamPlanOption, error) {
	if s == nil || s.settingRepo == nil {
		return DefaultGroupUpstreamPlansSeed(), nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyGroupUpstreamPlans)
	if err != nil {
		// 不存在或读失败：尝试 seed
		return s.seedGroupUpstreamPlans(ctx)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return s.seedGroupUpstreamPlans(ctx)
	}
	plans, err := ParseGroupUpstreamPlansJSON(raw)
	if err != nil {
		// 损坏数据：不覆盖，返回种子内存副本供读侧可用
		return DefaultGroupUpstreamPlansSeed(), nil
	}
	return plans, nil
}

func (s *SettingService) seedGroupUpstreamPlans(ctx context.Context) (map[string][]GroupUpstreamPlanOption, error) {
	seed := DefaultGroupUpstreamPlansSeed()
	raw, err := MarshalGroupUpstreamPlansJSON(seed)
	if err != nil {
		return seed, nil
	}
	if s.settingRepo != nil {
		_ = s.settingRepo.Set(ctx, SettingKeyGroupUpstreamPlans, raw)
	}
	return seed, nil
}

// ListGroupUpstreamPlansForPlatform 返回某平台可选档位；composite 恒为空。
func (s *SettingService) ListGroupUpstreamPlansForPlatform(ctx context.Context, platform string) ([]GroupUpstreamPlanOption, error) {
	platform = strings.TrimSpace(strings.ToLower(platform))
	if platform == "" || platform == PlatformComposite {
		return nil, nil
	}
	plans, err := s.GetGroupUpstreamPlans(ctx)
	if err != nil {
		return nil, err
	}
	return plans[platform], nil
}

// ValidateGroupUpstreamPlan 校验分组档位：空 OK；composite 非空拒绝；非空必须 ∈ 配置。
// 返回规范化后的 code（空串表示未指定）。
func (s *SettingService) ValidateGroupUpstreamPlan(ctx context.Context, platform, plan string) (string, error) {
	code := NormalizeUpstreamPlanCode(plan)
	platform = strings.TrimSpace(strings.ToLower(platform))
	if platform == "" {
		platform = PlatformAnthropic
	}
	if code == "" {
		return "", nil
	}
	if platform == PlatformComposite {
		return "", infraerrors.New(http.StatusBadRequest, "INVALID_GROUP_UPSTREAM_PLAN", "composite groups cannot have upstream_plan")
	}
	opts, err := s.ListGroupUpstreamPlansForPlatform(ctx, platform)
	if err != nil {
		return "", err
	}
	for _, o := range opts {
		if o.Code == code {
			return code, nil
		}
	}
	return "", infraerrors.Newf(http.StatusBadRequest, "INVALID_GROUP_UPSTREAM_PLAN", "upstream_plan %q is not allowed for platform %s", code, platform)
}
