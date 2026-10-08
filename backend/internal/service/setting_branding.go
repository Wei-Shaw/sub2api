package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"regexp"
	"strings"
)

// brandingAssetPathPrefix deliberately lives under /api/v1/settings/public/ so
// that existing reverse proxies that already forward /api to the backend keep
// working without any additional location rule.
const brandingAssetPathPrefix = "/api/v1/settings/public/logo-"

var brandingAssetPathPattern = regexp.MustCompile(`^/api/v1/settings/public/logo-[0-9a-f]{64}\.(png|jpg|gif|webp|ico|svg)$`)

// siteBrandingAsset is an immutable, content-addressed representation of an
// inline site logo. Its path changes whenever its content changes.
type siteBrandingAsset struct {
	source      string
	path        string
	contentType string
	content     []byte
}

// publicSiteLogo returns a browser-accessible logo URL. External and relative
// URLs keep their existing behavior; valid inline image data becomes a
// same-origin, cacheable asset.
func (s *SettingService) publicSiteLogo(raw string) string {
	if asset := s.siteBrandingAsset.Load(); asset != nil && asset.source == raw {
		return asset.path
	}
	asset := siteBrandingAssetFromDataURL(raw)
	s.siteBrandingAsset.Store(asset)
	if asset != nil {
		return asset.path
	}
	return raw
}

func (s *SettingService) refreshSiteBrandingAsset(raw string) {
	s.siteBrandingAsset.Store(siteBrandingAssetFromDataURL(raw))
}

// GetSiteBrandingAsset returns the current content-addressed logo when the
// requested path matches exactly. A cache miss reloads site_logo so a new URL
// also works on a cold process or an instance that has not seen the update yet.
func (s *SettingService) GetSiteBrandingAsset(ctx context.Context, requestedPath string) ([]byte, string, bool) {
	if s == nil || !brandingAssetPathPattern.MatchString(requestedPath) {
		return nil, "", false
	}

	asset := s.siteBrandingAsset.Load()
	if asset == nil || requestedPath != asset.path {
		if s.settingRepo == nil {
			return nil, "", false
		}
		raw, err := s.settingRepo.GetValue(ctx, SettingKeySiteLogo)
		if err != nil {
			return nil, "", false
		}
		asset = siteBrandingAssetFromDataURL(raw)
		s.siteBrandingAsset.Store(asset)
	}
	if asset == nil || requestedPath != asset.path {
		return nil, "", false
	}
	return asset.content, asset.contentType, true
}

func siteBrandingAssetFromDataURL(raw string) *siteBrandingAsset {
	value := strings.TrimSpace(raw)
	comma := strings.IndexByte(value, ',')
	if comma <= 0 {
		return nil
	}

	metadata := strings.ToLower(value[:comma])
	if !strings.HasPrefix(metadata, "data:image/") || !strings.HasSuffix(metadata, ";base64") {
		return nil
	}

	content, err := base64.StdEncoding.DecodeString(value[comma+1:])
	if err != nil || len(content) == 0 {
		return nil
	}
	contentType, extension, ok := brandingImageType(content)
	if !ok {
		return nil
	}

	digest := sha256.Sum256(content)
	return &siteBrandingAsset{
		source:      raw,
		path:        brandingAssetPathPrefix + hex.EncodeToString(digest[:]) + "." + extension,
		contentType: contentType,
		content:     content,
	}
}

func brandingImageType(content []byte) (contentType, extension string, ok bool) {
	switch {
	case len(content) >= 8 && bytes.Equal(content[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png", "png", true
	case len(content) >= 3 && bytes.Equal(content[:3], []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg", "jpg", true
	case len(content) >= 6 && (bytes.Equal(content[:6], []byte("GIF87a")) || bytes.Equal(content[:6], []byte("GIF89a"))):
		return "image/gif", "gif", true
	case len(content) >= 12 && bytes.Equal(content[:4], []byte("RIFF")) && bytes.Equal(content[8:12], []byte("WEBP")):
		return "image/webp", "webp", true
	case len(content) >= 4 && bytes.Equal(content[:4], []byte{0x00, 0x00, 0x01, 0x00}):
		return "image/x-icon", "ico", true
	}

	trimmed := bytes.TrimPrefix(content, []byte{0xef, 0xbb, 0xbf})
	decoder := xml.NewDecoder(bytes.NewReader(trimmed))
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", "", false
		}
		if root, ok := token.(xml.StartElement); ok {
			if root.Name.Local == "svg" && (root.Name.Space == "" || root.Name.Space == "http://www.w3.org/2000/svg") {
				return "image/svg+xml", "svg", true
			}
			return "", "", false
		}
		if text, ok := token.(xml.CharData); ok && len(bytes.TrimSpace(text)) != 0 {
			return "", "", false
		}
	}
}
