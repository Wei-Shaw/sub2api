package repository

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func TestGetSharedReqClient_TLSProfileSeparatesCacheByContent(t *testing.T) {
	sharedReqClients = sync.Map{}
	first := reqClientOptions{Timeout: time.Second, TLSProfile: &tlsfingerprint.Profile{CipherSuites: []uint16{0x1301}}}
	second := reqClientOptions{Timeout: time.Second, TLSProfile: &tlsfingerprint.Profile{CipherSuites: []uint16{0x1302}}}

	firstClient, err := getSharedReqClient(first)
	require.NoError(t, err)
	secondClient, err := getSharedReqClient(second)
	require.NoError(t, err)

	require.NotSame(t, firstClient, secondClient)
	require.NotEqual(t, buildReqClientKey(first), buildReqClientKey(second))
}

func TestGetSharedReqClient_TLSProfileProxiesPlainHTTPWithoutDoubleProxyingHTTPS(t *testing.T) {
	sharedReqClients = sync.Map{}
	client, err := getSharedReqClient(reqClientOptions{
		ProxyURL:   "http://proxy.example:8080",
		Timeout:    time.Second,
		TLSProfile: &tlsfingerprint.Profile{CipherSuites: []uint16{0x1301}},
	})
	require.NoError(t, err)
	transport := client.GetTransport()
	require.NotNil(t, transport.DialTLSContext)
	require.NotNil(t, transport.Proxy)

	httpReq, err := http.NewRequest(http.MethodGet, "http://upstream.example/v1", nil)
	require.NoError(t, err)
	proxyURL, err := transport.Proxy(httpReq)
	require.NoError(t, err)
	require.NotNil(t, proxyURL)
	require.Equal(t, "http://proxy.example:8080", proxyURL.String())

	httpsReq, err := http.NewRequest(http.MethodGet, "https://upstream.example/v1", nil)
	require.NoError(t, err)
	proxyURL, err = transport.Proxy(httpsReq)
	require.NoError(t, err)
	require.Nil(t, proxyURL)
}

func TestCreateOpenAITLSProfileReqClientPreservesChromeImpersonation(t *testing.T) {
	sharedReqClients = sync.Map{}
	profile := &tlsfingerprint.Profile{Name: "account override", CipherSuites: []uint16{0x1301}}

	client, err := CreateOpenAITLSProfileReqClient("", profile)
	require.NoError(t, err)
	require.NotNil(t, client.GetTransport().DialTLSContext)
	require.NotEmpty(t, client.Headers.Get("Sec-Ch-Ua"))
	require.NotEmpty(t, client.Headers.Get("User-Agent"))
}

func forceHTTPVersion(t *testing.T, client *req.Client) string {
	t.Helper()
	transport := client.GetTransport()
	field := reflect.ValueOf(transport).Elem().FieldByName("forceHttpVersion")
	require.True(t, field.IsValid(), "forceHttpVersion field not found")
	require.True(t, field.CanAddr(), "forceHttpVersion field not addressable")
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().String()
}

func TestGetSharedReqClient_ForceHTTP2SeparatesCache(t *testing.T) {
	sharedReqClients = sync.Map{}
	base := reqClientOptions{
		ProxyURL: "http://proxy.local:8080",
		Timeout:  time.Second,
	}
	clientDefault, err := getSharedReqClient(base)
	require.NoError(t, err)

	force := base
	force.ForceHTTP2 = true
	clientForce, err := getSharedReqClient(force)
	require.NoError(t, err)

	require.NotSame(t, clientDefault, clientForce)
	require.NotEqual(t, buildReqClientKey(base), buildReqClientKey(force))
}

func TestGetSharedReqClient_ReuseCachedClient(t *testing.T) {
	sharedReqClients = sync.Map{}
	opts := reqClientOptions{
		ProxyURL: "http://proxy.local:8080",
		Timeout:  2 * time.Second,
	}
	first, err := getSharedReqClient(opts)
	require.NoError(t, err)
	second, err := getSharedReqClient(opts)
	require.NoError(t, err)
	require.Same(t, first, second)
}

func TestGetSharedReqClient_IgnoresNonClientCache(t *testing.T) {
	sharedReqClients = sync.Map{}
	opts := reqClientOptions{
		ProxyURL: " http://proxy.local:8080 ",
		Timeout:  3 * time.Second,
	}
	key := buildReqClientKey(opts)
	sharedReqClients.Store(key, "invalid")

	client, err := getSharedReqClient(opts)
	require.NoError(t, err)

	require.NotNil(t, client)
	loaded, ok := sharedReqClients.Load(key)
	require.True(t, ok)
	require.IsType(t, "invalid", loaded)
}

func TestGetSharedReqClient_ImpersonateAndProxy(t *testing.T) {
	sharedReqClients = sync.Map{}
	opts := reqClientOptions{
		ProxyURL:    "  http://proxy.local:8080  ",
		Timeout:     4 * time.Second,
		Impersonate: true,
	}
	client, err := getSharedReqClient(opts)
	require.NoError(t, err)

	require.NotNil(t, client)
	require.Equal(t, "http://proxy.local:8080|4s|true|false", buildReqClientKey(opts))
}

func TestGetSharedReqClient_InvalidProxyURL(t *testing.T) {
	sharedReqClients = sync.Map{}
	opts := reqClientOptions{
		ProxyURL: "://missing-scheme",
		Timeout:  time.Second,
	}
	_, err := getSharedReqClient(opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid proxy URL")
}

func TestGetSharedReqClient_ProxyURLMissingHost(t *testing.T) {
	sharedReqClients = sync.Map{}
	opts := reqClientOptions{
		ProxyURL: "http://",
		Timeout:  time.Second,
	}
	_, err := getSharedReqClient(opts)
	require.Error(t, err)
	require.Contains(t, err.Error(), "proxy URL missing host")
}

func TestCreateOpenAIReqClientForContext_Timeout120Seconds(t *testing.T) {
	sharedReqClients = sync.Map{}
	client, err := createOpenAIReqClientForContext(context.Background(), "http://proxy.local:8080")
	require.NoError(t, err)
	require.Equal(t, 120*time.Second, client.GetClient().Timeout)
}

func TestCreateGeminiReqClient_ForceHTTP2Disabled(t *testing.T) {
	sharedReqClients = sync.Map{}
	client, err := createGeminiReqClient("http://proxy.local:8080")
	require.NoError(t, err)
	require.Equal(t, "", forceHTTPVersion(t, client))
}

func TestInstrumentReqClientRecordsDependency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	collector := servertiming.New(time.Now())
	ctx := servertiming.WithCollector(context.Background(), collector)
	client := instrumentReqClient(req.C())
	response, err := client.R().SetContext(ctx).Get(server.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode)

	header := collector.HeaderValue(time.Now(), "bypass")
	require.True(t, strings.Contains(header, "dep_http;dur="), header)
}
