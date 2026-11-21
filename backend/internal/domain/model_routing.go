package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const DefaultModelRoutingAccountPriority = 50

// ModelRoutingMap is groups.model_routing JSONB: pattern -> rule.
// Values accept legacy []accountID or the structured rule object.
type ModelRoutingMap map[string]ModelRoutingRule

// ModelRoutingRule is a single model-routing preference pool.
type ModelRoutingRule struct {
	MaxConcurrency int                      `json:"max_concurrency"`
	Accounts       []ModelRoutingAccountRef `json:"accounts"`
}

// ModelRoutingAccountRef is a preferred account inside a routing rule.
type ModelRoutingAccountRef struct {
	ID          int64 `json:"id"`
	Priority    int   `json:"priority"`
	Concurrency int   `json:"concurrency"`
}

// MatchedModelRoutingRule is a rule plus the pattern key that matched a model.
type MatchedModelRoutingRule struct {
	Pattern string
	Rule    ModelRoutingRule
}

// LegacyModelRouting converts the historical pattern→[]accountID shape into the
// structured map used by the rest of the stack (tests and read-compat helpers).
func LegacyModelRouting(in map[string][]int64) ModelRoutingMap {
	if len(in) == 0 {
		return ModelRoutingMap{}
	}
	out := make(ModelRoutingMap, len(in))
	for pattern, ids := range in {
		if pattern == "" || len(ids) == 0 {
			continue
		}
		rule := ModelRoutingRule{Accounts: make([]ModelRoutingAccountRef, 0, len(ids))}
		for _, id := range ids {
			if id <= 0 {
				continue
			}
			rule.Accounts = append(rule.Accounts, ModelRoutingAccountRef{
				ID:          id,
				Priority:    DefaultModelRoutingAccountPriority,
				Concurrency: 0,
			})
		}
		if len(rule.Accounts) > 0 {
			out[pattern] = rule
		}
	}
	return out
}

// UnmarshalJSON accepts either a legacy int64 array or the structured object.
func (r *ModelRoutingRule) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*r = ModelRoutingRule{}
		return nil
	}
	if data[0] == '[' {
		var ids []int64
		if err := json.Unmarshal(data, &ids); err != nil {
			return fmt.Errorf("model routing legacy account list: %w", err)
		}
		r.MaxConcurrency = 0
		r.Accounts = make([]ModelRoutingAccountRef, 0, len(ids))
		for _, id := range ids {
			if id <= 0 {
				continue
			}
			r.Accounts = append(r.Accounts, ModelRoutingAccountRef{
				ID:          id,
				Priority:    DefaultModelRoutingAccountPriority,
				Concurrency: 0,
			})
		}
		return nil
	}
	type rawRule struct {
		MaxConcurrency int                      `json:"max_concurrency"`
		Accounts       []ModelRoutingAccountRef `json:"accounts"`
	}
	var raw rawRule
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("model routing rule object: %w", err)
	}
	r.MaxConcurrency = raw.MaxConcurrency
	if r.MaxConcurrency < 0 {
		r.MaxConcurrency = 0
	}
	r.Accounts = make([]ModelRoutingAccountRef, 0, len(raw.Accounts))
	for _, acc := range raw.Accounts {
		if acc.ID <= 0 {
			continue
		}
		// Priority 0 is valid (highest). Omitted JSON priority also lands as 0.
		if acc.Concurrency < 0 {
			acc.Concurrency = 0
		}
		r.Accounts = append(r.Accounts, acc)
	}
	return nil
}

// MarshalJSON always writes the structured object form.
func (r ModelRoutingRule) MarshalJSON() ([]byte, error) {
	type out struct {
		MaxConcurrency int                      `json:"max_concurrency"`
		Accounts       []ModelRoutingAccountRef `json:"accounts"`
	}
	accounts := r.Accounts
	if accounts == nil {
		accounts = []ModelRoutingAccountRef{}
	}
	return json.Marshal(out{
		MaxConcurrency: r.MaxConcurrency,
		Accounts:       accounts,
	})
}

// AccountIDs returns preferred account IDs in declaration order.
func (r ModelRoutingRule) AccountIDs() []int64 {
	if len(r.Accounts) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(r.Accounts))
	for _, acc := range r.Accounts {
		if acc.ID > 0 {
			ids = append(ids, acc.ID)
		}
	}
	return ids
}

// NormalizeModelRoutingMap drops empty patterns and invalid accounts.
func NormalizeModelRoutingMap(in ModelRoutingMap) ModelRoutingMap {
	if len(in) == 0 {
		return ModelRoutingMap{}
	}
	out := make(ModelRoutingMap, len(in))
	for pattern, rule := range in {
		if pattern == "" {
			continue
		}
		normalized := ModelRoutingRule{MaxConcurrency: rule.MaxConcurrency}
		if normalized.MaxConcurrency < 0 {
			normalized.MaxConcurrency = 0
		}
		seen := make(map[int64]struct{}, len(rule.Accounts))
		for _, acc := range rule.Accounts {
			if acc.ID <= 0 {
				continue
			}
			if _, dup := seen[acc.ID]; dup {
				continue
			}
			seen[acc.ID] = struct{}{}
			if acc.Concurrency < 0 {
				acc.Concurrency = 0
			}
			normalized.Accounts = append(normalized.Accounts, acc)
		}
		if len(normalized.Accounts) == 0 {
			continue
		}
		out[pattern] = normalized
	}
	return out
}

// MatchModelRouting finds the rule for requestedModel: exact pattern first, then wildcard suffix *.
func MatchModelRouting(routing ModelRoutingMap, requestedModel string) (MatchedModelRoutingRule, bool) {
	if len(routing) == 0 || requestedModel == "" {
		return MatchedModelRoutingRule{}, false
	}
	if rule, ok := routing[requestedModel]; ok && len(rule.Accounts) > 0 {
		return MatchedModelRoutingRule{Pattern: requestedModel, Rule: rule}, true
	}
	for pattern, rule := range routing {
		if len(rule.Accounts) == 0 {
			continue
		}
		if MatchModelPattern(pattern, requestedModel) {
			return MatchedModelRoutingRule{Pattern: pattern, Rule: rule}, true
		}
	}
	return MatchedModelRoutingRule{}, false
}

// MatchModelPattern reports whether model matches pattern (exact or trailing *).
func MatchModelPattern(pattern, model string) bool {
	if pattern == model {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(model, prefix)
	}
	return false
}
