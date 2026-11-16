package service

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const groupQuotaSnapshotTTL = 3 * time.Minute

var ErrGroupQuotaPolicyConflict = infraerrors.Conflict("GROUP_QUOTA_POLICY_CONFLICT", "quota policy changed; reload before saving")

type GroupQuotaPolicyConfig struct {
	Rules []GroupQuotaPolicyRule `json:"rules"`
}

type GroupQuotaPolicyRule struct {
	ID           string                      `json:"id"`
	Name         string                      `json:"name"`
	Priority     int                         `json:"priority"`
	Mode         string                      `json:"mode"`
	AccountIDs   []int64                     `json:"account_ids"`
	AccountMatch string                      `json:"account_match"`
	WindowMatch  string                      `json:"window_match"`
	Conditions   []GroupQuotaPolicyCondition `json:"conditions"`
	Recovery     string                      `json:"recovery"`
	Mappings     []GroupQuotaPolicyMapping   `json:"mappings"`
}

type GroupQuotaPolicyCondition struct {
	Window           string  `json:"window"`
	TriggerRemaining float64 `json:"trigger_remaining"`
	RecoverRemaining float64 `json:"recover_remaining"`
}

type GroupQuotaPolicyMapping struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

type GroupQuotaPolicyWindow struct {
	RemainingPercent float64 `json:"remaining_percent"`
	ResetAt          int64   `json:"reset_at"`
}

type GroupQuotaPolicySnapshot struct {
	AccountID int64                   `json:"account_id"`
	SampledAt int64                   `json:"sampled_at"`
	CheckedAt int64                   `json:"checked_at"`
	ErrorCode string                  `json:"error_code,omitempty"`
	FiveHour  *GroupQuotaPolicyWindow `json:"five_hour,omitempty"`
	SevenDay  *GroupQuotaPolicyWindow `json:"seven_day,omitempty"`
}

type GroupQuotaPolicyRuleState struct {
	RuleID     string `json:"rule_id"`
	Active     bool   `json:"active"`
	Suppressed bool   `json:"suppressed"`
	Status     string `json:"status"`
	ChangedAt  int64  `json:"changed_at"`
	CheckedAt  int64  `json:"checked_at"`
}

type GroupQuotaPolicyEvent struct {
	At      int64  `json:"at"`
	RuleID  string `json:"rule_id"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

type GroupQuotaPolicyRuntime struct {
	Snapshots []GroupQuotaPolicySnapshot  `json:"snapshots"`
	Rules     []GroupQuotaPolicyRuleState `json:"rules"`
	Events    []GroupQuotaPolicyEvent     `json:"events"`
}

type GroupQuotaPolicyAccount struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type GroupQuotaPolicyView struct {
	GroupID  int64                     `json:"group_id"`
	Revision int64                     `json:"revision"`
	Config   GroupQuotaPolicyConfig    `json:"config"`
	Runtime  GroupQuotaPolicyRuntime   `json:"runtime"`
	Accounts []GroupQuotaPolicyAccount `json:"accounts"`
}

type GroupQuotaPolicyResolution struct {
	Applied     bool   `json:"applied"`
	SourceModel string `json:"source_model"`
	TargetModel string `json:"target_model"`
	RuleID      string `json:"rule_id,omitempty"`
	RuleName    string `json:"rule_name,omitempty"`
	Revision    int64  `json:"revision"`
}

type GroupQuotaPolicyPreview struct {
	Runtime    GroupQuotaPolicyRuntime    `json:"runtime"`
	Resolution GroupQuotaPolicyResolution `json:"resolution"`
}

// RuntimeVersion is separate from the admin-visible config revision: quota
// samples and manual releases must not overwrite concurrent saves or each other.
type GroupQuotaPolicyRecord struct {
	GroupID        int64
	Revision       int64
	RuntimeVersion int64
	Config         GroupQuotaPolicyConfig
	Runtime        GroupQuotaPolicyRuntime
}

type GroupQuotaPolicyRepository interface {
	Get(ctx context.Context, groupID int64) (*GroupQuotaPolicyRecord, error)
	List(ctx context.Context) ([]*GroupQuotaPolicyRecord, error)
	SaveConfig(ctx context.Context, record *GroupQuotaPolicyRecord, revision, runtimeVersion int64) (bool, error)
	SaveRuntime(ctx context.Context, record *GroupQuotaPolicyRecord, revision, runtimeVersion int64) (bool, error)
}

func EmptyGroupQuotaPolicyRecord(groupID int64) *GroupQuotaPolicyRecord {
	return &GroupQuotaPolicyRecord{GroupID: groupID,
		Config:  GroupQuotaPolicyConfig{Rules: []GroupQuotaPolicyRule{}},
		Runtime: GroupQuotaPolicyRuntime{Snapshots: []GroupQuotaPolicySnapshot{}, Rules: []GroupQuotaPolicyRuleState{}, Events: []GroupQuotaPolicyEvent{}},
	}
}

func validateGroupQuotaPolicy(config GroupQuotaPolicyConfig, eligible map[int64]bool) error {
	bad := func(message string) error { return infraerrors.BadRequest("GROUP_QUOTA_POLICY_INVALID", message) }
	if len(config.Rules) > 20 {
		return bad("at most 20 quota rules are allowed")
	}
	ids := make(map[string]bool)
	for _, rule := range config.Rules {
		if strings.TrimSpace(rule.ID) == "" || len(rule.ID) > 80 || ids[rule.ID] {
			return bad("rule IDs must be unique and at most 80 bytes")
		}
		ids[rule.ID] = true
		if strings.TrimSpace(rule.Name) == "" || len([]rune(rule.Name)) > 80 || rule.Priority < -10000 || rule.Priority > 10000 {
			return bad("rule name or priority is invalid")
		}
		if rule.Mode != "disabled" && rule.Mode != "observe" && rule.Mode != "enforce" {
			return bad("mode must be disabled, observe or enforce")
		}
		if (rule.AccountMatch != "any" && rule.AccountMatch != "all") || (rule.WindowMatch != "any" && rule.WindowMatch != "all") {
			return bad("account_match and window_match must be any or all")
		}
		if rule.Recovery != "auto" && rule.Recovery != "manual" {
			return bad("recovery must be auto or manual")
		}
		if len(rule.AccountIDs) == 0 || len(rule.AccountIDs) > 50 {
			return bad("select between 1 and 50 monitored accounts per rule")
		}
		accounts := make(map[int64]bool)
		for _, id := range rule.AccountIDs {
			if id <= 0 || accounts[id] || (rule.Mode != "disabled" && !eligible[id]) {
				return bad(fmt.Sprintf("account %d must be a unique ordinary OpenAI OAuth account in this group", id))
			}
			accounts[id] = true
		}
		if len(rule.Conditions) < 1 || len(rule.Conditions) > 2 {
			return bad("select the 5h and/or 7d quota window")
		}
		windows := make(map[string]bool)
		for _, c := range rule.Conditions {
			if (c.Window != "5h" && c.Window != "7d") || windows[c.Window] {
				return bad("quota windows must be unique: 5h or 7d")
			}
			windows[c.Window] = true
			if math.IsNaN(c.TriggerRemaining) || math.IsNaN(c.RecoverRemaining) || c.TriggerRemaining < 0 || c.RecoverRemaining > 100 || c.TriggerRemaining >= c.RecoverRemaining {
				return bad("thresholds must satisfy 0 <= trigger < recovery <= 100")
			}
		}
		if len(rule.Mappings) == 0 || len(rule.Mappings) > 50 {
			return bad("specify between 1 and 50 exact model mappings per rule")
		}
		models := make(map[string]bool)
		for _, m := range rule.Mappings {
			validModel := func(v string) bool {
				return v != "" && len(v) <= 200 && !strings.Contains(v, "*") && strings.IndexFunc(v, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) < 0
			}
			if !validModel(m.Source) || !validModel(m.Target) || m.Source == m.Target || models[m.Source] {
				return bad("model mappings must have unique exact source names and distinct nonempty targets")
			}
			models[m.Source] = true
		}
	}
	return nil
}

func groupQuotaEligible(account *Account) bool {
	return account != nil && account.IsOpenAIOAuth() && !account.IsShadow() && account.QuotaDimensionOrDefault() == QuotaDimensionGlobal && !account.IsOpenAIPersonalAccessToken() && !account.IsOpenAIAgentIdentity()
}

// Normalize by duration: upstream primary/secondary ordering is not a contract.
// Missing or invalid windows remain unknown; never fabricate zero remaining.
func groupQuotaSnapshot(accountID int64, usage *OpenAIQuotaUsage, now time.Time) GroupQuotaPolicySnapshot {
	snapshot := GroupQuotaPolicySnapshot{AccountID: accountID, CheckedAt: now.Unix()}
	if usage == nil || usage.RateLimit == nil || usage.FetchedAt <= 0 || usage.FetchedAt > now.Add(time.Minute).Unix() {
		snapshot.ErrorCode = "QUOTA_WINDOWS_UNAVAILABLE"
		return snapshot
	}
	snapshot.SampledAt = usage.FetchedAt
	for _, w := range []*OpenAIRateLimitWindow{usage.RateLimit.PrimaryWindow, usage.RateLimit.SecondaryWindow} {
		if w == nil || math.IsNaN(w.UsedPercent) || math.IsInf(w.UsedPercent, 0) || w.UsedPercent < 0 || w.UsedPercent > 100 {
			continue
		}
		window := &GroupQuotaPolicyWindow{RemainingPercent: 100 - w.UsedPercent, ResetAt: w.ResetAt}
		if window.ResetAt <= 0 && w.ResetAfterSeconds > 0 {
			window.ResetAt = usage.FetchedAt + w.ResetAfterSeconds
		}
		switch w.LimitWindowSeconds {
		case 5 * 60 * 60:
			if snapshot.FiveHour == nil {
				snapshot.FiveHour = window
			}
		case 7 * 24 * 60 * 60:
			if snapshot.SevenDay == nil {
				snapshot.SevenDay = window
			}
		}
	}
	if snapshot.FiveHour == nil && snapshot.SevenDay == nil {
		snapshot.ErrorCode = "QUOTA_WINDOWS_UNAVAILABLE"
	}
	return snapshot
}

func groupQuotaAppendEvent(runtime *GroupQuotaPolicyRuntime, at int64, ruleID, kind, message string) {
	runtime.Events = append(runtime.Events, GroupQuotaPolicyEvent{At: at, RuleID: ruleID, Type: kind, Message: message})
	if len(runtime.Events) > 50 {
		runtime.Events = runtime.Events[len(runtime.Events)-50:]
	}
}

// Unknown/stale data freezes prior state. Recovery deliberately requires every
// selected account/window to cross its higher threshold, preventing flapping.
func evaluateGroupQuotaPolicy(config GroupQuotaPolicyConfig, previous GroupQuotaPolicyRuntime, now time.Time) GroupQuotaPolicyRuntime {
	runtime := GroupQuotaPolicyRuntime{Snapshots: append([]GroupQuotaPolicySnapshot{}, previous.Snapshots...), Rules: []GroupQuotaPolicyRuleState{}, Events: append([]GroupQuotaPolicyEvent{}, previous.Events...)}
	snapshots := make(map[int64]GroupQuotaPolicySnapshot)
	states := make(map[string]GroupQuotaPolicyRuleState)
	for _, snapshot := range previous.Snapshots {
		snapshots[snapshot.AccountID] = snapshot
	}
	for _, state := range previous.Rules {
		states[state.RuleID] = state
	}
	for _, rule := range config.Rules {
		state := states[rule.ID]
		state.RuleID, state.CheckedAt = rule.ID, now.Unix()
		wasActive, wasSuppressed := state.Active, state.Suppressed
		invalidated := false
		if rule.Mode == "disabled" {
			state.Active, state.Suppressed, state.Status = false, false, "disabled"
		} else {
			known, trigger, recovered, invalidAccount := true, rule.AccountMatch == "all", true, false
			for _, id := range rule.AccountIDs {
				snapshot, ok := snapshots[id]
				if snapshot.ErrorCode == "ACCOUNT_UNAVAILABLE" {
					invalidAccount = true
				}
				if !ok || snapshot.ErrorCode != "" || snapshot.SampledAt <= 0 || now.Unix()-snapshot.SampledAt > int64(groupQuotaSnapshotTTL.Seconds()) || snapshot.SampledAt > now.Add(time.Minute).Unix() {
					known = false
					continue
				}
				accountTrigger := rule.WindowMatch == "all"
				for _, condition := range rule.Conditions {
					window := snapshot.FiveHour
					if condition.Window == "7d" {
						window = snapshot.SevenDay
					}
					if window == nil || (window.ResetAt > 0 && window.ResetAt <= now.Unix()) {
						known = false
						continue
					}
					low := window.RemainingPercent <= condition.TriggerRemaining
					if rule.WindowMatch == "all" {
						accountTrigger = accountTrigger && low
					} else {
						accountTrigger = accountTrigger || low
					}
					recovered = recovered && window.RemainingPercent >= condition.RecoverRemaining
				}
				if rule.AccountMatch == "all" {
					trigger = trigger && accountTrigger
				} else {
					trigger = trigger || accountTrigger
				}
			}
			if known {
				if state.Suppressed && recovered {
					state.Suppressed = false
				}
				if state.Active && rule.Recovery == "auto" && recovered {
					state.Active = false
				}
				if !state.Active && !state.Suppressed && trigger {
					state.Active = true
				}
				state.Status = "normal"
				if state.Suppressed {
					state.Status = "suppressed"
				} else if state.Active {
					state.Status = "active"
					if rule.Mode == "observe" {
						state.Status = "observing"
					}
				}
			} else {
				state.Status = "unknown"
			}
			// Explicit removal/type changes are different from uncertain upstream
			// data: a rule may no longer act on accounts outside its group.
			if invalidAccount {
				state.Active, state.Suppressed, state.Status, invalidated = false, false, "unknown", true
			}
		}
		if state.Active != wasActive || state.Suppressed != wasSuppressed {
			state.ChangedAt = now.Unix()
			kind, message := "recovered", "Quota recovered; mapping released"
			if state.Active {
				kind, message = "triggered", "Quota threshold reached"
			} else if wasSuppressed {
				kind, message = "rearmed", "Quota recovered; rule rearmed"
			}
			if invalidated {
				kind, message = "account_unavailable", "Monitored account left the group or is no longer eligible; mapping released"
			}
			groupQuotaAppendEvent(&runtime, now.Unix(), rule.ID, kind, message)
		}
		runtime.Rules = append(runtime.Rules, state)
	}
	return runtime
}

// Preserve state only for unchanged rules; a deliberate config change starts
// that rule from fresh evidence without carrying an unrelated active decision.
func groupQuotaRuntimeForConfig(old *GroupQuotaPolicyRecord, config GroupQuotaPolicyConfig) GroupQuotaPolicyRuntime {
	runtime := old.Runtime
	runtime.Rules = []GroupQuotaPolicyRuleState{}
	unchanged := make(map[string]bool)
	for _, rule := range config.Rules {
		for _, prior := range old.Config.Rules {
			if rule.ID == prior.ID && reflect.DeepEqual(rule, prior) {
				unchanged[rule.ID] = true
			}
		}
	}
	for _, state := range old.Runtime.Rules {
		if unchanged[state.RuleID] {
			runtime.Rules = append(runtime.Rules, state)
		}
	}
	return runtime
}

func resolveGroupQuotaPolicy(config GroupQuotaPolicyConfig, runtime GroupQuotaPolicyRuntime, revision int64, model string) GroupQuotaPolicyResolution {
	result := GroupQuotaPolicyResolution{SourceModel: model, TargetModel: model, Revision: revision}
	active := make(map[string]bool)
	for _, state := range runtime.Rules {
		active[state.RuleID] = state.Active && !state.Suppressed
	}
	rules := append([]GroupQuotaPolicyRule(nil), config.Rules...)
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority > rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
	for _, rule := range rules {
		if rule.Mode != "enforce" || !active[rule.ID] {
			continue
		}
		for _, mapping := range rule.Mappings {
			if mapping.Source == model {
				result.Applied, result.TargetModel, result.RuleID, result.RuleName = true, mapping.Target, rule.ID, rule.Name
				return result
			}
		}
	}
	return result
}
