package service

import "time"

// Account-scoped overlap detection must continue to identify the original
// owner of every saved month. Credential rotation is allowed; repurposing a
// connection for a different cloud owner/product requires a new connection.
func validateBillingScopeEdit(provider string, previous, incoming map[string]string) error {
	if billingHasAccountScope(provider) && (previous["account_id"] != incoming["account_id"] || previous["product_code"] != incoming["product_code"]) {
		return billingInvalid("the billing owner and product scope cannot be changed; create a new connection for a different account or product")
	}
	return nil
}

// Omitted new fields retain existing values, so older API clients cannot
// inadvertently turn a strict allocation policy back into estimated charges.
func resolveBillingPolicy(input SaveBillingConnectionInput, previous *BillingConnection) (int, string, error) {
	lookback, mode := 2, "local_weighted"
	if previous != nil {
		lookback, mode = previous.SyncLookbackMonths, billingSnapshotMode(previous.AllocationMode)
	}
	if input.SyncLookbackMonths != nil {
		lookback = *input.SyncLookbackMonths
	}
	if input.AllocationMode != "" {
		mode = input.AllocationMode
	}
	if lookback < 1 || lookback > 6 {
		return 0, "", billingInvalid("sync lookback must contain 1 to 6 months including the current month")
	}
	if mode != "local_weighted" && mode != "official_only" && mode != "disabled" {
		return 0, "", billingInvalid("allocation mode must be local_weighted, official_only or disabled")
	}
	return lookback, mode, nil
}

func billingSnapshotMode(modes ...string) string {
	if len(modes) == 0 || modes[0] == "" {
		return "local_weighted"
	}
	return modes[0]
}

func billingScheduledMonths(now time.Time, location *time.Location, lookback int) []string {
	if lookback < 1 || lookback > 6 {
		lookback = 2
	}
	local := now.In(location)
	first := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
	months := make([]string, 0, lookback)
	// Newest first: a slow old bill must not prevent refreshing the current one.
	for offset := 0; offset < lookback; offset++ {
		months = append(months, first.AddDate(0, -offset, 0).Format("2006-01"))
	}
	return months
}
