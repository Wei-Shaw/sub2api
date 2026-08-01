package domain

// ReasoningEffortMapping rewrites one explicit OpenAI/Codex reasoning effort
// value to another before the group ceiling is applied.
type ReasoningEffortMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ModelReasoningEffortRule overrides the group default policy for one exact
// client-requested model. An empty ceiling explicitly means unlimited; remove
// the rule to inherit the group default policy again.
type ModelReasoningEffortRule struct {
	Model                   string                   `json:"model"`
	MaxReasoningEffort      string                   `json:"max_reasoning_effort"`
	ReasoningEffortMappings []ReasoningEffortMapping `json:"reasoning_effort_mappings"`
}
