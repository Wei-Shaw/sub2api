package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"net/url"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"golang.org/x/net/html"
)

const upstreamSiteLogoMaxBytes int64 = 64 * 1024
const upstreamSiteLogoMetadataMaxBytes int64 = 128 * 1024
const upstreamSiteLogoMaxPixels int64 = 4 * 1024 * 1024

var defaultUpstreamLogoHashes = map[string]struct{}{
	"6d3fade369f77916e92cf7a8301769cd40f3b7cddaf3a10b0c2f530b10f75c90": {},
	"c639eb5af36fb48aaa77615aa3824d533bd2d155772f324dcd4bab78b8ea2a24": {},
	"32132a307cad98d7f93b0384de87411f087bb34c148c932dcd5eea92101cbec3": {},
	"752c8f79e0e4c29601b162dc3e783198a764675ee2f3a7959213ebfe76791c90": {},
	"a6fdd69f10c17855cdc83856e4fb6680779db14dcc75a1a2716ebdfe4f2f2097": {},
}

var ErrUpstreamSiteLogoNotFound = infraerrors.NotFound(
	"UPSTREAM_SITE_LOGO_NOT_FOUND", "upstream site logo not found",
)

type UpstreamSiteLogo struct {
	ContentType string
	Data        []byte
}

type upstreamSiteLogoCache interface {
	GetUpstreamSiteLogoCache(context.Context, string) (*UpstreamSiteLogo, bool, error)
	PutUpstreamSiteLogoCache(context.Context, string, *UpstreamSiteLogo) error
}

func (s *UpstreamBillingProbeService) canCacheUpstreamSiteLogos() bool {
	if s == nil || s.accountRepo == nil {
		return false
	}
	_, ok := s.accountRepo.(upstreamSiteLogoCache)
	return ok
}

func (s *UpstreamBillingProbeService) resolveUpstreamSiteLogo(ctx context.Context, client *upstreamQuotaQueryClient, identity *upstreamIdentityProbeResult) string {
	store, ok := s.accountRepo.(upstreamSiteLogoCache)
	if !ok || client == nil || identity == nil {
		return ""
	}
	key, err := upstreamSiteLogoCacheKey(client.baseURL)
	if err != nil {
		return ""
	}
	cached, found, err := store.GetUpstreamSiteLogoCache(ctx, key)
	if err != nil {
		logger.LegacyPrintf("service.upstream_identity", "site_logo_cache_read_failed: account_id=%d err=%v", client.account.ID, err)
		return ""
	}
	if found {
		if cached != nil {
			return key
		}
		return ""
	}

	logo, err := client.discoverUpstreamSiteLogo(ctx, identity)
	if err != nil {
		logger.LegacyPrintf("service.upstream_identity", "site_logo_discovery_failed: account_id=%d err=%v", client.account.ID, err)
		return ""
	}
	if err := store.PutUpstreamSiteLogoCache(ctx, key, logo); err != nil {
		logger.LegacyPrintf("service.upstream_identity", "site_logo_cache_write_failed: account_id=%d err=%v", client.account.ID, err)
		return ""
	}
	if logo == nil {
		return ""
	}
	return key
}

func (s *UpstreamBillingProbeService) GetUpstreamSiteLogo(ctx context.Context, key string) (*UpstreamSiteLogo, error) {
	if !validUpstreamSiteLogoKey(key) {
		return nil, ErrUpstreamSiteLogoNotFound
	}
	store, ok := s.accountRepo.(upstreamSiteLogoCache)
	if !ok {
		return nil, ErrUpstreamBillingProbeUnavailable
	}
	logo, found, err := store.GetUpstreamSiteLogoCache(ctx, key)
	if err != nil {
		return nil, err
	}
	if !found || logo == nil {
		return nil, ErrUpstreamSiteLogoNotFound
	}
	return normalizeUpstreamSiteLogoForDisplay(logo), nil
}

func normalizeUpstreamSiteLogoForDisplay(logo *UpstreamSiteLogo) *UpstreamSiteLogo {
	if logo == nil || len(logo.Data) == 0 {
		return logo
	}
	data := logo.Data
	extractedICO := false
	if logo.ContentType == "image/x-icon" {
		data = largestEmbeddedPNGFromICO(data)
		if len(data) == 0 {
			return logo
		}
		extractedICO = true
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > upstreamSiteLogoMaxPixels {
		return logo
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return logo
	}
	contentBounds, ok := upstreamSiteLogoContentBounds(source)
	if !ok {
		return logo
	}
	contentBounds = padUpstreamSiteLogoBounds(contentBounds, source.Bounds())
	if contentBounds == source.Bounds() {
		if extractedICO {
			return &UpstreamSiteLogo{ContentType: "image/png", Data: data}
		}
		return logo
	}

	var cropped image.Image
	if subImage, ok := source.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		cropped = subImage.SubImage(contentBounds)
	} else {
		copy := image.NewNRGBA(image.Rect(0, 0, contentBounds.Dx(), contentBounds.Dy()))
		draw.Draw(copy, copy.Bounds(), source, contentBounds.Min, draw.Src)
		cropped = copy
	}
	var output bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&output, cropped); err != nil || output.Len() == 0 || int64(output.Len()) > upstreamSiteLogoMaxBytes {
		return logo
	}
	return &UpstreamSiteLogo{ContentType: "image/png", Data: output.Bytes()}
}

func largestEmbeddedPNGFromICO(data []byte) []byte {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count <= 0 || len(data) < 6+count*16 {
		return nil
	}
	var best []byte
	bestArea := 0
	for i := 0; i < count; i++ {
		entry := data[6+i*16 : 6+(i+1)*16]
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		size := uint64(binary.LittleEndian.Uint32(entry[8:12]))
		offset := uint64(binary.LittleEndian.Uint32(entry[12:16]))
		if size < 8 || offset+size > uint64(len(data)) {
			continue
		}
		candidate := data[offset : offset+size]
		if string(candidate[:8]) != "\x89PNG\r\n\x1a\n" || width*height <= bestArea {
			continue
		}
		best, bestArea = candidate, width*height
	}
	return best
}

func upstreamSiteLogoContentBounds(source image.Image) (image.Rectangle, bool) {
	bounds := source.Bounds()
	visible := scanUpstreamSiteLogoBounds(source, func(r, g, b, a uint32) bool {
		return a > 0x1000
	})
	if visible.Empty() {
		return image.Rectangle{}, false
	}
	if visible != bounds || !upstreamSiteLogoHasWhiteCorners(source) {
		return visible, true
	}
	nonWhite := scanUpstreamSiteLogoBounds(source, func(r, g, b, a uint32) bool {
		return a > 0x1000 && (r < 0xf000 || g < 0xf000 || b < 0xf000)
	})
	if nonWhite.Empty() {
		return bounds, true
	}
	return nonWhite, true
}

func scanUpstreamSiteLogoBounds(source image.Image, visible func(uint32, uint32, uint32, uint32) bool) image.Rectangle {
	bounds := source.Bounds()
	minX, minY, maxX, maxY := bounds.Max.X, bounds.Max.Y, bounds.Min.X-1, bounds.Min.Y-1
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if !visible(source.At(x, y).RGBA()) {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func upstreamSiteLogoHasWhiteCorners(source image.Image) bool {
	bounds := source.Bounds()
	for _, point := range []image.Point{
		bounds.Min,
		{X: bounds.Max.X - 1, Y: bounds.Min.Y},
		{X: bounds.Min.X, Y: bounds.Max.Y - 1},
		{X: bounds.Max.X - 1, Y: bounds.Max.Y - 1},
	} {
		r, g, b, a := source.At(point.X, point.Y).RGBA()
		if a < 0xf000 || r < 0xf000 || g < 0xf000 || b < 0xf000 {
			return false
		}
	}
	return true
}

func padUpstreamSiteLogoBounds(content, full image.Rectangle) image.Rectangle {
	padding := max(content.Dx(), content.Dy()) / 32
	if padding < 1 {
		padding = 1
	} else if padding > 4 {
		padding = 4
	}
	content = content.Inset(-padding).Intersect(full)
	return content
}

func upstreamSiteLogoCacheKey(base string) (string, error) {
	root, err := upstreamSiteRootURL(base)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(root.String()))
	return hex.EncodeToString(sum[:]), nil
}

func validUpstreamSiteLogoKey(key string) bool {
	if len(key) != sha256.Size*2 || strings.ToLower(key) != key {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

func upstreamSiteRootURL(base string) (*url.URL, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("invalid upstream base URL")
	}
	parsed.User = nil
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	path := strings.TrimRight(parsed.Path, "/")
	if openAIBaseURLHasVersionSuffix(path) {
		path = path[:strings.LastIndex(path, "/")]
	}
	parsed.Path = strings.TrimRight(path, "/")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.ForceQuery = false
	return parsed, nil
}

func (c *upstreamQuotaQueryClient) discoverUpstreamSiteLogo(ctx context.Context, identity *upstreamIdentityProbeResult) (*UpstreamSiteLogo, error) {
	root, err := upstreamSiteRootURL(c.baseURL)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	var lastErr error
	candidate := strings.TrimSpace(identity.logoCandidate)
	if identity.provider == UpstreamIdentityProviderSub2API {
		settingsURL := *root
		settingsURL.Path = strings.TrimRight(settingsURL.Path, "/") + "/api/v1/settings/public"
		response, requestErr := c.getLimited(ctx, settingsURL.String(), false, "application/json", upstreamSiteLogoMetadataMaxBytes)
		if requestErr != nil {
			lastErr = requestErr
		} else if response.status >= http.StatusOK && response.status < http.StatusMultipleChoices {
			candidate = parseSub2APISiteLogoCandidate(response.body)
		}
	}
	if logo, fetchErr := c.fetchUpstreamSiteLogoCandidate(ctx, root, candidate, seen); logo != nil {
		return logo, nil
	} else if fetchErr != nil {
		lastErr = fetchErr
	}

	pageURL := *root
	pageURL.Path = strings.TrimRight(pageURL.Path, "/") + "/"
	pageResponse, pageErr := c.getLimited(ctx, pageURL.String(), false, "text/html,application/xhtml+xml", upstreamSiteLogoMetadataMaxBytes)
	if pageErr != nil {
		lastErr = pageErr
	} else if pageResponse.status >= http.StatusOK && pageResponse.status < http.StatusMultipleChoices {
		if logo, fetchErr := c.fetchUpstreamSiteLogoCandidate(ctx, root, parseHTMLIconHref(pageResponse.body), seen); logo != nil {
			return logo, nil
		} else if fetchErr != nil {
			lastErr = fetchErr
		}
	}

	faviconURL := *root
	faviconURL.Path = strings.TrimRight(faviconURL.Path, "/") + "/favicon.ico"
	if logo, fetchErr := c.fetchUpstreamSiteLogoCandidate(ctx, root, faviconURL.String(), seen); logo != nil {
		return logo, nil
	} else if fetchErr != nil {
		lastErr = fetchErr
	}
	return nil, lastErr
}

func parseSub2APISiteLogoCandidate(body []byte) string {
	var response struct {
		Code *int `json:"code"`
		Data *struct {
			SiteLogo string `json:"site_logo"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &response) != nil || response.Code == nil || *response.Code != 0 || response.Data == nil {
		return ""
	}
	return strings.TrimSpace(response.Data.SiteLogo)
}

func parseHTMLIconHref(body []byte) string {
	tokenizer := html.NewTokenizer(strings.NewReader(string(body)))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return ""
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			if !strings.EqualFold(token.Data, "link") {
				continue
			}
			var rel, href string
			for _, attr := range token.Attr {
				switch strings.ToLower(attr.Key) {
				case "rel":
					rel = attr.Val
				case "href":
					href = attr.Val
				}
			}
			for _, value := range strings.Fields(strings.ToLower(rel)) {
				if value == "icon" && strings.TrimSpace(href) != "" {
					return strings.TrimSpace(href)
				}
			}
		}
	}
}

func (c *upstreamQuotaQueryClient) fetchUpstreamSiteLogoCandidate(ctx context.Context, root *url.URL, candidate string, seen map[string]struct{}) (*UpstreamSiteLogo, error) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return nil, nil
	}
	if strings.HasPrefix(strings.ToLower(candidate), "data:") {
		data, err := decodeSiteLogoDataURL(candidate)
		if err != nil {
			return nil, nil
		}
		return validatedUpstreamSiteLogo(data), nil
	}
	resolutionBase := *root
	resolutionBase.Path = strings.TrimRight(resolutionBase.Path, "/") + "/"
	resolved, err := resolutionBase.Parse(candidate)
	if err != nil || !sameURLOrigin(root, resolved) {
		return nil, nil
	}
	resolved.User = nil
	resolved.Fragment = ""
	key := resolved.String()
	if _, exists := seen[key]; exists {
		return nil, nil
	}
	seen[key] = struct{}{}
	response, err := c.getLimited(ctx, key, false, "image/png,image/x-icon,image/webp;q=0.9,*/*;q=0.1", upstreamSiteLogoMaxBytes)
	if err != nil {
		return nil, err
	}
	if response.status == http.StatusNotFound || response.status == http.StatusMethodNotAllowed {
		return nil, nil
	}
	if response.status < http.StatusOK || response.status >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("site logo request returned HTTP %d", response.status)
	}
	return validatedUpstreamSiteLogo(response.body), nil
}

func decodeSiteLogoDataURL(value string) ([]byte, error) {
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return nil, errors.New("invalid data URL")
	}
	metadata := strings.ToLower(value[:comma])
	if !strings.HasPrefix(metadata, "data:image/") {
		return nil, errors.New("unsupported data URL")
	}
	payload := value[comma+1:]
	if strings.Contains(metadata, ";base64") {
		data, err := base64.StdEncoding.DecodeString(payload)
		if err != nil || int64(len(data)) > upstreamSiteLogoMaxBytes {
			return nil, errors.New("invalid image data URL")
		}
		return data, nil
	}
	decoded, err := url.PathUnescape(payload)
	if err != nil || int64(len(decoded)) > upstreamSiteLogoMaxBytes {
		return nil, errors.New("invalid image data URL")
	}
	return []byte(decoded), nil
}

func validatedUpstreamSiteLogo(data []byte) *UpstreamSiteLogo {
	if len(data) == 0 || int64(len(data)) > upstreamSiteLogoMaxBytes {
		return nil
	}
	contentType := ""
	switch {
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		contentType = "image/png"
	case len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 1 && data[3] == 0:
		contentType = "image/x-icon"
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		contentType = "image/webp"
	default:
		return nil
	}
	sum := sha256.Sum256(data)
	if _, isDefault := defaultUpstreamLogoHashes[hex.EncodeToString(sum[:])]; isDefault {
		return nil
	}
	return &UpstreamSiteLogo{ContentType: contentType, Data: append([]byte(nil), data...)}
}

func sameURLOrigin(left, right *url.URL) bool {
	return left != nil && right != nil && strings.EqualFold(left.Scheme, right.Scheme) && strings.EqualFold(left.Host, right.Host)
}
