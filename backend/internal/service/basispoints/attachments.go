package basispoints

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"mime/multipart"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

const AttachmentsURL = "https://bps.openai.com/basispoints/api/attachments"

var ErrAttachmentBusy = errors.New("basispoints attachment upload capacity exhausted")

// InlineAttachment retains only the existing encoded input, never a second
// decoded image buffer. Both validation and multipart upload stream its bytes.
type InlineAttachment struct {
	MIME    string
	payload string
	Size    int64
	digest  [32]byte
}

func (a InlineAttachment) Multipart() (io.Reader, string, int64, error) {
	var framing bytes.Buffer
	writer := multipart.NewWriter(&framing)
	extension := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp"}[a.MIME]
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", "form-data; name=\"file\"; filename=\"image."+extension+"\"")
	header.Set("Content-Type", a.MIME)
	if _, err := writer.CreatePart(header); err != nil {
		return nil, "", 0, err
	}
	prefixLen := framing.Len()
	if err := writer.Close(); err != nil {
		return nil, "", 0, err
	}
	data := framing.Bytes()
	return io.MultiReader(bytes.NewReader(data[:prefixLen]), base64.NewDecoder(base64.StdEncoding, strings.NewReader(a.payload)), bytes.NewReader(data[prefixLen:])), writer.FormDataContentType(), int64(len(data)) + a.Size, nil
}

type nativeImagePart struct {
	part  object
	image InlineAttachment
}
type NativeImages struct {
	source      object
	parts       []nativeImagePart
	raw         []byte
	toolImages  map[string]bool
	inlineCount int
}

// PrepareNativeImages checks every inline image before any upload. Only typed
// message/tool-output content is traversed; arbitrary tool arguments are opaque.
func PrepareNativeImages(raw []byte) (*NativeImages, error) {
	return PrepareNativeImagesWithLimit(raw, imageRelayMaxRequestImages)
}

// PrepareNativeImagesWithLimit is PrepareNativeImages with an administrator-
// configurable per-request inline image limit.
func PrepareNativeImagesWithLimit(raw []byte, maxImages int) (*NativeImages, error) {
	if maxImages < 1 {
		maxImages = imageRelayMaxRequestImages
	}
	var source object
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&source); err != nil {
		return nil, fmt.Errorf("invalid basispoints image request")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("invalid basispoints image request")
	}
	plan := &NativeImages{source: source, raw: raw}
	var total int64
	input, _ := source["input"].([]any)
	for _, entry := range input {
		item, _ := entry.(map[string]any)
		field := ""
		switch text(item["type"]) {
		case "", "message", "agent_message":
			field = "content"
		case "function_call_output", "custom_tool_call_output":
			field = "output"
		default:
			continue
		}
		parts, _ := item[field].([]any)
		for _, value := range parts {
			part, _ := value.(map[string]any)
			if text(part["type"]) != "input_image" {
				continue
			}
			rawURL, _ := part["image_url"].(string)
			if !strings.HasPrefix(strings.ToLower(rawURL), "data:") {
				continue
			}
			if _, exists := part["file_id"]; exists {
				return nil, fmt.Errorf("basispoints input_image requires exactly one image reference")
			}
			if plan.inlineCount >= maxImages {
				return nil, fmt.Errorf("local gateway is configured for at most %d inline images per request", maxImages)
			}
			plan.inlineCount++
			// Apply the same administrator-configured image size and count
			// safeguards as the temporary HTTPS relay before uploading natively.
			mimeType, payload, err := relayImagePayload(rawURL, imageRelayMaxImageBytes>>20)
			if err != nil {
				return nil, err
			}
			hash := sha256.New()
			reader := base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload))
			size, err := io.Copy(hash, io.LimitReader(reader, imageRelayMaxImageBytes+1))
			if err != nil || size == 0 {
				return nil, fmt.Errorf("basispoints inline image contains invalid base64 data")
			}
			if size > imageRelayMaxImageBytes {
				return nil, fmt.Errorf("basispoints inline image exceeds the 20 MiB limit")
			}
			config, format, err := image.DecodeConfig(base64.NewDecoder(base64.StdEncoding, strings.NewReader(payload)))
			if err != nil || "image/"+format != mimeType {
				return nil, fmt.Errorf("basispoints inline image content does not match its media type")
			}
			if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > imageRelayMaxPixels {
				return nil, fmt.Errorf("basispoints inline image dimensions exceed the 64 megapixel limit")
			}
			total += size
			if total > imageRelayMaxRequestBytes {
				return nil, fmt.Errorf("basispoints inline images exceed the 32 MiB request limit")
			}
			attachment := InlineAttachment{MIME: mimeType, payload: payload, Size: size}
			copy(attachment.digest[:], hash.Sum(nil))
			if part["detail"] == nil {
				part["detail"] = "auto"
			}
			delete(part, "image_url")
			part["file_id"] = "file-preflight"
			if err := validateImage(part); err != nil {
				return nil, err
			}
			if field == "output" {
				// BPS tool screenshots use inline image_url, not attachment IDs.
				delete(part, "file_id")
				part["image_url"] = rawURL
				if plan.toolImages == nil {
					plan.toolImages = make(map[string]bool)
				}
				plan.toolImages[rawURL] = true
			} else {
				plan.parts = append(plan.parts, nativeImagePart{part: part, image: attachment})
			}
		}
	}
	return plan, nil
}

// HasImages reports whether any message images require attachment upload.
// Validated inline tool screenshots do not require an upload or attachment lease.
func (p *NativeImages) HasImages() bool { return len(p.parts) != 0 }

func (p *NativeImages) Body() ([]byte, error) {
	if p.inlineCount == 0 {
		return p.raw, nil
	}
	return json.Marshal(p.source)
}

func ValidAttachmentID(id string) bool {
	if !strings.HasPrefix(id, "file-") || len(id) < 6 || len(id) > 256 {
		return false
	}
	for _, ch := range id[5:] {
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9', ch == '-', ch == '_':
		default:
			return false
		}
	}
	return true
}

type AttachmentUpload func(context.Context, InlineAttachment) (string, error)
type attachmentEntry struct {
	id            string
	expires, used time.Time
}
type attachmentFlight struct {
	done chan struct{}
	id   string
	err  error
}

// AttachmentCache holds at most 512 IDs for 30 minutes, scoped by credential,
// account, API key and trusted conversation identity. Images are never cached.
// In-flight work is bounded separately and waiters respect their own context.
type AttachmentCache struct {
	mu      sync.Mutex
	entries map[[32]byte]attachmentEntry
	flights map[[32]byte]*attachmentFlight
	active  int
	now     func() time.Time
}

func (c *AttachmentCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (p *NativeImages) Upload(ctx context.Context, cache *AttachmentCache, scope string, upload AttachmentUpload) ([]byte, error) {
	ids := make(map[[32]byte]string)
	for _, part := range p.parts {
		id := ids[part.image.digest]
		if id == "" {
			var err error
			id, err = cache.get(ctx, scope, part.image, upload)
			if err != nil {
				return nil, err
			}
			ids[part.image.digest] = id
		}
		part.part["file_id"] = id
	}
	return p.Body()
}

func (c *AttachmentCache) get(ctx context.Context, scope string, img InlineAttachment, upload AttachmentUpload) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte(scope + "\x00" + img.MIME + "\x00" + string(img.digest[:])))
	c.mu.Lock()
	now := c.clock()
	for k, entry := range c.entries {
		if !now.Before(entry.expires) {
			delete(c.entries, k)
		}
	}
	if scope != "" {
		if entry, ok := c.entries[key]; ok {
			entry.used = now
			c.entries[key] = entry
			c.mu.Unlock()
			return entry.id, nil
		}
		if flight := c.flights[key]; flight != nil {
			c.mu.Unlock()
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-flight.done:
				return flight.id, flight.err
			}
		}
	}
	if c.active >= 32 {
		c.mu.Unlock()
		return "", ErrAttachmentBusy
	}
	c.active++
	flight := &attachmentFlight{done: make(chan struct{})}
	if scope != "" {
		if c.flights == nil {
			c.flights = make(map[[32]byte]*attachmentFlight)
		}
		c.flights[key] = flight
	}
	c.mu.Unlock()
	id, err := upload(ctx, img)
	if err == nil && !ValidAttachmentID(id) {
		err = fmt.Errorf("basispoints returned an invalid attachment ID")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active--
	if scope != "" {
		delete(c.flights, key)
		if err == nil && c.clock().Before(now.Add(30*time.Minute)) {
			if c.entries == nil {
				c.entries = make(map[[32]byte]attachmentEntry)
			}
			if len(c.entries) >= 512 {
				var oldest [32]byte
				var used time.Time
				for k, entry := range c.entries {
					if used.IsZero() || entry.used.Before(used) {
						oldest = k
						used = entry.used
					}
				}
				delete(c.entries, oldest)
			}
			c.entries[key] = attachmentEntry{id: id, expires: now.Add(30 * time.Minute), used: c.clock()}
		}
	}
	flight.id, flight.err = id, err
	close(flight.done)
	return id, err
}

// PrepareWithCatalog permits only the tool screenshots validated by this native
// request plan. Ordinary Prepare calls keep rejecting inline images.
func (p *NativeImages) PrepareWithCatalog(scope string, replay *ReplayCache, cache *CatalogCache) ([]byte, *Bridge, error) {
	raw, err := p.Body()
	if err != nil {
		return nil, nil, err
	}
	return prepareWithCatalog(raw, scope, replay, cache, p.toolImages)
}
