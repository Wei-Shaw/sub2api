package adminaccess

import "testing"

func TestStatisticsAndOpenAIPermissions(t *testing.T) {
	for action, routes := range map[string][]string{
		"accounts.stats.read":     {"GET /accounts/:id/today-stats", "POST /accounts/today-stats/batch", "GET /accounts/:id/stats"},
		"accounts.usage.read":     {"POST /accounts/usage/batch"},
		"openai.quota.read":       {"GET /openai/accounts/:id/quota", "POST /openai/accounts/:id/quota/refresh"},
		"openai.quota.reset":      {"POST /openai/accounts/:id/reset-quota"},
		"openai.referrals.read":   {"POST /openai/accounts/:id/referrals/refresh"},
		"openai.referrals.invite": {"POST /openai/accounts/:id/referrals/invite"},
	} {
		p := Policy{Rules: []Rule{{Permissions: []string{action}, GroupIDs: []int64{1}, AccountTypes: []string{"oauth"}}}}
		if err := ValidatePolicy(p); err != nil {
			t.Errorf("%s: %v", action, err)
		}
		for _, route := range routes {
			var method, path string
			for i, c := range route {
				if c == ' ' {
					method, path = route[:i], route[i+1:]
					break
				}
			}
			if !RouteAllowed(p, method, "/api/v1/admin"+path) {
				t.Errorf("%s 无法进入 %s", action, route)
			}
		}
	}
}
