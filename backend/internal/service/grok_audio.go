package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// supportedGrokVoiceHTTPEndpoints are xAI Voice HTTP paths we forward as-is.
var supportedGrokVoiceHTTPEndpoints = map[string]struct{REDACTED{
	"tts":           {REDACTED,
	"stt":           {REDACTED,
	"custom-voices": {REDACTED,
REDACTED

// ForwardGrokVoice forwards the official xAI Voice HTTP APIs (/tts, /stt, /custom-voices).
// The response is intentionally passed through because TTS returns audio bytes
// while STT returns JSON and xAI may add format-specific headers.
func (s *OpenAIGatewayService) ForwardGrokVoice(ctx context.Context, c *gin.Context, account *Account, endpoint string, body []byte, contentType string) (*OpenAIForwardResult, error) {
	if s == nil || account == nil {
		return nil, fmt.Errorf("grok voice service/account is required")
REDACTED
	if account.Platform != PlatformGrok {
		return nil, fmt.Errorf("account platform %s is not supported for grok voice", account.Platform)
REDACTED
	endpoint = strings.Trim(strings.TrimSpace(endpoint), "/")
	if _, ok := supportedGrokVoiceHTTPEndpoints[endpoint]; !ok {
		return nil, fmt.Errorf("unsupported grok voice endpoint: %s", endpoint)
REDACTED
	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
REDACTED
	targetURL, err := buildGrokVoiceURL(account, s.cfg, endpoint)
	if err != nil {
		return nil, err
REDACTED
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	req, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
REDACTED
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json, audio/*")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
REDACTED
	req.Header.Set("Content-Type", contentType)
	// Voice hits api.x.ai (not CLI proxy). Still stamp CLI identity headers for
	// consistency with other Grok outbound calls; transport will not rewrite
	// non-CLI-proxy hosts.
	applyGrokCLIHeaders(req.Header)
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	started := time.Now()
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
REDACTED
	defer func() { _ = resp.Body.Close() REDACTED()
	if resp.StatusCode >= 400 {
		return s.handleGrokMediaErrorResponse(ctx, resp, c, account, resp.Header.Get("x-request-id"), endpoint)
REDACTED
	data, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
REDACTED
	writeGrokMediaResponse(c, resp, data, s.responseHeaderFilter)
	audioUsage := estimateGrokVoiceAudioUsage(endpoint, body, contentType, data, time.Since(started))
	return &OpenAIForwardResult{
		RequestID:     firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id")),
		Model:         endpoint,
		UpstreamModel: endpoint,
		Duration:      time.Since(started),
		AudioUsage:    audioUsage,
REDACTED, nil
REDACTED

// ProxyGrokRealtime relays JSON Realtime events to xAI's native Voice WS.
// Audio is carried as base64 inside JSON events, so preserving the JSON bytes
// is sufficient and avoids translating protocol event types.
func (s *OpenAIGatewayService) ProxyGrokRealtime(ctx context.Context, c *gin.Context, client *coderws.Conn, account *Account, token, model string) error {
	if s == nil || client == nil || account == nil {
		return fmt.Errorf("realtime service, client, and account are required")
REDACTED
	if account.Platform != PlatformGrok {
		return fmt.Errorf("account platform %s is not supported for grok realtime", account.Platform)
REDACTED
	base, err := buildGrokVoiceURL(account, s.cfg, "realtime")
	if err != nil {
		return err
REDACTED
	u, err := url.Parse(base)
	if err != nil {
		return err
REDACTED
	u.Scheme = "wss"
	u.RawQuery = "model=" + url.QueryEscape(firstNonEmpty(model, "grok-voice-latest"))
	headers := http.Header{"Authorization": []string{"Bearer " + tokenREDACTEDREDACTED
	// Stamp CLI identity for consistency (host is api.x.ai; no CLI-proxy rewrite).
	applyGrokCLIHeaders(headers)
	if account != nil {
		account.ApplyHeaderOverrides(headers)
REDACTED

	dialer := s.getOpenAIWSPassthroughDialer()
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
REDACTED
	upstream, _, _, err := dialer.Dial(ctx, u.String(), headers, proxyURL)
	if err != nil {
		return err
REDACTED
	defer func() { _ = upstream.Close() REDACTED()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)

	// Upstream → client
	go func() {
		for {
			msg, readErr := upstream.ReadMessage(ctx)
			if readErr != nil {
				errCh <- readErr
				return
		REDACTED
			if writeErr := client.Write(ctx, coderws.MessageText, msg); writeErr != nil {
				errCh <- writeErr
				return
		REDACTED
	REDACTED
REDACTED()

	// Client → upstream (JSON events only)
	go func() {
		for {
			kind, msg, readErr := client.Read(ctx)
			if readErr != nil {
				errCh <- readErr
				return
		REDACTED
			if kind != coderws.MessageText && kind != coderws.MessageBinary {
				continue
		REDACTED
			var raw json.RawMessage
			if unmarshalErr := json.Unmarshal(msg, &raw); unmarshalErr != nil {
				errCh <- fmt.Errorf("invalid realtime event: %w", unmarshalErr)
				return
		REDACTED
			if writeErr := upstream.WriteJSON(ctx, raw); writeErr != nil {
				errCh <- writeErr
				return
		REDACTED
	REDACTED
REDACTED()

	return <-errCh
REDACTED

// estimateGrokVoiceAudioUsage derives billing units from the request/response.
// TTS: million characters of input text; STT: hours approximated from request body size
// when duration is unknown; custom-voices: no units (nil).
func estimateGrokVoiceAudioUsage(endpoint string, reqBody []byte, contentType string, respBody []byte, elapsed time.Duration) *AudioUsage {
	switch strings.TrimSpace(endpoint) {
	case "tts":
		// Prefer JSON "input" / "text" fields; fallback to raw body length.
		chars := 0
		if gjson.ValidBytes(reqBody) {
			for _, key := range []string{"input", "text", "prompt"REDACTED {
				if s := strings.TrimSpace(gjson.GetBytes(reqBody, key).String()); s != "" {
					chars = len([]rune(s))
					break
			REDACTED
		REDACTED
	REDACTED
		if chars <= 0 {
			chars = len(reqBody)
	REDACTED
		if chars <= 0 {
			return nil
	REDACTED
		return &AudioUsage{Mode: "tts", DurationOrUnits: float64(chars) / 1_000_000.0REDACTED
	case "stt":
		// Prefer response duration when present; do not trust client duration_seconds alone
		// (under-report would underbill). Floor against body-size heuristic and elapsed.
		secs := 0.0
		if gjson.ValidBytes(respBody) {
			for _, path := range []string{"duration", "duration_seconds", "audio_duration", "usage.seconds"REDACTED {
				if v := gjson.GetBytes(respBody, path); v.Exists() && v.Type == gjson.Number && v.Float() > 0 {
					secs = v.Float()
					break
			REDACTED
		REDACTED
	REDACTED
		// Multipart / body size heuristic: ~16KB/s for compressed speech (lower bound).
		sizeFloor := 0.0
		if len(reqBody) > 0 {
			sizeFloor = float64(len(reqBody)) / 16000.0
	REDACTED
		clientSecs := 0.0
		if gjson.ValidBytes(reqBody) {
			if v := gjson.GetBytes(reqBody, "duration_seconds"); v.Exists() && v.Type == gjson.Number {
				clientSecs = v.Float()
		REDACTED
	REDACTED
		if secs <= 0 {
			secs = elapsed.Seconds()
	REDACTED
		if secs <= 0 {
			secs = clientSecs
	REDACTED
		if secs <= 0 {
			secs = sizeFloor
	REDACTED
		// Cap untrusted client under-report: if client duration is much smaller than
		// size/elapsed floors, bill the larger of floors (anti underbill).
		if clientSecs > 0 && secs == clientSecs {
			floor := sizeFloor
			if elapsed.Seconds() > floor {
				floor = elapsed.Seconds()
		REDACTED
			if floor > 0 && clientSecs < floor*0.5 {
				secs = floor
		REDACTED
	REDACTED
		if secs <= 0 {
			return nil
	REDACTED
		return &AudioUsage{Mode: "stt", DurationOrUnits: secs / 3600.0REDACTED
	case "realtime":
		mins := elapsed.Minutes()
		if mins <= 0 {
			return nil
	REDACTED
		return &AudioUsage{Mode: "realtime", DurationOrUnits: minsREDACTED
	default:
		return nil
REDACTED
REDACTED
