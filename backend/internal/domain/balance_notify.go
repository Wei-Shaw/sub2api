package domain

// Balance/quota threshold labels used by the balance-notify flow. Leaf strings
// relocated from service in Phase C Step 1 (Account BC prep); re-exported from
// service/balance_notify_service.go under their original unexported names.
const (
	// ThresholdTypeFixed is the fixed-amount threshold type.
	ThresholdTypeFixed = "fixed"
	// Quota dimension labels.
	QuotaDimDaily  = "daily"
	QuotaDimWeekly = "weekly"
	QuotaDimTotal  = "total"
)
