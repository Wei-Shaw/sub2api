package upstreampricing

import (
	"context"
	"net/url"
	"sync"
	"time"
)

// BrowserImports holds short-lived panel credentials supplied by the manual
// form or a station-scoped userscript. Server-side browser profiles are never read.
type BrowserImports struct {
	mu      sync.Mutex
	entries map[string]*browserImport
	now     func() time.Time
}
type browserImport struct {
	ID                              string    `json:"id"`
	Ticket                          string    `json:"-"`
	ExpiresAt                       time.Time `json:"expires_at"`
	Status                          string    `json:"status"`
	UserID                          string    `json:"user_id"`
	Groups                          []Group   `json:"groups,omitempty"`
	Error                           string    `json:"error,omitempty"`
	owner, account, revision        int64
	fingerprint, origin, credential string
	config                          Config
}
type BrowserImportStart struct {
	ID        string    `json:"id"`
	Ticket    string    `json:"ticket"`
	ExpiresAt time.Time `json:"expires_at"`
}
type BrowserConnectionInfo struct {
	BaseURL  string `json:"base_url"`
	Provider string `json:"provider"`
	UserID   string `json:"user_id"`
}

func newBrowserImports() *BrowserImports {
	return &BrowserImports{entries: map[string]*browserImport{}, now: time.Now}
}
func (b *BrowserImports) cleanup() {
	for id, e := range b.entries {
		if !b.now().Before(e.ExpiresAt) {
			delete(b.entries, id)
		}
	}
}
func (b *BrowserImports) Prune() { b.mu.Lock(); defer b.mu.Unlock(); b.cleanup() }
func (b *BrowserImports) start(owner, account, revision int64, fp, origin string, c Config) (BrowserImportStart, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanup()
	if len(b.entries) >= 128 {
		return BrowserImportStart{}, safeError("连接请求过多，请稍后重试")
	}
	id, ticket := randomToken(), randomToken()
	e := &browserImport{ID: id, Ticket: ticket, ExpiresAt: b.now().Add(5 * time.Minute), Status: "waiting", owner: owner, account: account, revision: revision, fingerprint: fp, origin: origin, config: publicConfig(c)}
	b.entries[id] = e
	return BrowserImportStart{id, ticket, e.ExpiresAt}, nil
}
func (b *BrowserImports) Status(id string, owner, account int64) (browserImport, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanup()
	e := b.entries[id]
	if e == nil || e.owner != owner || e.account != account {
		return browserImport{}, safeError("连接已过期，请重新发起")
	}
	copy := *e
	copy.Ticket = ""
	return copy, nil
}
func (b *BrowserImports) ticket(ticket, origin string, claim bool) (browserImport, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanup()
	for _, e := range b.entries {
		if e.Ticket == ticket && e.origin == origin && e.Status == "waiting" {
			if claim {
				e.Status = "validating"
				e.Ticket = ""
			}
			return *e, nil
		}
	}
	return browserImport{}, safeError("连接无效、已使用或已过期，请重新发起")
}
func (b *BrowserImports) finish(id, credential, userID string, groups []Group, err error) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanup()
	e := b.entries[id]
	if e == nil {
		return false
	}
	if err != nil {
		e.Status = "failed"
		e.Error = "凭证验证失败，请检查站点、用户 ID 和面板令牌后重新连接"
		return true
	}
	e.Status = "ready"
	e.credential = credential
	e.UserID = userID
	e.config.UserID = userID
	e.Groups = groups
	return true
}
func (b *BrowserImports) resolve(id string, account, revision int64, fp string, c Config) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cleanup()
	e := b.entries[id]
	if e == nil || e.Status != "ready" || e.account != account || e.revision != revision || e.fingerprint != fp || e.config.Provider != c.Provider || e.config.BaseURL != c.BaseURL || e.config.UserID != c.UserID {
		return "", safeError("导入已过期或配置已改变，请重新连接")
	}
	return e.credential, nil
}

func (m *Manager) StartBrowserImport(ctx context.Context, owner, id, revision int64, origin string, c Config) (BrowserImportStart, error) {
	c.Token = ""
	c.BrowserImportID = ""
	if err := ValidateConfig(&c); err != nil {
		return BrowserImportStart{}, err
	}
	src, err := m.Store.Source(ctx, id)
	if err != nil {
		return BrowserImportStart{}, err
	}
	if src.Revision != revision {
		return BrowserImportStart{}, ErrConflict
	}
	fp, err := m.Store.AccountFingerprint(ctx, id)
	if err != nil {
		return BrowserImportStart{}, err
	}
	return m.Imports.start(owner, id, revision, fp, origin, c)
}
func (m *Manager) BrowserImportInfo(ticket, origin string) (BrowserConnectionInfo, error) {
	e, err := m.Imports.ticket(ticket, origin, false)
	return BrowserConnectionInfo{e.config.BaseURL, e.config.Provider, e.config.UserID}, err
}

// FailBrowserImport consumes only the selected ticket. Browser diagnostics are
// never accepted into the server's state or returned to another page.
func (m *Manager) FailBrowserImport(ticket, origin string) error {
	e, err := m.Imports.ticket(ticket, origin, true)
	if err != nil {
		return err
	}
	if !m.Imports.finish(e.ID, "", "", nil, safeError("browser connection failed")) {
		return safeError("连接已过期，请重新发起")
	}
	return nil
}
func (m *Manager) CompleteBrowserImport(ctx context.Context, ticket, origin, token, userID string) error {
	e, err := m.Imports.ticket(ticket, origin, true)
	if err != nil {
		return err
	}
	c := e.config
	c.Token = token
	if c.Provider == "newapi" {
		if c.UserID != "" && c.UserID != userID {
			err = safeError("用户 ID 与连接配置不一致")
		} else {
			c.UserID = userID
		}
	}
	if err == nil {
		err = ValidateConfig(&c)
	}
	var groups []Group
	var encrypted string
	if err == nil && c.Token == "" {
		err = safeError("请输入面板访问令牌")
	}
	if err == nil {
		groups, err = m.Remote.Groups(ctx, c)
	}
	if err == nil {
		var src *Source
		src, err = m.Store.Source(ctx, e.account)
		if err == nil && src.Revision != e.revision {
			err = ErrConflict
		}
	}
	if err == nil {
		var fp string
		fp, err = m.Store.AccountFingerprint(ctx, e.account)
		if err == nil && fp != e.fingerprint {
			err = ErrConflict
		}
	}
	if err == nil {
		encrypted, err = m.Cipher.Encrypt(c.Token)
	}
	if !m.Imports.finish(e.ID, encrypted, c.UserID, groups, err) {
		return safeError("连接已过期，请重新发起")
	}
	if err != nil {
		return safeError("凭证验证失败，请检查站点、用户 ID 和面板令牌后重新连接")
	}
	return nil
}

// Local page import is deliberately limited to the same loopback origin.
// Remote deployments keep the existing authenticated manual token form.
func ValidateBrowserOrigin(raw, host, scheme string) error {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Host != host || u.Scheme != scheme || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") {
		return safeError("站点页导入需从本机 Sub2API 地址发起；其他部署请使用手动凭证输入")
	}
	return nil
}
