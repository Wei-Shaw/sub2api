package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

// blockingHashRemoteClient 的 FetchHashText 一直阻塞到 release 关闭，模拟远端不可达时等满超时。
type blockingHashRemoteClient struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *blockingHashRemoteClient) FetchPricingJSON(context.Context, string) ([]byte, error) {
	return nil, context.DeadlineExceeded
}

func (c *blockingHashRemoteClient) FetchHashText(context.Context, string) (string, error) {
	c.once.Do(func() { close(c.started) })
	<-c.release
	return "", context.DeadlineExceeded
}

// failingHashRemoteClient 的哈希请求立即失败，下载请求只计数。
type failingHashRemoteClient struct {
	mu        sync.Mutex
	hashCalls int
	downloads int
}

func (c *failingHashRemoteClient) FetchPricingJSON(context.Context, string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.downloads++
	return nil, errors.New("unexpected download")
}

func (c *failingHashRemoteClient) FetchHashText(context.Context, string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hashCalls++
	return "", errors.New("hash endpoint unreachable")
}

func (c *failingHashRemoteClient) counts() (hashCalls, downloads int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hashCalls, c.downloads
}

func newStartupPricingService(t *testing.T, remote PricingRemoteClient) *PricingService {
	t.Helper()
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{
		DataDir:   t.TempDir(),
		RemoteURL: "https://example.com/pricing.json",
		HashURL:   "https://example.com/pricing.sha256",
	}}, remote)
	require.NoError(t, os.WriteFile(svc.getPricingFilePath(), []byte(hotReloadCatalogJSON), 0644))
	return svc
}

// 本地价格文件已在时，启动检查不得等远端哈希：远端不可达也要立即返回，价格表已可用。
func TestPricingStartupDoesNotWaitForRemoteHash(t *testing.T) {
	remote := &blockingHashRemoteClient{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() { close(remote.release) })
	svc := newStartupPricingService(t, remote)

	done := make(chan error, 1)
	go func() { done <- svc.checkAndUpdatePricing() }()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("startup must not block on the remote hash fetch")
	}
	require.NotNil(t, svc.GetModelPricing("remote-model"), "本地价格表启动即可用")
	select {
	case <-remote.started:
	case <-time.After(2 * time.Second):
		t.Fatal("remote hash check must still run in the background")
	}
}

// 后台哈希校验发现远端哈希不同时，仍要下载并替换价格表。
func TestPricingStartupBackgroundCheckDownloadsWhenHashDiffers(t *testing.T) {
	newBody := `{` + hotReloadModelJSON("remote-model", 3e-6, 6e-6) + `}`
	newHash := fmt.Sprintf("%x", sha256.Sum256([]byte(newBody)))
	remote := &stubPricingRemoteClient{body: newBody, hash: newHash}
	svc := newStartupPricingService(t, remote)

	require.NoError(t, svc.checkAndUpdatePricing())

	require.Eventually(t, func() bool {
		p := svc.GetModelPricing("remote-model")
		return p != nil && p.InputCostPerToken == 3e-6
	}, 2*time.Second, 10*time.Millisecond, "后台校验应下载并替换价格表")
	svc.mu.RLock()
	require.Equal(t, newHash, svc.localHash)
	svc.mu.RUnlock()
	saved, err := os.ReadFile(svc.getPricingFilePath())
	require.NoError(t, err)
	require.Equal(t, newBody, string(saved))
}

// 后台哈希获取失败时沿用已加载的本地价格表，只记日志，不触发下载。
func TestPricingStartupBackgroundCheckKeepsLocalDataWhenHashFetchFails(t *testing.T) {
	logs, restore := captureStructuredLog(t)
	defer restore()
	remote := &failingHashRemoteClient{}
	svc := newStartupPricingService(t, remote)

	require.NoError(t, svc.checkAndUpdatePricing())

	require.Eventually(t, func() bool {
		return logs.ContainsMessage("Failed to fetch remote hash on startup: hash endpoint unreachable")
	}, 2*time.Second, 10*time.Millisecond, "哈希获取失败应记日志")
	hashCalls, downloads := remote.counts()
	require.Equal(t, 1, hashCalls)
	require.Zero(t, downloads)
	p := svc.GetModelPricing("remote-model")
	require.NotNil(t, p)
	require.InDelta(t, 1e-6, p.InputCostPerToken, 1e-12)
}
