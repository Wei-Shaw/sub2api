package tlsfingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// CacheKey 返回 TLS 指纹模板的稳定缓存键。
// Transport 缓存必须包含完整指纹参数，避免账号切换模板后复用旧握手配置。
func CacheKey(profile *Profile) string {
	if profile == nil {
		return "none"
	}
	payload := struct {
		Name                string   `json:"name"`
		CipherSuites        []uint16 `json:"cipher_suites"`
		Curves              []uint16 `json:"curves"`
		PointFormats        []uint16 `json:"point_formats"`
		EnableGREASE        bool     `json:"enable_grease"`
		SignatureAlgorithms []uint16 `json:"signature_algorithms"`
		ALPNProtocols       []string `json:"alpn_protocols"`
		SupportedVersions   []uint16 `json:"supported_versions"`
		KeyShareGroups      []uint16 `json:"key_share_groups"`
		PSKModes            []uint16 `json:"psk_modes"`
		Extensions          []uint16 `json:"extensions"`
	}{
		Name:                profile.Name,
		CipherSuites:        profile.CipherSuites,
		Curves:              profile.Curves,
		PointFormats:        profile.PointFormats,
		EnableGREASE:        profile.EnableGREASE,
		SignatureAlgorithms: profile.SignatureAlgorithms,
		ALPNProtocols:       profile.ALPNProtocols,
		SupportedVersions:   profile.SupportedVersions,
		KeyShareGroups:      profile.KeyShareGroups,
		PSKModes:            profile.PSKModes,
		Extensions:          profile.Extensions,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "marshal-error"
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
