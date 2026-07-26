package domain

// Privacy-mode values stored under Account.Extra["privacy_mode"] for the OpenAI
// and Antigravity platforms. Canonical home is domain so Account methods can read
// them without importing service. Re-exported from their original service homes.
const (
	// PrivacyModeTrainingOff marks an OpenAI account whose "Improve the model for
	// everyone" setting has been disabled via the ChatGPT settings API.
	PrivacyModeTrainingOff = "training_off"
	// AntigravityPrivacySet marks an Antigravity account whose privacy has been
	// verified via the Antigravity user-settings API.
	AntigravityPrivacySet = "privacy_set"
)
