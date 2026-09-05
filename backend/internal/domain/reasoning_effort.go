package domain

const (
	ReasoningEffortMatchExact  = "exact"
	ReasoningEffortMatchPrefix = "prefix"
	ReasoningEffortMatchSuffix = "suffix"
)

// ReasoningEffortMapping rewrites one explicit OpenAI/Codex reasoning effort
// value to another before the group ceiling is applied.
//
// Model and MatchType optionally scope the rewrite to a request model:
// exact matches the full model id, prefix/suffix match a model-id affix.
// Empty MatchType and empty Model mean the mapping applies to every model.
type ReasoningEffortMapping struct {
	From      string `json:"from"`
	To        string `json:"to"`
	MatchType string `json:"match_type,omitempty"`
	Model     string `json:"model,omitempty"`
}
