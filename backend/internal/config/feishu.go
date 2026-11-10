package config

import (
	"fmt"
	"net/url"
	"strings"
)

// FeishuConnectConfig configures native, tenant-scoped enterprise sign-in.
// Secrets are server-side only; public settings expose only the enabled flag.
type FeishuConnectConfig struct {
	Enabled           bool     `mapstructure:"enabled"`
	ClientID          string   `mapstructure:"client_id"`
	ClientSecret      string   `mapstructure:"client_secret"`
	RedirectURL       string   `mapstructure:"redirect_url"`
	AllowedTenantKeys []string `mapstructure:"allowed_tenant_keys"`
}

func ValidateFeishuConfig(c FeishuConnectConfig) error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "" {
		return fmt.Errorf("client_id and client_secret are required when enabled")
	}
	u, err := url.Parse(c.RedirectURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return fmt.Errorf("redirect_url must be an absolute callback URL without credentials, query or fragment")
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return fmt.Errorf("redirect_url must use HTTPS (HTTP is allowed only for loopback development)")
	}
	if len(c.AllowedTenantKeys) == 0 {
		return fmt.Errorf("allowed_tenant_keys is required when enabled")
	}
	for _, tenant := range c.AllowedTenantKeys {
		if strings.TrimSpace(tenant) == "" {
			return fmt.Errorf("allowed_tenant_keys must not contain empty entries")
		}
	}
	return nil
}
