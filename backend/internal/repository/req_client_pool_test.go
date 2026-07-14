package repository

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	resinpkg "github.com/Wei-Shaw/sub2api/internal/pkg/resin"
	"github.com/Wei-Shaw/sub2api/internal/pkg/servertiming"
	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

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

func TestCreateOpenAIReqClient_Timeout120Seconds(t *testing.T) {
	sharedReqClients = sync.Map{}
	client, err := createOpenAIReqClient("http://proxy.local:8080")
	require.NoError(t, err)
	require.Equal(t, 120*time.Second, client.GetClient().Timeout)
}

func TestCreateGeminiReqClient_ForceHTTP2Disabled(t *testing.T) {
	sharedReqClients = sync.Map{}
	client, err := createGeminiReqClient("http://proxy.local:8080")
	require.NoError(t, err)
	require.Equal(t, "", forceHTTPVersion(t, client))
}

func TestGetSharedReqClient_ResinSharedProxyAddsConnectAuthFromContext(t *testing.T) {
	sharedReqClients = sync.Map{}

	gotMethod := make(chan string, 1)
	gotAuth := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case gotMethod <- r.Method:
		default:
		}
		select {
		case gotAuth <- r.Header.Get("Proxy-Authorization"):
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()

	rawResinURL := strings.Replace(proxy.URL, "://", "://openai:token123@", 1) + "#resin"
	cfg, err := resinpkg.Parse(rawResinURL)
	require.NoError(t, err)

	client, err := getSharedReqClient(reqClientOptions{
		ProxyURL: rawResinURL,
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)

	_, err = client.R().
		SetContext(resinpkg.WithAccountID(context.Background(), 42)).
		Get("https://example.com")
	require.Error(t, err)

	select {
	case method := <-gotMethod:
		require.Equal(t, http.MethodConnect, method)
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not receive CONNECT request")
	}

	select {
	case auth := <-gotAuth:
		require.Equal(t, cfg.ProxyAuthorizationHeader(42), auth)
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not receive Proxy-Authorization header")
	}
}

func TestGetSharedReqClient_ResinSOCKS5UsesAccountCredentialsInProxyURL(t *testing.T) {
	sharedReqClients = sync.Map{}

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer upstream.Close()

	socksSrv := newSOCKS5TestServer(t)

	cfg, err := resinpkg.Parse("socks5h://openai:token123@" + socksSrv.Addr() + "#resin")
	require.NoError(t, err)

	client, err := getSharedReqClient(reqClientOptions{
		ProxyURL: cfg.ForwardProxyURLForAccount(42),
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)

	resp, err := client.R().Get(upstream.URL)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	record, ok := socksSrv.LastRecord()
	require.True(t, ok, "expected socks5 auth record")
	require.Equal(t, "openai.acct-42", record.Username)
	require.Equal(t, "token123", record.Password)
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
