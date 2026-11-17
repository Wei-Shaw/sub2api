package securityaudit

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Digest of decisions_policy/ when the testdata responses were captured from the
// live API. Update it together with the policy files and the captured responses.
const validatedDecisionsPolicyDigest = "2eda03ec0e0f303490b42c6b48b2342a81fc11b2fbb5472ca1dc31360ae1e548"

func TestDecisionsPolicyMatchesValidatedPolicy(t *testing.T) {
	policy := promptDecisionsPolicy
	require.Equal(t, validatedDecisionsPolicyDigest, policy.Digest)
	require.Equal(t, AllScannerIDs, policy.Categories)
	require.Len(t, policy.Questions, 10)
	require.Equal(t, []string{"Safe", "Controversial", "Unsafe"}, policy.Levels)
}

// testdata holds the request that produced the captured jailbreak-dan response;
// any change to the request shape has to be deliberate.
func TestDecisionsRequestMatchesCapturedRequest(t *testing.T) {
	raw, err := os.ReadFile("testdata/decisions-request-jailbreak-dan.json")
	require.NoError(t, err)
	var fixture struct {
		Text    string          `json:"text"`
		Request json.RawMessage `json:"request"`
	}
	require.NoError(t, json.Unmarshal(raw, &fixture))

	body, err := promptDecisionsPolicy.requestBody(DefaultDecisionsModel, fixture.Text)
	require.NoError(t, err)
	var got, want any
	require.NoError(t, json.Unmarshal(body, &got))
	require.NoError(t, json.Unmarshal(fixture.Request, &want))
	require.Equal(t, want, got)
}

func loadDecisionsFixture(t *testing.T, sample string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/decisions-" + sample + ".json")
	require.NoError(t, err)
	return raw
}

func verdictFor(t *testing.T, body []byte, enabled []string) *NormalizedResult {
	t.Helper()
	answers, err := promptDecisionsPolicy.parseDecisionsResponse(body)
	require.NoError(t, err)
	return decisionsVerdict(answers, DefaultCategoryThreshold, enabled)
}

// Fixtures are real /v1/decisions responses captured with the validated policy.
func TestDecisionsRealResponsesMapToGatewayActions(t *testing.T) {
	jailbreak := verdictFor(t, loadDecisionsFixture(t, "jailbreak-dan"), AllScannerIDs)
	require.Equal(t, "Unsafe", jailbreak.Safety)
	require.Contains(t, jailbreak.MatchedScanners, "jailbreak")
	require.Equal(t, ActionBlock, jailbreak.Action)
	require.Greater(t, jailbreak.ScannerScores["jailbreak"], DefaultCategoryThreshold)

	violent := verdictFor(t, loadDecisionsFixture(t, "violent-explosive"), AllScannerIDs)
	require.Equal(t, "Unsafe", violent.Safety)
	require.Contains(t, violent.MatchedScanners, "violent")
	require.Equal(t, ActionBlock, violent.Action)

	benign := verdictFor(t, loadDecisionsFixture(t, "benign-long-compaction-summary"), AllScannerIDs)
	require.Equal(t, "Safe", benign.Safety)
	require.Empty(t, benign.Categories)
	require.Equal(t, ActionAllow, benign.Action)
}

// Unsafe with categories that are all disabled warns instead of blocking,
// the same rule ParseQwen3Guard applies.
func TestDecisionsUnsafeOnlyDisabledCategoriesWarns(t *testing.T) {
	result := verdictFor(t, loadDecisionsFixture(t, "violent-explosive"), []string{"pii"})
	require.Equal(t, "Unsafe", result.Safety)
	require.Empty(t, result.MatchedScanners)
	require.Equal(t, ActionWarn, result.Action)
}

func mutateDecisionsFixture(t *testing.T, sample string, mutate func(answers []map[string]any) []map[string]any) []byte {
	t.Helper()
	var response map[string]any
	require.NoError(t, json.Unmarshal(loadDecisionsFixture(t, sample), &response))
	rawAnswers, ok := response["answers"].([]any)
	require.True(t, ok)
	answers := make([]map[string]any, 0, len(rawAnswers))
	for _, item := range rawAnswers {
		answer, ok := item.(map[string]any)
		require.True(t, ok)
		answers = append(answers, answer)
	}
	response["answers"] = mutate(answers)
	body, err := json.Marshal(response)
	require.NoError(t, err)
	return body
}

func TestDecisionsInvalidAnswersAreRejected(t *testing.T) {
	cases := map[string]func([]map[string]any) []map[string]any{
		"missing answer": func(a []map[string]any) []map[string]any { return a[:len(a)-1] },
		"repeated answer": func(a []map[string]any) []map[string]any {
			return append(a, a[1])
		},
		"unknown level": func(a []map[string]any) []map[string]any {
			a[0]["choice"] = "Maybe"
			return a
		},
		"probability out of range": func(a []map[string]any) []map[string]any {
			a[2]["probability"] = 1.5
			return a
		},
		"wrong type": func(a []map[string]any) []map[string]any {
			a[2]["type"] = "choice"
			return a
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := promptDecisionsPolicy.parseDecisionsResponse(mutateDecisionsFixture(t, "jailbreak-dan", mutate))
			require.Error(t, err)
		})
	}
	_, err := promptDecisionsPolicy.parseDecisionsResponse([]byte(`{"error":{"type":"invalid_request_error"}}`))
	require.Error(t, err)
}

func refuse(names ...string) func([]map[string]any) []map[string]any {
	return func(answers []map[string]any) []map[string]any {
		for i, answer := range answers {
			for _, name := range names {
				if answer["name"] == name {
					answers[i] = map[string]any{"type": "refusal", "name": name, "refusal": "I can't help with that."}
				}
			}
		}
		return answers
	}
}

// A refusal blocks even when the answered questions would allow the text,
// and even when the refused category is disabled.
func TestDecisionsRefusalBlocks(t *testing.T) {
	for name, body := range map[string][]byte{
		"category refused on benign text": mutateDecisionsFixture(t, "benign-long-compaction-summary", refuse("pii")),
		"level refused":                   mutateDecisionsFixture(t, "benign-long-compaction-summary", refuse("safety_level")),
		"every question refused":          mutateDecisionsFixture(t, "jailbreak-dan", refuse(append([]string{"safety_level"}, AllScannerIDs...)...)),
	} {
		t.Run(name, func(t *testing.T) {
			result := verdictFor(t, body, []string{"violent"})
			require.Equal(t, ActionBlock, result.Action)
			require.Equal(t, EventCritical, result.Decision)
			require.Contains(t, result.Categories, DecisionsRefusalCategory)
			require.Contains(t, result.ScannerEvidence[DecisionsRefusalCategory], "refused: ")
			summaries := BuildIssueSummaries(*result)
			require.NotEmpty(t, summaries)
			require.Equal(t, "审核模型拒答", summaries[len(summaries)-1].Title)
		})
	}
	result := verdictFor(t, mutateDecisionsFixture(t, "benign-long-compaction-summary", refuse("pii", "jailbreak")), AllScannerIDs)
	require.Equal(t, "refused: pii, jailbreak", result.ScannerEvidence[DecisionsRefusalCategory])
}

func TestDecisionsEndpointConfigDefaultsAndLimits(t *testing.T) {
	cfg, err := ParseStorageConfig(`{"enabled":true,"endpoints":[{"id":"d1","name":"Decisions","protocol":"openai_decisions","base_url":"https://api.openai.com","timeout_ms":8000,"input_limit":16000,"enabled":true}]}`)
	require.NoError(t, err)
	ep := cfg.Endpoints[0]
	require.Equal(t, DefaultDecisionsModel, ep.Model)
	require.Equal(t, DefaultCategoryThreshold, ep.CategoryThreshold)
	active, err := ActiveFromStorage(cfg, true, nil)
	require.NoError(t, err)
	require.Equal(t, DefaultCategoryThreshold, active.Endpoints[0].CategoryThreshold)
	require.Equal(t, DefaultCategoryThreshold, PublicFromStorage(cfg, true, nil).Endpoints[0].CategoryThreshold)

	url, err := DecisionsURL("https://api.openai.com/v1/")
	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/decisions", url)

	for name, raw := range map[string]string{
		"input limit above input_json cap": `{"endpoints":[{"id":"d1","name":"D","protocol":"openai_decisions","base_url":"https://api.openai.com","input_limit":40001}]}`,
		"threshold above one":              `{"endpoints":[{"id":"d1","name":"D","protocol":"openai_decisions","base_url":"https://api.openai.com","category_threshold":1.5}]}`,
		"negative threshold":               `{"endpoints":[{"id":"d1","name":"D","protocol":"openai_decisions","base_url":"https://api.openai.com","category_threshold":-0.1}]}`,
		"unknown protocol":                 `{"endpoints":[{"id":"d1","name":"D","protocol":"anthropic","base_url":"https://api.openai.com"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseStorageConfig(raw)
			require.Error(t, err)
		})
	}

	qwen, err := ParseStorageConfig(`{"endpoints":[{"id":"q1","name":"Qwen","base_url":"http://ollama:11434"}]}`)
	require.NoError(t, err)
	require.Equal(t, ProtocolOpenAICompatible, qwen.Endpoints[0].Protocol)
	require.Equal(t, DefaultGuardModel, qwen.Endpoints[0].Model)
	require.Zero(t, qwen.Endpoints[0].CategoryThreshold)
	require.True(t, strings.HasPrefix(promptDecisionsPolicy.CacheKey, "prompt-audit-2eda03ec"))
}
