//go:build integration

package repository

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// The binding script on the Redis the integration suite provides.
type MessageThreadStoreSuite struct {
	IntegrationRedisSuite
	store service.MessageThreadStore
}

func (s *MessageThreadStoreSuite) SetupTest() {
	s.IntegrationRedisSuite.SetupTest()
	s.store = NewGatewayCache(s.rdb).(service.MessageThreadStore)
}

func (s *MessageThreadStoreSuite) binding(accountID int64) service.MessageThreadBinding {
	return service.MessageThreadBinding{
		AccountID: accountID, Identity: strings.Repeat("a", 64), Model: "claude-sonnet-4-5", ClientModel: "claude-sonnet-4-5",
		Wire: strings.Repeat("b", 32), Beta: "message-threads-2026-08-12", ContextTokens: 1200,
		ClientBeta: strings.Repeat("c", 64), ClientFixed: map[string]string{},
	}
}

func (s *MessageThreadStoreSuite) TestBindReadAndExpiry() {
	binding := s.binding(7)
	s.RequireNoError(s.store.BindMessageThread(s.ctx, "1:2:3", "msg_1", binding))
	got, err := s.store.ReadMessageThread(s.ctx, "1:2:3", "msg_1")
	s.RequireNoError(err)
	require.Equal(s.T(), binding, *got)

	ttl, err := s.rdb.PTTL(s.ctx, messageThreadBindingKey("1:2:3", "msg_1")).Result()
	s.RequireNoError(err)
	s.AssertTTLWithin(ttl, service.MessageThreadTTL-10*time.Second, service.MessageThreadTTL)

	other, err := s.store.ReadMessageThread(s.ctx, "9:2:3", "msg_1")
	s.RequireNoError(err)
	require.Nil(s.T(), other)
}

func (s *MessageThreadStoreSuite) TestConflictKeepsTheIDUnusable() {
	s.RequireNoError(s.store.BindMessageThread(s.ctx, "1:2:3", "msg_1", s.binding(7)))
	s.RequireNoError(s.store.BindMessageThread(s.ctx, "1:2:3", "msg_1", s.binding(7)), "the same binding again is harmless")
	for _, account := range []int64{8, 9, 7} {
		require.ErrorIs(s.T(), s.store.BindMessageThread(s.ctx, "1:2:3", "msg_1", s.binding(account)), errMessageThreadConflict)
		got, err := s.store.ReadMessageThread(s.ctx, "1:2:3", "msg_1")
		s.RequireNoError(err)
		require.Nil(s.T(), got)
	}
	ttl, err := s.rdb.PTTL(s.ctx, messageThreadBindingKey("1:2:3", "msg_1")).Result()
	s.RequireNoError(err)
	s.AssertTTLWithin(ttl, service.MessageThreadTTL-10*time.Second, service.MessageThreadTTL)
}

func (s *MessageThreadStoreSuite) TestConcurrentDifferentBinds() {
	const writers = 16
	errs := make([]error, writers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = s.store.BindMessageThread(s.ctx, "1:2:3", "msg_1", s.binding(int64(i+1)))
		}()
	}
	close(start)
	wg.Wait()
	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
		} else {
			require.ErrorIs(s.T(), err, errMessageThreadConflict)
		}
	}
	require.Equal(s.T(), 1, succeeded)
	got, err := s.store.ReadMessageThread(s.ctx, "1:2:3", "msg_1")
	s.RequireNoError(err)
	require.Nil(s.T(), got)
}

func TestMessageThreadStoreSuite(t *testing.T) {
	suite.Run(t, new(MessageThreadStoreSuite))
}
