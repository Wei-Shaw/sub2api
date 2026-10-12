package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/tidwall/gjson"
)

const anthropicFirstOutputWait = 30 * time.Second

// Pool-mode error policy must not bypass the model cooldown.
func (s *GatewayService) recordAnthropicRateLimit(ctx context.Context, account *Account, model string, headers http.Header) {
	if !anthropicStreamRecoveryAccount(account) || ctx.Err() != nil {
		return
	}
	now := time.Now()
	cooldown := 30 * time.Second
	if reset := parseRetryAfterResetTime(headers, now); reset != nil && reset.After(now) {
		cooldown = reset.Sub(now)
	}
	s.anthropicStreamRecovery().recordFailureWithInitialCooldown(account.ID, model, now, cooldown,
		openAITransientFailureOptions{requireProbe: true, reason: "anthropic_rate_limit", backoffBase: 30 * time.Second, backoffCap: 5 * time.Minute})
}

func (s *GatewayService) doAnthropicAvailabilityRequest(ctx context.Context, req *http.Request, proxyURL string, account *Account, model string, profile *tlsfingerprint.Profile) (*http.Response, error) {
	if !anthropicStreamRecoveryAccount(account) {
		return s.httpUpstream.DoWithTLS(req, proxyURL, account.ID, account.Concurrency, profile)
	}
	resp, err := guardAnthropicFirstOutput(ctx, req, anthropicFirstOutputWait, func(r *http.Request) (*http.Response, error) {
		return s.httpUpstream.DoWithTLS(r, proxyURL, account.ID, account.Concurrency, profile)
	}, func(headers http.Header) { s.recordAnthropicRateLimit(ctx, account, model, headers) })
	if err == nil && resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		s.recordAnthropicRateLimit(ctx, account, model, resp.Header)
	}
	return resp, err
}

type anthropicGuardBody struct {
	io.Reader
	body   io.Closer
	cancel context.CancelFunc
}

func (b *anthropicGuardBody) Close() error { b.cancel(); return b.body.Close() }

// Bound headers and pre-output SSE heartbeats using one deadline. Replay the
// exact prefix so protocol parsers and billing retain their existing behavior.
// Observed usage is a commit boundary too: retrying a potentially billable
// attempt is unsafe even if the upstream hasn't emitted visible text yet.
func guardAnthropicFirstOutput(clientCtx context.Context, req *http.Request, wait time.Duration, do func(*http.Request) (*http.Response, error), rateLimited func(http.Header)) (*http.Response, error) {
	upstreamCtx, cancel := context.WithCancel(req.Context())
	var mu sync.Mutex
	active, expired := true, false
	var body io.ReadCloser
	timer := time.AfterFunc(wait, func() {
		mu.Lock()
		if !active {
			mu.Unlock()
			return
		}
		expired = true
		b := body
		mu.Unlock()
		cancel()
		if b != nil {
			_ = b.Close()
		}
	})
	stopClient := context.AfterFunc(clientCtx, cancel)
	stop := func() bool {
		mu.Lock()
		active = false
		timedOut := expired
		mu.Unlock()
		timer.Stop()
		stopClient()
		return timedOut
	}
	resp, err := do(req.Clone(upstreamCtx))
	if err != nil {
		timedOut := stop()
		cancel()
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		if clientCtx.Err() != nil {
			return nil, clientCtx.Err()
		}
		if timedOut {
			return nil, fmt.Errorf("anthropic first output timeout: %w", context.DeadlineExceeded)
		}
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		stop()
		cancel()
		// Preserve each forward path's existing empty-response handling.
		return resp, nil
	}
	mu.Lock()
	body = resp.Body
	alreadyExpired := expired
	mu.Unlock()
	if alreadyExpired {
		stop()
		cancel()
		_ = body.Close()
		return nil, fmt.Errorf("anthropic first output timeout: %w", context.DeadlineExceeded)
	}
	finish := func(prefix []byte, reader io.Reader) (*http.Response, error) {
		if stop() {
			cancel()
			_ = body.Close()
			if clientCtx.Err() != nil {
				return nil, clientCtx.Err()
			}
			return nil, fmt.Errorf("anthropic first output timeout: %w", context.DeadlineExceeded)
		}
		resp.Body = &anthropicGuardBody{Reader: io.MultiReader(bytes.NewReader(prefix), reader), body: body, cancel: cancel}
		return resp, nil
	}
	// Error bodies and non-streaming responses retain their existing parsers.
	if resp.StatusCode >= 400 || !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return finish(nil, body)
	}
	reader := bufio.NewReader(body)
	var prefix bytes.Buffer
	var line bytes.Buffer
	for {
		part, readErr := reader.ReadSlice('\n')
		if prefix.Len()+len(part) > openAIFirstOutputStageMaxBytes {
			stop()
			cancel()
			_ = body.Close()
			return nil, errOpenAIFirstOutputStageLimit
		}
		prefix.Write(part)
		line.Write(part)
		if readErr == bufio.ErrBufferFull {
			continue
		}
		value := strings.TrimSpace(line.String())
		line.Reset()
		if data, ok := extractAnthropicSSEDataLine(value); ok {
			kind := gjson.Get(data, "type").String()
			if kind == "error" && anthropicStreamErrorStatus(data) == 429 && rateLimited != nil {
				rateLimited(resp.Header)
			}
			usage := gjson.Get(data, "message.usage")
			if !usage.Exists() {
				usage = gjson.Get(data, "usage")
			}
			billable := false
			usage.ForEach(func(_, v gjson.Result) bool {
				if v.Type == gjson.Number && v.Int() > 0 {
					billable = true
				}
				return true
			})
			if anthropicStreamHasSemanticOutput(data) || kind == "error" || anthropicStreamEventIsTerminal("", data) || billable {
				return finish(prefix.Bytes(), reader)
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return finish(prefix.Bytes(), reader)
			}
			timedOut := stop()
			cancel()
			_ = body.Close()
			if clientCtx.Err() != nil {
				return nil, clientCtx.Err()
			}
			if timedOut {
				return nil, fmt.Errorf("anthropic first output timeout: %w", context.DeadlineExceeded)
			}
			return nil, readErr
		}
	}
}
