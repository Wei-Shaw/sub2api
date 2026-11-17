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

// opendesignParseWindow validates one window entry against the expected
// duration and converts it to the shared CN tier shape.
func opendesignParseWindow(raw map[string]any, durationSeconds int, window string) (CNQuotaTier, bool) {
	if v, ok := raw["durationSeconds"].(float64); !ok || int(v) != durationSeconds {
		return CNQuotaTier{}, false
	}
	used, okUsed := opendesignParseCredits(raw["usedCredits"])
	limit, okLimit := opendesignParseCredits(raw["limitCredits"])
	remaining, okRemaining := opendesignParseCredits(raw["remainingCredits"])
	if !okUsed || !okLimit || !okRemaining || limit <= 0 || remaining < 0 || remaining > limit {
		return CNQuotaTier{}, false
	}
	_ = remaining
	tier := CNQuotaTier{
		Window:      window,
		UsedPercent: float64(used) / float64(limit) * 100,
	}
	if resetsAt, ok := raw["resetsAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, resetsAt); err == nil {
			tier.ResetAt = t.UTC().Format(time.RFC3339)
		}
	}
	return tier, true
}

// parseOpenDesignUsageTiers converts a funding-check response into CN quota
// tiers. A response that explicitly reports no coding plan (codingPlan null or
// eligible=false) yields an empty tier list with no error: the account simply
// has no plan and stays schedulable. A missing codingPlan field or a malformed
// one is an upstream schema change and returns an error so the caller keeps the
// previous snapshot instead of writing a bogus "no usage" one.
func parseOpenDesignUsageTiers(body []byte) ([]CNQuotaTier, string, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, "", fmt.Errorf("decode funding-check: %w", err)
	}
	rawCodingPlan, hasCodingPlan := envelope["codingPlan"]
	if !hasCodingPlan {
		return nil, "", fmt.Errorf("funding-check: response has no codingPlan field (upstream schema may have changed)")
	}

	var payload struct {
		BalanceUsd string          `json:"balanceUsd"`
		CodingPlan json.RawMessage `json:"codingPlan"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", fmt.Errorf("decode funding-check: %w", err)
	}

	if strings.TrimSpace(string(rawCodingPlan)) == "null" {
		return nil, payload.BalanceUsd, nil
	}

	var plan struct {
		Eligible bool             `json:"eligible"`
		Tier     any              `json:"tier"`
		Windows  []map[string]any `json:"windows"`
	}
	if err := json.Unmarshal(rawCodingPlan, &plan); err != nil {
		return nil, "", fmt.Errorf("decode funding-check codingPlan: %w", err)
	}
	if !plan.Eligible {
		return nil, payload.BalanceUsd, nil
	}
	tierName := ""
	if tier, ok := plan.Tier.(string); ok {
		tierName = tier
	}

	var tiers []CNQuotaTier
	for _, raw := range plan.Windows {
		if tier, ok := opendesignParseWindow(raw, openDesignUsageWindow5HSeconds, "5h"); ok {
			tiers = append(tiers, tier)
			continue
		}
		if tier, ok := opendesignParseWindow(raw, openDesignUsageWindow7DSeconds, "weekly"); ok {
			tiers = append(tiers, tier)
		}
	}
	if len(tiers) == 0 {
		return nil, "", fmt.Errorf("funding-check: coding plan eligible but no usable usage window parsed")
	}
	return tiers, tierName, nil
}

// opendesignBalanceUSD extracts the balance string for display (may be empty).
func opendesignBalanceUSD(body []byte) string {
	return strings.TrimSpace(gjson.GetBytes(body, "balanceUsd").String())
}