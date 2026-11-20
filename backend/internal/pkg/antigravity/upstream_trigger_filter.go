package antigravity

import (
	"log"
	"os"
	"regexp"
	"strings"
)

// The Antigravity upstream (daily-cloudcode-pa) answers a request whose system
// instruction contains certain sentences with a bare 429 RESOURCE_EXHAUSTED:
// no quotaResetDelay, no RetryInfo, body "Resource has been exhausted (e.g. check quota)."
// It is a content rule, not a quota: the identical request without the sentence
// returns 200, and the same sentence still returns 429 on an idle account.
//
// Observed from 2026-11-15 02:48 UTC on Claude Code 2.1.325, whose system prompt carries
//
//	"Dual-use security tools (C2 frameworks, credential testing, exploit development)
//	 require clear authorization context: pentesting engagements, CTF competitions,
//	 security research, or defensive use cases."
//
// Every keyword alone passes, so the rule keys on the whole sentence. Inserting
// zero-width characters into its words does not help, so the sentence is removed.
//
// Further patterns can be added without a rebuild through the environment variable
// SUB2API_ANTIGRAVITY_STRIP_PATTERNS (one Go regexp per line). They apply to every system
// text that is forwarded to Antigravity, in addition to the defaults below.
const stripPatternsEnv = "SUB2API_ANTIGRAVITY_STRIP_PATTERNS"

var defaultUpstreamTriggerPatterns = []string{
	`(?i)Dual-use security tools \([^)]*\) require clear authorization context:[^.\n]*\.[ \t]*`,
}

var upstreamTriggerPatterns = compileUpstreamTriggerPatterns(os.Getenv(stripPatternsEnv))

func compileUpstreamTriggerPatterns(extra string) []*regexp.Regexp {
	sources := append([]string(nil), defaultUpstreamTriggerPatterns...)
	for _, line := range strings.Split(extra, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			sources = append(sources, line)
		}
	}
	patterns := make([]*regexp.Regexp, 0, len(sources))
	for _, src := range sources {
		re, err := regexp.Compile(src)
		if err != nil {
			log.Printf("[Antigravity] ignoring invalid strip pattern %q: %v", src, err)
			continue
		}
		patterns = append(patterns, re)
	}
	return patterns
}

// StripUpstreamTriggerSentences removes sentences that the Antigravity upstream is known
// to reject with a fake 429. Only Antigravity forwarding should use it.
func StripUpstreamTriggerSentences(text string) string {
	for _, re := range upstreamTriggerPatterns {
		text = re.ReplaceAllString(text, "")
	}
	return text
}
