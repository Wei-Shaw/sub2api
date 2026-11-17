package upstreampricing

import (
	"context"
	"time"
)

func (m *Manager) activityLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		m.activityCycle()
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (m *Manager) activityCycle() {
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
	ids, err := m.Store.activityDue(ctx, time.Now())
	cancel()
	if err != nil {
		return
	}
	for _, id := range ids {
		if m.ctx.Err() != nil {
			return
		}
		ctx, cancel = context.WithTimeout(m.ctx, 2*time.Minute)
		_ = m.syncActivity(ctx, id, time.Now())
		cancel()
	}
}

func (m *Manager) syncActivity(ctx context.Context, id int64, now time.Time) (err error) {
	token, err := m.Store.claimActivity(ctx, id, now)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			m.Store.failActivity(cleanup, id, token)
		}
	}()
	src, err := m.Store.Source(ctx, id)
	if err != nil {
		return err
	}
	if !src.HasCredential || src.ActiveVersion == nil {
		return safeError("请先保存上游定价的面板配置")
	}
	fp, err := m.Store.AccountFingerprint(ctx, id)
	if err != nil || fp != src.AccountFingerprint {
		return ErrConflict
	}
	c := src.Config
	if err = ValidateConfig(&c); err != nil {
		return err
	}
	c.Token, err = m.Cipher.Decrypt(src.Credential)
	if err != nil {
		return safeError("面板凭证无法解密")
	}
	state, err := m.Store.activityState(ctx, id)
	if err != nil {
		return err
	}
	identity := activityIdentity(src)
	start, end := activityWindow(now, state.LastSuccess, state.Identity == identity)
	active, err := m.Store.activityHasCalls(ctx, id, start, end)
	if err != nil {
		return err
	}
	var records []TimingRecord
	var through *time.Time
	detailError := ""
	if active {
		// A bounded overlap absorbs station timestamp/recording lag. It never
		// crosses today's midnight and upserts make repeated rows idempotent.
		queryStart := start.Add(-3 * time.Minute)
		midnight, _ := activityWindow(now, nil, false)
		if queryStart.Before(midnight) {
			queryStart = midnight
		}
		records, err = m.Remote.activityRecords(ctx, c, queryStart, end)
		if err == nil {
			through = &end
		} else {
			detailError = "明细同步失败，保留上次成功进度，下个周期重试"
			records = nil
		}
	}
	// Wallet freshness is independent of local inference activity and price sync.
	var balance *string
	walletError := ""
	value, walletErr := m.Remote.wallet(ctx, c)
	if walletErr == nil {
		balance = &value
	} else if c.Provider != "sub2api" {
		walletError = "当前站点类型暂不支持钱包余额查询"
	} else {
		walletError = "钱包刷新失败，请检查面板凭证和站点接口"
	}
	return m.Store.publishActivity(ctx, src, token, identity, now, through, records, balance, walletError, detailError)
}

func (m *Manager) Activity(ctx context.Context, id int64) (*ActivityState, error) {
	src, err := m.Store.Source(ctx, id)
	if err != nil {
		return nil, err
	}
	if !src.HasCredential || src.ActiveVersion == nil {
		return &ActivityState{}, nil
	}
	fp, err := m.Store.AccountFingerprint(ctx, id)
	if err != nil || fp != src.AccountFingerprint {
		return nil, ErrConflict
	}
	state, err := m.Store.activityState(ctx, id)
	if err != nil {
		return nil, err
	}
	if state.Identity != activityIdentity(src) {
		return &ActivityState{Configured: true, NextAttempt: state.NextAttempt, LastError: state.LastError}, nil
	}
	state.Configured = true
	return state, nil
}
