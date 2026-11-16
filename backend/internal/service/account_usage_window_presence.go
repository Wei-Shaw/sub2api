package service

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Preserve null versus a real {utilization: 0, resets_at: null} window.
// Ultra verification is account-specific, so a plan label alone is insufficient.
func (r *ClaudeUsageResponse) UnmarshalJSON(data []byte) error {
	type response ClaudeUsageResponse
	var decoded response
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*r = ClaudeUsageResponse(decoded)
	if raw, ok := fields["five_hour"]; ok {
		available := !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
		r.FiveHourAvailable = &available
	}
	return nil
}

// OpenAI policy checked 2026-11-15: all personal Pro tiers have no 5h limit.
// https://help.openai.com/en/articles/20001516-managing-usage-with-gpt-6-astra-in-work-and-codex
// Plus, workspace plans and unknown future plans retain observed limits.
func openAIPlanHasNoFiveHour(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuth() || account.IsShadow() {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(account.GetCredential("plan_type"))) {
	case "prolite", "pro_lite", "pro", "chatgpt_pro", "promax":
		return true
	}
	return false
}
