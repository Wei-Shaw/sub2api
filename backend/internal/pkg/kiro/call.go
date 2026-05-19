package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// CallOptions parameterises a single Call invocation.
type CallOptions struct {
	// AccessToken is the bearer token. Required.
	AccessToken string

	// MachineID is the sticky per-account UUID stored on extra.machine_id.
	MachineID string

	// PreferredEndpoint chooses which of the 3 upstream endpoints to try
	// first. Empty means "auto" (Kiro IDE first, then fallbacks). Other
	// values: "kiro", "codewhisperer", "amazonq".
	PreferredEndpoint string

	// EndpointFallback controls whether to fall through to the other
	// endpoints on 429/5xx. Defaults to true.
	EndpointFallback bool

	// Payload is the request body (TransformAnthropicRequest output).
	Payload *Payload

	// HTTPClient is the *http.Client used for the streaming request.
	// Must have a long enough Timeout; HTTPClient() in client.go is for
	// REST calls — gateway callers should pass a longer-timeout client.
	HTTPClient *http.Client

	// OnAttempt is invoked with the endpoint name before each attempt.
	// Useful for ops logging / failover tracking. Optional.
	OnAttempt func(endpoint string)
}

// CallResult bundles the outcome of a successful Call. Either Response
// is set (caller owns the body, must close it) or an error is returned.
type CallResult struct {
	Response *http.Response
	Endpoint Endpoint
}

// Call posts the payload to Kiro and returns the open streaming response
// from the first endpoint that succeeds. The caller is responsible for
// decoding the event stream (DecodeEventStream) and closing resp.Body.
//
// Behaviour matches Kiro-Go's endpoint fallback rules:
//   - 200 → return immediately
//   - 429 → try next endpoint (quota exhausted on the primary)
//   - 401/403/402 → return immediately (auth/payment errors don't help)
//   - other non-2xx → try next endpoint
//   - transport errors → try next endpoint
//
// All endpoints exhausted → returns the last error.
func Call(ctx context.Context, opts CallOptions) (*CallResult, error) {
	if opts.HTTPClient == nil {
		return nil, fmt.Errorf("kiro: HTTPClient required")
	}
	if opts.Payload == nil {
		return nil, fmt.Errorf("kiro: Payload required")
	}
	if opts.AccessToken == "" {
		return nil, fmt.Errorf("kiro: AccessToken required")
	}

	endpoints := EndpointPreference(opts.PreferredEndpoint)
	if !opts.EndpointFallback {
		// Only try the first endpoint.
		endpoints = endpoints[:1]
	}

	var lastErr error
	for _, ep := range endpoints {
		if opts.OnAttempt != nil {
			opts.OnAttempt(ep.Name)
		}

		// Re-marshal per attempt because we mutate the origin field.
		opts.Payload.ConversationState.CurrentMessage.UserInputMessage.Origin = ep.Origin
		body, err := MarshalPayload(opts.Payload)
		if err != nil {
			return nil, fmt.Errorf("kiro: marshal payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}

		host := ""
		if parsedURL, parseErr := url.Parse(ep.URL); parseErr == nil {
			host = parsedURL.Host
		}
		values := BuildStreamingHeaderValues(opts.MachineID, host)
		ApplyBaseHeaders(req, opts.AccessToken, values)
		if ep.AmzTarget != "" {
			req.Header.Set("X-Amz-Target", ep.AmzTarget)
		}

		resp, err := opts.HTTPClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("kiro: %s: %w", ep.Name, err)
			continue
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return &CallResult{Response: resp, Endpoint: ep}, nil
		case http.StatusTooManyRequests:
			drainAndClose(resp)
			lastErr = fmt.Errorf("kiro: %s: quota exhausted (429)", ep.Name)
			continue
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusPaymentRequired:
			errBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return nil, &HTTPError{StatusCode: resp.StatusCode, Body: errBody}
		default:
			errBody, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastErr = &HTTPError{StatusCode: resp.StatusCode, Body: errBody}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("kiro: all endpoints failed")
	}
	return nil, lastErr
}

// drainAndClose flushes and closes a response body so the connection can
// be reused. Cheaper than letting it GC.
func drainAndClose(resp *http.Response) {
	if resp == nil {
		return
	}
	if resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}

// jsonMarshalIndent is only used by tests for pretty-printing diagnostic
// payloads in fixture generation.
func jsonMarshalIndent(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
