package antigravity

import (
	"regexp"
	"strings"
)

var systemIdentityOpeners = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^[ \t\r\n]*You are a Claude agent, built on Anthropic'?s Claude Agent SDK\.?`),
	regexp.MustCompile(`(?i)^[ \t\r\n]*You are Claude Code, Anthropic'?s official CLI for Claude\.?`),
	regexp.MustCompile(`(?i)^[ \t\r\n]*You are Codex,? (?:a coding agent |an agent )?based on GPT-[0-9]+(?:\.[0-9]+)*(?:-[a-z0-9]+)*\.?`),
	regexp.MustCompile(`(?i)^[ \t\r\n]*You are GPT-[0-9]+(?:\.[0-9]+)*(?:-[a-z0-9]+)* running in the Codex CLI\.?`),
}

// NeutralizeSystemIdentity only changes leading client identity metadata on
// Antigravity requests. Native Anthropic forwarding retains its attribution.
func NeutralizeSystemIdentity(text string) string {
	text = stripClaudeAttribution(text)
	for _, re := range systemIdentityOpeners {
		if loc := re.FindStringIndex(text); loc != nil {
			rest := text[loc[1]:]
			if strings.HasPrefix(rest, ",") {
				return "You are an AI agent" + rest
			}
			if len(rest) > 0 && !strings.ContainsAny(rest[:1], " \t\r\n") {
				continue
			}
			return "You are an AI agent." + rest
		}
	}
	return text
}
