package service

import (
	"math"
	mathrand "math/rand"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// openAIUpstreamChannelKey groups API-key accounts by their normalized upstream
// endpoint. Subscription accounts remain independent because they do not share
// an upstream API-key channel even when their request host is the same.
func openAIUpstreamChannelKey(account *Account) string {
	if account == nil {
		return "account:nil"
	}
	if account.Type != AccountTypeAPIKey {
		return "account:" + strconv.FormatInt(account.ID, 10)
	}

	var baseURL string
	switch account.Platform {
	case PlatformGrok:
		baseURL = account.GetGrokBaseURL()
	default:
		baseURL = account.GetOpenAIBaseURL()
	}
	return "apikey:" + account.Platform + ":" + normalizeOpenAIUpstreamEndpoint(baseURL)
}

func normalizeOpenAIUpstreamEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if normalized, ok := normalizeCommonHTTPUpstreamEndpoint(raw); ok {
		return normalized
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return strings.ToLower(strings.TrimRight(raw, "/"))
	}

	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	port := parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	host := hostname
	if port != "" {
		host += ":" + port
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	path = strings.TrimSuffix(path, "/v1")
	return scheme + "://" + host + path
}

func normalizeCommonHTTPUpstreamEndpoint(raw string) (string, bool) {
	if strings.ContainsAny(raw, "%\\ \t\r\n") {
		return "", false
	}
	schemeEnd := strings.Index(raw, "://")
	if schemeEnd <= 0 {
		return "", false
	}
	scheme := raw[:schemeEnd]
	switch {
	case strings.EqualFold(scheme, "https"):
		scheme = "https"
	case strings.EqualFold(scheme, "http"):
		scheme = "http"
	default:
		return "", false
	}

	rest := raw[schemeEnd+3:]
	authorityEnd := len(rest)
	if idx := strings.IndexAny(rest, "/?#"); idx >= 0 {
		authorityEnd = idx
	}
	authority := rest[:authorityEnd]
	if authority == "" || strings.ContainsAny(authority, "@[]") {
		return "", false
	}

	host, port := authority, ""
	if colon := strings.LastIndexByte(authority, ':'); colon >= 0 {
		if strings.IndexByte(authority, ':') != colon {
			return "", false
		}
		host, port = authority[:colon], authority[colon+1:]
		if host == "" || port == "" {
			return "", false
		}
		for _, digit := range port {
			if digit < '0' || digit > '9' {
				return "", false
			}
		}
	}
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}

	path := ""
	if authorityEnd < len(rest) && rest[authorityEnd] == '/' {
		path = rest[authorityEnd:]
		if end := strings.IndexAny(path, "?#"); end >= 0 {
			path = path[:end]
		}
		path = strings.TrimRight(path, "/")
		path = strings.TrimSuffix(path, "/v1")
	}

	host = strings.ToLower(host)
	var normalized strings.Builder
	normalized.Grow(len(raw))
	_, _ = normalized.WriteString(scheme)
	_, _ = normalized.WriteString("://")
	_, _ = normalized.WriteString(host)
	if port != "" {
		_ = normalized.WriteByte(':')
		_, _ = normalized.WriteString(port)
	}
	_, _ = normalized.WriteString(path)
	return normalized.String(), true
}

type openAIChannelCandidateGroup struct {
	key        string
	candidates []openAIAccountCandidateScore
}

func openAICandidateChannelKey(candidate openAIAccountCandidateScore) string {
	if candidate.channelKey != "" {
		return candidate.channelKey
	}
	return openAIUpstreamChannelKey(candidate.account)
}

func hasMultipleOpenAIAPIKeyChannels(candidates []openAIAccountCandidateScore) bool {
	firstChannel := ""
	for _, candidate := range candidates {
		if candidate.account == nil || candidate.account.Type != AccountTypeAPIKey {
			continue
		}
		channel := openAICandidateChannelKey(candidate)
		if firstChannel == "" {
			firstChannel = channel
			continue
		}
		if channel != firstChannel {
			return true
		}
	}
	return false
}

func groupOpenAIAccountCandidatesByChannel(candidates []openAIAccountCandidateScore) []openAIChannelCandidateGroup {
	if len(candidates) == 0 {
		return nil
	}
	capacityHint := min(len(candidates), 32)
	groups := make([]openAIChannelCandidateGroup, 0, capacityHint)
	groupIndex := make(map[string]int, capacityHint)
	keys := make([]string, len(candidates))
	counts := make([]int, 0, capacityHint)
	for i, candidate := range candidates {
		key := openAICandidateChannelKey(candidate)
		keys[i] = key
		idx, ok := groupIndex[key]
		if !ok {
			idx = len(groups)
			groupIndex[key] = idx
			groups = append(groups, openAIChannelCandidateGroup{key: key})
			counts = append(counts, 0)
		}
		counts[idx]++
	}
	for i := range groups {
		groups[i].candidates = make([]openAIAccountCandidateScore, 0, counts[i])
	}
	for i, candidate := range candidates {
		idx := groupIndex[keys[i]]
		groups[idx].candidates = append(groups[idx].candidates, candidate)
	}
	return groups
}

// selectTopKOpenAICandidatesByChannel fills top-K in channel rounds. The best
// account from each upstream is considered before a second account from an
// already represented upstream, preventing a large channel from crowding every
// other channel out of the candidate pool.
func selectTopKOpenAICandidatesByChannel(candidates []openAIAccountCandidateScore, topK int) []openAIAccountCandidateScore {
	if len(candidates) == 0 {
		return nil
	}
	if !hasMultipleOpenAIAPIKeyChannels(candidates) {
		return selectTopKOpenAICandidates(candidates, topK)
	}
	if topK <= 0 {
		topK = 1
	}
	if topK > len(candidates) {
		topK = len(candidates)
	}

	groups := groupOpenAIAccountCandidatesByChannel(candidates)
	for i := range groups {
		// Only top-K entries from one channel can contribute to the final top-K.
		// The bounded selector keeps small pools allocation-light and preserves
		// O(n log k) behavior for channels containing large account pools.
		groups[i].candidates = selectTopKOpenAICandidates(groups[i].candidates, topK)
	}

	selected := make([]openAIAccountCandidateScore, 0, topK)
	for round := 0; len(selected) < topK; round++ {
		roundCandidates := make([]openAIAccountCandidateScore, 0, len(groups))
		for _, group := range groups {
			if round < len(group.candidates) {
				roundCandidates = append(roundCandidates, group.candidates[round])
			}
		}
		if len(roundCandidates) == 0 {
			break
		}
		remaining := topK - len(selected)
		roundCandidates = selectTopKOpenAICandidates(roundCandidates, remaining)
		selected = append(selected, roundCandidates...)
	}
	return selected
}

func finiteOpenAICandidateScore(score float64) float64 {
	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}
	return score
}

func openAICandidateAvailableCapacity(candidate openAIAccountCandidateScore) float64 {
	if candidate.account == nil {
		return 1
	}
	capacity := candidate.account.EffectiveLoadFactor()
	current := 0
	if candidate.loadInfo != nil {
		current = candidate.loadInfo.CurrentConcurrency
	}
	available := capacity - current
	if available < 1 {
		available = 1
	}
	return float64(available)
}

func openAICandidateWeights(candidates []openAIAccountCandidateScore) []float64 {
	weights := make([]float64, len(candidates))
	if len(candidates) == 0 {
		return weights
	}
	minScore := finiteOpenAICandidateScore(candidates[0].score)
	for i := 1; i < len(candidates); i++ {
		if score := finiteOpenAICandidateScore(candidates[i].score); score < minScore {
			minScore = score
		}
	}
	for i, candidate := range candidates {
		quality := finiteOpenAICandidateScore(candidate.score) - minScore + 1
		weight := quality * math.Sqrt(openAICandidateAvailableCapacity(candidate))
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
			weight = 1
		}
		weights[i] = weight
	}
	return weights
}

func weightedOpenAIIndex(weights []float64, rng *openAISelectionRNG) int {
	if len(weights) <= 1 {
		return 0
	}
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return int(rng.nextUint64() % uint64(len(weights)))
	}
	target := rng.nextFloat64() * total
	accumulated := 0.0
	for i, weight := range weights {
		accumulated += weight
		if target < accumulated {
			return i
		}
	}
	return len(weights) - 1
}

func openAIChannelWeight(group openAIChannelCandidateGroup) float64 {
	if len(group.candidates) == 0 {
		return 0
	}
	bestScore := finiteOpenAICandidateScore(group.candidates[0].score)
	capacity := 0.0
	for _, candidate := range group.candidates {
		if score := finiteOpenAICandidateScore(candidate.score); score > bestScore {
			bestScore = score
		}
		capacity += openAICandidateAvailableCapacity(candidate)
	}
	// Square-root capacity scaling preserves configured capacity differences
	// without making account cardinality a linear traffic multiplier.
	return math.Max(1, bestScore+1) * math.Sqrt(math.Max(1, capacity))
}

// buildOpenAIChannelAwareWeightedSelectionOrder creates a weighted channel
// permutation per round, then picks one weighted account from every channel in
// that round. Retries therefore cross upstream boundaries before returning to
// another account on the same upstream.
func buildOpenAIChannelAwareWeightedSelectionOrder(
	candidates []openAIAccountCandidateScore,
	req OpenAIAccountScheduleRequest,
) []openAIAccountCandidateScore {
	if len(candidates) <= 1 {
		return append([]openAIAccountCandidateScore(nil), candidates...)
	}
	if !hasMultipleOpenAIAPIKeyChannels(candidates) {
		return buildOpenAILegacyWeightedSelectionOrder(candidates, req)
	}

	groups := groupOpenAIAccountCandidatesByChannel(candidates)
	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req))
	order := make([]openAIAccountCandidateScore, 0, len(candidates))
	for len(groups) > 0 {
		roundGroups := append([]openAIChannelCandidateGroup(nil), groups...)
		channelOrder := make([]openAIChannelCandidateGroup, 0, len(roundGroups))
		for len(roundGroups) > 0 {
			weights := make([]float64, len(roundGroups))
			for i, group := range roundGroups {
				weights[i] = openAIChannelWeight(group)
			}
			idx := weightedOpenAIIndex(weights, &rng)
			channelOrder = append(channelOrder, roundGroups[idx])
			roundGroups = append(roundGroups[:idx], roundGroups[idx+1:]...)
		}

		remaining := make([]openAIChannelCandidateGroup, 0, len(groups))
		for _, selectedGroup := range channelOrder {
			idx := -1
			for i := range groups {
				if groups[i].key == selectedGroup.key {
					idx = i
					break
				}
			}
			if idx < 0 || len(groups[idx].candidates) == 0 {
				continue
			}
			accountWeights := openAICandidateWeights(groups[idx].candidates)
			accountIdx := weightedOpenAIIndex(accountWeights, &rng)
			order = append(order, groups[idx].candidates[accountIdx])
			groups[idx].candidates = append(groups[idx].candidates[:accountIdx], groups[idx].candidates[accountIdx+1:]...)
		}
		for _, group := range groups {
			if len(group.candidates) > 0 {
				remaining = append(remaining, group)
			}
		}
		groups = remaining
	}
	return order
}

func buildOpenAILegacyWeightedSelectionOrder(
	candidates []openAIAccountCandidateScore,
	req OpenAIAccountScheduleRequest,
) []openAIAccountCandidateScore {
	pool := append([]openAIAccountCandidateScore(nil), candidates...)
	weights := make([]float64, len(pool))
	minScore := pool[0].score
	for i := 1; i < len(pool); i++ {
		if pool[i].score < minScore {
			minScore = pool[i].score
		}
	}
	for i := range pool {
		weight := (pool[i].score - minScore) + 1
		if math.IsNaN(weight) || math.IsInf(weight, 0) || weight <= 0 {
			weight = 1
		}
		weights[i] = weight
	}

	order := make([]openAIAccountCandidateScore, 0, len(pool))
	rng := newOpenAISelectionRNG(deriveOpenAISelectionSeed(req))
	for len(pool) > 0 {
		selectedIdx := weightedOpenAIIndex(weights, &rng)
		order = append(order, pool[selectedIdx])
		pool = append(pool[:selectedIdx], pool[selectedIdx+1:]...)
		weights = append(weights[:selectedIdx], weights[selectedIdx+1:]...)
	}
	return order
}

type openAIAccountLoadChannelGroup struct {
	key      string
	accounts []accountWithLoad
}

func sameOpenAIAccountLoadRank(left, right accountWithLoad) bool {
	if left.account == nil || right.account == nil || left.loadInfo == nil || right.loadInfo == nil {
		return false
	}
	if left.account.Priority != right.account.Priority || left.loadInfo.LoadRate != right.loadInfo.LoadRate {
		return false
	}
	switch {
	case left.account.LastUsedAt == nil && right.account.LastUsedAt == nil:
		return true
	case left.account.LastUsedAt == nil || right.account.LastUsedAt == nil:
		return false
	default:
		return left.account.LastUsedAt.Equal(*right.account.LastUsedAt)
	}
}

func betterOpenAIAccountLoadRank(left, right accountWithLoad) bool {
	if left.account.Priority != right.account.Priority {
		return left.account.Priority < right.account.Priority
	}
	if left.loadInfo.LoadRate != right.loadInfo.LoadRate {
		return left.loadInfo.LoadRate < right.loadInfo.LoadRate
	}
	switch {
	case left.account.LastUsedAt == nil && right.account.LastUsedAt != nil:
		return true
	case left.account.LastUsedAt != nil && right.account.LastUsedAt == nil:
		return false
	case left.account.LastUsedAt == nil && right.account.LastUsedAt == nil:
		return false
	default:
		return left.account.LastUsedAt.Before(*right.account.LastUsedAt)
	}
}

func interleaveOpenAIAPIKeyChannelsByLoad(accounts []accountWithLoad) []accountWithLoad {
	if len(accounts) <= 1 {
		return accounts
	}
	result := make([]accountWithLoad, 0, len(accounts))
	for start := 0; start < len(accounts); {
		end := start + 1
		for end < len(accounts) &&
			accounts[start].account.Priority == accounts[end].account.Priority &&
			accounts[start].loadInfo.LoadRate == accounts[end].loadInfo.LoadRate {
			end++
		}
		result = append(result, interleaveOpenAIAPIKeyChannelLoadBucket(accounts[start:end])...)
		start = end
	}
	return result
}

func interleaveOpenAIAPIKeyChannelLoadBucket(accounts []accountWithLoad) []accountWithLoad {
	groups := make([]openAIAccountLoadChannelGroup, 0, len(accounts))
	indexes := make(map[string]int, len(accounts))
	apiKeyChannels := make(map[string]struct{})
	for _, item := range accounts {
		key := openAIUpstreamChannelKey(item.account)
		if item.account != nil && item.account.Type == AccountTypeAPIKey {
			apiKeyChannels[key] = struct{}{}
		}
		idx, ok := indexes[key]
		if !ok {
			idx = len(groups)
			indexes[key] = idx
			groups = append(groups, openAIAccountLoadChannelGroup{key: key})
		}
		groups[idx].accounts = append(groups[idx].accounts, item)
	}
	if len(apiKeyChannels) <= 1 {
		return accounts
	}

	sort.SliceStable(groups, func(i, j int) bool {
		return betterOpenAIAccountLoadRank(groups[i].accounts[0], groups[j].accounts[0])
	})
	for start := 0; start < len(groups); {
		end := start + 1
		for end < len(groups) && sameOpenAIAccountLoadRank(groups[start].accounts[0], groups[end].accounts[0]) {
			end++
		}
		if end-start > 1 {
			mathrand.Shuffle(end-start, func(i, j int) {
				groups[start+i], groups[start+j] = groups[start+j], groups[start+i]
			})
		}
		start = end
	}

	result := make([]accountWithLoad, 0, len(accounts))
	for round := 0; len(result) < len(accounts); round++ {
		for _, group := range groups {
			if round < len(group.accounts) {
				result = append(result, group.accounts[round])
			}
		}
	}
	return result
}

type openAIAccountChannelGroup struct {
	key      string
	accounts []*Account
}

func sameOpenAIAccountRank(left, right *Account) bool {
	if left == nil || right == nil || left.Priority != right.Priority {
		return false
	}
	switch {
	case left.LastUsedAt == nil && right.LastUsedAt == nil:
		return true
	case left.LastUsedAt == nil || right.LastUsedAt == nil:
		return false
	default:
		return left.LastUsedAt.Equal(*right.LastUsedAt)
	}
}

func betterOpenAIAccountRank(left, right *Account) bool {
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	if left.Priority != right.Priority {
		return left.Priority < right.Priority
	}
	switch {
	case left.LastUsedAt == nil && right.LastUsedAt != nil:
		return true
	case left.LastUsedAt != nil && right.LastUsedAt == nil:
		return false
	case left.LastUsedAt == nil && right.LastUsedAt == nil:
		return false
	default:
		return left.LastUsedAt.Before(*right.LastUsedAt)
	}
}

func interleaveOpenAIAPIKeyChannels(accounts []*Account) []*Account {
	if len(accounts) <= 1 {
		return accounts
	}
	result := make([]*Account, 0, len(accounts))
	for start := 0; start < len(accounts); {
		end := start + 1
		for end < len(accounts) && accounts[start].Priority == accounts[end].Priority {
			end++
		}
		result = append(result, interleaveOpenAIAPIKeyChannelPriorityBucket(accounts[start:end])...)
		start = end
	}
	return result
}

func interleaveOpenAIAPIKeyChannelPriorityBucket(accounts []*Account) []*Account {
	groups := make([]openAIAccountChannelGroup, 0, len(accounts))
	indexes := make(map[string]int, len(accounts))
	apiKeyChannels := make(map[string]struct{})
	for _, account := range accounts {
		key := openAIUpstreamChannelKey(account)
		if account != nil && account.Type == AccountTypeAPIKey {
			apiKeyChannels[key] = struct{}{}
		}
		idx, ok := indexes[key]
		if !ok {
			idx = len(groups)
			indexes[key] = idx
			groups = append(groups, openAIAccountChannelGroup{key: key})
		}
		groups[idx].accounts = append(groups[idx].accounts, account)
	}
	if len(apiKeyChannels) <= 1 {
		return accounts
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return betterOpenAIAccountRank(groups[i].accounts[0], groups[j].accounts[0])
	})
	for start := 0; start < len(groups); {
		end := start + 1
		for end < len(groups) && sameOpenAIAccountRank(groups[start].accounts[0], groups[end].accounts[0]) {
			end++
		}
		if end-start > 1 {
			mathrand.Shuffle(end-start, func(i, j int) {
				groups[start+i], groups[start+j] = groups[start+j], groups[start+i]
			})
		}
		start = end
	}

	result := make([]*Account, 0, len(accounts))
	for round := 0; len(result) < len(accounts); round++ {
		for _, group := range groups {
			if round < len(group.accounts) {
				result = append(result, group.accounts[round])
			}
		}
	}
	return result
}

func selectOpenAIAccountByChannelLRU(accounts []*Account) *Account {
	if len(accounts) == 0 {
		return nil
	}
	if len(accounts) == 1 {
		return accounts[0]
	}

	groups := make([]openAIAccountChannelGroup, 0, len(accounts))
	indexes := make(map[string]int, len(accounts))
	apiKeyChannels := make(map[string]struct{})
	for _, account := range accounts {
		key := openAIUpstreamChannelKey(account)
		if account != nil && account.Type == AccountTypeAPIKey {
			apiKeyChannels[key] = struct{}{}
		}
		idx, ok := indexes[key]
		if !ok {
			idx = len(groups)
			indexes[key] = idx
			groups = append(groups, openAIAccountChannelGroup{key: key})
		}
		groups[idx].accounts = append(groups[idx].accounts, account)
	}
	if len(apiKeyChannels) <= 1 {
		selected := accounts[0]
		for _, account := range accounts[1:] {
			if betterOpenAIAccountRank(account, selected) {
				selected = account
			}
		}
		return selected
	}

	channelLastUsed := func(group openAIAccountChannelGroup) *Account {
		var newest *Account
		for _, account := range group.accounts {
			if account == nil || account.LastUsedAt == nil {
				continue
			}
			if newest == nil || newest.LastUsedAt.Before(*account.LastUsedAt) {
				newest = account
			}
		}
		return newest
	}
	bestGroups := make([]int, 0, len(groups))
	var oldestChannelUse *Account
	for i, group := range groups {
		lastUsed := channelLastUsed(group)
		switch {
		case len(bestGroups) == 0:
			bestGroups = append(bestGroups, i)
			oldestChannelUse = lastUsed
		case lastUsed == nil && oldestChannelUse != nil:
			bestGroups = []int{i}
			oldestChannelUse = nil
		case lastUsed != nil && oldestChannelUse == nil:
			continue
		case lastUsed == nil && oldestChannelUse == nil:
			bestGroups = append(bestGroups, i)
		case lastUsed.LastUsedAt.Before(*oldestChannelUse.LastUsedAt):
			bestGroups = []int{i}
			oldestChannelUse = lastUsed
		case lastUsed.LastUsedAt.Equal(*oldestChannelUse.LastUsedAt):
			bestGroups = append(bestGroups, i)
		}
	}
	selectedGroup := groups[bestGroups[mathrand.Intn(len(bestGroups))]]
	bestAccounts := make([]*Account, 0, len(selectedGroup.accounts))
	var oldestAccount *Account
	for _, account := range selectedGroup.accounts {
		switch {
		case len(bestAccounts) == 0:
			bestAccounts = append(bestAccounts, account)
			oldestAccount = account
		case account.LastUsedAt == nil && oldestAccount.LastUsedAt != nil:
			bestAccounts = []*Account{account}
			oldestAccount = account
		case account.LastUsedAt != nil && oldestAccount.LastUsedAt == nil:
			continue
		case account.LastUsedAt == nil && oldestAccount.LastUsedAt == nil:
			bestAccounts = append(bestAccounts, account)
		case account.LastUsedAt.Before(*oldestAccount.LastUsedAt):
			bestAccounts = []*Account{account}
			oldestAccount = account
		case account.LastUsedAt.Equal(*oldestAccount.LastUsedAt):
			bestAccounts = append(bestAccounts, account)
		}
	}
	return bestAccounts[mathrand.Intn(len(bestAccounts))]
}
