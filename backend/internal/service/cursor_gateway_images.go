package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
)

// Cursor image input: OpenAI image_url content parts on the active user
// message map onto UserMessage.selected_context as raw bytes. Replayed
// history stays text-only (images are the dominant prompt-size risk and hold
// little value after their turn).

const (
	// cursorImageMaxBytes caps a single image after decode/download. Cursor's
	// own clients cap around 4-8MB; generous enough for screenshots.
	cursorImageMaxBytes     = 8 << 20
	cursorImageFetchTimeout = 10 * time.Second
	cursorMaxImagesPerTurn  = 8
)

// cursorImageFetcher downloads remote image URLs; a variable for tests.
var cursorImageFetcher = func(url string) ([]byte, string, error) {
	client := &http.Client{Timeout: cursorImageFetchTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, cursorImageMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > cursorImageMaxBytes {
		return nil, "", fmt.Errorf("image exceeds %d bytes", cursorImageMaxBytes)
	}
	mime := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = "image/png"
	}
	return data, mime, nil
}

// collectCursorImages extracts image parts from the LAST user message and
// resolves them to bytes: data URIs inline, remote URLs via download.
// Unresolvable images are skipped with a log line — the text still flows.
func collectCursorImages(messages []apicompat.ChatMessage) []cursor.AgentImage {
	var lastUserRaw []byte
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserRaw = messages[i].Content
			break
		}
	}
	if len(lastUserRaw) == 0 {
		return nil
	}
	var parts []struct {
		Type     string `json:"type"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(lastUserRaw, &parts); err != nil {
		return nil
	}
	var images []cursor.AgentImage
	for _, part := range parts {
		if len(images) >= cursorMaxImagesPerTurn {
			break
		}
		if part.Type != "image_url" || part.ImageURL == nil {
			continue
		}
		if image, err := resolveCursorImage(part.ImageURL.URL); err == nil {
			images = append(images, image)
		} else {
			logger.LegacyPrintf("service.cursor", "[Cursor] image skipped: %v", err)
		}
	}
	return images
}

// resolveCursorImage converts a data URI or fetchable URL into raw bytes.
func resolveCursorImage(url string) (cursor.AgentImage, error) {
	if data, mime, ok := parseCursorDataURL(url); ok {
		if len(data) > cursorImageMaxBytes {
			return cursor.AgentImage{}, fmt.Errorf("image exceeds %d bytes", cursorImageMaxBytes)
		}
		return cursor.AgentImage{Mime: mime, Data: data}, nil
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		data, mime, err := cursorImageFetcher(url)
		if err != nil {
			return cursor.AgentImage{}, err
		}
		return cursor.AgentImage{Mime: mime, Data: data}, nil
	}
	return cursor.AgentImage{}, fmt.Errorf("unsupported image url")
}

// parseCursorDataURL decodes data:<mime>;base64,<payload>.
func parseCursorDataURL(url string) ([]byte, string, bool) {
	if !strings.HasPrefix(url, "data:") {
		return nil, "", false
	}
	comma := strings.Index(url, ",")
	if comma < 0 {
		return nil, "", false
	}
	meta := url[len("data:"):comma]
	payload := url[comma+1:]
	mime := "image/png"
	for _, segment := range strings.Split(meta, ";") {
		if strings.HasPrefix(segment, "image/") {
			mime = segment
		}
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// Tolerate URL-safe base64 some clients emit.
		data, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return nil, "", false
		}
	}
	return data, mime, true
}
