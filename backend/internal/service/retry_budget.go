package service

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

func WithQuotaStickyPreference(ctx context.Context, apiKey *APIKey) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	enabled, _ := apiKey.EffectiveQuotaSticky()
	if !enabled {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.QuotaStickyPreferred, true)
}

func quotaStickyPreferred(ctx context.Context) bool {
	enabled, _ := ctx.Value(ctxkey.QuotaStickyPreferred).(bool)
	return enabled
}

const clientRetryHeader = "X-Sub2API-Client-Retry"
const retryBudgetGinKey = "sub2api_retry_budget"

type RetryBudget struct {
	MaxAttempts        int
	MaxAccountSwitches int
	Deadline           time.Time
	ClientRetryCapable bool
	ClientRuleID       string
	attempts           atomic.Int32
	switches           atomic.Int32
}

type ClientRetryRule struct {
	ID                  string   `json:"id"`
	ClientFamily        string   `json:"client_family,omitempty"`
	Originators         []string `json:"originator"`
	UAContainsAll       []string `json:"ua_contains_all"`
	MinVersion          string   `json:"min_version,omitempty"`
	MaxVersion          string   `json:"max_version,omitempty"`
	RetryCapable        bool     `json:"retry_capable"`
	GatewayAttemptLimit int      `json:"gateway_attempt_limit"`
	Disabled            bool     `json:"disabled,omitempty"`
}

var embeddedClientRetryRules = []ClientRetryRule{
	{ID: "codex-cli", Originators: []string{"codex_cli_rs", "codex-tui", "codex_exec"}, UAContainsAll: []string{"codex"}, RetryCapable: true, GatewayAttemptLimit: 2},
	{ID: "codex-app", Originators: []string{"codex_app", "codex_chatgpt_desktop", "codex_atlas"}, UAContainsAll: []string{"codex"}, RetryCapable: true, GatewayAttemptLimit: 2},
	{ID: "codex-vscode", Originators: []string{"codex_vscode", "codex_vscode_copilot"}, UAContainsAll: []string{"codex"}, RetryCapable: true, GatewayAttemptLimit: 2},
	{ID: "codex-sdk-ts", Originators: []string{"codex_sdk_ts"}, UAContainsAll: []string{"codex"}, RetryCapable: true, GatewayAttemptLimit: 2},
}

var activeClientRetryRules atomic.Value

func init() {
	activeClientRetryRules.Store(append([]ClientRetryRule(nil), embeddedClientRetryRules...))
}

func currentClientRetryRules() []ClientRetryRule {
	rules, _ := activeClientRetryRules.Load().([]ClientRetryRule)
	return rules
}

func matchClientRetryRule(userAgent, originator string, rules []ClientRetryRule) (ClientRetryRule, bool) {
	ua, origin := strings.ToLower(strings.TrimSpace(userAgent)), strings.ToLower(strings.TrimSpace(originator))
	for _, rule := range rules {
		if rule.Disabled {
			continue
		}
		originMatched := false
		for _, candidate := range rule.Originators {
			if origin == strings.ToLower(strings.TrimSpace(candidate)) {
				originMatched = true
				break
			}
		}
		if !originMatched || len(rule.UAContainsAll) == 0 {
			continue
		}
		matched := true
		for _, marker := range rule.UAContainsAll {
			marker = strings.ToLower(strings.TrimSpace(marker))
			if marker == "" || !strings.Contains(ua, marker) {
				matched = false
				break
			}
		}
		if matched {
			if rule.MinVersion != "" || rule.MaxVersion != "" {
				version, ok := openai.ParseCodexEngineVersion(userAgent)
				if !ok || (rule.MinVersion != "" && CompareVersions(version, rule.MinVersion) < 0) || (rule.MaxVersion != "" && CompareVersions(version, rule.MaxVersion) > 0) {
					continue
				}
			}
			return rule, true
		}
	}
	return ClientRetryRule{}, false
}

func GetOrCreateRetryBudget(c *gin.Context) *RetryBudget {
	if c == nil {
		return nil
	}
	if existing, ok := c.Get(retryBudgetGinKey); ok {
		budget, _ := existing.(*RetryBudget)
		return budget
	}
	apiKey := getAPIKeyFromContext(c)
	if apiKey == nil || apiKey.Group == nil || !apiKey.Group.UnifiedRetryBudgetEnabled {
		c.Request.Header.Del(clientRetryHeader)
		return nil
	}

	capable := strings.TrimSpace(c.GetHeader(clientRetryHeader)) == "1"
	ruleID := "explicit_header"
	limit := 2
	if !capable {
		ruleID = "unknown"
		limit = 3
		if rule, ok := matchClientRetryRule(c.GetHeader("User-Agent"), c.GetHeader("originator"), currentClientRetryRules()); ok && rule.RetryCapable {
			capable, ruleID = true, rule.ID
			if rule.GatewayAttemptLimit > 0 {
				limit = rule.GatewayAttemptLimit
			}
		}
	}
	c.Request.Header.Del(clientRetryHeader)
	duration := 5 * time.Second
	if capable {
		duration = 3 * time.Second
	}
	budget := &RetryBudget{MaxAttempts: limit, MaxAccountSwitches: 1, Deadline: time.Now().Add(duration), ClientRetryCapable: capable, ClientRuleID: ruleID}
	c.Set(retryBudgetGinKey, budget)
	return budget
}

func EffectiveMaxAccountSwitches(c *gin.Context, configured int) int {
	budget := GetOrCreateRetryBudget(c)
	if budget == nil || configured <= budget.MaxAccountSwitches {
		return configured
	}
	return budget.MaxAccountSwitches
}

func (b *RetryBudget) TryAttempt() bool {
	if b == nil || time.Now().After(b.Deadline) {
		return false
	}
	return int(b.attempts.Add(1)) <= b.MaxAttempts
}

func (b *RetryBudget) TryAccountSwitch() bool {
	if b == nil || time.Now().After(b.Deadline) {
		return false
	}
	return int(b.switches.Add(1)) <= b.MaxAccountSwitches
}

func (b *RetryBudget) Remaining() time.Duration {
	if b == nil {
		return 0
	}
	remaining := time.Until(b.Deadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}
