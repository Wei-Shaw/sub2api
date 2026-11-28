package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// retryStubPricingRemoteClient 记录调用次数，并按配置的前 N 次失败模拟瞬时网络故障。
type retryStubPricingRemoteClient struct {
	body         string
	hashValue    string
	hashFailures int
	jsonFailures int
	hashCalls    int
	jsonCalls    int
}

func (c *retryStubPricingRemoteClient) FetchHashText(context.Context, string) (string, error) {
	c.hashCalls++
	if c.hashCalls <= c.hashFailures {
		return "", errors.New("TLS handshake timeout")
	}
	return c.hashValue, nil
}

func (c *retryStubPricingRemoteClient) FetchPricingJSON(context.Context, string) ([]byte, error) {
	c.jsonCalls++
	if c.jsonCalls <= c.jsonFailures {
		return nil, errors.New("EOF")
	}
	return []byte(c.body), nil
}

// recordingWait 记录每次退避时长并立即返回，避免测试真实等待。
func recordingWait() (func(time.Duration), *[]time.Duration) {
	var mu sync.Mutex
	delays := &[]time.Duration{}
	return func(delay time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		*delays = append(*delays, delay)
	}, delays
}

func newRemoteRetryPricingService(t *testing.T, client PricingRemoteClient, hashURL string) *PricingService {
	t.Helper()
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{
		RemoteURL: "https://example.com/pricing.json",
		HashURL:   hashURL,
		DataDir:   t.TempDir(),
	}}, nil)
	svc.remoteClient = client
	return svc
}

// 单次 TLS 握手超时属于瞬时故障：哈希探测必须重试，而不是让整个更新周期被跳过。
func TestPricingFetchRemoteHashRetriesTransientFailures(t *testing.T) {
	client := &retryStubPricingRemoteClient{hashValue: "abc123", hashFailures: 2}
	svc := newRemoteRetryPricingService(t, client, "https://example.com/pricing.sha256")
	wait, delays := recordingWait()

	hash, err := svc.fetchRemoteHashWithWait(wait)

	require.NoError(t, err)
	require.Equal(t, "abc123", hash)
	require.Equal(t, 3, client.hashCalls, "两次失败后第三次成功")
	require.Equal(t, []time.Duration{pricingRemoteFetchBaseBackoff, 2 * pricingRemoteFetchBaseBackoff}, *delays)
}

// 重试次数用尽后仍然失败：错误必须带上尝试次数，便于区分瞬时抖动与地址/配置错误。
func TestPricingFetchRemoteHashGivesUpAfterMaxAttempts(t *testing.T) {
	client := &retryStubPricingRemoteClient{hashValue: "abc123", hashFailures: 99}
	svc := newRemoteRetryPricingService(t, client, "https://example.com/pricing.sha256")
	wait, delays := recordingWait()

	hash, err := svc.fetchRemoteHashWithWait(wait)

	require.Error(t, err)
	require.Empty(t, hash)
	require.Contains(t, err.Error(), "gave up after 3 attempt(s)")
	require.Contains(t, err.Error(), "TLS handshake timeout")
	require.Equal(t, 3, client.hashCalls)
	require.Len(t, *delays, 2, "最后一次失败后不再等待")
}

// URL 校验失败属于配置错误，重试没有意义：不得发起任何请求。
func TestPricingFetchRemoteHashDoesNotRetryInvalidURL(t *testing.T) {
	client := &retryStubPricingRemoteClient{hashValue: "abc123"}
	svc := newRemoteRetryPricingService(t, client, "://invalid")
	wait, delays := recordingWait()

	_, err := svc.fetchRemoteHashWithWait(wait)

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid pricing url")
	require.Zero(t, client.hashCalls, "配置错误不应发起请求")
	require.Empty(t, *delays)
}

// 目录下载按同一退避预算重试，成功后照常解析、落盘并更新内存数据。
func TestPricingDownloadRetriesTransientFailures(t *testing.T) {
	client := &retryStubPricingRemoteClient{body: hotReloadCatalogJSON, jsonFailures: 2}
	svc := newRemoteRetryPricingService(t, client, "")
	wait, delays := recordingWait()

	body, err := svc.fetchPricingJSONWithWait("https://example.com/pricing.json", wait)

	require.NoError(t, err)
	require.JSONEq(t, hotReloadCatalogJSON, string(body))
	require.Equal(t, 3, client.jsonCalls)
	require.Equal(t, []time.Duration{pricingRemoteFetchBaseBackoff, 2 * pricingRemoteFetchBaseBackoff}, *delays)
}

func TestPricingDownloadGivesUpAfterMaxAttempts(t *testing.T) {
	client := &retryStubPricingRemoteClient{body: hotReloadCatalogJSON, jsonFailures: 99}
	svc := newRemoteRetryPricingService(t, client, "")

	err := svc.downloadPricingData()

	require.Error(t, err)
	require.Contains(t, err.Error(), "gave up after 3 attempt(s)")
	require.Contains(t, err.Error(), "download failed")
	require.Equal(t, 3, client.jsonCalls)
}

// 单次尝试吃掉大部分预算时，退避后已无剩余预算：必须停止重试，避免总耗时失控。
func TestPricingRemoteRetryStopsWhenBudgetExhausted(t *testing.T) {
	wait, delays := recordingWait()
	calls := 0
	attempts, err := withPricingRemoteRetry("test fetch", 60*time.Millisecond, wait, func(context.Context) error {
		calls++
		time.Sleep(40 * time.Millisecond)
		return errors.New("EOF")
	})

	require.Error(t, err)
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, calls, "剩余预算不足一次退避时不得再次请求")
	require.Empty(t, *delays)
}

func TestPricingRemoteRetryBackoff(t *testing.T) {
	require.Equal(t, pricingRemoteFetchBaseBackoff, pricingRemoteRetryBackoff(0), "非法入参按首次退避处理")
	require.Equal(t, pricingRemoteFetchBaseBackoff, pricingRemoteRetryBackoff(1))
	require.Equal(t, 2*pricingRemoteFetchBaseBackoff, pricingRemoteRetryBackoff(2))
	require.Equal(t, 4*pricingRemoteFetchBaseBackoff, pricingRemoteRetryBackoff(3))
	require.Equal(t, pricingRemoteFetchMaxBackoff, pricingRemoteRetryBackoff(4))
	require.Equal(t, pricingRemoteFetchMaxBackoff, pricingRemoteRetryBackoff(64), "退避必须封顶且不移位溢出")
}
