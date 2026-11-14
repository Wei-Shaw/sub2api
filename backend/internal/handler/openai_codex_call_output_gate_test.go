//go:build unit

package handler

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCodexCallOutputGateSkipsDeclaredToolsWithoutCandidate(t *testing.T) {
	body := []byte(`{"model":"gpt-5","tools":[{"type":"function","name":"automation_update"},{"type":"function","name":"create_thread"}],` +
		`"input":[{"type":"function_call","call_id":"c1","name":"shell"},{"type":"function_call_output","call_id":"c1","output":"ok"}]}`)
	require.False(t, mayContainCodexCallOutputCandidate(body, "automation_update"))
	require.False(t, mayContainCodexCallOutputCandidate(body, "create_thread", "send_message_to_thread"))
	for name, normalize := range map[string]func([]byte) ([]byte, bool){
		"page":       normalizeCodexPageContext,
		"automation": normalizeCodexAutomationBootstrap,
		"delegation": normalizeCodexDelegationBootstrap,
	} {
		t.Run(name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(10, func() {
				got, changed := normalize(body)
				if changed || &got[0] != &body[0] {
					t.Fatal("non-candidate body must be returned unchanged")
				}
			})
			require.Zero(t, allocs)
		})
	}
}

// Whenever the ungated bootstrap path rewrites a body, the gate must admit it.
func TestCodexCallOutputGateAdmitsEveryRewrite(t *testing.T) {
	u := func(hex string) string { return `\` + "u" + hex }
	output := mustJSON(t, codexAutomationBootstrap("wiki", "never", automationBootstrapPrompt))
	items := []string{
		`{"type":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + output + `}`,
		`{"type":"function_call_output","namespace":"codex_app","name":"` + u("0061") + `utomation_update","output":` + output + `}`,
		`{"` + u("0074") + `ype":"function_call_output","namespace":"codex_app","name":"automation_update","output":` + output + `}`,
		`{"type":"function_call_output","namespace":"codex_app","name":"automation_update","name":"other","output":` + output + `}`,
		`{"type":"function_call_output","call_id":"c","output":"ok"}`,
		`{"type":"message","role":"user","content":"automation_update function_call_output caf` + u("00e9") + `"}`,
		`{"type":"function_call","call_id":"c","name":"automation_update"}`,
	}
	rng := rand.New(rand.NewSource(11))
	rewrites := 0
	for i := 0; i < 3000; i++ {
		var parts []string
		for n := rng.Intn(3); n >= 0; n-- {
			parts = append(parts, items[rng.Intn(len(items))])
		}
		body := []byte(`{"model":"gpt-5","tools":[{"type":"function","name":"automation_update"}],"input":[` + strings.Join(parts, ",") + `]}`)
		_, rewritten := normalizeCodexCallOutputBootstrap(body, isCodexAutomationCandidate, false)
		if rewritten {
			rewrites++
			require.True(t, mayContainCodexCallOutputCandidate(body, "automation_update"), "%s", body)
		}
	}
	require.Positive(t, rewrites)
}
