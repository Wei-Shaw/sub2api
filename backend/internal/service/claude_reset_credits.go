package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httpclient"
)

var claudeResetGrantIDPattern = regexp.MustCompile(`^[a-z0-9_-]{1,40}$`)

const claudeResetUsageURL = "https://api.anthropic.com/api/oauth/usage?cedar_ember=1&skip_spend=1"

// ClaudeResetCredit is deliberately free of upstream grant and organization IDs.
type ClaudeResetCredit struct {
	SelectionToken   string             `json:"selection_token"`
	Label            string             `json:"label"`
	ResetsLeft       int                `json:"resets_left"`
	StartsAt         *time.Time         `json:"starts_at,omitempty"`
	ExpiresAt        *time.Time         `json:"expires_at,omitempty"`
	Clears           []string           `json:"clears"`
	PercentUsed      map[string]float64 `json:"percent_used"`
	Blocking         []string           `json:"blocking"`
	UseRequiresLimit bool               `json:"use_requires_limit"`
	Redeemable       bool               `json:"redeemable"`
}

type ClaudeResetCredits struct {
	Eligible       bool                `json:"eligible"`
	AvailableCount int                 `json:"available_count"`
	Credits        []ClaudeResetCredit `json:"credits"`
	CooldownUntil  *time.Time          `json:"cooldown_until,omitempty"`
	WeeklyResetsAt *time.Time          `json:"weekly_resets_at,omitempty"`
	FetchedAt      time.Time           `json:"fetched_at"`
}

type claudeResetGrant struct {
	ID               string             `json:"id"`
	Label            string             `json:"label"`
	ResetsLeft       int                `json:"resets_left"`
	StartsAt         *time.Time         `json:"starts_at"`
	EndsAt           *time.Time         `json:"ends_at"`
	Clears           []string           `json:"clears"`
	Paused           bool               `json:"paused"`
	UsableNow        bool               `json:"usable_now"`
	UseRequiresLimit *bool              `json:"use_requires_limit"`
	PercentUsed      map[string]float64 `json:"percent_used"`
	Blocking         []string           `json:"blocking"`
}
type claudeResetBlock struct {
	Eligible       bool               `json:"eligible"`
	AtLimit        bool               `json:"at_limit"`
	Grants         []claudeResetGrant `json:"grants"`
	NextGrantID    string             `json:"next_grant_id"`
	CooldownUntil  *time.Time         `json:"cooldown_until"`
	WeeklyResetsAt *time.Time         `json:"weekly_resets_at"`
}

type claudeResetAccounts interface {
	GetByID(context.Context, int64) (*Account, error)
}
type claudeResetTokens interface {
	GetAccessToken(context.Context, *Account) (string, error)
}

type ClaudeResetCreditService struct {
	writer      claudeResetWriter
	idempotency *IdempotencyCoordinator
	locks       LeaderLockCache
	accounts    claudeResetAccounts
	tokens      claudeResetTokens
	proxies     ProxyRepository
	settings    *SettingService
	do          func(*http.Request, string) (*http.Response, error)
	now         func() time.Time
}

func NewClaudeResetCreditService(accounts AccountRepository, tokens *ClaudeTokenProvider, proxies ProxyRepository, settings *SettingService) *ClaudeResetCreditService {
	s := &ClaudeResetCreditService{accounts: accounts, tokens: tokens, proxies: proxies, settings: settings, now: time.Now}
	s.do = func(req *http.Request, proxy string) (*http.Response, error) {
		client, err := httpclient.GetClient(httpclient.Options{ProxyURL: proxy, Timeout: 25 * time.Second, ValidateResolvedIP: true})
		if err != nil {
			return nil, err
		}
		// Never forward OAuth credentials across redirects, even to another public host.
		isolated := *client
		isolated.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		return isolated.Do(req)
	}
	return s
}

func (s *ClaudeResetCreditService) account(ctx context.Context, id int64) (*Account, string, string, error) {
	a, err := s.accounts.GetByID(ctx, id)
	if err != nil {
		return nil, "", "", err
	}
	if a == nil || a.Platform != PlatformAnthropic || a.Type != AccountTypeOAuth {
		return nil, "", "", infraerrors.BadRequest("CLAUDE_RESET_OAUTH_REQUIRED", "Claude OAuth account required")
	}
	profile := false
	for _, scope := range strings.Fields(a.GetCredential("scope")) {
		if scope == "user:profile" {
			profile = true
		}
	}
	if !profile {
		return nil, "", "", infraerrors.BadRequest("CLAUDE_RESET_PROFILE_SCOPE_REQUIRED", "user:profile scope required")
	}
	proxy := ""
	if a.ProxyID != nil {
		p, e := s.proxies.GetByID(ctx, *a.ProxyID)
		if e != nil || p == nil {
			return nil, "", "", infraerrors.ServiceUnavailable("CLAUDE_RESET_PROXY_UNAVAILABLE", "account proxy unavailable")
		}
		proxy = p.URL()
	}
	token, err := s.tokens.GetAccessToken(ctx, a)
	if err != nil || strings.TrimSpace(token) == "" {
		return nil, "", "", infraerrors.ServiceUnavailable("CLAUDE_RESET_TOKEN_UNAVAILABLE", "OAuth token unavailable")
	}
	return a, token, proxy, nil
}

func (s *ClaudeResetCreditService) headers(ctx context.Context, req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")
	req.Header.Set("x-app", "cli")
	req.Header.Set("User-Agent", "claude-cli/"+s.settings.GetClaudeCodeClientVersion(ctx)+" (external, cli)")
}

func (s *ClaudeResetCreditService) query(ctx context.Context, id int64) (*ClaudeResetCredits, *claudeResetBlock, error) {
	_, token, proxy, err := s.account(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return s.queryWithToken(ctx, token, proxy)
}

func (s *ClaudeResetCreditService) queryWithToken(ctx context.Context, token, proxy string) (*ClaudeResetCredits, *claudeResetBlock, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, claudeResetUsageURL, nil)
	if err != nil {
		return nil, nil, err
	}
	s.headers(ctx, req, token)
	resp, err := s.do(req, proxy)
	if err != nil {
		return nil, nil, infraerrors.ServiceUnavailable("CLAUDE_RESET_QUERY_FAILED", "reset status request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, infraerrors.New(http.StatusBadGateway, "CLAUDE_RESET_QUERY_FAILED", fmt.Sprintf("reset status upstream HTTP %d", resp.StatusCode))
	}
	var envelope map[string]json.RawMessage
	if err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); err != nil || envelope == nil {
		return nil, nil, infraerrors.New(http.StatusBadGateway, "CLAUDE_RESET_STATUS_INVALID", "invalid reset status")
	}
	if _, ok := envelope["error"]; ok {
		return nil, nil, infraerrors.New(http.StatusBadGateway, "CLAUDE_RESET_STATUS_INVALID", "invalid reset status")
	}
	var block *claudeResetBlock
	raw, present := envelope["cedar_ember"]
	if present && string(raw) != "null" {
		if err = json.Unmarshal(raw, &block); err != nil || block == nil || block.Grants == nil {
			return nil, nil, infraerrors.New(http.StatusBadGateway, "CLAUDE_RESET_STATUS_INVALID", "invalid reset grants")
		}
	}
	result := projectClaudeResetCredits(block, s.now())
	return result, block, nil
}

func (s *ClaudeResetCreditService) Query(ctx context.Context, id int64) (*ClaudeResetCredits, error) {
	r, _, e := s.query(ctx, id)
	return r, e
}

func claudeGrantSelection(g claudeResetGrant) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%v:%v", g.ID, g.ResetsLeft, g.StartsAt, g.EndsAt)))
	return hex.EncodeToString(h[:])
}

func projectClaudeResetCredits(b *claudeResetBlock, now time.Time) *ClaudeResetCredits {
	r := &ClaudeResetCredits{Credits: []ClaudeResetCredit{}, FetchedAt: now.UTC()}
	if b == nil {
		return r
	}
	r.Eligible = b.Eligible
	r.CooldownUntil = b.CooldownUntil
	r.WeeklyResetsAt = b.WeeklyResetsAt
	for _, g := range b.Grants {
		if !claudeResetGrantIDPattern.MatchString(g.ID) || len(g.Clears) == 0 || g.ResetsLeft <= 0 || g.Paused || (g.StartsAt != nil && now.Before(*g.StartsAt)) || (g.EndsAt != nil && !now.Before(*g.EndsAt)) {
			continue
		}
		requires := g.UseRequiresLimit == nil || *g.UseRequiresLimit
		usable := b.Eligible && g.UsableNow && g.ID == b.NextGrantID && (!requires || b.AtLimit) && len(g.Blocking) == 0 && (b.CooldownUntil == nil || !now.Before(*b.CooldownUntil))
		used := map[string]float64{}
		for k, v := range g.PercentUsed {
			if v >= 0 && v <= 100 {
				used[k] = v
			}
		}
		r.Credits = append(r.Credits, ClaudeResetCredit{SelectionToken: claudeGrantSelection(g), Label: g.Label, ResetsLeft: g.ResetsLeft, StartsAt: g.StartsAt, ExpiresAt: g.EndsAt, Clears: g.Clears, PercentUsed: used, Blocking: g.Blocking, UseRequiresLimit: requires, Redeemable: usable})
		if usable {
			r.AvailableCount += g.ResetsLeft
		}
	}
	return r
}
