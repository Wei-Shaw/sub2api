package basispoints

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp"
)

var (
	ErrImageRelayFull    = errors.New("basispoints temporary image storage is full; retry after images expire")
	ErrImageRelayStorage = errors.New("basispoints temporary image storage is unavailable")
)

const (
	ImageRelayPath             = "/api/bps-images/"
	imageRelayTTL              = 30 * time.Minute
	imageRelayMaxBytes         = 1 << 30
	imageRelayMaxEntries       = 512
	imageRelayMaxImageBytes    = 20 << 20
	imageRelayMaxRequestBytes  = 32 << 20
	imageRelayMaxRequestImages = 20
	imageRelayMaxPixels        = 64 * 1024 * 1024
	imageRelayCleanupInterval  = time.Minute
)

// Image bytes live in private disk files; only bounded metadata lives on heap.
// Pending uploads reserve disk capacity before decoding. Files are never exposed
// as a static directory and downloads use bounded streaming buffers.
type ImageRelay struct {
	baseURL         string
	limits          ImageRelayLimits
	key             [32]byte
	mu              sync.Mutex
	entries         map[string]*relayImage
	bytes           int64
	reservedBytes   int64
	reservedEntries int
	retiredEntries  int
	root            string
	dir             string
	closed          bool
	stop            chan struct{}
	done            chan struct{}
	downloads       chan struct{}
}

type relayImage struct {
	path        string
	size        int
	reserved    int64
	contentType string
	expires     time.Time
	readers     int
	retired     bool
}

func ValidateImageRelayOrigin(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(baseURL, "#") || (parsed.Path != "" && parsed.Path != "/") {
		return fmt.Errorf("BPS image relay requires an HTTPS public origin without credentials, path, query or fragment")
	}
	return nil
}

func NewImageRelay(baseURL, storageRoot string) (*ImageRelay, error) {
	if err := ValidateImageRelayOrigin(baseURL); err != nil {
		return nil, err
	}
	if storageRoot == "" {
		return nil, ErrImageRelayStorage
	}
	relay := &ImageRelay{limits: DefaultImageRelayLimits(), baseURL: strings.TrimRight(baseURL, "/"), entries: make(map[string]*relayImage), root: storageRoot, stop: make(chan struct{}), done: make(chan struct{}), downloads: make(chan struct{}, 32)}
	if _, err := rand.Read(relay.key[:]); err != nil {
		return nil, ErrImageRelayStorage
	}
	if err := os.MkdirAll(storageRoot, 0700); err != nil {
		return nil, ErrImageRelayStorage
	}
	dir, err := os.MkdirTemp(storageRoot, "session-")
	if err != nil {
		return nil, ErrImageRelayStorage
	}
	relay.dir = dir
	relay.cleanupOrphans(time.Now())
	go relay.cleanupLoop()
	return relay, nil
}

func (r *ImageRelay) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.stop)
	}
	r.mu.Unlock()
	<-r.done
	if err := os.RemoveAll(r.dir); err != nil {
		return ErrImageRelayStorage
	}
	return nil
}

func (r *ImageRelay) cleanupLoop() {
	defer close(r.done)
	ticker := time.NewTicker(imageRelayCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.stop:
			return
		case now := <-ticker.C:
			r.mu.Lock()
			r.pruneLocked(now)
			_ = os.Chtimes(r.dir, now, now)
			r.mu.Unlock()
			r.cleanupOrphans(now)
		}
	}
}

// Each live instance refreshes its directory every minute. After an abnormal
// exit, a later/running instance removes directories idle for the fixed grace
// period. This is independent of configurable link expiry in live instances.
func (r *ImageRelay) cleanupOrphans(now time.Time) {
	entries, err := os.ReadDir(r.root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(r.root, entry.Name())
		if path == r.dir || !entry.IsDir() || !strings.HasPrefix(entry.Name(), "session-") {
			continue
		}
		info, err := entry.Info()
		if err == nil && now.Sub(info.ModTime()) > imageRelayTTL+imageRelayCleanupInterval {
			_ = os.RemoveAll(path)
		}
	}
}

// SetPublicOrigin changes new links without discarding in-flight images.
func (r *ImageRelay) SetPublicOrigin(baseURL string) error {
	if err := ValidateImageRelayOrigin(baseURL); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrImageRelayStorage
	}
	r.baseURL = strings.TrimRight(baseURL, "/")
	return nil
}

func (r *ImageRelay) Rewrite(raw []byte, scope string) ([]byte, error) {
	return r.RewriteWithImageLimit(raw, scope, 0)
}

// RewriteWithImageLimit permits a separately validated history/new-input pair.
// It does not modify the shared relay configuration or any byte/storage limit.
func (r *ImageRelay) RewriteWithImageLimit(raw []byte, scope string, maxImages int) ([]byte, error) {
	if r == nil {
		return raw, nil
	}
	r.mu.Lock()
	baseURL, closed, limits := r.baseURL, r.closed, r.limits
	r.mu.Unlock()
	if maxImages > 0 {
		limits.MaxImages = maxImages
	}
	if closed {
		return nil, ErrImageRelayStorage
	}
	var source object
	if err := decode(raw, &source); err != nil || source == nil {
		return nil, fmt.Errorf("invalid Basispoints request JSON")
	}
	input, _ := source["input"].([]any)
	images := make(map[string]*relayImage)
	var staged []*relayImage
	// Every failed/duplicate batch releases both files and quota.
	defer func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, img := range staged {
			if img.reserved > 0 {
				r.discardLocked(img)
			}
		}
	}()
	totalBytes := 0
	for _, rawItem := range input {
		item, _ := rawItem.(object)
		for _, field := range []string{"content", "output"} {
			if field == "output" && text(item["type"]) != "function_call_output" && text(item["type"]) != "custom_tool_call_output" {
				continue
			}
			parts, _ := item[field].([]any)
			for _, rawPart := range parts {
				part, _ := rawPart.(object)
				if text(part["type"]) != "input_image" {
					continue
				}
				rawURL := text(part["image_url"])
				if len(rawURL) < len("data:") || !strings.EqualFold(rawURL[:len("data:")], "data:") {
					continue
				}
				if len(staged) >= limits.MaxImages {
					return nil, fmt.Errorf("image relay is configured for at most %d inline images per request", limits.MaxImages)
				}
				img, token, err := r.storeImageWithLimit(rawURL, scope, limits.MaxImageMiB)
				if err != nil {
					return nil, err
				}
				staged = append(staged, img)
				totalBytes += img.size
				if totalBytes > limits.MaxTotalMiB<<20 {
					return nil, fmt.Errorf("image relay inline images exceed the configured %d MiB request limit", limits.MaxTotalMiB)
				}
				part["image_url"] = baseURL + ImageRelayPath + token
				if err := validateImage(part); err != nil {
					return nil, err
				}
				images[token] = img
			}
		}
	}
	if len(staged) == 0 {
		return raw, nil
	}
	out, err := json.Marshal(source)
	if err != nil {
		return nil, fmt.Errorf("encode basispoints image request")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrImageRelayStorage
	}
	now := time.Now()
	r.pruneLocked(now)
	for token, img := range images {
		if existing := r.entries[token]; existing != nil {
			existing.expires = now.Add(time.Duration(limits.TTLMinutes) * time.Minute)
			continue
		}
		r.reservedBytes -= img.reserved
		r.reservedEntries--
		img.reserved = 0
		img.expires = now.Add(time.Duration(limits.TTLMinutes) * time.Minute)
		r.entries[token] = img
		r.bytes += int64(img.size)
	}
	return out, nil
}

func relayImagePayload(raw string, maxImageMiB int) (string, string, error) {
	header, payload, ok := strings.Cut(raw[len("data:"):], ",")
	if !ok || !strings.HasSuffix(strings.ToLower(header), ";base64") {
		return "", "", fmt.Errorf("basispoints inline image requires a base64 image data URL")
	}
	declared, params, err := mime.ParseMediaType(header[:len(header)-len(";base64")])
	if err != nil || len(params) != 0 {
		return "", "", fmt.Errorf("basispoints inline image has an invalid media type")
	}
	switch declared {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return "", "", fmt.Errorf("basispoints inline images must be PNG, JPEG, GIF or WebP")
	}
	if len(payload) > base64.StdEncoding.EncodedLen(maxImageMiB<<20) {
		return "", "", fmt.Errorf("image relay inline image exceeds the configured %d MiB limit", maxImageMiB)
	}
	if payload == "" {
		return "", "", fmt.Errorf("basispoints inline image contains invalid base64 data")
	}
	return declared, payload, nil
}

func (r *ImageRelay) storeImage(raw, scope string) (*relayImage, string, error) {
	r.mu.Lock()
	maxImageMiB := r.limits.MaxImageMiB
	r.mu.Unlock()
	return r.storeImageWithLimit(raw, scope, maxImageMiB)
}

func (r *ImageRelay) storeImageWithLimit(raw, scope string, maxImageMiB int) (*relayImage, string, error) {
	declared, payload, err := relayImagePayload(raw, maxImageMiB)
	if err != nil {
		return nil, "", err
	}
	reservation := int64(base64.StdEncoding.DecodedLen(len(payload))) + 1
	r.mu.Lock()
	r.pruneLocked(time.Now())
	if r.closed {
		r.mu.Unlock()
		return nil, "", ErrImageRelayStorage
	}
	if r.bytes+r.reservedBytes+reservation > int64(r.limits.StorageMiB)<<20 || len(r.entries)+r.reservedEntries+r.retiredEntries >= r.limits.StorageEntries {
		r.mu.Unlock()
		return nil, "", ErrImageRelayFull
	}
	r.reservedBytes += reservation
	r.reservedEntries++
	r.mu.Unlock()
	img := &relayImage{reserved: reservation}
	success := false
	defer func() {
		if !success {
			r.mu.Lock()
			r.discardLocked(img)
			r.mu.Unlock()
		}
	}()
	file, err := os.CreateTemp(r.dir, "image-")
	if err != nil {
		return nil, "", ErrImageRelayStorage
	}
	img.path = file.Name()
	defer func() { _ = file.Close() }()
	mac := hmac.New(sha256.New, r.key[:])
	_, _ = mac.Write([]byte(scope))
	_, _ = mac.Write([]byte{0})
	reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
	n, err := io.CopyBuffer(io.MultiWriter(file, mac), io.LimitReader(reader, (int64(maxImageMiB)<<20)+1), make([]byte, 32*1024))
	if err != nil {
		var corrupt base64.CorruptInputError
		if errors.As(err, &corrupt) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, "", fmt.Errorf("basispoints inline image contains invalid base64 data")
		}
		return nil, "", ErrImageRelayStorage
	}
	if n == 0 {
		return nil, "", fmt.Errorf("basispoints inline image contains invalid base64 data")
	}
	if n > int64(maxImageMiB)<<20 {
		return nil, "", fmt.Errorf("image relay inline image exceeds the configured %d MiB limit", maxImageMiB)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", ErrImageRelayStorage
	}
	dimensions, format, err := image.DecodeConfig(file)
	if err != nil || dimensions.Width <= 0 || dimensions.Height <= 0 || int64(dimensions.Width)*int64(dimensions.Height) > imageRelayMaxPixels {
		return nil, "", fmt.Errorf("basispoints inline image is invalid or exceeds 64 megapixels")
	}
	if "image/"+format != declared {
		return nil, "", fmt.Errorf("basispoints inline image media type does not match its contents")
	}
	if err := file.Close(); err != nil {
		return nil, "", ErrImageRelayStorage
	}
	img.size, img.contentType = int(n), declared
	success = true
	return img, base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (r *ImageRelay) discardLocked(img *relayImage) {
	if img.path != "" {
		_ = os.Remove(img.path)
	}
	r.reservedBytes -= img.reserved
	r.reservedEntries--
	img.reserved = 0
}

func (r *ImageRelay) removeLocked(token string, img *relayImage) {
	if img.readers > 0 {
		img.retired = true
		r.retiredEntries++
		delete(r.entries, token)
		return
	}
	_ = os.Remove(img.path)
	r.bytes -= int64(img.size)
	delete(r.entries, token)
}

func (r *ImageRelay) finishRead(img *relayImage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	img.readers--
	if img.retired && img.readers == 0 {
		_ = os.Remove(img.path)
		r.bytes -= int64(img.size)
		r.retiredEntries--
	}
}

func (r *ImageRelay) pruneLocked(now time.Time) {
	for token, img := range r.entries {
		if !now.Before(img.expires) {
			r.removeLocked(token, img)
		}
	}
}

func (r *ImageRelay) expire(token string, expected *relayImage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if img := r.entries[token]; img == expected && img != nil && !time.Now().Before(img.expires) {
		r.removeLocked(token, img)
	}
}

func (r *ImageRelay) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	token, ok := strings.CutPrefix(req.URL.Path, ImageRelayPath)
	if r == nil || !ok || len(token) != 43 {
		http.NotFound(w, req)
		return
	}
	select {
	case r.downloads <- struct{}{}:
		defer func() { <-r.downloads }()
	default:
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	r.mu.Lock()
	img := r.entries[token]
	if img != nil && !time.Now().Before(img.expires) {
		r.removeLocked(token, img)
		img = nil
	}
	if img == nil || r.closed {
		r.mu.Unlock()
		http.NotFound(w, req)
		return
	}
	file, err := os.Open(img.path)
	if err == nil {
		img.readers++
	}
	r.mu.Unlock()
	if err != nil {
		http.NotFound(w, req)
		return
	}
	defer func() {
		_ = file.Close()
		r.finishRead(img)
	}()
	w.Header().Set("Content-Type", img.contentType)
	w.Header().Set("Content-Length", strconv.Itoa(img.size))
	w.WriteHeader(http.StatusOK)
	if req.Method == http.MethodGet {
		_, _ = io.Copy(w, file)
	}
}
