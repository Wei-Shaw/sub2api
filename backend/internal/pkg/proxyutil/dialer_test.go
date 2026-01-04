package proxyutil

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureTransportProxy_Nil(t *testing.T) {
	transport := &http.Transport{REDACTED
	err := ConfigureTransportProxy(transport, nil)

REDACTED
	assert.Nil(t, transport.Proxy, "nil proxy should not set Proxy")
	assert.Nil(t, transport.DialContext, "nil proxy should not set DialContext")
REDACTED

func TestConfigureTransportProxy_HTTP(t *testing.T) {
	transport := &http.Transport{REDACTED
	proxyURL, _ := url.Parse("http://proxy.example.com:8080")

	err := ConfigureTransportProxy(transport, proxyURL)

REDACTED
	assert.NotNil(t, transport.Proxy, "HTTP proxy should set Proxy")
	assert.Nil(t, transport.DialContext, "HTTP proxy should not set DialContext")
REDACTED

func TestConfigureTransportProxy_HTTPS(t *testing.T) {
	transport := &http.Transport{REDACTED
	proxyURL, _ := url.Parse("https://secure-proxy.example.com:8443")

	err := ConfigureTransportProxy(transport, proxyURL)

REDACTED
	assert.NotNil(t, transport.Proxy, "HTTPS proxy should set Proxy")
	assert.Nil(t, transport.DialContext, "HTTPS proxy should not set DialContext")
REDACTED

func TestConfigureTransportProxy_SOCKS5(t *testing.T) {
	transport := &http.Transport{REDACTED
	proxyURL, _ := url.Parse("socks5://socks.example.com:1080")

	err := ConfigureTransportProxy(transport, proxyURL)

REDACTED
	assert.Nil(t, transport.Proxy, "SOCKS5 proxy should not set Proxy")
	assert.NotNil(t, transport.DialContext, "SOCKS5 proxy should set DialContext")
REDACTED

func TestConfigureTransportProxy_SOCKS5H(t *testing.T) {
	transport := &http.Transport{REDACTED
	proxyURL, _ := url.Parse("socks5h://socks.example.com:1080")

	err := ConfigureTransportProxy(transport, proxyURL)

REDACTED
	assert.Nil(t, transport.Proxy, "SOCKS5H proxy should not set Proxy")
	assert.NotNil(t, transport.DialContext, "SOCKS5H proxy should set DialContext")
REDACTED

func TestConfigureTransportProxy_CaseInsensitive(t *testing.T) {
	testCases := []struct {
		scheme   string
		useProxy bool // true = uses Transport.Proxy, false = uses DialContext
REDACTED{
		{"HTTP://proxy.example.com:8080", trueREDACTED,
		{"Http://proxy.example.com:8080", trueREDACTED,
		{"HTTPS://proxy.example.com:8443", trueREDACTED,
		{"Https://proxy.example.com:8443", trueREDACTED,
		{"SOCKS5://socks.example.com:1080", falseREDACTED,
		{"Socks5://socks.example.com:1080", falseREDACTED,
		{"SOCKS5H://socks.example.com:1080", falseREDACTED,
		{"Socks5h://socks.example.com:1080", falseREDACTED,
REDACTED

	for _, tc := range testCases {
		t.Run(tc.scheme, func(t *testing.T) {
			transport := &http.Transport{REDACTED
			proxyURL, _ := url.Parse(tc.scheme)

			err := ConfigureTransportProxy(transport, proxyURL)

		REDACTED
			if tc.useProxy {
				assert.NotNil(t, transport.Proxy)
				assert.Nil(t, transport.DialContext)
		REDACTED else {
				assert.Nil(t, transport.Proxy)
				assert.NotNil(t, transport.DialContext)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestConfigureTransportProxy_Unsupported(t *testing.T) {
	testCases := []string{
		"ftp://ftp.example.com",
		"file:///path/to/file",
		"unknown://example.com",
REDACTED

	for _, tc := range testCases {
		t.Run(tc, func(t *testing.T) {
			transport := &http.Transport{REDACTED
			proxyURL, _ := url.Parse(tc)

			err := ConfigureTransportProxy(transport, proxyURL)

		REDACTED
			assert.Contains(t, err.Error(), "unsupported proxy scheme")
	REDACTED)
REDACTED
REDACTED

func TestConfigureTransportProxy_WithAuth(t *testing.T) {
	transport := &http.Transport{REDACTED
	proxyURL, _ := url.Parse("socks5://user:password@socks.example.com:1080")

	err := ConfigureTransportProxy(transport, proxyURL)

REDACTED
	assert.NotNil(t, transport.DialContext, "SOCKS5 with auth should set DialContext")
REDACTED

func TestConfigureTransportProxy_EmptyScheme(t *testing.T) {
	transport := &http.Transport{REDACTED
	// 空 scheme 的 URL
	proxyURL := &url.URL{Host: "proxy.example.com:8080"REDACTED

	err := ConfigureTransportProxy(transport, proxyURL)

REDACTED
	assert.Contains(t, err.Error(), "unsupported proxy scheme")
REDACTED

func TestConfigureTransportProxy_PreservesExistingConfig(t *testing.T) {
	// 验证代理配置不会覆盖 Transport 的其他配置
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
REDACTED
	proxyURL, _ := url.Parse("socks5://socks.example.com:1080")

	err := ConfigureTransportProxy(transport, proxyURL)

REDACTED
	assert.Equal(t, 100, transport.MaxIdleConns, "MaxIdleConns should be preserved")
	assert.Equal(t, 10, transport.MaxIdleConnsPerHost, "MaxIdleConnsPerHost should be preserved")
	assert.NotNil(t, transport.DialContext, "DialContext should be set")
REDACTED

func TestConfigureTransportProxy_IPv6(t *testing.T) {
	testCases := []struct {
		name     string
		proxyURL string
REDACTED{
		{"SOCKS5H with IPv6 loopback", "socks5h://[::1]:1080"REDACTED,
		{"SOCKS5 with full IPv6", "socks5://[2001:db8::1]:1080"REDACTED,
		{"HTTP with IPv6", "http://[::1]:8080"REDACTED,
REDACTED

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &http.Transport{REDACTED
			proxyURL, err := url.Parse(tc.proxyURL)
			require.NoError(t, err, "URL should be parseable")

			err = ConfigureTransportProxy(transport, proxyURL)
		REDACTED
	REDACTED)
REDACTED
REDACTED

func TestConfigureTransportProxy_SpecialCharsInPassword(t *testing.T) {
	testCases := []struct {
		name     string
		proxyURL string
REDACTED{
		// 密码包含 @ 符号（URL 编码为 %40）
		{"password with @", "socks5://user:p%40ssword@proxy.example.com:1080"REDACTED,
		// 密码包含 : 符号（URL 编码为 %3A）
		{"password with :", "socks5://user:pass%3Aword@proxy.example.com:1080"REDACTED,
		// 密码包含 / 符号（URL 编码为 %2F）
		{"password with /", "socks5://user:pass%2Fword@proxy.example.com:1080"REDACTED,
		// 复杂密码
		{"complex password", "socks5h://admin:P%40ss%3Aw0rd%2F123@proxy.example.com:1080"REDACTED,
REDACTED

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &http.Transport{REDACTED
			proxyURL, err := url.Parse(tc.proxyURL)
			require.NoError(t, err, "URL should be parseable")

			err = ConfigureTransportProxy(transport, proxyURL)
		REDACTED
			assert.NotNil(t, transport.DialContext, "SOCKS5 should set DialContext")
	REDACTED)
REDACTED
REDACTED
