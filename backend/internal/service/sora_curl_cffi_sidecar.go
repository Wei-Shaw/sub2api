package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/logredact"
)

const soraCurlCFFISidecarDefaultTimeoutSeconds = 60

type soraCurlCFFISidecarRequest struct {
	Method         string              `json:"method"`
	URL            string              `json:"url"`
	Headers        map[string][]string `json:"headers,omitempty"`
	BodyBase64     string              `json:"body_base64,omitempty"`
	ProxyURL       string              `json:"proxy_url,omitempty"`
	Impersonate    string              `json:"impersonate,omitempty"`
	TimeoutSeconds int                 `json:"timeout_seconds,omitempty"`
REDACTED

type soraCurlCFFISidecarResponse struct {
	StatusCode int            `json:"status_code"`
	Status     int            `json:"status"`
	Headers    map[string]any `json:"headers"`
	BodyBase64 string         `json:"body_base64"`
	Body       string         `json:"body"`
	Error      string         `json:"error"`
REDACTED

func (c *SoraDirectClient) doHTTPViaCurlCFFISidecar(req *http.Request, proxyURL string) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("request url is nil")
REDACTED
	if c == nil || c.cfg == nil {
		return nil, errors.New("sora curl_cffi sidecar config is nil")
REDACTED
	if !c.cfg.Sora.Client.CurlCFFISidecar.Enabled {
		return nil, errors.New("sora curl_cffi sidecar is disabled")
REDACTED
	endpoint := c.curlCFFISidecarEndpoint()
	if endpoint == "" {
		return nil, errors.New("sora curl_cffi sidecar base_url is empty")
REDACTED

	bodyBytes, err := readAndRestoreRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("sora curl_cffi sidecar read request body failed: %w", err)
REDACTED

	headers := make(map[string][]string, len(req.Header)+1)
	for key, vals := range req.Header {
		copied := make([]string, len(vals))
		copy(copied, vals)
		headers[key] = copied
REDACTED
	if strings.TrimSpace(req.Host) != "" {
		if _, ok := headers["Host"]; !ok {
			headers["Host"] = []string{req.HostREDACTED
	REDACTED
REDACTED

	payload := soraCurlCFFISidecarRequest{
		Method:         req.Method,
		URL:            req.URL.String(),
		Headers:        headers,
		ProxyURL:       strings.TrimSpace(proxyURL),
		Impersonate:    c.curlCFFIImpersonate(),
		TimeoutSeconds: c.curlCFFISidecarTimeoutSeconds(),
REDACTED
	if len(bodyBytes) > 0 {
		payload.BodyBase64 = base64.StdEncoding.EncodeToString(bodyBytes)
REDACTED

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("sora curl_cffi sidecar marshal request failed: %w", err)
REDACTED

	sidecarReq, err := http.NewRequestWithContext(req.Context(), http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("sora curl_cffi sidecar build request failed: %w", err)
REDACTED
	sidecarReq.Header.Set("Content-Type", "application/json")
	sidecarReq.Header.Set("Accept", "application/json")

	httpClient := &http.Client{Timeout: time.Duration(payload.TimeoutSeconds) * time.SecondREDACTED
	sidecarResp, err := httpClient.Do(sidecarReq)
	if err != nil {
		return nil, fmt.Errorf("sora curl_cffi sidecar request failed: %w", err)
REDACTED
	defer sidecarResp.Body.Close()

	sidecarRespBody, err := io.ReadAll(io.LimitReader(sidecarResp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("sora curl_cffi sidecar read response failed: %w", err)
REDACTED
	if sidecarResp.StatusCode != http.StatusOK {
		redacted := truncateForLog([]byte(logredact.RedactText(string(sidecarRespBody))), 512)
		return nil, fmt.Errorf("sora curl_cffi sidecar http status=%d body=%s", sidecarResp.StatusCode, redacted)
REDACTED

	var payloadResp soraCurlCFFISidecarResponse
	if err := json.Unmarshal(sidecarRespBody, &payloadResp); err != nil {
		return nil, fmt.Errorf("sora curl_cffi sidecar parse response failed: %w", err)
REDACTED
	if msg := strings.TrimSpace(payloadResp.Error); msg != "" {
		return nil, fmt.Errorf("sora curl_cffi sidecar upstream error: %s", msg)
REDACTED
	statusCode := payloadResp.StatusCode
	if statusCode <= 0 {
		statusCode = payloadResp.Status
REDACTED
	if statusCode <= 0 {
		return nil, errors.New("sora curl_cffi sidecar response missing status code")
REDACTED

	responseBody := []byte(payloadResp.Body)
	if strings.TrimSpace(payloadResp.BodyBase64) != "" {
		decoded, err := base64.StdEncoding.DecodeString(payloadResp.BodyBase64)
		if err != nil {
			return nil, fmt.Errorf("sora curl_cffi sidecar decode body failed: %w", err)
	REDACTED
		responseBody = decoded
REDACTED

	respHeaders := make(http.Header)
	for key, rawVal := range payloadResp.Headers {
		for _, v := range convertSidecarHeaderValue(rawVal) {
			respHeaders.Add(key, v)
	REDACTED
REDACTED

	return &http.Response{
		StatusCode:    statusCode,
		Header:        respHeaders,
		Body:          io.NopCloser(bytes.NewReader(responseBody)),
		ContentLength: int64(len(responseBody)),
		Request:       req,
REDACTED, nil
REDACTED

func readAndRestoreRequestBody(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
REDACTED
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
REDACTED
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	req.ContentLength = int64(len(bodyBytes))
	return bodyBytes, nil
REDACTED

func (c *SoraDirectClient) curlCFFISidecarEndpoint() string {
	if c == nil || c.cfg == nil {
		return ""
REDACTED
	raw := strings.TrimSpace(c.cfg.Sora.Client.CurlCFFISidecar.BaseURL)
	if raw == "" {
		return ""
REDACTED
	parsed, err := url.Parse(raw)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return raw
REDACTED
	if path := strings.TrimSpace(parsed.Path); path == "" || path == "/" {
		parsed.Path = "/request"
REDACTED
	return parsed.String()
REDACTED

func (c *SoraDirectClient) curlCFFISidecarTimeoutSeconds() int {
	if c == nil || c.cfg == nil {
		return soraCurlCFFISidecarDefaultTimeoutSeconds
REDACTED
	timeoutSeconds := c.cfg.Sora.Client.CurlCFFISidecar.TimeoutSeconds
	if timeoutSeconds <= 0 {
		return soraCurlCFFISidecarDefaultTimeoutSeconds
REDACTED
	return timeoutSeconds
REDACTED

func (c *SoraDirectClient) curlCFFIImpersonate() string {
	if c == nil || c.cfg == nil {
		return "chrome131"
REDACTED
	impersonate := strings.TrimSpace(c.cfg.Sora.Client.CurlCFFISidecar.Impersonate)
	if impersonate == "" {
		return "chrome131"
REDACTED
	return impersonate
REDACTED

func convertSidecarHeaderValue(raw any) []string {
	switch val := raw.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(val) == "" {
			return nil
	REDACTED
		return []string{valREDACTED
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" {
				out = append(out, s)
		REDACTED
	REDACTED
		return out
	case []string:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if strings.TrimSpace(item) != "" {
				out = append(out, item)
		REDACTED
	REDACTED
		return out
	default:
		s := strings.TrimSpace(fmt.Sprint(val))
		if s == "" {
			return nil
	REDACTED
		return []string{sREDACTED
REDACTED
REDACTED
