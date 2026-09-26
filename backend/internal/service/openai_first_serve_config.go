package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Stored in accounts.extra; omitted reuse scope shares the account HTTP combination.
type OpenAIFirstServeConfig struct {
	ReuseScope    string `json:"reuse_scope"`
	RotateSeconds int    `json:"rotate_seconds"`
	// Legacy fields are retained for JSON compatibility. They no longer affect
	// first serve rotation decisions.
	TTLMinutes      int     `json:"ttl_minutes"`
	TTFTSeconds     int     `json:"ttft_seconds"`
	MaxSwitches     int     `json:"max_switches"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	ProxyMode       string  `json:"proxy_mode"`
	ProxyIDs        []int64 `json:"proxy_ids"`
}

func defaultOpenAIFirstServeConfig() OpenAIFirstServeConfig {
	return OpenAIFirstServeConfig{ReuseScope: "account", RotateSeconds: 240, TTLMinutes: 30, TTFTSeconds: 15, MaxSwitches: 3, CooldownSeconds: 0, ProxyMode: "all", ProxyIDs: []int64{}}
}

func (a *Account) firstServeConfig() (OpenAIFirstServeConfig, error) {
	cfg := defaultOpenAIFirstServeConfig()
	if a == nil {
		return cfg, nil
	}
	raw, exists := a.Extra["openai_first_serve"]
	if !exists {
		return cfg, nil
	}
	invalid := func(detail string) (OpenAIFirstServeConfig, error) {
		return cfg, infraerrors.BadRequest("FIRST_SERVE_CONFIG_INVALID", fmt.Sprintf("账号 %s：首服模式配置无效（%s），请修改后保存。", a.Name, detail))
	}
	data, err := json.Marshal(raw)
	if err != nil || len(data) == 0 || data[0] != '{' {
		return invalid("openai_first_serve 必须是对象")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return invalid("配置格式错误")
	}
	for key, value := range fields {
		if bytes.Equal(value, []byte("null")) {
			return invalid(key + " 不能为 null")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return invalid("字段名或类型错误，时间和次数必须为整数")
	}
	for _, field := range []struct {
		name            string
		value, min, max int
	}{
		{"rotate_seconds", cfg.RotateSeconds, 1, 86400},
		{"ttl_minutes", cfg.TTLMinutes, 1, 1440},
		{"ttft_seconds", cfg.TTFTSeconds, 1, 300},
		{"max_switches", cfg.MaxSwitches, 1, 20},
		{"cooldown_seconds", cfg.CooldownSeconds, 0, 3600},
	} {
		if field.value < field.min || field.value > field.max {
			return invalid(fmt.Sprintf("%s 范围为 %d–%d", field.name, field.min, field.max))
		}
	}
	if cfg.ReuseScope != "session" && cfg.ReuseScope != "account" {
		return invalid("reuse_scope 必须为 session 或 account")
	}
	if cfg.ProxyMode != "all" && cfg.ProxyMode != "selected" {
		return invalid("proxy_mode 必须为 all 或 selected")
	}
	if len(cfg.ProxyIDs) > 1000 {
		return invalid("指定代理不能超过 1000 个")
	}
	for _, id := range cfg.ProxyIDs {
		if id <= 0 || id > 9007199254740991 {
			return invalid("proxy_ids 必须为有效的正整数 ID")
		}
	}
	slices.Sort(cfg.ProxyIDs)
	cfg.ProxyIDs = slices.Compact(cfg.ProxyIDs)
	if cfg.ProxyMode == "selected" && len(cfg.ProxyIDs) < 2 {
		return invalid("请指定至少两个不同出口的代理")
	}
	if cfg.ProxyMode == "all" && len(cfg.ProxyIDs) > 0 {
		return invalid("全组模式不能同时指定代理，请选择 selected 或清空 proxy_ids")
	}
	return cfg, nil
}

func (c OpenAIFirstServeConfig) ttl() time.Duration {
	return time.Duration(c.RotateSeconds) * time.Second
}
func (c OpenAIFirstServeConfig) allows(id int64) bool {
	return c.ProxyMode == "all" || slices.Contains(c.ProxyIDs, id)
}

// New settings must not silently reuse a connection created with an old proxy policy.
func (c OpenAIFirstServeConfig) key(groupID int64) string {
	data, _ := json.Marshal(c)
	return fmt.Sprintf("%d:%x", groupID, sha256.Sum256(data))
}

func validateOpenAIFirstServeProxies(ctx context.Context, account *Account) error {
	if !account.IsOpenAIFirstServe() {
		return nil
	}
	cfg, err := account.firstServeConfig()
	if err != nil || cfg.ProxyMode != "selected" {
		return err
	}
	defaultProxyGroupResolver.RLock()
	reader, ok := defaultProxyGroupResolver.resolver.(interface {
		Get(context.Context, int64) (*ProxyGroup, error)
	})
	defaultProxyGroupResolver.RUnlock()
	if !ok || account.ProxyGroupID == nil {
		return infraerrors.BadRequest("FIRST_SERVE_PROXY_GROUP_UNAVAILABLE", fmt.Sprintf("账号 %s：无法验证首服代理范围，请检查代理组后重试。", account.Name))
	}
	group, err := reader.Get(ctx, *account.ProxyGroupID)
	if err != nil {
		return err
	}
	for _, id := range cfg.ProxyIDs {
		if group == nil || !slices.Contains(group.ProxyIDs, id) {
			return infraerrors.BadRequest("FIRST_SERVE_PROXY_OUTSIDE_GROUP", fmt.Sprintf("账号 %s：代理 #%d 不属于绑定的代理组，请重新选择代理。", account.Name, id))
		}
	}
	return nil
}

// Mirror the create form's default group selection for API and file imports.
// If no group is ready, retain the enabled setting so it can be completed later.
func assignDefaultFirstServeProxyGroup(ctx context.Context, account *Account) error {
	if !account.IsOpenAIFirstServe() || (account.ProxyGroupID != nil && *account.ProxyGroupID > 0) {
		return nil
	}
	defaultProxyGroupResolver.RLock()
	reader, ok := defaultProxyGroupResolver.resolver.(interface {
		ListAll(context.Context) ([]ProxyGroup, error)
	})
	defaultProxyGroupResolver.RUnlock()
	if !ok {
		return nil
	}
	groups, err := reader.ListAll(ctx)
	if err != nil {
		return err
	}
	cfg, err := account.firstServeConfig()
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.Status != ProxyGroupStatusActive || group.AvailableMemberCount < 2 {
			continue
		}
		if account.ProxyID != nil && *account.ProxyID > 0 && !slices.Contains(group.ProxyIDs, *account.ProxyID) {
			continue
		}
		if cfg.ProxyMode == "selected" && !allFirstServeProxiesInGroup(cfg.ProxyIDs, group.ProxyIDs) {
			continue
		}
		id := group.ID
		account.ProxyGroupID, account.ProxyID, account.Proxy = &id, nil, nil
		return nil
	}
	return nil
}

func allFirstServeProxiesInGroup(ids, members []int64) bool {
	for _, id := range ids {
		if !slices.Contains(members, id) {
			return false
		}
	}
	return true
}
