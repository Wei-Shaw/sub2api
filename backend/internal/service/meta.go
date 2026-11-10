package service

import (
	"net/url"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
)

const (
	DefaultMetaTestModel = "muse-spark-1.3"
	MetaAuthKindOAuth    = "oauth"
	MetaAuthKindAPIKey   = "apikey"
)

const DefaultMetaBaseURL = meta.DefaultBaseURL

var DefaultMetaModelIDs = []string{
	"muse-spark-1.3",
	"muse-spark-1.3-contributor",
	"muse-spark-1.2",
	"muse-spark-1.2-contributor",
	"muse-spark-1.1",
	"muse-wonder-1.0",
	"muse-wonder-1.0-contributor",
	"muse-wonder-full-1.0",
	"muse-wonder-full-1.0-contributor",
}

func DefaultMetaProtocolRules() []ProtocolRule {
	return []ProtocolRule{{Pattern: "*", Protocol: APIProtocolResponses}}
}

func (a *Account) IsMeta() bool {
	return a != nil && a.Platform == PlatformMeta
}

func (a *Account) IsMetaOAuth() bool {
	return a.IsMeta() && a.Type == AccountTypeOAuth
}

func (a *Account) GetMetaAPIKey() string {
	if !a.IsMeta() {
		return ""
	}
	key := strings.TrimSpace(a.GetCredential("api_key"))
	if strings.HasPrefix(key, "dca:") {
		return ""
	}
	return key
}

func (a *Account) GetMetaDCAToken() string {
	if !a.IsMeta() {
		return ""
	}
	if token := strings.TrimSpace(a.GetCredential("dca_token")); token != "" {
		return token
	}
	if token := strings.TrimSpace(a.GetCredential("access_token")); strings.HasPrefix(token, "dca:") {
		return token
	}
	return ""
}

func (a *Account) GetMetaBaseURL() string {
	baseURL := ""
	if a != nil {
		baseURL = strings.TrimSpace(a.GetCredential("base_url"))
	}
	if baseURL == "" {
		return DefaultMetaBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return DefaultMetaBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}

func (a *Account) NeedsMetaKeyMint() bool {
	return a.IsMeta() && a.GetMetaAPIKey() == "" && a.GetMetaDCAToken() != ""
}
