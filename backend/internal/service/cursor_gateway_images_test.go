package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/cursor"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseCursorDataURL(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	data, mime, ok := parseCursorDataURL("data:image/jpeg;base64," + payload)
	require.True(t, ok)
	require.Equal(t, "image/jpeg", mime)
	require.Equal(t, []byte{1, 2, 3}, data)

	_, _, ok = parseCursorDataURL("https://example.com/a.png")
	require.False(t, ok)
	_, _, ok = parseCursorDataURL("data:image/png;base64,!!!not-base64!!!")
	require.False(t, ok)
}

func TestCollectCursorImagesFromActiveUserMessage(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString([]byte{0x89, 0x50})
	messages := []apicompat.ChatMessage{
		{Role: "user", Content: json.RawMessage(`[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + pngB64 + `"}},{"type":"text","text":"hello"}]`)},
		{Role: "assistant", Content: json.RawMessage(`"hi"`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"and this"},{"type":"image_url","image_url":{"url":"data:image/gif;base64,` + pngB64 + `"}}]`)},
	}
	images := collectCursorImages(messages)
	require.Len(t, images, 1, "only the LAST user message's images count")
	require.Equal(t, "image/gif", images[0].Mime)

	// Remote URLs go through the fetcher; failures are skipped, not fatal.
	prev := cursorImageFetcher
	t.Cleanup(func() { cursorImageFetcher = prev })
	cursorImageFetcher = func(url string) ([]byte, string, error) {
		require.Equal(t, "https://cdn.example.com/pic.png", url)
		return []byte{9, 9}, "image/png", nil
	}
	remote := []apicompat.ChatMessage{
		{Role: "user", Content: json.RawMessage(`[{"type":"image_url","image_url":{"url":"https://cdn.example.com/pic.png"}}]`)},
	}
	images = collectCursorImages(remote)
	require.Len(t, images, 1)
	require.Equal(t, []byte{9, 9}, images[0].Data)
}

func TestForwardAsChatCompletionsPassesImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := cursorAccountWithFreshToken(61)
	svc := NewCursorGatewayService(nil, nil)
	svc.availableModels = func(context.Context, cursor.Credentials) ([]cursor.AvailableModel, error) {
		return nil, fmt.Errorf("catalog unused")
	}
	var captured cursor.AgentRunRequest
	svc.streamChat = func(_ context.Context, _ cursor.Credentials, req cursor.AgentRunRequest) (*http.Response, error) {
		captured = req
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}

	pngB64 := base64.StdEncoding.EncodeToString([]byte{1})
	body := []byte(`{"model":"grok-4.6","stream":false,"messages":[{"role":"user","content":[{"type":"text","text":"look"},{"type":"image_url","image_url":{"url":"data:image/png;base64,` + pngB64 + `"}}]}]}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Len(t, captured.Images, 1)
	require.Equal(t, "image/png", captured.Images[0].Mime)
	require.Equal(t, []byte{1}, captured.Images[0].Data)
}
