package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"strings"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type VideoAdapter interface {
	Provider() string
	CreateTask(ctx context.Context, account *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error)
	PollTask(ctx context.Context, account *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error)
	CancelTask(ctx context.Context, account *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error)
	NormalizeStatus(upstream string) string
	BuildCreatePayload(account *VideoProviderAccount, task *VideoTask) map[string]any
}

type VideoAdapterResult struct {
	UpstreamTaskID   string
	Status           string
	ResultURL        string
	UsageTotalTokens *int64
	ActualResolution string
	ActualDuration   *int
	LastFrameURL     string
	ErrorMessage     string
	CostEstimate     float64
	Payload          map[string]any
}

func NewVideoAdapterRegistry() map[string]VideoAdapter {
	adapters := []VideoAdapter{
		&mockVideoAdapter{},
		&seedanceVideoAdapter{},
		&klingVideoAdapter{},
	}
	out := make(map[string]VideoAdapter, len(adapters))
	for _, adapter := range adapters {
		out[adapter.Provider()] = adapter
	}
	return out
}

type mockVideoAdapter struct{}

func (a *mockVideoAdapter) Provider() string { return VideoProviderMock }

func mockVideoResultURL(taskID int64) string {
	return fmt.Sprintf("/api/v1/video/mock-assets/%d.svg", taskID)
}

func (a *mockVideoAdapter) CreateTask(_ context.Context, _ *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{
		UpstreamTaskID: fmt.Sprintf("mock-video-%d", task.ID),
		Status:         VideoStatusSubmitted,
		Payload: map[string]any{
			"provider":         VideoProviderMock,
			"mock_should_fail": mockShouldFail(task.Prompt),
			"created_at":       time.Now().UTC().Format(time.RFC3339),
		},
	}, nil
}

func (a *mockVideoAdapter) PollTask(_ context.Context, _ *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error) {
	if task.Status == VideoStatusSubmitted {
		return &VideoAdapterResult{
			Status: VideoStatusRunning,
			Payload: map[string]any{
				"stage": "mock_render_started",
			},
		}, nil
	}
	if mockShouldFail(task.Prompt) {
		return &VideoAdapterResult{
			Status:       VideoStatusFailed,
			ErrorMessage: "mock provider forced failure for P0 validation",
			Payload: map[string]any{
				"stage":  "mock_render_failed",
				"reason": "prompt contains failure trigger",
			},
		}, nil
	}
	resultURL := mockVideoResultURL(task.ID)
	return &VideoAdapterResult{
		Status:       VideoStatusSucceeded,
		ResultURL:    resultURL,
		CostEstimate: 0,
		Payload: map[string]any{
			"stage":      "mock_render_completed",
			"result_url": resultURL,
		},
	}, nil
}

func (a *mockVideoAdapter) CancelTask(_ context.Context, _ *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{
		Status: VideoStatusCancelled,
		Payload: map[string]any{
			"upstream_task_id": task.UpstreamTaskID,
			"stage":            "mock_cancelled",
		},
	}, nil
}

func (a *mockVideoAdapter) NormalizeStatus(upstream string) string {
	return normalizeVideoStatus(upstream)
}

func (a *mockVideoAdapter) BuildCreatePayload(_ *VideoProviderAccount, task *VideoTask) map[string]any {
	return map[string]any{
		"model":               task.Model,
		"task_type":           task.TaskType,
		"prompt":              task.Prompt,
		"negative_prompt":     task.NegativePrompt,
		"reference_image_url": task.ReferenceImageURL,
		"reference_video_url": task.ReferenceVideoURL,
		"content":             task.Content,
		"has_video_input":     task.HasVideoInput,
		"aspect_ratio":        task.AspectRatio,
		"duration":            task.Duration,
		"resolution":          task.Resolution,
	}
}

func mockShouldFail(prompt string) bool {
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, "[fail]") ||
		strings.Contains(lower, "mock:fail") ||
		strings.Contains(prompt, "\u5931\u8d25")
}

type seedanceVideoAdapter struct{}

func (a *seedanceVideoAdapter) Provider() string { return VideoProviderSeedance }

func (a *seedanceVideoAdapter) CreateTask(ctx context.Context, account *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error) {
	if !account.APIKeyConfigured || strings.TrimSpace(account.PlainAPIKey) == "" {
		return nil, ErrVideoProviderDisabled.WithMetadata(map[string]string{
			"provider": "seedance",
			"reason":   "api key is not configured; real call skipped",
		})
	}
	if reasons := seedanceSmokeGateBlockedReasons(account, task); len(reasons) > 0 {
		return nil, ErrVideoProviderDisabled.WithMetadata(map[string]string{
			"provider":           "seedance",
			"reason":             strings.Join(reasons, "; "),
			"real_call_executed": "false",
		})
	}
	// PRE-ARM redaction self-check (fail-closed). Runs at the adapter chokepoint so it
	// guards EVERY real call 闁?the Form A worker path AND the direct Form B path 闁?before
	// any socket is opened. Aborts with no network call if the configured key would
	// survive into any echo channel. (Closes B-0 gap-4: the worker path previously had
	// no pre-arm self-check; only the Form B test did.)
	if err := seedancePreArmRedactionSelfCheck(account); err != nil {
		return nil, err
	}

	baseURL := account.BaseURL
	if baseURL == "" {
		baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}

	model := task.Model
	if model == "" {
		model = "doubao-seedance-2-0-260128"
	}

	content := []map[string]any{{"type": "text", "text": task.Prompt}}
	var err error
	if task.ReferenceImageURL != "" {
		if err := validateExternalVideoURL(task.ReferenceImageURL); err != nil {
			// The validator error echoes the rejected URL; redact (key-aware) before it
			// reaches the API response / routed event so no credential-bearing URL leaks.
			return nil, infraerrors.BadRequest("VIDEO_UNSAFE_REFERENCE_URL",
				"reference_image_url failed SSRF/allowlist validation: "+seedanceRedactBody(account, err.Error()))
		}
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": task.ReferenceImageURL}})
	}
	if task.ReferenceVideoURL != "" {
		if err := validateExternalVideoURL(task.ReferenceVideoURL); err != nil {
			return nil, infraerrors.BadRequest("VIDEO_UNSAFE_REFERENCE_URL",
				"reference_video_url failed SSRF/allowlist validation: "+seedanceRedactBody(account, err.Error()))
		}
		// Attach the reference video to the content array, mirroring the image_url
		// shape + the same SSRF/allowlist gate. The `video_url` request shape is
		// covered by local contract tests; a paid v2v confirmation is still tracked
		// outside this backend patch before broad production rollout.
		content = append(content, map[string]any{"type": "video_url", "video_url": map[string]string{"url": task.ReferenceVideoURL}})
	}
	if len(task.Content) > 0 {
		content, err = buildSeedanceCreateContent(account, task)
		if err != nil {
			return nil, err
		}
	}

	payload := map[string]any{
		"model":   model,
		"content": content,
	}
	applySeedanceBoolOption(payload, "generate_audio", task.GenerateAudio)
	applySeedanceBoolOption(payload, "watermark", task.Watermark)
	applySeedanceBoolOption(payload, "camera_fixed", task.CameraFixed)
	applySeedanceBoolOption(payload, "return_last_frame", task.ReturnLastFrame)
	if task.NegativePrompt != "" {
		payload["negative_prompt"] = task.NegativePrompt
	}
	// Explicitly send generation parameters so the smoke/production gate duration
	// is applied upstream instead of relying on Ark defaults. The create payload
	// field names (`duration`, `resolution`, `ratio`) are covered by local snapshot
	// tests and archived Ark-shaped fixtures; whether `ratio:9:16` yields the
	// intended portrait clip still needs one real paid confirmation.
	if task.Duration > 0 {
		payload["duration"] = task.Duration
	}
	if task.Resolution != "" {
		payload["resolution"] = task.Resolution
	}
	if ratio := normalizeSeedanceRatio(task.AspectRatio); ratio != "" {
		payload["ratio"] = ratio
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("seedance: marshal create payload: %w", err)
	}

	url := strings.TrimRight(baseURL, "/") + "/contents/generations/tasks"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("seedance: build create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+account.PlainAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := executeVideoDispatchRequest(client, req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readSeedanceCreateResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	// Audit the real upstream response (secrets stripped) to the redacted event
	// log that the smoke gate requires. Fail-closed: no audit => abort. Key-aware so
	// the configured key is stripped from the audited body regardless of its shape.
	if auditErr := appendRedactedVideoEvent(account.PlainAPIKey, "create", resp.StatusCode, string(respBody)); auditErr != nil {
		return nil, infraerrorsUnavailable("SEEDANCE_AUDIT_LOG_FAILED", "Seedance create: redacted audit log unavailable: "+auditErr.Error())
	}

	// Fail-closed if the upstream echoed our configured key ANYWHERE in the body 闁?including a
	// STRUCTURAL field (e.g. {"id":"<key>"}) that would otherwise be stored raw as
	// upstream_task_id / event payload, bypassing the body redactor. Audited (redacted) above
	// first, so evidence is kept; the returned error carries no key.
	if seedanceUpstreamEchoedKey(account, string(respBody)) {
		return nil, infraerrorsUnavailable("SEEDANCE_UPSTREAM_ECHOED_CREDENTIAL",
			"Seedance create aborted: upstream response echoed the configured credential in a stored field; refusing to persist it (key value intentionally not included)")
	}
	return parseSeedanceCreateResponse(account, model, resp.StatusCode, respBody)
}

func readSeedanceCreateResponseBody(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, &VideoDispatchTransportError{
			Err:                    infraerrorsUnavailable("SEEDANCE_CREATE_RESPONSE_READ_ERROR", "Seedance create response body could not be read completely"),
			RequestMayHaveBeenSent: true,
		}
	}
	return body, nil
}

func parseSeedanceCreateResponse(account *VideoProviderAccount, model string, statusCode int, responseBody []byte) (*VideoAdapterResult, error) {
	if statusCode < 200 || statusCode >= 300 {
		// Redact the upstream body before it flows into task.ErrorMessage (DB),
		// video_task_events.Message (DB) and the API response: a 401/403 body
		// can echo the Authorization header or an API-key prefix.
		return nil, infraerrorsUnavailable("SEEDANCE_CREATE_UPSTREAM_ERROR",
			fmt.Sprintf("Seedance create task returned %d (%s): %s", statusCode, seedanceHTTPErrorType(statusCode, string(responseBody)), truncate(seedanceRedactBody(account, string(responseBody)), 500)))
	}

	var parsed struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Error  struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return nil, &VideoDispatchTransportError{
			Err:                    infraerrorsUnavailable("SEEDANCE_CREATE_RESPONSE_INVALID", "Seedance create returned an invalid success response"),
			RequestMayHaveBeenSent: true,
		}
	}
	// Escape-proof structural-field guard: a JSON-escaped key (e.g. "A...") slips past the
	// raw-body check above but decodes back to the key in parsed.ID, which is stored raw as
	// upstream_task_id and the payload `upstream_id`. Check the DECODED field and refuse it.
	if seedanceUpstreamEchoedKey(account, parsed.ID) {
		return nil, infraerrorsUnavailable("SEEDANCE_UPSTREAM_ECHOED_CREDENTIAL",
			"Seedance create aborted: upstream id field echoed the configured credential; refusing to persist it (key value intentionally not included)")
	}

	if parsed.Error.Message != "" {
		// A 200-OK body can still carry an error.message that echoes the request
		// or a credential prefix; redact before it reaches DB/events/API response.
		return nil, infraerrorsUnavailable("SEEDANCE_CREATE_BUSINESS_ERROR", "Seedance: "+seedanceRedactBody(account, parsed.Error.Message))
	}
	if strings.TrimSpace(parsed.ID) == "" {
		return nil, &VideoDispatchTransportError{
			Err:                    infraerrorsUnavailable("SEEDANCE_CREATE_RESPONSE_INVALID", "Seedance create success response did not include an upstream task id"),
			RequestMayHaveBeenSent: true,
		}
	}

	adapter := &seedanceVideoAdapter{}
	normalizedStatus := adapter.NormalizeStatus(parsed.Status)
	return &VideoAdapterResult{
		UpstreamTaskID: parsed.ID,
		Status:         normalizedStatus,
		Payload: map[string]any{
			"provider":          "seedance",
			"upstream_id":       parsed.ID,
			"model":             model,
			"normalized_status": normalizedStatus,
			"qcanvas_status":    qcanvasVideoContractStatus(normalizedStatus, ""),
			"redacted_event":    true,
			"created_at":        time.Now().UTC().Format(time.RFC3339),
		},
	}, nil
}

func executeVideoDispatchRequest(client *http.Client, request *http.Request) (*http.Response, error) {
	if client == nil || request == nil {
		return nil, &VideoDispatchTransportError{
			Err: infraerrorsUnavailable("SEEDANCE_CREATE_HTTP_ERROR", "Seedance create task HTTP client or request is unavailable"),
		}
	}
	if client.Transport != nil {
		if _, ok := client.Transport.(*http.Transport); !ok {
			response, err := client.Do(request)
			if err != nil {
				return nil, &VideoDispatchTransportError{
					Err:                    infraerrorsUnavailable("SEEDANCE_CREATE_HTTP_ERROR", "Seedance create task custom HTTP transport failed"),
					RequestMayHaveBeenSent: true,
				}
			}
			return response, nil
		}
	}
	evidence := &videoDispatchWriteEvidence{}
	dispatchClient, transport, err := cloneVideoDispatchHTTPClient(client, evidence)
	if err != nil {
		return nil, &VideoDispatchTransportError{Err: err}
	}
	defer transport.CloseIdleConnections()
	trace := &httptrace.ClientTrace{
		WroteHeaders: evidence.markHeadersBuffered,
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))
	response, err := dispatchClient.Do(request)
	if err != nil {
		return nil, &VideoDispatchTransportError{
			Err:                    infraerrorsUnavailable("SEEDANCE_CREATE_HTTP_ERROR", "Seedance create task HTTP error: "+err.Error()),
			RequestMayHaveBeenSent: evidence.requestBytesWrittenAfterHeaders(),
		}
	}
	return response, nil
}

type videoDispatchWriteEvidence struct {
	connection      atomic.Pointer[countingVideoDispatchConn]
	headersBuffered atomic.Bool
	bytesAtHeaders  atomic.Int64
}

func (e *videoDispatchWriteEvidence) wrapConnection(connection net.Conn) net.Conn {
	counted := &countingVideoDispatchConn{Conn: connection}
	e.connection.Store(counted)
	return counted
}

func (e *videoDispatchWriteEvidence) markHeadersBuffered() {
	if connection := e.connection.Load(); connection != nil {
		e.bytesAtHeaders.Store(connection.bytesWritten.Load())
	}
	e.headersBuffered.Store(true)
}

func (e *videoDispatchWriteEvidence) requestBytesWrittenAfterHeaders() bool {
	if !e.headersBuffered.Load() {
		return false
	}
	connection := e.connection.Load()
	return connection != nil && connection.bytesWritten.Load() > e.bytesAtHeaders.Load()
}

type countingVideoDispatchConn struct {
	net.Conn
	bytesWritten atomic.Int64
}

func (c *countingVideoDispatchConn) Write(payload []byte) (int, error) {
	written, err := c.Conn.Write(payload)
	if written > 0 {
		c.bytesWritten.Add(int64(written))
	}
	return written, err
}

func cloneVideoDispatchHTTPClient(client *http.Client, evidence *videoDispatchWriteEvidence) (*http.Client, *http.Transport, error) {
	baseRoundTripper := client.Transport
	if baseRoundTripper == nil {
		baseRoundTripper = http.DefaultTransport
	}
	baseTransport, ok := baseRoundTripper.(*http.Transport)
	if !ok {
		return nil, nil, infraerrorsUnavailable("SEEDANCE_CREATE_HTTP_ERROR", "Seedance create task HTTP transport does not expose connection write evidence")
	}
	transport := baseTransport.Clone()
	transport.DisableKeepAlives = true

	if dialContext := transport.DialContext; dialContext != nil {
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			connection, err := dialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return evidence.wrapConnection(connection), nil
		}
	} else if dial := transport.Dial; dial != nil { //nolint:staticcheck // preserve legacy custom Dial precedence and write evidence
		transport.Dial = func(network, address string) (net.Conn, error) { //nolint:staticcheck // do not replace a configured legacy Dial
			connection, err := dial(network, address)
			if err != nil {
				return nil, err
			}
			return evidence.wrapConnection(connection), nil
		}
	} else {
		dialContext := (&net.Dialer{}).DialContext
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			connection, err := dialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return evidence.wrapConnection(connection), nil
		}
	}
	if dialTLSContext := transport.DialTLSContext; dialTLSContext != nil {
		transport.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			connection, err := dialTLSContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return evidence.wrapConnection(connection), nil
		}
	}
	if dialTLS := transport.DialTLS; dialTLS != nil { //nolint:staticcheck // legacy custom transports must not bypass request-byte evidence
		transport.DialTLS = func(network, address string) (net.Conn, error) { //nolint:staticcheck // preserve evidence for legacy custom transports
			connection, err := dialTLS(network, address)
			if err != nil {
				return nil, err
			}
			return evidence.wrapConnection(connection), nil
		}
	}

	dispatchClient := *client
	dispatchClient.Transport = transport
	return &dispatchClient, transport, nil
}

func buildSeedanceCreateContent(account *VideoProviderAccount, task *VideoTask) ([]map[string]any, error) {
	items := task.Content
	if len(items) == 0 {
		items = []VideoTaskContentItem{{Type: VideoContentTypeText, Text: task.Prompt}}
	}
	content := make([]map[string]any, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case VideoContentTypeText:
			text := strings.TrimSpace(item.Text)
			if text == "" {
				text = task.Prompt
			}
			content = append(content, map[string]any{"type": "text", "text": text})
		case VideoContentTypeImageURL:
			if err := validateExternalVideoURL(item.URL); err != nil {
				return nil, infraerrors.BadRequest("VIDEO_UNSAFE_REFERENCE_URL",
					"content.image_url failed SSRF/allowlist validation: "+seedanceRedactBody(account, err.Error()))
			}
			content = append(content, seedanceURLContentItem("image_url", "image_url", item.Role, item.URL))
		case VideoContentTypeVideoURL:
			if err := validateExternalVideoURL(item.URL); err != nil {
				return nil, infraerrors.BadRequest("VIDEO_UNSAFE_REFERENCE_URL",
					"content.video_url failed SSRF/allowlist validation: "+seedanceRedactBody(account, err.Error()))
			}
			content = append(content, seedanceURLContentItem("video_url", "video_url", item.Role, item.URL))
		case VideoContentTypeAudioURL:
			if err := validateExternalVideoURL(item.URL); err != nil {
				return nil, infraerrors.BadRequest("VIDEO_UNSAFE_REFERENCE_URL",
					"content.audio_url failed SSRF/allowlist validation: "+seedanceRedactBody(account, err.Error()))
			}
			content = append(content, seedanceURLContentItem("audio_url", "audio_url", item.Role, item.URL))
		}
	}
	return content, nil
}

func seedanceURLContentItem(contentType, urlField, role, rawURL string) map[string]any {
	item := map[string]any{
		"type":   contentType,
		urlField: map[string]string{"url": rawURL},
	}
	if strings.TrimSpace(role) != "" {
		item["role"] = strings.TrimSpace(role)
	}
	return item
}

func applySeedanceBoolOption(payload map[string]any, key string, value *bool) {
	if value == nil {
		return
	}
	payload[key] = *value
}

func (a *seedanceVideoAdapter) PollTask(ctx context.Context, account *VideoProviderAccount, task *VideoTask) (*VideoAdapterResult, error) {
	if !account.APIKeyConfigured || strings.TrimSpace(account.PlainAPIKey) == "" {
		return nil, ErrVideoProviderDisabled.WithMetadata(map[string]string{
			"provider": "seedance",
			"reason":   "api key is not configured; poll skipped",
		})
	}
	if reasons := seedanceSmokeGateBlockedReasons(account, task); len(reasons) > 0 {
		return nil, ErrVideoProviderDisabled.WithMetadata(map[string]string{
			"provider":           "seedance",
			"reason":             strings.Join(reasons, "; "),
			"real_call_executed": "false",
		})
	}
	// PRE-ARM redaction self-check (fail-closed) 闁?poll also sends Authorization:
	// Bearer <key> and a 401/403 poll body can echo it, so the same guard applies here.
	if err := seedancePreArmRedactionSelfCheck(account); err != nil {
		return nil, err
	}

	baseURL := account.BaseURL
	if baseURL == "" {
		baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	}

	url := strings.TrimRight(baseURL, "/") + "/contents/generations/tasks/" + task.UpstreamTaskID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("seedance: build poll request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+account.PlainAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, infraerrorsUnavailable("SEEDANCE_POLL_HTTP_ERROR", "Seedance poll task HTTP error: "+err.Error())
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("seedance: read poll response: %w", err)
	}

	// Audit the real upstream response (secrets stripped) to the redacted event
	// log that the smoke gate requires. Fail-closed: no audit => abort. Key-aware so
	// the configured key is stripped from the audited body regardless of its shape.
	if auditErr := appendRedactedVideoEvent(account.PlainAPIKey, "poll", resp.StatusCode, string(respBody)); auditErr != nil {
		return nil, infraerrorsUnavailable("SEEDANCE_AUDIT_LOG_FAILED", "Seedance poll: redacted audit log unavailable: "+auditErr.Error())
	}

	// Fail-closed if the upstream echoed our configured key anywhere in the poll body 闁?	// including the structural `id` (闁?upstream_task_id) or a result-url path segment (闁?	// result_url) that would be stored raw, bypassing the body redactor.
	if seedanceUpstreamEchoedKey(account, string(respBody)) {
		return nil, infraerrorsUnavailable("SEEDANCE_UPSTREAM_ECHOED_CREDENTIAL",
			"Seedance poll aborted: upstream response echoed the configured credential in a stored field; refusing to persist it (key value intentionally not included)")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Redact the upstream body before it flows into task.ErrorMessage (DB),
		// video_task_events.Message (DB) and the API response.
		return nil, infraerrorsUnavailable("SEEDANCE_POLL_UPSTREAM_ERROR",
			fmt.Sprintf("Seedance poll task returned %d (%s): %s", resp.StatusCode, seedanceHTTPErrorType(resp.StatusCode, string(respBody)), truncate(seedanceRedactBody(account, string(respBody)), 500)))
	}

	var parsed struct {
		ID               string `json:"id"`
		Status           string `json:"status"`
		Ratio            string `json:"ratio"` // Module C: Ark returns the aspect ratio as "ratio" (e.g. "16:9"), NOT "aspect_ratio"
		Resolution       string `json:"resolution"`
		ActualResolution string `json:"actual_resolution"`
		Duration         *int   `json:"duration"`
		ActualDuration   *int   `json:"actual_duration"`
		LastFrameURL     string `json:"last_frame_url"`
		Usage            struct {
			TotalTokens *int64 `json:"total_tokens"`
		} `json:"usage"`
		Content struct {
			VideoURL         string `json:"video_url"`
			Ratio            string `json:"ratio"` // tolerate nested shape too until a paid sample confirms nesting
			Resolution       string `json:"resolution"`
			ActualResolution string `json:"actual_resolution"`
			Duration         *int   `json:"duration"`
			ActualDuration   *int   `json:"actual_duration"`
			LastFrameURL     string `json:"last_frame_url"`
			LastFrame        struct {
				URL      string `json:"url"`
				ImageURL string `json:"image_url"`
			} `json:"last_frame"`
		} `json:"content"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("seedance: parse poll response: %w", err)
	}
	// Escape-proof structural-field guard: parsed.ID is stored raw as upstream_task_id and
	// parsed.Content.VideoURL as result_url. A JSON-escaped key in either decodes back to the
	// key past the raw-body check; check the DECODED fields and refuse them.
	lastFrameURL := firstNonEmptyVideo(parsed.LastFrameURL, parsed.Content.LastFrameURL, parsed.Content.LastFrame.URL, parsed.Content.LastFrame.ImageURL)
	if seedanceUpstreamEchoedKey(account, parsed.ID) ||
		seedanceUpstreamEchoedKey(account, parsed.Content.VideoURL) ||
		seedanceUpstreamEchoedKey(account, lastFrameURL) {
		return nil, infraerrorsUnavailable("SEEDANCE_UPSTREAM_ECHOED_CREDENTIAL",
			"Seedance poll aborted: upstream id/result_url/last_frame_url echoed the configured credential; refusing to persist it (key value intentionally not included)")
	}

	result := &VideoAdapterResult{
		UpstreamTaskID: parsed.ID,
		Status:         a.NormalizeStatus(parsed.Status),
		Payload: map[string]any{
			"provider":          "seedance",
			"normalized_status": a.NormalizeStatus(parsed.Status),
			"qcanvas_status":    qcanvasVideoContractStatus(a.NormalizeStatus(parsed.Status), parsed.Error.Message),
			"redacted_event":    true,
			"polled_at":         time.Now().UTC().Format(time.RFC3339),
		},
	}

	// Module C response field alignment: Ark poll fixtures expose top-level `ratio`;
	// the parser also tolerates nested content.ratio until a paid sample confirms
	// the exact upstream nesting. The request side now sends `ratio` too.
	// Gate on a strict "W:H" shape so unexpected upstream text cannot enter
	// persisted event payloads or API responses.
	if ratio := firstNonEmptyVideo(parsed.Ratio, parsed.Content.Ratio); looksLikeAspectRatio(ratio) {
		result.Payload["ratio"] = ratio
	}
	if parsed.Usage.TotalTokens != nil && *parsed.Usage.TotalTokens >= 0 {
		v := *parsed.Usage.TotalTokens
		result.UsageTotalTokens = &v
		result.Payload["usage"] = map[string]any{"total_tokens": v}
		result.Payload["usage_total_tokens"] = v
	}
	if resolution := firstNonEmptyVideo(parsed.ActualResolution, parsed.Resolution, parsed.Content.ActualResolution, parsed.Content.Resolution); looksLikeVideoResolution(resolution) {
		result.ActualResolution = strings.ToLower(strings.TrimSpace(resolution))
		result.Payload["actual_resolution"] = result.ActualResolution
	}
	if duration := firstNonNilInt(parsed.ActualDuration, parsed.Duration, parsed.Content.ActualDuration, parsed.Content.Duration); duration != nil && looksLikeActualVideoDuration(*duration) {
		v := *duration
		result.ActualDuration = &v
		result.Payload["actual_duration"] = v
	}

	// Do not trust the upstream-returned result_url blindly: validate scheme and
	// host (SSRF / domain allowlist) before storing it for the frontend to play
	// and write to Day0. A rejected URL fails the task instead of propagating.
	if parsed.Content.VideoURL != "" {
		if err := validateExternalVideoURL(parsed.Content.VideoURL); err != nil {
			result.Status = VideoStatusFailed
			// The validator error echoes the rejected upstream URL (which can carry a
			// presigned signature/token in its query string); redact (key-aware) before it
			// lands in task.ErrorMessage (DB), the failed event and the poll API response.
			result.ErrorMessage = "upstream result_url failed validation: " + seedanceRedactBody(account, err.Error())
			result.Payload["result_url_rejected"] = true
			return result, nil
		}
		result.ResultURL = parsed.Content.VideoURL
	}
	if lastFrameURL != "" {
		if err := validateExternalVideoURL(lastFrameURL); err != nil {
			result.Status = VideoStatusFailed
			result.ErrorMessage = "upstream last_frame_url failed validation: " + seedanceRedactBody(account, err.Error())
			result.Payload["last_frame_url_rejected"] = true
			return result, nil
		}
		result.LastFrameURL = lastFrameURL
		result.Payload["last_frame_url"] = lastFrameURL
	}

	if parsed.Error.Message != "" {
		// Same leak channel as create: a 200-OK error.message lands in
		// task.ErrorMessage (DB), the failed event, and the poll API response.
		result.ErrorMessage = seedanceRedactBody(account, parsed.Error.Message)
		result.Status = VideoStatusFailed
	}

	return result, nil
}

func (a *seedanceVideoAdapter) CancelTask(_ context.Context, _ *VideoProviderAccount, _ *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{Status: VideoStatusCancelled, Payload: map[string]any{"provider": "seedance", "mode": "skeleton"}}, nil
}

func (a *seedanceVideoAdapter) NormalizeStatus(upstream string) string {
	switch strings.ToLower(strings.TrimSpace(upstream)) {
	case "queued", "pending", "created":
		return VideoStatusQueued
	case "submitted", "in_queue":
		return VideoStatusSubmitted
	case "running", "processing":
		return VideoStatusRunning
	case "succeeded", "success", "completed":
		return VideoStatusSucceeded
	case "failed", "error":
		return VideoStatusFailed
	case "timeout", "timed_out", "expired":
		return VideoStatusFailed
	case "cancelled", "canceled", "deleted":
		return VideoStatusCancelled
	default:
		return VideoStatusRunning
	}
}

func (a *seedanceVideoAdapter) BuildCreatePayload(account *VideoProviderAccount, task *VideoTask) map[string]any {
	content := buildSeedancePayloadPreviewContent(task)
	payload := map[string]any{
		"base_url":         account.BaseURL,
		"model":            task.Model,
		"content":          content,
		"content_contract": task.Content,
		"has_video_input":  task.HasVideoInput,
		"negative_prompt":  task.NegativePrompt,
		// B1: Ark's create field is `ratio` (16:9 / 9:16 / 1:1), NOT `aspect_ratio`.
		"ratio":                       normalizeSeedanceRatio(task.AspectRatio),
		"duration":                    task.Duration,
		"resolution":                  task.Resolution,
		"smoke_gate_required":         true,
		"asset_persistable":           false,
		"asset_reuse_allowed":         false,
		"qcanvas_contract_statuses":   []string{"queued", "running", "succeeded", "failed", "canceled"},
		"qcanvas_contract_errors":     []string{"auth", "quota", "rate_limit", "provider_down", "invalid_prompt", "unsafe_content", "timeout", "unknown"},
		"redacted_event_log_required": true,
		"source_docs":                 "https://www.volcengine.com/docs/82379/1520757?lang=zh",
	}
	applySeedanceBoolOption(payload, "generate_audio", task.GenerateAudio)
	applySeedanceBoolOption(payload, "watermark", task.Watermark)
	applySeedanceBoolOption(payload, "camera_fixed", task.CameraFixed)
	applySeedanceBoolOption(payload, "return_last_frame", task.ReturnLastFrame)
	return payload
}

func buildSeedancePayloadPreviewContent(task *VideoTask) []map[string]any {
	if task == nil {
		return nil
	}
	items := task.Content
	if len(items) == 0 {
		items = []VideoTaskContentItem{{Type: VideoContentTypeText, Text: task.Prompt}}
		if task.ReferenceImageURL != "" {
			items = append(items, VideoTaskContentItem{Type: VideoContentTypeImageURL, Role: VideoContentRoleReferenceImage, URL: task.ReferenceImageURL})
		}
		if task.ReferenceVideoURL != "" {
			items = append(items, VideoTaskContentItem{Type: VideoContentTypeVideoURL, Role: VideoContentRoleReferenceVideo, URL: task.ReferenceVideoURL})
		}
	}
	content := make([]map[string]any, 0, len(items))
	for _, item := range items {
		switch item.Type {
		case VideoContentTypeText:
			text := strings.TrimSpace(item.Text)
			if text == "" {
				text = task.Prompt
			}
			content = append(content, map[string]any{"type": "text", "text": text})
		case VideoContentTypeImageURL:
			content = append(content, seedanceURLContentItem("image_url", "image_url", item.Role, item.URL))
		case VideoContentTypeVideoURL:
			content = append(content, seedanceURLContentItem("video_url", "video_url", item.Role, item.URL))
		case VideoContentTypeAudioURL:
			content = append(content, seedanceURLContentItem("audio_url", "audio_url", item.Role, item.URL))
		}
	}
	return content
}

// seedanceRedactBody is the real-call redaction chokepoint. It strips the account's
// configured key (shape-agnostic, via stripKnownVideoSecret) and then the shared/
// pattern secrets, so the configured key can never survive into the DB, an API error,
// or the audit log 闁?regardless of whether its shape falls in a pattern blind spot
// (12-19 chars / pure-letter / pure-digit). Every adapter path that turns an upstream
// body or message into stored/returned text goes through here.
func seedanceRedactBody(account *VideoProviderAccount, s string) string {
	key := ""
	if account != nil {
		key = account.PlainAPIKey
	}
	return redactVideoUpstreamSecretsForKey(s, key)
}

// seedanceUpstreamEchoedKey reports whether the raw upstream body contains the configured
// key verbatim. A legitimate provider response NEVER echoes your own API key; its presence
// 闁?whether in a free-text message OR in a STRUCTURAL field the adapter stores raw (the task
// `id` 闁?upstream_task_id and the create/poll payload `upstream_id`, a result-url path
// segment, etc.) 闁?signals a hostile or broken upstream. The body/message redactor only
// covers error STRINGS, not parsed structural fields, so this is the fail-closed backstop
// that stops the key from ever being PERSISTED via a structural field. (A key shorter than
// the floor cannot reach here: the pre-arm self-check already aborted such a key.)
func seedanceUpstreamEchoedKey(account *VideoProviderAccount, rawBody string) bool {
	if account == nil {
		return false
	}
	key := strings.TrimSpace(account.PlainAPIKey)
	if len(key) < videoKnownSecretMinLen {
		return false
	}
	return strings.Contains(rawBody, key)
}

// seedancePreArmRedactionSelfCheck is the FORM-A-path equivalent of the Form B real-
// smoke test's pre-arm self-check, lifted to the adapter chokepoint so it guards EVERY
// real call (the worker path AND the direct Form B path) BEFORE any socket is opened.
// It proves the configured key is stripped in every shape an upstream echo could take
// (bare, Authorization-prefixed, and embedded in JSON error envelopes) by the SAME
// redaction path the adapter uses on the real response. If any shape would leak the
// key it returns an error and the caller aborts with NO network call (fail-closed).
// The key value is NEVER included in the error or any log.
//
// With key-aware redaction (seedanceRedactBody) this passes for any credible key
// (length >= videoKnownSecretMinLen, any charset). It still fails-closed for the one
// genuinely dangerous case the pattern passes alone cannot cover 闁?a sub-floor key
// that also lands in a pattern blind spot 闁?rather than billing a call whose echo
// could leak it.
func seedancePreArmRedactionSelfCheck(account *VideoProviderAccount) error {
	if account == nil {
		return nil
	}
	key := strings.TrimSpace(account.PlainAPIKey)
	if key == "" {
		// An empty/whitespace key is rejected earlier by the api-key-configured guard;
		// there is nothing to leak here.
		return nil
	}
	for _, probe := range []string{
		key,
		"Bearer " + key,
		`{"message":"` + key + `"}`,
		`{"error":{"message":"rejected token ` + key + `"}}`,
	} {
		if strings.Contains(seedanceRedactBody(account, probe), key) {
			return infraerrorsUnavailable("SEEDANCE_REDACTION_SELF_CHECK_FAILED",
				"pre-arm redaction self-check failed: the configured key is not stripped in every upstream echo shape; "+
					"aborting before any billed call (key value intentionally not included)")
		}
	}
	return nil
}

func seedanceSmokeGateBlockedReasons(account *VideoProviderAccount, task *VideoTask) []string {
	reasons := []string{}
	productionAuthorized := seedanceProductionAuthorized(account)
	if strings.TrimSpace(os.Getenv("SUB2API_VIDEO_REAL_SMOKE_ENABLED")) != "1" {
		reasons = append(reasons, "SUB2API_VIDEO_REAL_SMOKE_ENABLED is not 1")
	}
	if !seedanceRealCallAuthorized(account) {
		reasons = append(reasons, "provider metadata does not record single smoke authorization or production authorization")
	}
	if strings.TrimSpace(os.Getenv("SUB2API_VIDEO_REDACTED_EVENT_LOG")) == "" {
		reasons = append(reasons, "redacted event log path is missing")
	}
	if configuredMediaURLAllowlist() == "" {
		// Fail-closed SSRF posture: the real path must not run with the loose
		// (no-allowlist) URL validation branch. Operators must pin the trusted
		// media domains (Ark result CDN + permitted reference-image hosts).
		reasons = append(reasons, mediaURLAllowlistMissingReason())
	}
	if strings.TrimSpace(task.Model) == "" || !strings.Contains(strings.ToLower(task.Model), "seedance") {
		reasons = append(reasons, "seedance model is not explicit")
	}
	if !productionAuthorized && (task.Duration <= 0 || task.Duration > 5) {
		reasons = append(reasons, "single smoke duration must be between 1 and 5 seconds")
	}
	return reasons
}

func seedanceRealCallAuthorized(account *VideoProviderAccount) bool {
	if account == nil {
		return false
	}
	return metadataBool(account.Metadata, "single_smoke_authorized") ||
		metadataBool(account.Metadata, "real_smoke_authorized") ||
		seedanceProductionAuthorized(account)
}

func seedanceProductionAuthorized(account *VideoProviderAccount) bool {
	if account == nil {
		return false
	}
	return metadataBool(account.Metadata, "production_authorized")
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	value, ok := metadata[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		return normalized == "1" || normalized == "true" || normalized == "yes"
	default:
		return false
	}
}

func seedanceHTTPErrorType(statusCode int, body string) string {
	lower := strings.ToLower(body)
	switch {
	case statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden:
		return "auth"
	case statusCode == http.StatusPaymentRequired || strings.Contains(lower, "quota") || strings.Contains(lower, "insufficient"):
		return "quota"
	case statusCode == http.StatusTooManyRequests:
		return "rate_limit"
	case statusCode == http.StatusRequestTimeout || strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
		return "timeout"
	case statusCode >= 500:
		return "provider_down"
	case strings.Contains(lower, "prompt"):
		return "invalid_prompt"
	case strings.Contains(lower, "unsafe") || strings.Contains(lower, "safety"):
		return "unsafe_content"
	default:
		return "unknown"
	}
}

func qcanvasVideoContractStatus(status string, errorMessage string) string {
	switch normalizeVideoStatus(status) {
	case VideoStatusQueued, VideoStatusSubmitted:
		return "queued"
	case VideoStatusRunning:
		return "running"
	case VideoStatusSucceeded:
		return "succeeded"
	case VideoStatusCancelled:
		return "canceled"
	case VideoStatusFailed:
		return "failed"
	default:
		if strings.Contains(strings.ToLower(errorMessage), "timeout") {
			return "failed"
		}
		return "running"
	}
}

type klingVideoAdapter struct{}

func (a *klingVideoAdapter) Provider() string { return VideoProviderKling }

func (a *klingVideoAdapter) CreateTask(_ context.Context, account *VideoProviderAccount, _ *VideoTask) (*VideoAdapterResult, error) {
	if !account.APIKeyConfigured || strings.TrimSpace(account.PlainAPIKey) == "" {
		return nil, ErrVideoProviderDisabled.WithMetadata(map[string]string{
			"provider": "kling",
			"reason":   "api key is not configured; real call skipped",
		})
	}
	return nil, infraerrorsUnavailable("KLING_REAL_CALL_DISABLED", "Kling adapter skeleton is mapped but real upstream calls are disabled in P0")
}

func (a *klingVideoAdapter) PollTask(_ context.Context, account *VideoProviderAccount, _ *VideoTask) (*VideoAdapterResult, error) {
	if !account.APIKeyConfigured || strings.TrimSpace(account.PlainAPIKey) == "" {
		return nil, ErrVideoProviderDisabled.WithMetadata(map[string]string{
			"provider": "kling",
			"reason":   "api key is not configured; poll skipped",
		})
	}
	return nil, infraerrorsUnavailable("KLING_REAL_POLL_DISABLED", "Kling poll skeleton is mapped but real upstream calls are disabled in P0")
}

func (a *klingVideoAdapter) CancelTask(_ context.Context, _ *VideoProviderAccount, _ *VideoTask) (*VideoAdapterResult, error) {
	return &VideoAdapterResult{Status: VideoStatusCancelled, Payload: map[string]any{"provider": "kling", "mode": "skeleton"}}, nil
}

func (a *klingVideoAdapter) NormalizeStatus(upstream string) string {
	switch strings.ToLower(strings.TrimSpace(upstream)) {
	case "submitted", "created", "queued", "pending":
		return VideoStatusSubmitted
	case "processing", "running":
		return VideoStatusRunning
	case "succeed", "succeeded", "success", "completed":
		return VideoStatusSucceeded
	case "failed", "fail", "error":
		return VideoStatusFailed
	case "cancelled", "canceled":
		return VideoStatusCancelled
	default:
		return VideoStatusRunning
	}
}

func (a *klingVideoAdapter) BuildCreatePayload(account *VideoProviderAccount, task *VideoTask) map[string]any {
	payload := map[string]any{
		"base_url":        account.BaseURL,
		"model_name":      task.Model,
		"prompt":          task.Prompt,
		"negative_prompt": task.NegativePrompt,
		"aspect_ratio":    task.AspectRatio,
		"duration":        task.Duration,
		"resolution":      task.Resolution,
		"content":         task.Content,
		"has_video_input": task.HasVideoInput,
		"source_docs":     "https://app.klingai.com/cn/dev/document-api/apiReference/updateNotice",
	}
	if task.ReferenceImageURL != "" {
		payload["image"] = task.ReferenceImageURL
	}
	if task.ReferenceVideoURL != "" {
		payload["reference_video"] = task.ReferenceVideoURL
	}
	return payload
}

func normalizeVideoStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case VideoStatusQueued:
		return VideoStatusQueued
	case VideoStatusSubmitted:
		return VideoStatusSubmitted
	case VideoStatusRunning:
		return VideoStatusRunning
	case VideoStatusSucceeded, "success", "completed", "complete":
		return VideoStatusSucceeded
	case VideoStatusFailed, "error":
		return VideoStatusFailed
	case VideoStatusCancelled, "canceled":
		return VideoStatusCancelled
	default:
		return VideoStatusRunning
	}
}

// normalizeSeedanceRatio maps a requested aspect/orientation to Ark's create-side
// `ratio` field value (16:9 / 9:16 / 1:1). It accepts logical orientation keywords
// (portrait/landscape/square and 濞戞搩鍘介弸?缂佹梹鐗曢惈?婵☆垼浜滈惈?闁哄倻鎳撻懜? AND already-valid "W:H" ratios
// (passed through verbatim so non-mapped-but-valid ratios still reach Ark unchanged).
// Empty input yields "" so the caller omits the field. This is the B1 fix: the field
// name is `ratio` (not `aspect_ratio`) and portrait now resolves to 9:16.
func normalizeSeedanceRatio(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "":
		return ""
	case "9:16", "portrait", "vertical", "\u7ad6\u5c4f", "\u7ad6\u7248", "\u7ed4\u6827\u7746":
		return "9:16"
	case "16:9", "landscape", "horizontal", "\u6a2a\u5c4f", "\u6a2a\u7248", "\u59af\ue044\u7746":
		return "16:9"
	case "1:1", "square", "\u65b9\u5f62", "\u6b63\u65b9\u5f62", "\u93c2\u7470\u8230":
		return "1:1"
	default:
		// Pass through only shape-valid "W:H" ratios (same gate as the response-side
		// looksLikeAspectRatio), so a non-canonical-but-valid ratio (e.g. "4:3") still
		// reaches Ark while arbitrary text ("banana", "16x9") is dropped 闁?field omitted
		// 闁?Ark default. Keeps the request side from forwarding unvalidated input.
		if v := strings.TrimSpace(input); looksLikeAspectRatio(v) {
			return v
		}
		return ""
	}
}

// looksLikeAspectRatio reports whether s is a plausible "W:H" aspect ratio such as
// "16:9". It gates what the poll parser surfaces from the upstream "ratio" field so
// an unexpected or oversized value cannot smuggle un-redacted content into the
// persisted event payload / API response.
func looksLikeAspectRatio(s string) bool {
	if len(s) == 0 || len(s) > 9 {
		return false
	}
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 || len(part) > 4 {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func looksLikeVideoResolution(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "480p", "720p", "1080p":
		return true
	default:
		return false
	}
}

func firstNonNilInt(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func looksLikeActualVideoDuration(v int) bool {
	return v > 0 && v <= 3600
}

func infraerrorsUnavailable(reason, message string) error {
	return infraerrors.ServiceUnavailable(reason, message)
}
