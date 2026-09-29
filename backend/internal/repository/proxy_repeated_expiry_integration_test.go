//go:build integration

package repository

import (
	"encoding/json"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *ProxyExpirySuite) TestSweep_RepeatedExpiryPreservesOriginalProxy() {
	for _, direct := range []bool{false, true} {
		s.Run(map[bool]string{false: "next backup", true: "direct"}[direct], func() {
			now := time.Now()
			past := now.Add(-time.Hour)
			soon := now.Add(time.Hour)
			later := now.Add(48 * time.Hour)
			originalID := s.mkProxy("repeated-original", service.FallbackModeProxy, &past, nil)
			middleID := s.mkProxy("repeated-middle", service.FallbackModeDirect, &soon, nil)
			lastID := s.mkProxy("repeated-last", service.FallbackModeNone, &later, nil)
			// Populate directed foreign keys directly so this regression does not depend
			// on the separate ORM backup-edge fix.
			_, err := s.tx.ExecContext(s.ctx, `UPDATE proxies SET backup_proxy_id=$1 WHERE id=$2`, middleID, originalID)
			s.Require().NoError(err)
			if !direct {
				_, err = s.tx.ExecContext(s.ctx, `UPDATE proxies SET fallback_mode='proxy',backup_proxy_id=$1 WHERE id=$2`, lastID, middleID)
				s.Require().NoError(err)
			}
			accountID := s.mkAccountWithProxy(originalID)
			changed, err := s.repo.SweepExpiredProxies(s.ctx, now)
			s.Require().NoError(err)
			s.Require().EqualValues(1, changed)
			s.Require().Equal(&middleID, s.accountProxyID(accountID))

			changed, err = s.repo.SweepExpiredProxies(s.ctx, now.Add(2*time.Hour))
			s.Require().NoError(err)
			s.Require().EqualValues(1, changed, "accounts already in fallback must still be rerouted")
			if direct {
				s.Require().Nil(s.accountProxyID(accountID))
			} else {
				s.Require().Equal(&lastID, s.accountProxyID(accountID))
			}
			var origin *int64
			err = scanSingleRow(s.ctx, s.tx, `SELECT proxy_fallback_origin_id FROM accounts WHERE id=$1`, []any{accountID}, &origin)
			s.Require().NoError(err)
			s.Require().Equal(&originalID, origin, "manual revert must retain the first original proxy")

			var payloadRaw []byte
			err = scanSingleRow(s.ctx, s.tx, `SELECT payload FROM scheduler_outbox WHERE event_type=$1 ORDER BY id DESC LIMIT 1`, []any{service.SchedulerOutboxEventAccountBulkChanged}, &payloadRaw)
			s.Require().NoError(err)
			var payload struct {
				AccountIDs []int64 `json:"account_ids"`
			}
			s.Require().NoError(json.Unmarshal(payloadRaw, &payload))
			s.Require().Equal([]int64{accountID}, payload.AccountIDs)

			changed, err = s.repo.SweepExpiredProxies(s.ctx, now.Add(3*time.Hour))
			s.Require().NoError(err)
			s.Require().Zero(changed, "repeating the scan must be idempotent")
			accounts := newAccountRepositoryWithSQL(s.tx.Client(), s.tx, nil)
			s.Require().NoError(accounts.RevertProxyFallback(s.ctx, accountID))
			s.Require().Equal(&originalID, s.accountProxyID(accountID))
			err = scanSingleRow(s.ctx, s.tx, `SELECT proxy_fallback_origin_id FROM accounts WHERE id=$1`, []any{accountID}, &origin)
			s.Require().NoError(err)
			s.Require().Nil(origin)
		})
	}
}
