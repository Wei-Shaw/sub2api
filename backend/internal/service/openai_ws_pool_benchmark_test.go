package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

func BenchmarkOpenAIWSPoolAcquire(b *testing.B) {
	cfg := &config.Config{REDACTED
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 8
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 4
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 256
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 1

	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCountingDialer{REDACTED)

	account := &Account{ID: 1001, Platform: PlatformOpenAI, Type: AccountTypeAPIKeyREDACTED
	req := openAIWSAcquireRequest{
		Account: account,
		WSURL:   "wss://example.com/v1/responses",
REDACTED
	ctx := context.Background()

	lease, err := pool.Acquire(ctx, req)
	if err != nil {
		b.Fatalf("warm acquire failed: %v", err)
REDACTED
	lease.Release()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			var (
				got        *openAIWSConnLease
				acquireErr error
			)
			for retry := 0; retry < 3; retry++ {
				got, acquireErr = pool.Acquire(ctx, req)
				if acquireErr == nil {
					break
			REDACTED
				if !errors.Is(acquireErr, errOpenAIWSConnClosed) {
					break
			REDACTED
		REDACTED
			if acquireErr != nil {
				b.Fatalf("acquire failed: %v", acquireErr)
		REDACTED
			got.Release()
	REDACTED
REDACTED)
REDACTED
