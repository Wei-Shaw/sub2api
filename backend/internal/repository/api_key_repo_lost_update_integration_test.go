//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// api_keys 上的用量列由计费热路径原子递增（IncrementQuotaUsed /
// IncrementRateLimitUsage）。编辑 Key 时若整行回写，
// 并发累计的配额与限流计数就会被旧快照覆盖。

func (s *APIKeyRepoSuite) TestUpdate_DoesNotRevertConcurrentQuotaUsage() {
	user := s.mustCreateUser("apikey-lost-update-quota@example.com")
	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-lost-update-quota",
		Name:   "before",
		Status: service.StatusActive,
		Quota:  100,
	}
	s.Require().NoError(s.repo.Create(s.ctx, key), "Create")

	stale, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Zero(stale.QuotaUsed)

	newUsed, err := s.repo.IncrementQuotaUsed(s.ctx, key.ID, 30)
	s.Require().NoError(err, "IncrementQuotaUsed")
	s.Require().InDelta(30, newUsed, 1e-9)

	stale.Name = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.APIKeyUpdateFields{Name: true}),
		"Update",
	)

	got, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().Equal("after", got.Name, "declared column must still be written")
	s.Require().InDelta(30, got.QuotaUsed, 1e-9, "quota_used must not be reverted by a stale key edit")
}

func (s *APIKeyRepoSuite) TestUpdate_DoesNotRevertConcurrentRateLimitUsage() {
	user := s.mustCreateUser("apikey-lost-update-ratelimit@example.com")
	key := &service.APIKey{
		UserID:      user.ID,
		Key:         "sk-lost-update-ratelimit",
		Name:        "before",
		Status:      service.StatusActive,
		RateLimit5h: 100,
	}
	s.Require().NoError(s.repo.Create(s.ctx, key), "Create")

	stale, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID")
	s.Require().Zero(stale.Usage5h)

	s.Require().NoError(s.repo.IncrementRateLimitUsage(s.ctx, key.ID, 42), "IncrementRateLimitUsage")

	stale.Name = "after"
	s.Require().NoError(
		s.repo.Update(s.ctx, stale, service.APIKeyUpdateFields{Name: true}),
		"Update",
	)

	got, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID after update")
	s.Require().InDelta(42, got.Usage5h, 1e-9, "usage_5h must not be reverted by a stale key edit")
	s.Require().InDelta(42, got.Usage1d, 1e-9, "usage_1d must not be reverted by a stale key edit")
	s.Require().InDelta(42, got.Usage7d, 1e-9, "usage_7d must not be reverted by a stale key edit")
}

// 显式重置仍然必须生效，避免收窄写入列时把功能改坏。
func (s *APIKeyRepoSuite) TestUpdate_StillResetsUsageWhenDeclared() {
	user := s.mustCreateUser("apikey-reset-usage@example.com")
	key := &service.APIKey{
		UserID: user.ID,
		Key:    "sk-reset-usage",
		Name:   "reset",
		Status: service.StatusActive,
		Quota:  100,
	}
	s.Require().NoError(s.repo.Create(s.ctx, key), "Create")

	_, err := s.repo.IncrementQuotaUsed(s.ctx, key.ID, 30)
	s.Require().NoError(err, "IncrementQuotaUsed")
	s.Require().NoError(s.repo.IncrementRateLimitUsage(s.ctx, key.ID, 42), "IncrementRateLimitUsage")

	current, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID")
	current.QuotaUsed = 0
	current.Usage5h = 0
	current.Usage1d = 0
	current.Usage7d = 0
	current.Window5hStart = nil
	current.Window1dStart = nil
	current.Window7dStart = nil
	s.Require().NoError(
		s.repo.Update(s.ctx, current, service.APIKeyUpdateFields{QuotaUsed: true, RateLimitUsage: true}),
		"Update",
	)

	got, err := s.repo.GetByID(s.ctx, key.ID)
	s.Require().NoError(err, "GetByID after reset")
	s.Require().Zero(got.QuotaUsed, "explicit quota reset must still apply")
	s.Require().Zero(got.Usage5h, "explicit rate limit reset must still apply")
	s.Require().Zero(got.Usage1d)
	s.Require().Zero(got.Usage7d)
	s.Require().Nil(got.Window5hStart)
}
