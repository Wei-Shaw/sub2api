package service

import (
	"net/http"
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

var DefaultMetaModelIDs = meta.DefaultModelIDs()

func DefaultMetaProtocolRules() []ProtocolRule {
	return []ProtocolRule{{Pattern: "*", Protocol: APIProtocolResponses}}
}

func isMetaImageModel(model string) bool {
	return meta.IsImageModelID(model)
}

func applyMetaImagesClientIdentity(account *Account, header http.Header) {
	if !account.IsMeta() {
		return
	}
	stripOpenAIClientCodexIdentityHeaders(header)
	meta.ApplyClientIdentity(header, "")
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
