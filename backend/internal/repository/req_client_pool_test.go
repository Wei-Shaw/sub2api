package repository

import (
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func forceHTTPVersion(t *testing.T, client *req.Client) string {
REDACTED
	transport := client.GetTransport()
	field := reflect.ValueOf(transport).Elem().FieldByName("forceHttpVersion")
	require.True(t, field.IsValid(), "forceHttpVersion field not found")
	require.True(t, field.CanAddr(), "forceHttpVersion field not addressable")
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().String()
REDACTED

func TestGetSharedReqClient_ForceHTTP2SeparatesCache(t *testing.T) {
	sharedReqClients = sync.Map{REDACTED
	base := reqClientOptions{
		ProxyURL: "http://proxy.local:8080",
		Timeout:  time.Second,
REDACTED
	clientDefault := getSharedReqClient(base)

	force := base
	force.ForceHTTP2 = true
	clientForce := getSharedReqClient(force)

	require.NotSame(t, clientDefault, clientForce)
	require.NotEqual(t, buildReqClientKey(base), buildReqClientKey(force))
REDACTED

func TestGetSharedReqClient_ReuseCachedClient(t *testing.T) {
	sharedReqClients = sync.Map{REDACTED
	opts := reqClientOptions{
		ProxyURL: "http://proxy.local:8080",
		Timeout:  2 * time.Second,
REDACTED
	first := getSharedReqClient(opts)
	second := getSharedReqClient(opts)
	require.Same(t, first, second)
REDACTED

func TestGetSharedReqClient_IgnoresNonClientCache(t *testing.T) {
	sharedReqClients = sync.Map{REDACTED
	opts := reqClientOptions{
		ProxyURL: " http://proxy.local:8080 ",
		Timeout:  3 * time.Second,
REDACTED
	key := buildReqClientKey(opts)
	sharedReqClients.Store(key, "invalid")

	client := getSharedReqClient(opts)

	require.NotNil(t, client)
	loaded, ok := sharedReqClients.Load(key)
	require.True(t, ok)
	require.IsType(t, "invalid", loaded)
REDACTED

func TestGetSharedReqClient_ImpersonateAndProxy(t *testing.T) {
	sharedReqClients = sync.Map{REDACTED
	opts := reqClientOptions{
		ProxyURL:    "  http://proxy.local:8080  ",
		Timeout:     4 * time.Second,
		Impersonate: true,
REDACTED
	client := getSharedReqClient(opts)

	require.NotNil(t, client)
	require.Equal(t, "http://proxy.local:8080|4s|true|false", buildReqClientKey(opts))
REDACTED

func TestCreateOpenAIReqClient_Timeout120Seconds(t *testing.T) {
	sharedReqClients = sync.Map{REDACTED
	client := createOpenAIReqClient("http://proxy.local:8080")
	require.Equal(t, 120*time.Second, client.GetClient().Timeout)
REDACTED

func TestCreateGeminiReqClient_ForceHTTP2Disabled(t *testing.T) {
	sharedReqClients = sync.Map{REDACTED
	client := createGeminiReqClient("http://proxy.local:8080")
	require.Equal(t, "", forceHTTPVersion(t, client))
REDACTED
