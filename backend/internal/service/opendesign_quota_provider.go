package service

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// OpenDesign usage probing rides the shared CN provider quota framework. The
// data source is the vendor funding-check endpoint: one request returns the
// Go-tier dual rolling windows (5h / 7d) in credits. Any parse failure or
// non-2xx keeps the previous snapshot (fail-open) and never parks the account.

const (
	// openDesignFundingCheckPath is appended to the account base URL.
	openDesignFundingCheckPath = "/api/v1/workspaces/%s/billing/funding-check"

	// DefaultOpenDesignAuthAPIBaseURL is the vendor console API host (device
	// authorization, wallet lookup and the funding-check probe).
	DefaultOpenDesignAuthAPIBaseURL = "https://amr-api.open-design.ai"

	// OpenDesign usage windows in seconds, as reported by funding-check.
	openDesignUsageWindow5HSeconds = 18000
	openDesignUsageWindow7DSeconds = 604800
)

// opendesignFundingCheckURL builds the funding-check endpoint from the account
// base URL and the workspace id stored by device auth.
func opendesignFundingCheckURL(baseURL, workspaceID string) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !isValidOpenDesignWorkspaceID(workspaceID) {
		return "", fmt.Errorf("invalid open_design workspace id")
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return "", fmt.Errorf("open_design account has no base url")
	}
	parsed, err := url.Parse(base + fmt.Sprintf(openDesignFundingCheckPath, url.PathEscape(workspaceID)))
	if err != nil || parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid open_design funding-check url")
	}
	return parsed.String(), nil
}

// opendesignParseCredits accepts the string-or-number credits fields the
// funding-check payload uses.
func opendesignParseCredits(raw any) (int64, bool) {
	switch v := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		n, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// opendesignWindowName maps a funding-check window duration to the shared CN
// window name. Unknown durations return ok=false so a future extra window is
// ignored instead of failing the whole response.
func opendesignWindowName(durationSeconds int) (string, bool) {
	switch durationSeconds {
	case openDesignUsageWindow5HSeconds:
		return "5h", true
	case openDesignUsageWindow7DSeconds:
		return "weekly", true
	default:
		return "", false
	}
}

// opendesignParseWindow validates one window entry and converts it to the
// shared CN tier shape. A window whose duration matches but whose numbers or
// reset time are unusable is a schema break: returning an error keeps the
// previous snapshot instead of writing a mixed "new usage + old reset" one.
func opendesignParseWindow(raw map[string]any, window string) (CNQuotaTier, error) {
	used, okUsed := opendesignParseCredits(raw["usedCredits"])
	limit, okLimit := opendesignParseCredits(raw["limitCredits"])
	remaining, okRemaining := opendesignParseCredits(raw["remainingCredits"])
	if !okUsed || !okLimit || !okRemaining {
		return CNQuotaTier{}, fmt.Errorf("%s window has non-numeric credits", window)
	}
	if used < 0 || limit <= 0 || remaining < 0 || remaining > limit {
		return CNQuotaTier{}, fmt.Errorf("%s window has out-of-range credits", window)
	}
	resetsAt, okReset := raw["resetsAt"].(string)
	if !okReset {
		return CNQuotaTier{}, fmt.Errorf("%s window has no resetsAt", window)
	}
	resetTime, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(resetsAt))
	if err != nil {
		return CNQuotaTier{}, fmt.Errorf("%s window has invalid resetsAt %q", window, resetsAt)
	}
	return CNQuotaTier{
		Window:      window,
		UsedPercent: float64(used) / float64(limit) * 100,
		ResetAt:     resetTime.UTC().Format(time.RFC3339),
	}, nil
}

// parseOpenDesignUsageTiers converts a funding-check response into CN quota
// tiers.
//
// A response that explicitly reports no coding plan (codingPlan null, or an
// eligible field that is present and false) returns noPlan=true with no error:
// the account simply has no plan, so the caller clears the usage windows
// instead of keeping a stale "exhausted" snapshot that would park it forever.
//
// A missing codingPlan field, a missing eligible field or any unusable window
// is an upstream schema change and returns an error, so the caller keeps the
// previous snapshot rather than writing a bogus one.
func parseOpenDesignUsageTiers(body []byte) (tiers []CNQuotaTier, planLevel string, noPlan bool, err error) {
	var payload struct {
		BalanceUsd string          `json:"balanceUsd"`
		CodingPlan json.RawMessage `json:"codingPlan"`
	}
	if decodeErr := json.Unmarshal(body, &payload); decodeErr != nil {
		return nil, "", false, fmt.Errorf("decode funding-check: %w", decodeErr)
	}
	if len(payload.CodingPlan) == 0 {
		return nil, "", false, fmt.Errorf("funding-check: response has no codingPlan field (upstream schema may have changed)")
	}
	balance := strings.TrimSpace(payload.BalanceUsd)

	if strings.TrimSpace(string(payload.CodingPlan)) == "null" {
		return nil, balance, true, nil
	}

	var plan struct {
		// Pointer so "absent" and "explicitly false" stay distinguishable: an
		// absent flag usually means the schema changed, not "no plan".
		Eligible *bool            `json:"eligible"`
		Tier     any              `json:"tier"`
		Windows  []map[string]any `json:"windows"`
	}
	if decodeErr := json.Unmarshal(payload.CodingPlan, &plan); decodeErr != nil {
		return nil, "", false, fmt.Errorf("decode funding-check codingPlan: %w", decodeErr)
	}
	if plan.Eligible == nil {
		return nil, "", false, fmt.Errorf("funding-check: codingPlan has no eligible field (upstream schema may have changed)")
	}
	if !*plan.Eligible {
		return nil, balance, true, nil
	}

	tierName := ""
	if tier, ok := plan.Tier.(string); ok {
		tierName = strings.TrimSpace(tier)
	}

	byWindow := make(map[string]CNQuotaTier, 2)
	for _, raw := range plan.Windows {
		duration, ok := raw["durationSeconds"].(float64)
		if !ok {
			return nil, "", false, fmt.Errorf("funding-check: window has no numeric durationSeconds")
		}
		window, known := opendesignWindowName(int(duration))
		if !known {
			continue
		}
		tier, windowErr := opendesignParseWindow(raw, window)
		if windowErr != nil {
			return nil, "", false, windowErr
		}
		byWindow[window] = tier
	}
	for _, window := range []string{"5h", "weekly"} {
		if tier, ok := byWindow[window]; ok {
			tiers = append(tiers, tier)
		}
	}
	if len(tiers) == 0 {
		return nil, "", false, fmt.Errorf("funding-check: coding plan eligible but no usable usage window parsed")
	}
	return tiers, tierName, false, nil
}

// opendesignBalanceUSD extracts the balance string for display (may be empty).
func opendesignBalanceUSD(body []byte) string {
	return strings.TrimSpace(gjson.GetBytes(body, "balanceUsd").String())
}
