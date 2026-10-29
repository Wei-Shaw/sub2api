//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (s *ProxyExpirySuite) TestSweep_SkipsInactiveBackup() {
	for _, chain := range []bool{false, true} {
		s.Run(map[bool]string{false: "unresolved", true: "next active backup"}[chain], func() {
			now := time.Now()
			past := now.Add(-time.Hour)
			future := now.Add(time.Hour)
			healthy := s.mkProxy("active-tail", service.FallbackModeNone, &future, nil)
			backup := s.mkProxy("disabled-backup", service.FallbackModeNone, &future, nil)
			disabled, err := s.repo.GetByID(s.ctx, backup)
			s.Require().NoError(err)
			disabled.Status = "inactive"
			if chain {
				disabled.FallbackMode = service.FallbackModeProxy
				disabled.BackupProxyID = &healthy
			}
			s.Require().NoError(s.repo.Update(s.ctx, disabled))
			source := s.mkProxy("expired-source", service.FallbackModeProxy, &past, &backup)
			account := s.mkAccountWithProxy(source)
			_, err = s.repo.SweepExpiredProxies(s.ctx, now)
			s.Require().NoError(err)
			if chain {
				s.Equal(&healthy, s.accountProxyID(account))
			} else {
				s.Equal(&source, s.accountProxyID(account))
			}
			got, err := s.repo.GetByID(s.ctx, backup)
			s.Require().NoError(err)
			s.Equal("inactive", got.Status, "fallback traversal must not reactivate disabled proxies")
		})
	}
}
