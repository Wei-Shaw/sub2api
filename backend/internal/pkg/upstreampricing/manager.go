package upstreampricing

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

type Manager struct {
	Store   *Store
	Remote  *Remote
	Cipher  Cipher
	Imports *BrowserImports
	ctx     context.Context
	cancel  context.CancelFunc
	jobs    chan UsageJob
	wg      sync.WaitGroup
	once    sync.Once
}

func NewManager(store *Store, remote *Remote, cipher Cipher) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{Store: store, Remote: remote, Cipher: cipher, Imports: newBrowserImports(), ctx: ctx, cancel: cancel, jobs: make(chan UsageJob, 1024)}
}

func (m *Manager) Start() {
	m.once.Do(func() {
		m.wg.Add(3)
		go m.jobLoop()
		go m.syncLoop()
		go m.activityLoop()
	})
}
func (m *Manager) Stop() { m.cancel(); m.wg.Wait() }

func (m *Manager) prepare(ctx context.Context, id int64, c Config) (Config, *Source, string, string, error) {
	if err := ValidateConfig(&c); err != nil {
		return c, nil, "", "", err
	}
	src, err := m.Store.Source(ctx, id)
	if err != nil {
		return c, nil, "", "", err
	}
	fp, err := m.Store.AccountFingerprint(ctx, id)
	if err != nil {
		return c, nil, "", "", err
	}
	if c.Token == "" && c.BrowserImportID != "" {
		var encrypted string
		encrypted, err = m.Imports.resolve(c.BrowserImportID, id, src.Revision, fp, c)
		if err != nil {
			return c, nil, "", "", err
		}
		c.Token, err = m.Cipher.Decrypt(encrypted)
		if err != nil {
			return c, nil, "", "", safeError("无法解密暂存凭证，请重新连接")
		}
	}
	if c.Token == "" && src.HasCredential {
		if c.Provider != src.Config.Provider || c.BaseURL != src.Config.BaseURL || c.UserID != src.Config.UserID || fp != src.AccountFingerprint {
			return c, nil, "", "", safeError("站点或账号身份变化后，请重新填写面板读取凭证")
		}
		c.Token, err = m.Cipher.Decrypt(src.Credential)
		if err != nil {
			return c, nil, "", "", safeError("无法解密面板凭证，请重新填写")
		}
	}
	if c.Token == "" {
		return c, nil, "", "", safeError("请填写面板读取凭证（与模型调用 Key 分开）")
	}
	credential, err := m.Cipher.Encrypt(c.Token)
	if err != nil {
		return c, nil, "", "", safeError("无法保存加密凭证")
	}
	return c, src, credential, fp, nil
}

func (m *Manager) Groups(ctx context.Context, id int64, c Config) ([]Group, error) {
	c, _, _, _, err := m.prepare(ctx, id, c)
	if err != nil {
		return nil, err
	}
	return m.Remote.Groups(ctx, c)
}

func (m *Manager) Preview(ctx context.Context, id, revision int64, c Config) (Preview, error) {
	c, src, credential, fp, err := m.prepare(ctx, id, c)
	if err != nil {
		return Preview{}, err
	}
	if src.Revision != revision {
		return Preview{}, ErrConflict
	}
	cat, err := m.Remote.Fetch(ctx, c)
	if err != nil {
		return Preview{}, err
	}
	return m.Store.SavePreview(ctx, id, revision, c, credential, fp, cat)
}

func (m *Manager) Sync(ctx context.Context, id int64, trigger string) (int64, error) {
	src, token, err := m.Store.Claim(ctx, id)
	if err != nil {
		return 0, err
	}
	var cat Catalog
	fp, err := m.Store.AccountFingerprint(ctx, id)
	if err == nil && fp != src.AccountFingerprint {
		err = safeError("上游账号地址或 Key 已变化，请重新预览并应用定价")
	}
	if err == nil {
		var plain string
		plain, err = m.Cipher.Decrypt(src.Credential)
		if err != nil {
			err = safeError("面板凭证无法解密，请重新填写")
		} else {
			c := src.Config
			c.Token = plain
			cat, err = m.Remote.Fetch(ctx, c)
		}
	}
	if err == nil {
		var version int64
		version, err = m.Store.PublishSync(ctx, src, token, cat, trigger)
		if err == nil {
			return version, nil
		}
	}
	failureCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	message := "同步失败，上一份有效价格已保留"
	// Only errors produced by our validators/HTTP adapter are safe to display.
	if errors.Is(err, ErrConflict) {
		message = ErrConflict.Error()
	} else if cat.Fingerprint == "" {
		message = err.Error()
	}
	_ = m.Store.Fail(failureCtx, src, token, trigger, message)
	return 0, safeError(message)
}

func (m *Manager) RollbackPreview(ctx context.Context, id, version int64) (Preview, error) {
	src, err := m.Store.Source(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	fp, err := m.Store.AccountFingerprint(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	versions, err := m.Store.Versions(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	for _, v := range versions {
		if v.ID == version {
			if v.Config.Provider != src.Config.Provider || v.Config.BaseURL != src.Config.BaseURL || v.Config.UserID != src.Config.UserID || v.Config.Group != src.Config.Group || v.Config.GroupID != src.Config.GroupID || fp != src.AccountFingerprint || v.AccountFingerprint != fp {
				return Preview{}, safeError("仅可回滚当前账号与相同站点、分组的价格版本")
			}
			c := v.Config
			c.Automatic = src.Config.Automatic
			c.IntervalMinutes = src.Config.IntervalMinutes
			return m.Store.SavePreview(ctx, id, src.Revision, c, src.Credential, fp, v.Catalog)
		}
	}
	return Preview{}, safeError("版本不存在或不在最近 20 个版本内")
}

// Submit happens only after upstream usage persistence. Cost work cannot fail billing.
func (m *Manager) Submit(j UsageJob) {
	select {
	case <-m.ctx.Done():
		return
	default:
	}
	select {
	case m.jobs <- j:
	default:
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if m.Store.Enqueue(ctx, j) != nil {
			slog.Warn("upstream pricing: estimate enqueue unavailable")
		}
	}
}

func (m *Manager) jobLoop() {
	defer m.wg.Done()
	defer m.flushJobs()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case j := <-m.jobs:
			for attempt := 0; attempt < 3; attempt++ {
				// Finish the current evidence write even during graceful shutdown.
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				err := m.Store.Enqueue(ctx, j)
				cancel()
				if err == nil {
					break
				}
				if attempt == 2 {
					slog.Warn("upstream pricing: estimate evidence could not be saved")
					break
				}
				select {
				case <-m.ctx.Done():
					slog.Warn("upstream pricing: estimate evidence write interrupted after database failure")
					return
				case <-time.After(200 * time.Millisecond):
				}
			}
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
			err := m.Store.Process(ctx)
			cancel()
			if err != nil && m.ctx.Err() == nil {
				slog.Warn("upstream pricing: estimate worker unavailable")
			}
		}
	}
}

func (m *Manager) flushJobs() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		select {
		case j := <-m.jobs:
			if m.Store.Enqueue(ctx, j) != nil {
				slog.Warn("upstream pricing: shutdown estimate flush unavailable")
			}
		default:
			return
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (m *Manager) syncLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.Imports.Prune()
			ctx, cancel := context.WithTimeout(m.ctx, 5*time.Second)
			ids, err := m.Store.Due(ctx)
			cancel()
			if err != nil {
				continue
			}
			for _, id := range ids {
				ctx, cancel := context.WithTimeout(m.ctx, 60*time.Second)
				_, _ = m.Sync(ctx, id, "scheduled")
				cancel()
				if m.ctx.Err() != nil {
					return
				}
			}
		}
	}
}
