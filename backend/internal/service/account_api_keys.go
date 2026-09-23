package service

import (
	"strconv"
	"strings"
	"sync"
)

const (
	apiKeyStrategyRoundRobin = "round_robin"
	apiKeyStrategyWeighted   = "weighted"
	apiKeySlotPrimaryID      = "primary"
)

// apiKeyEntry 是一次调度可选的上游 Key。
type apiKeyEntry struct {
	id      string
	key     string
	weight  int
	enabled bool
}

type apiKeyPoolState struct {
	mu          sync.Mutex
	fingerprint string
	rr          uint64
	current     []int
}

var apiKeyPools sync.Map

// pinAccountAPIKey 在一次请求内固定选中的上游 Key。
// 账号对象可能来自调度缓存，因此多 Key 时返回副本，避免并发请求互相覆盖。
// 同一请求里重复调用不会再次轮换。
func pinAccountAPIKey(account *Account) *Account {
	if account == nil || account.apiKeyPinned || account.Type != AccountTypeAPIKey {
		return account
	}
	selected, ok := nextAccountAPIKey(account)
	if !ok || selected == "" {
		return account
	}
	cloned := *account
	creds := make(map[string]any, len(account.Credentials)+1)
	for k, v := range account.Credentials {
		creds[k] = v
	}
	creds["api_key"] = selected
	cloned.Credentials = creds
	cloned.apiKeyPinned = true
	return &cloned
}

func nextAccountAPIKey(account *Account) (string, bool) {
	if account == nil {
		return "", false
	}
	entries := enabledAPIKeyEntries(account.Credentials)
	if len(entries) < 2 {
		return "", false
	}
	strategy := apiKeyStrategyRoundRobin
	if strings.TrimSpace(account.GetCredential("api_key_strategy")) == apiKeyStrategyWeighted {
		strategy = apiKeyStrategyWeighted
	}
	state := loadAPIKeyPoolState(account.ID)
	state.mu.Lock()
	defer state.mu.Unlock()
	fp := apiKeyPoolFingerprint(strategy, entries)
	if state.fingerprint != fp {
		state.fingerprint = fp
		state.rr = 0
		state.current = nil
	}
	if strategy == apiKeyStrategyWeighted {
		return pickWeightedAPIKey(state, entries), true
	}
	idx := int(state.rr % uint64(len(entries)))
	state.rr++
	return entries[idx].key, true
}

func loadAPIKeyPoolState(accountID int64) *apiKeyPoolState {
	if v, ok := apiKeyPools.Load(accountID); ok {
		if state, ok := v.(*apiKeyPoolState); ok {
			return state
		}
	}
	state := &apiKeyPoolState{}
	actual, _ := apiKeyPools.LoadOrStore(accountID, state)
	if stored, ok := actual.(*apiKeyPoolState); ok {
		return stored
	}
	return state
}

func pickWeightedAPIKey(state *apiKeyPoolState, entries []apiKeyEntry) string {
	if len(state.current) != len(entries) {
		state.current = make([]int, len(entries))
	}
	total := 0
	best := 0
	for i, entry := range entries {
		weight := entry.weight
		if weight < 1 {
			weight = 1
		}
		total += weight
		state.current[i] += weight
		if state.current[i] > state.current[best] {
			best = i
		}
	}
	if total < 1 {
		total = 1
	}
	state.current[best] -= total
	return entries[best].key
}

func apiKeyPoolFingerprint(strategy string, entries []apiKeyEntry) string {
	var b strings.Builder
	b.WriteString(strategy)
	for _, entry := range entries {
		b.WriteByte('|')
		b.WriteString(entry.id)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(entry.weight))
		b.WriteByte(':')
		b.WriteString(entry.key)
	}
	return b.String()
}

func enabledAPIKeyEntries(creds map[string]any) []apiKeyEntry {
	if creds == nil {
		return nil
	}
	secrets := parseAPIKeySecrets(creds["api_keys"])
	if len(secrets) == 0 {
		return nil
	}
	slots := parseAPIKeySlots(creds["api_key_slots"])
	if len(slots) == 0 {
		entries := make([]apiKeyEntry, 0, len(secrets))
		for id, key := range secrets {
			entries = append(entries, apiKeyEntry{id: id, key: key, weight: 1, enabled: true})
		}
		return entries
	}
	entries := make([]apiKeyEntry, 0, len(slots))
	for _, slot := range slots {
		if !slot.enabled {
			continue
		}
		key := strings.TrimSpace(secrets[slot.id])
		if key == "" {
			continue
		}
		weight := slot.weight
		if weight < 1 {
			weight = 1
		}
		entries = append(entries, apiKeyEntry{id: slot.id, key: key, weight: weight, enabled: true})
	}
	return entries
}

type apiKeySlot struct {
	id      string
	label   string
	weight  int
	enabled bool
}

func parseAPIKeySlots(raw any) []apiKeySlot {
	items := apiKeyAsObjectSlice(raw)
	slots := make([]apiKeySlot, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		id := strings.TrimSpace(apiKeyAsString(item["id"]))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		weight := apiKeyAsPositiveInt(item["weight"], 1)
		if weight > 10000 {
			weight = 10000
		}
		enabled := true
		if v, ok := item["enabled"].(bool); ok {
			enabled = v
		}
		slots = append(slots, apiKeySlot{
			id:      id,
			label:   strings.TrimSpace(apiKeyAsString(item["label"])),
			weight:  weight,
			enabled: enabled,
		})
	}
	return slots
}

func parseAPIKeySecrets(raw any) map[string]string {
	out := map[string]string{}
	for _, item := range apiKeyAsObjectSlice(raw) {
		id := strings.TrimSpace(apiKeyAsString(item["id"]))
		key := strings.TrimSpace(apiKeyAsString(item["key"]))
		if id == "" || key == "" {
			continue
		}
		out[id] = key
	}
	return out
}

func apiKeyAsObjectSlice(raw any) []map[string]any {
	switch items := raw.(type) {
	case []map[string]any:
		return items
	case []any:
		out := make([]map[string]any, 0, len(items))
		for _, item := range items {
			switch obj := item.(type) {
			case map[string]any:
				out = append(out, obj)
			}
		}
		return out
	default:
		return nil
	}
}

func apiKeyAsString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}

func apiKeyAsPositiveInt(v any, fallback int) int {
	switch val := v.(type) {
	case int:
		if val > 0 {
			return val
		}
	case int64:
		if val > 0 {
			return int(val)
		}
	case float64:
		if val > 0 {
			return int(val)
		}
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(val))
		if err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// reconcileAPIKeyPool 把前端提交的槽位和密钥对齐。
// 槽位是非敏感配置，密钥在 api_keys 里；编辑时未提交的密钥沿用已保存的值。
// 少于两把启用密钥时清掉池配置，退回单 Key。
func reconcileAPIKeyPool(existing, out map[string]any) {
	if out == nil {
		return
	}
	raw, hasSlots := out["api_key_slots"]
	slots := parseAPIKeySlots(raw)
	existingHadSlots := existing != nil && existing["api_key_slots"] != nil
	if !hasSlots || len(slots) < 2 {
		if hasSlots || existingHadSlots {
			delete(out, "api_key_slots")
			delete(out, "api_key_strategy")
			delete(out, "api_keys")
		}
		return
	}
	strategy := strings.TrimSpace(apiKeyAsString(out["api_key_strategy"]))
	if strategy != apiKeyStrategyWeighted {
		strategy = apiKeyStrategyRoundRobin
	}
	out["api_key_strategy"] = strategy

	existingSecrets := map[string]string{}
	if existing != nil {
		existingSecrets = parseAPIKeySecrets(existing["api_keys"])
	}
	incomingSecrets := parseAPIKeySecrets(out["api_keys"])
	if key := strings.TrimSpace(apiKeyAsString(out["api_key"])); key != "" {
		incomingSecrets[apiKeySlotPrimaryID] = key
	}

	publicSlots := make([]map[string]any, 0, len(slots))
	secrets := make([]map[string]any, 0, len(slots))
	var firstEnabled string
	for _, slot := range slots {
		key := incomingSecrets[slot.id]
		if key == "" {
			key = existingSecrets[slot.id]
		}
		if key == "" {
			continue
		}
		publicSlots = append(publicSlots, map[string]any{
			"id":      slot.id,
			"label":   slot.label,
			"weight":  slot.weight,
			"enabled": slot.enabled,
		})
		secrets = append(secrets, map[string]any{"id": slot.id, "key": key})
		if slot.enabled && firstEnabled == "" {
			firstEnabled = key
		}
	}
	if len(publicSlots) < 2 {
		delete(out, "api_key_slots")
		delete(out, "api_key_strategy")
		delete(out, "api_keys")
		return
	}
	out["api_key_slots"] = publicSlots
	out["api_keys"] = secrets
	if firstEnabled != "" {
		out["api_key"] = firstEnabled
	}
}
