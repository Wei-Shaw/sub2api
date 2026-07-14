package xai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	SSOBuildScope        = "openid profile email offline_access grok-cli:access api:access conversations:read conversations:write"
	SSOAccountsURL       = "https://accounts.x.ai/"
	SSODeviceURL         = OAuthIssuer + "/oauth2/device/code"
	SSOVerifyURL         = OAuthIssuer + "/oauth2/device/verify"
	SSOApproveURL        = OAuthIssuer + "/oauth2/device/approve"
	SSOTokenURL          = OAuthIssuer + "/oauth2/token"
	SSOConversionTimeout = 90 * time.Second

	ssoMaxAuthBody     = 2 << 20
	ssoDefaultUA       = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	ssoDefaultTokenTTL = 6 * time.Hour
)

var (
	ErrSSOUnauthorized        = errors.New("xai sso unauthorized")
	ErrSSOAuthorizationDenied = errors.New("xai device authorization denied")
)

type SSOHTTPError struct{ Status int REDACTED

func (e SSOHTTPError) Error() string { return fmt.Sprintf("xAI OAuth HTTP %d", e.Status) REDACTED

type SSODeviceHTTPClient interface {
	Do(*http.Request) (*http.Response, error)
REDACTED

type SSODeviceOptions struct {
	HTTPClient SSODeviceHTTPClient
	UserAgent  string
	Sleep      func(context.Context, time.Duration) error
REDACTED

type ssoDeviceFlow struct {
	client    SSODeviceHTTPClient
	userAgent string
	cookies   map[string]string
	sleep     func(context.Context, time.Duration) error
REDACTED

func ConvertSSOToBuild(ctx context.Context, ssoToken string, opts *SSODeviceOptions) (*TokenResponse, error) {
	ssoToken = NormalizeSSOToken(ssoToken)
	if ssoToken == "" {
		return nil, ErrSSOUnauthorized
REDACTED
	if opts == nil {
		opts = &SSODeviceOptions{REDACTED
REDACTED
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: SSOConversionTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
		REDACTED,
	REDACTED
REDACTED
	userAgent := strings.TrimSpace(opts.UserAgent)
	if userAgent == "" {
		userAgent = ssoDefaultUA
REDACTED
	sleep := opts.Sleep
	if sleep == nil {
		sleep = sleepContext
REDACTED

	flow := &ssoDeviceFlow{
		client:    client,
		userAgent: userAgent,
		cookies:   map[string]string{"sso": ssoToken, "sso-rw": ssoTokenREDACTED,
		sleep:     sleep,
REDACTED
	return flow.convert(ctx)
REDACTED

func (f *ssoDeviceFlow) convert(ctx context.Context) (*TokenResponse, error) {
	status, finalURL, _, err := f.do(ctx, http.MethodGet, SSOAccountsURL, nil)
	if err != nil {
		return nil, err
REDACTED
	if status == http.StatusUnauthorized || strings.Contains(finalURL, "sign-in") || strings.Contains(finalURL, "sign-up") {
		return nil, ErrSSOUnauthorized
REDACTED
	if status < 200 || status >= 400 {
		return nil, fmt.Errorf("validate Grok Web SSO: %w", SSOHTTPError{Status: statusREDACTED)
REDACTED

	status, _, body, err := f.do(ctx, http.MethodPost, SSODeviceURL, url.Values{
		"client_id": {DefaultClientIDREDACTED,
		"scope":     {SSOBuildScopeREDACTED,
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("start xAI device flow: %w", SSOHTTPError{Status: statusREDACTED)
REDACTED
	var device struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		Interval                int    `json:"interval"`
		ExpiresIn               int    `json:"expires_in"`
REDACTED
	if err := json.Unmarshal(body, &device); err != nil {
		return nil, fmt.Errorf("parse xAI device flow response: %w", err)
REDACTED
	if device.DeviceCode == "" || device.UserCode == "" || !safeXAIAuthURL(device.VerificationURIComplete) {
		return nil, errors.New("xAI device flow response is incomplete")
REDACTED
	if device.Interval <= 0 {
		device.Interval = 5
REDACTED
	if device.ExpiresIn <= 0 {
		device.ExpiresIn = 1800
REDACTED

	status, _, _, err = f.do(ctx, http.MethodGet, device.VerificationURIComplete, nil)
	if err != nil {
		return nil, err
REDACTED
	if status < 200 || status >= 400 {
		return nil, fmt.Errorf("open xAI device verification page: %w", SSOHTTPError{Status: statusREDACTED)
REDACTED

	status, finalURL, _, err = f.do(ctx, http.MethodPost, SSOVerifyURL, url.Values{"user_code": {device.UserCodeREDACTEDREDACTED)
	if err != nil {
		return nil, err
REDACTED
	if status < 200 || status >= 400 {
		return nil, fmt.Errorf("verify xAI device code: %w", SSOHTTPError{Status: statusREDACTED)
REDACTED
	if !strings.Contains(finalURL, "consent") {
		return nil, errors.New("xAI device verification did not reach consent page")
REDACTED

	status, finalURL, _, err = f.do(ctx, http.MethodPost, SSOApproveURL, url.Values{
		"user_code":      {device.UserCodeREDACTED,
		"action":         {"allow"REDACTED,
		"principal_type": {"User"REDACTED,
		"principal_id":   {""REDACTED,
REDACTED)
	if err != nil {
		return nil, err
REDACTED
	if status < 200 || status >= 400 {
		return nil, fmt.Errorf("approve xAI device code: %w", SSOHTTPError{Status: statusREDACTED)
REDACTED
	if !strings.Contains(finalURL, "done") {
		return nil, errors.New("xAI device approval did not reach done page")
REDACTED

	return f.pollToken(ctx, device.DeviceCode, time.Duration(device.Interval)*time.Second, time.Duration(device.ExpiresIn)*time.Second)
REDACTED

func (f *ssoDeviceFlow) pollToken(ctx context.Context, deviceCode string, interval, expiresIn time.Duration) (*TokenResponse, error) {
	if interval < time.Second {
		interval = time.Second
REDACTED
	deadline := time.Now().Add(minDuration(expiresIn, 75*time.Second))
	for time.Now().Before(deadline) {
		if err := f.sleep(ctx, interval); err != nil {
			return nil, err
	REDACTED
		status, _, body, err := f.do(ctx, http.MethodPost, SSOTokenURL, url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"REDACTED,
			"client_id":   {DefaultClientIDREDACTED,
			"device_code": {deviceCodeREDACTED,
	REDACTED)
		if err != nil {
			return nil, err
	REDACTED
		var payload struct {
			AccessToken      string `json:"access_token"`
			RefreshToken     string `json:"refresh_token"`
			IDToken          string `json:"id_token"`
			TokenType        string `json:"token_type"`
			ExpiresIn        int64  `json:"expires_in"`
			Scope            string `json:"scope"`
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
	REDACTED
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("parse xAI token response: %w", err)
	REDACTED
		if status >= 200 && status < 300 && payload.AccessToken != "" {
			if payload.ExpiresIn <= 0 {
				payload.ExpiresIn = int64(ssoDefaultTokenTTL.Seconds())
		REDACTED
			if payload.TokenType == "" {
				payload.TokenType = "Bearer"
		REDACTED
			return &TokenResponse{
				AccessToken:  payload.AccessToken,
				RefreshToken: payload.RefreshToken,
				IDToken:      payload.IDToken,
				TokenType:    payload.TokenType,
				ExpiresIn:    payload.ExpiresIn,
				Scope:        payload.Scope,
		REDACTED, nil
	REDACTED
		switch payload.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied", "expired_token":
			return nil, ErrSSOAuthorizationDenied
		default:
			if status >= 400 {
				return nil, fmt.Errorf("xAI token polling failed (%s): %w", firstNonEmpty(payload.ErrorDescription, payload.Error), SSOHTTPError{Status: statusREDACTED)
		REDACTED
			return nil, fmt.Errorf("xAI token polling failed: %s", firstNonEmpty(payload.ErrorDescription, payload.Error, strconv.Itoa(status)))
	REDACTED
REDACTED
	return nil, errors.New("xAI device flow token polling timed out")
REDACTED

func (f *ssoDeviceFlow) do(ctx context.Context, method, endpoint string, form url.Values) (int, string, []byte, error) {
	if !safeXAIAuthURL(endpoint) {
		return 0, "", nil, errors.New("xAI OAuth URL is not trusted")
REDACTED
	currentURL := endpoint
	currentMethod := method
	currentForm := form
	for redirects := 0; redirects <= 8; redirects++ {
		var body io.Reader
		if currentForm != nil {
			body = strings.NewReader(currentForm.Encode())
	REDACTED
		request, err := http.NewRequestWithContext(ctx, currentMethod, currentURL, body)
		if err != nil {
			return 0, currentURL, nil, err
	REDACTED
		request.Header.Set("Accept", "application/json, text/html;q=0.9, */*;q=0.8")
		request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
		request.Header.Set("User-Agent", f.userAgent)
		if cookie := f.cookieHeader(); cookie != "" {
			request.Header.Set("Cookie", cookie)
	REDACTED
		if currentForm != nil {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	REDACTED

		response, err := f.client.Do(request)
		if err != nil {
			return 0, currentURL, nil, err
	REDACTED
		f.captureCookies(response)
		data, readErr := io.ReadAll(io.LimitReader(response.Body, ssoMaxAuthBody+1))
		_ = response.Body.Close()
		if readErr != nil {
			return response.StatusCode, currentURL, nil, readErr
	REDACTED
		if len(data) > ssoMaxAuthBody {
			return response.StatusCode, currentURL, nil, errors.New("xAI OAuth response exceeds 2 MiB")
	REDACTED
		if response.StatusCode < 300 || response.StatusCode > 399 {
			return response.StatusCode, currentURL, data, nil
	REDACTED

		location := strings.TrimSpace(response.Header.Get("Location"))
		if location == "" {
			return response.StatusCode, currentURL, data, errors.New("xAI OAuth redirect missing Location")
	REDACTED
		base, _ := url.Parse(currentURL)
		next, err := url.Parse(location)
		if err != nil {
			return response.StatusCode, currentURL, data, err
	REDACTED
		currentURL = base.ResolveReference(next).String()
		if !safeXAIAuthURL(currentURL) {
			return response.StatusCode, currentURL, data, errors.New("xAI OAuth redirected to untrusted host")
	REDACTED
		if response.StatusCode == http.StatusSeeOther || ((response.StatusCode == http.StatusMovedPermanently || response.StatusCode == http.StatusFound) && currentMethod != http.MethodGet && currentMethod != http.MethodHead) {
			currentMethod = http.MethodGet
			currentForm = nil
	REDACTED
REDACTED
	return 0, currentURL, nil, errors.New("xAI OAuth redirected too many times")
REDACTED

func (f *ssoDeviceFlow) captureCookies(response *http.Response) {
	for _, cookie := range response.Cookies() {
		name := strings.TrimSpace(cookie.Name)
		value := strings.TrimSpace(cookie.Value)
		if name == "" || len(name) > 128 || len(value) > 16384 || strings.ContainsAny(name+value, "\r\n\x00") {
			continue
	REDACTED
		if cookie.MaxAge < 0 {
			delete(f.cookies, name)
			continue
	REDACTED
		f.cookies[name] = value
REDACTED
REDACTED

func (f *ssoDeviceFlow) cookieHeader() string {
	keys := make([]string, 0, len(f.cookies))
	for key := range f.cookies {
		keys = append(keys, key)
REDACTED
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+f.cookies[key])
REDACTED
	return strings.Join(parts, "; ")
REDACTED

func safeXAIAuthURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return false
REDACTED
	if AllowUnsafeURLOverrides() {
		return parsed.Scheme != "" && parsed.Host != ""
REDACTED
	if parsed.Scheme != "https" {
		return false
REDACTED
	host := strings.ToLower(parsed.Hostname())
	return host == "x.ai" || strings.HasSuffix(host, ".x.ai")
REDACTED

func NormalizeSSOToken(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "cookie:") {
		value = strings.TrimSpace(value[len("cookie:"):])
REDACTED
	for _, part := range strings.Split(value, ";") {
		name, token, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
	REDACTED
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "sso", "sso-rw":
			return sanitizeSSOToken(token)
	REDACTED
REDACTED
	if token, _, found := strings.Cut(value, ";"); found {
		value = strings.TrimSpace(token)
REDACTED
	return sanitizeSSOToken(value)
REDACTED

func sanitizeSSOToken(value string) string {
	return strings.NewReplacer("\r", "", "\n", "", "\x00", "").Replace(strings.TrimSpace(value))
REDACTED

func DecodeJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil
REDACTED
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
REDACTED
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
REDACTED
	return claims
REDACTED

func JWTClaimString(claims map[string]any, key string) string {
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
REDACTED

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
REDACTED
REDACTED

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
REDACTED
	if a < b {
		return a
REDACTED
	return b
REDACTED

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
	REDACTED
REDACTED
	return ""
REDACTED
