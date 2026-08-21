package domain

// ReasoningEffortMapping rewrites one explicit OpenAI/Codex reasoning effort
// value to another before the group ceiling is applied. Model optionally scopes
// the mapping to one client-requested model; an empty model matches all models.
type ReasoningEffortMapping struct {
	Model string `json:"model,omitempty"`
	From  string `json:"from"`
	To    string `json:"to"`
}
