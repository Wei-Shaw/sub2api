//go:build unit

package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type chatStreamFailoverBody struct {
	payload []byte
	err     error
	onRead  func()
}

func (b *chatStreamFailoverBody) Read(p []byte) (int, error) {
	if len(b.payload) != 0 {
		n := copy(p, b.payload)
		b.payload = b.payload[n:]
		return n, nil
	}
	if b.onRead != nil {
		b.onRead()
		b.onRead = nil
	}
	return 0, b.err
}
func (b *chatStreamFailoverBody) Close() error { return nil }

type chatStreamFailoverUpstream struct {
	service.HTTPUpstream
	accountIDs []int64
	firstBody  io.ReadCloser
	secondBody io.ReadCloser
}

func (u *chatStreamFailoverUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	u.accountIDs = append(u.accountIDs, accountID)
	body := u.firstBody
	if len(u.accountIDs) > 1 {
		body = u.secondBody
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       body,
	}, nil
}

func newChatStreamFailoverContext(t *testing.T, ctx context.Context) (*gin.Context, *bytes.Buffer) {
	t.Helper()
	c, rec := newOpenAIResponsesFailoverTestContext(t, ctx)
	body := `{"model":"gpt-5.1","stream":true,"messages":[{"role":"user","content":"hello"}]}`
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	return c, rec.Body
}

func TestOpenAIChatCompletionsHandler_StreamReadFailoverBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const delta = "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"replacement\"}\n\n"
	const partial = "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"
	transportErr := errors.New("stream error: stream ID 7; INTERNAL_ERROR; received from peer")
	for _, tt := range []struct {
		name        string
		firstBody   func(context.CancelFunc) io.ReadCloser
		wantCalls   []int64
		wantReplace bool
		wantPartial bool
	}{
		{
			name:        "pre-output read error switches account",
			firstBody:   func(context.CancelFunc) io.ReadCloser { return &chatStreamFailoverBody{err: transportErr} },
			wantCalls:   []int64{1, 2},
			wantReplace: true,
		},
		{
			name: "post-output read error never replays",
			firstBody: func(context.CancelFunc) io.ReadCloser {
				return &chatStreamFailoverBody{payload: []byte(partial), err: transportErr}
			},
			wantCalls:   []int64{1},
			wantPartial: true,
		},
		{
			name: "cancelled client never switches account",
			firstBody: func(cancel context.CancelFunc) io.ReadCloser {
				return &chatStreamFailoverBody{err: transportErr, onRead: cancel}
			},
			wantCalls: []int64{1},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			upstream := &chatStreamFailoverUpstream{
				firstBody:  tt.firstBody(cancel),
				secondBody: io.NopCloser(strings.NewReader(delta + "data: [DONE]\n\n")),
			}
			h := newOpenAIResponsesFailoverTestHandler(t, upstream)
			c, output := newChatStreamFailoverContext(t, ctx)
			h.ChatCompletions(c)
			require.Equal(t, tt.wantCalls, upstream.accountIDs)
			if tt.wantReplace {
				require.Contains(t, output.String(), "replacement")
				require.NotContains(t, output.String(), "partial")
				require.NotContains(t, output.String(), "INTERNAL_ERROR")
			}
			if tt.wantPartial {
				require.Contains(t, output.String(), "partial")
				require.NotContains(t, output.String(), "replacement")
			}
			if tt.name == "cancelled client never switches account" {
				require.NotContains(t, output.String(), "replacement")
			}
		})
	}
}
