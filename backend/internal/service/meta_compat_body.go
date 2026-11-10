package service

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"math"
	"net/http"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/pkg/meta"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var metaCompatIntegerTimestampPaths = []string{"created_at", "response.created_at"}

type metaCompatResponseBody struct {
	source  io.ReadCloser
	reader  *bufio.Reader
	pending []byte
	usage   *meta.SubscriptionUsage
	onClose func(*meta.SubscriptionUsage)
	once    sync.Once
}

func (s *OpenAIGatewayService) wrapMetaCompatResponse(ctx context.Context, account *Account, resp *http.Response) {
	if resp == nil || resp.Body == nil || resp.StatusCode >= http.StatusBadRequest || !account.IsMeta() {
		return
	}
	resp.Body = newMetaCompatResponseBody(resp.Body, func(usage *meta.SubscriptionUsage) {
		s.sampleMetaSubscriptionUsageWithTimeout(ctx, account, usage)
	})
}

func newMetaCompatResponseBody(source io.ReadCloser, onClose func(*meta.SubscriptionUsage)) *metaCompatResponseBody {
	return &metaCompatResponseBody{source: source, reader: bufio.NewReader(source), onClose: onClose}
}

func (b *metaCompatResponseBody) Read(p []byte) (int, error) {
	for len(b.pending) == 0 {
		line, err := b.reader.ReadBytes('\n')
		if len(line) > 0 {
			b.pending = b.normalizeLine(line)
		}
		if err != nil {
			if len(b.pending) > 0 {
				break
			}
			return 0, err
		}
	}
	n := copy(p, b.pending)
	b.pending = b.pending[n:]
	return n, nil
}

func (b *metaCompatResponseBody) Close() error {
	b.once.Do(func() {
		if b.usage != nil && b.onClose != nil {
			b.onClose(b.usage)
		}
	})
	return b.source.Close()
}

func (b *metaCompatResponseBody) normalizeLine(line []byte) []byte {
	payload, ok := bytes.CutPrefix(line, []byte("data:"))
	if !ok {
		return line
	}
	payload = bytes.TrimSpace(payload)
	if meta.IsSubscriptionUsageEvent("", payload) {
		if usage, ok := meta.ParseSubscriptionUsage(payload); ok {
			b.usage = usage
		}
		return line
	}
	if !bytes.Contains(payload, []byte("created_at")) {
		return line
	}
	normalized, changed := normalizeMetaCompatTimestamps(payload)
	if !changed {
		return line
	}
	out := make([]byte, 0, len(normalized)+8)
	out = append(out, "data: "...)
	out = append(out, normalized...)
	return append(out, '\n')
}

func normalizeMetaCompatTimestamps(payload []byte) ([]byte, bool) {
	changed := false
	for _, path := range metaCompatIntegerTimestampPaths {
		value := gjson.GetBytes(payload, path)
		if value.Type != gjson.Number || !bytes.ContainsAny([]byte(value.Raw), ".eE") {
			continue
		}
		updated, err := sjson.SetBytes(payload, path, int64(math.Trunc(value.Float())))
		if err != nil {
			continue
		}
		payload = updated
		changed = true
	}
	return payload, changed
}
