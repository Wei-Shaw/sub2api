//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGrokCatalogParserParity(t *testing.T) {
	t.Parallel()

	inputs := []string{
		`{"data":[{"id":"display","model":"grok-4.7"},{"model_id":"grok-4.6"}]}`,
		`{"models":[{"modelId":"grok-4.6"},{"model":"grok-4.7"}]}`,
		`["grok-4.7",{"id":"grok-4.6"},"grok-4.7"]`,
	}
	for _, input := range inputs {
		manual, err := extractGrokUpstreamModelIDs([]byte(input))
		require.NoError(t, err)
		observed, err := extractGrokModelIDsFromModelsBody([]byte(input))
		require.NoError(t, err)
		parsed, err := parseGrokCatalogModelIDs([]byte(input))
		require.NoError(t, err)
		require.Equal(t, []string{"grok-4.6", "grok-4.7"}, manual)
		require.Equal(t, manual, observed)
		require.Equal(t, manual, parsed)
	}
}

func TestGrokCatalogParserRejectsBadInput(t *testing.T) {
	t.Parallel()

	inputs := []string{
		`{"error":{"message":"denied"}}`,
		`{"error":{"message":"denied"},"data":[{"model":"grok-4.7"}]}`,
		`{"data":[{"id":123}]}`,
		`{"data":[{}]}`,
		`{"data":[]}`,
		`{"data":`,
		`null`,
		`{"models":{}}`,
		`{"models":{"grok-4.7":{"model":"grok-4.7"}}}`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			for _, parse := range []func([]byte) ([]string, error){
				parseGrokCatalogModelIDs,
				extractGrokUpstreamModelIDs,
				extractGrokModelIDsFromModelsBody,
			} {
				ids, err := parse([]byte(input))
				require.Error(t, err)
				require.Empty(t, ids)
				require.NotContains(t, err.Error(), "denied")
				require.NotContains(t, err.Error(), "grok-4.7")
			}
		})
	}
}

func TestGrokCatalogParserSkipsBlankIDsButRejectsEmpty(t *testing.T) {
	t.Parallel()

	ids, err := parseGrokCatalogModelIDs([]byte(`{"data":["  grok-4.7  ","   ",{"model":" grok-4.6 "}]}`))
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.6", "grok-4.7"}, ids)

	for _, input := range []string{
		`{"data":["  ",""]}`,
		`{"data":[{"id":"   ","name":"  "}]}`,
	} {
		ids, err := parseGrokCatalogModelIDs([]byte(input))
		require.Error(t, err)
		require.Empty(t, ids)
	}
}

func TestGrokCatalogParserKeepsFieldPriorityAndModelsPrefix(t *testing.T) {
	t.Parallel()

	priority := `{
		"data":[
			{"name":"display-name","slug":"slug-id","id":"id-val","model_id":"snake-id","modelId":"camel-id","model":"model-id"},
			{"name":"display-name","slug":"slug-id","id":"id-val","model_id":"snake-id","modelId":"camel-id"},
			{"name":"display-name","slug":"slug-id","id":"id-val","model_id":"snake-id"},
			{"name":"display-name","slug":"slug-id","id":"id-val"},
			{"name":"display-name","slug":"slug-id"},
			{"name":"display-name","_meta":{"slug":"meta-slug","model":"meta-model"}},
			{"name":"display-name","_meta":{"name":"meta-name"}},
			{"name":"display-name"}
		]
	}`
	want := []string{"camel-id", "display-name", "id-val", "meta-model", "meta-name", "model-id", "slug-id", "snake-id"}
	manual, err := extractGrokUpstreamModelIDs([]byte(priority))
	require.NoError(t, err)
	observed, err := extractGrokModelIDsFromModelsBody([]byte(priority))
	require.NoError(t, err)
	require.Equal(t, want, manual)
	require.Equal(t, manual, observed)

	prefixed := `{"data":["grok-4.5"],"models":[{"model":"models/grok-4.7"},{"id":"  models/grok-4.6  "}]}`
	manual, err = extractGrokUpstreamModelIDs([]byte(prefixed))
	require.NoError(t, err)
	observed, err = extractGrokModelIDsFromModelsBody([]byte(prefixed))
	require.NoError(t, err)
	require.Equal(t, []string{"grok-4.5", "grok-4.6", "grok-4.7"}, manual)
	require.Equal(t, manual, observed)

	literal, err := parseGrokCatalogModelIDs([]byte(`["models/grok-4.5"]`))
	require.NoError(t, err)
	require.Equal(t, []string{"models/grok-4.5"}, literal)
}

func TestGrokCatalogScopeSanitizesURLAndSeparatesQuerySources(t *testing.T) {
	t.Parallel()

	scope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	require.Equal(t, int64(25), scope.AccountID)
	require.Equal(t, AccountTypeOAuth, scope.AccountType)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", scope.Endpoint)
	require.Equal(t, "4f0a3c6f55ad161d4e89ce04ab61826abe5f8c42e432782f8a799514b0d020b9", scope.Fingerprint)

	cased, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://User:s3cret@CLI-CHAT-PROXY.grok.com/v1/models/#frag")
	require.NoError(t, err)
	require.Equal(t, scope.Endpoint, cased.Endpoint)
	require.Equal(t, scope.Fingerprint, cased.Fingerprint)
	require.NotContains(t, cased.Endpoint, "s3cret")
	require.NotContains(t, cased.Endpoint, "User")
	require.NotContains(t, cased.Fingerprint, "s3cret")
	require.NotContains(t, cased.Endpoint, "@")
	require.NotContains(t, cased.Endpoint, "#")
	require.NotContains(t, cased.Endpoint, "?")

	trailing, err := NewGrokCatalogScope(25, AccountTypeOAuth, "HTTPS://CLI-CHAT-PROXY.grok.com/v1/models/")
	require.NoError(t, err)
	require.Equal(t, scope.Endpoint, trailing.Endpoint)
	require.Equal(t, scope.Fingerprint, trailing.Fingerprint)

	withQuery, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://user:s3cret@cli-chat-proxy.grok.com/v1/models?foo=1#frag")
	require.NoError(t, err)
	require.Equal(t, scope.Endpoint, withQuery.Endpoint)
	require.Equal(t, "5e5c9708071c7d686bb14327cd19a2302fb6ee6a0205b39d3e79ece45103d9e8", withQuery.Fingerprint)
	require.NotContains(t, withQuery.Endpoint, "foo")
	require.NotContains(t, withQuery.Endpoint, "s3cret")
	require.NotContains(t, withQuery.Fingerprint, "s3cret")

	otherQuery, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models?foo=2")
	require.NoError(t, err)
	require.Equal(t, withQuery.Endpoint, otherQuery.Endpoint)
	require.NotEqual(t, withQuery.Fingerprint, otherQuery.Fingerprint)
	require.Equal(t, "b15abf83f8afe57da5762b87d1f0088f9d6fc9824dbcf37395246486baea7a4f", otherQuery.Fingerprint)

	otherAccount, err := NewGrokCatalogScope(26, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	require.NotEqual(t, scope.Fingerprint, otherAccount.Fingerprint)
	require.Equal(t, "2fbf3925d18e8f57fc3ce9ba1d220b980f5aae12f839a872e91d4254bf0455f5", otherAccount.Fingerprint)

	otherType, err := NewGrokCatalogScope(25, AccountTypeAPIKey, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	require.NotEqual(t, scope.Fingerprint, otherType.Fingerprint)
	require.Equal(t, "0d5edf88280f126ae1b1a05caff23134583dd5a372e10aa15ab2baae693d68f7", otherType.Fingerprint)

	pathCase, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/Models")
	require.NoError(t, err)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/Models", pathCase.Endpoint)
	require.NotEqual(t, scope.Fingerprint, pathCase.Fingerprint)
	require.Equal(t, "b46e0a9625f3301f6fe281e669f586a08eb1952560207f929d99bb98e29d35ef", pathCase.Fingerprint)

	_, err = NewGrokCatalogScope(25, AccountTypeOAuth, "https://user:s3cret@/v1/models")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "s3cret")
	_, err = NewGrokCatalogScope(25, AccountTypeOAuth, "ftp://cli-chat-proxy.grok.com/v1/models")
	require.Error(t, err)
	_, err = NewGrokCatalogScope(25, AccountTypeOAuth, "/v1/models")
	require.Error(t, err)
	_, err = NewGrokCatalogScope(25, AccountTypeOAuth, "")
	require.Error(t, err)
}

func TestGrokCatalogMergeKeepsNewerSuccessWhenOlderArrivesLater(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	scope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	older := GrokCatalogObservation{Models: []string{"old"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Format(time.RFC3339Nano), FinishedAt: t0.Add(3 * time.Second).Format(time.RFC3339Nano), Status: "success", Scope: scope,
	}}
	newer := GrokCatalogObservation{Models: []string{"new"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Add(time.Second).Format(time.RFC3339Nano), FinishedAt: t0.Add(2 * time.Second).Format(time.RFC3339Nano), Status: "success", Scope: scope,
	}}
	latest, changed, err := MergeGrokCatalogObservation(nil, newer)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"new"}, latest.Models)
	require.Equal(t, newer.Attempt.FinishedAt, latest.FetchedAt)
	require.Equal(t, newer.Attempt.StartedAt, latest.SuccessStartedAt)
	require.Equal(t, "success", latest.LastAttempt.Status)
	require.Equal(t, scope.Fingerprint, latest.Scope.Fingerprint)

	merged, changed, err := MergeGrokCatalogObservation(latest, older)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, []string{"new"}, merged.Models)
	require.Equal(t, latest.FetchedAt, merged.FetchedAt)
	require.Equal(t, latest.SuccessStartedAt, merged.SuccessStartedAt)
	require.Equal(t, latest.LastAttempt.StartedAt, merged.LastAttempt.StartedAt)

	replaced, changed, err := MergeGrokCatalogObservation(merged, newer)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, []string{"new"}, replaced.Models)

	first, _, err := MergeGrokCatalogObservation(nil, older)
	require.NoError(t, err)
	advanced, changed, err := MergeGrokCatalogObservation(first, newer)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"new"}, advanced.Models)
	require.Equal(t, newer.Attempt.FinishedAt, advanced.FetchedAt)
	require.Equal(t, newer.Attempt.StartedAt, advanced.SuccessStartedAt)
	require.Equal(t, newer.Attempt.StartedAt, advanced.LastAttempt.StartedAt)
	require.Equal(t, scope.Fingerprint, advanced.Scope.Fingerprint)
}

func TestGrokCatalogMergeFailurePreservesSuccessScope(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	oldScope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	newScope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://api.x.ai/v1/models?key=secret-key")
	require.NoError(t, err)
	require.Equal(t, "https://api.x.ai/v1/models", newScope.Endpoint)
	require.NotContains(t, newScope.Endpoint, "secret-key")
	require.NotContains(t, newScope.Fingerprint, "secret-key")
	require.NotEqual(t, oldScope.Fingerprint, newScope.Fingerprint)

	success := GrokCatalogObservation{Models: []string{"new"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Format(time.RFC3339Nano), FinishedAt: t0.Add(2 * time.Second).Format(time.RFC3339Nano), Status: "success", Scope: oldScope,
	}}
	latest, _, err := MergeGrokCatalogObservation(nil, success)
	require.NoError(t, err)

	failure := GrokCatalogObservation{Models: []string{"should-not-apply"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Add(3 * time.Second).Format(time.RFC3339Nano), FinishedAt: t0.Add(4 * time.Second).Format(time.RFC3339Nano),
		Status: "failed", ErrorCode: "http_error", Scope: newScope,
	}}
	merged, changed, err := MergeGrokCatalogObservation(latest, failure)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"new"}, merged.Models)
	require.Equal(t, latest.FetchedAt, merged.FetchedAt)
	require.Equal(t, latest.SuccessStartedAt, merged.SuccessStartedAt)
	require.Equal(t, oldScope.Endpoint, merged.Scope.Endpoint)
	require.Equal(t, oldScope.Fingerprint, merged.Scope.Fingerprint)
	require.Equal(t, "failed", merged.LastAttempt.Status)
	require.Equal(t, "http_error", merged.LastAttempt.ErrorCode)
	require.Equal(t, newScope.Endpoint, merged.LastAttempt.Scope.Endpoint)
	require.Equal(t, newScope.Fingerprint, merged.LastAttempt.Scope.Fingerprint)
}

func TestGrokCatalogMergeEarlierSuccessDoesNotOverwriteNewerAttempt(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	scope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	failure := GrokCatalogObservation{Models: []string{"failed-models"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Add(time.Second).Format(time.RFC3339Nano), FinishedAt: t0.Add(2 * time.Second).Format(time.RFC3339Nano),
		Status: "failed", ErrorCode: "transport", Scope: scope,
	}}
	failedFirst, changed, err := MergeGrokCatalogObservation(nil, failure)
	require.NoError(t, err)
	require.True(t, changed)
	require.Empty(t, failedFirst.Models)
	require.Empty(t, failedFirst.FetchedAt)
	require.Empty(t, failedFirst.SuccessStartedAt)
	require.Nil(t, failedFirst.Scope)
	require.Equal(t, "failed", failedFirst.LastAttempt.Status)

	earlier := GrokCatalogObservation{Models: []string{"old"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Format(time.RFC3339Nano), FinishedAt: t0.Add(3 * time.Second).Format(time.RFC3339Nano), Status: "success", Scope: scope,
	}}
	merged, changed, err := MergeGrokCatalogObservation(failedFirst, earlier)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"old"}, merged.Models)
	require.Equal(t, earlier.Attempt.FinishedAt, merged.FetchedAt)
	require.Equal(t, earlier.Attempt.StartedAt, merged.SuccessStartedAt)
	require.Equal(t, scope.Fingerprint, merged.Scope.Fingerprint)
	require.Equal(t, "failed", merged.LastAttempt.Status)
	require.Equal(t, failure.Attempt.StartedAt, merged.LastAttempt.StartedAt)
	require.Equal(t, "transport", merged.LastAttempt.ErrorCode)
}

func TestGrokCatalogMergeConfiguredFallbackDoesNotRefreshSuccess(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	oldScope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	newScope, err := NewGrokCatalogScope(25, AccountTypeAPIKey, "https://api.x.ai/v1/models")
	require.NoError(t, err)
	success := GrokCatalogObservation{Models: []string{"new"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Format(time.RFC3339Nano), FinishedAt: t0.Add(2 * time.Second).Format(time.RFC3339Nano), Status: "success", Scope: oldScope,
	}}
	latest, _, err := MergeGrokCatalogObservation(nil, success)
	require.NoError(t, err)

	fallback := GrokCatalogObservation{Models: []string{"configured-id"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Add(5 * time.Second).Format(time.RFC3339Nano), FinishedAt: t0.Add(6 * time.Second).Format(time.RFC3339Nano),
		Status: "configured_fallback", Scope: newScope,
	}}
	merged, changed, err := MergeGrokCatalogObservation(latest, fallback)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"new"}, merged.Models)
	require.Equal(t, latest.FetchedAt, merged.FetchedAt)
	require.Equal(t, latest.SuccessStartedAt, merged.SuccessStartedAt)
	require.Equal(t, oldScope.Fingerprint, merged.Scope.Fingerprint)
	require.Equal(t, "configured_fallback", merged.LastAttempt.Status)
	require.Equal(t, newScope.Fingerprint, merged.LastAttempt.Scope.Fingerprint)
}

func TestGrokCatalogMergeDuplicateIsIdempotent(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	scope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	newer := GrokCatalogObservation{Models: []string{"new"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Add(time.Second).Format(time.RFC3339Nano), FinishedAt: t0.Add(2 * time.Second).Format(time.RFC3339Nano), Status: "success", Scope: scope,
	}}
	latest, _, err := MergeGrokCatalogObservation(nil, newer)
	require.NoError(t, err)
	again, changed, err := MergeGrokCatalogObservation(latest, newer)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, latest.Models, again.Models)
	require.Equal(t, latest.FetchedAt, again.FetchedAt)
	require.Equal(t, latest.SuccessStartedAt, again.SuccessStartedAt)
	require.Equal(t, latest.Scope.Fingerprint, again.Scope.Fingerprint)
	require.Equal(t, latest.LastAttempt.StartedAt, again.LastAttempt.StartedAt)
	require.NotSame(t, latest, again)
}

func TestGrokCatalogMergeDoesNotMutateInputs(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	scope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	prevModels := []string{"new"}
	prevScope := *scope
	prevAttempt := GrokCatalogAttempt{
		StartedAt: t0.Add(time.Second).Format(time.RFC3339Nano), FinishedAt: t0.Add(2 * time.Second).Format(time.RFC3339Nano),
		Status: "success", Scope: &prevScope,
	}
	prev := &GrokObservedModelsSnapshot{
		Models: prevModels, FetchedAt: prevAttempt.FinishedAt, SuccessStartedAt: prevAttempt.StartedAt,
		Source: "upstream_v1_models", Scope: &prevScope, LastAttempt: &prevAttempt,
	}
	obsModels := []string{"old"}
	obsScope := *scope
	older := GrokCatalogObservation{Models: obsModels, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Format(time.RFC3339Nano), FinishedAt: t0.Add(3 * time.Second).Format(time.RFC3339Nano), Status: "success", Scope: &obsScope,
	}}

	merged, changed, err := MergeGrokCatalogObservation(prev, older)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, []string{"new"}, merged.Models)

	prevModels[0] = "mutated-prev"
	obsModels[0] = "mutated-obs"
	prevScope.Endpoint = "https://mutated.example/v1/models"
	obsScope.Endpoint = "https://mutated-obs.example/v1/models"
	prevAttempt.Status = "failed"

	require.Equal(t, []string{"new"}, merged.Models)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", merged.Scope.Endpoint)
	require.Equal(t, "success", merged.LastAttempt.Status)
	require.Equal(t, "https://cli-chat-proxy.grok.com/v1/models", merged.LastAttempt.Scope.Endpoint)

	merged.Models[0] = "mutated-result"
	merged.Scope.Endpoint = "https://result.example/v1/models"
	require.Equal(t, "mutated-prev", prev.Models[0])
	require.Equal(t, "https://mutated.example/v1/models", prev.Scope.Endpoint)
}

func TestGrokCatalogMergeEqualTimestampDoesNotFlip(t *testing.T) {
	t.Parallel()

	scope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	first := GrokCatalogObservation{Models: []string{"kept"}, Attempt: GrokCatalogAttempt{
		StartedAt: "2026-10-25T02:00:00Z", FinishedAt: "2026-10-25T02:00:01Z", Status: "success", Scope: scope,
	}}
	sameInstant := GrokCatalogObservation{Models: []string{"other"}, Attempt: GrokCatalogAttempt{
		StartedAt: "2026-10-25T03:00:00+01:00", FinishedAt: "2026-10-25T03:00:05+01:00", Status: "success", Scope: scope,
	}}

	prev, _, err := MergeGrokCatalogObservation(nil, first)
	require.NoError(t, err)
	merged, changed, err := MergeGrokCatalogObservation(prev, sameInstant)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, []string{"kept"}, merged.Models)
	require.Equal(t, "2026-10-25T02:00:01Z", merged.FetchedAt)
	require.Equal(t, "2026-10-25T02:00:00Z", merged.LastAttempt.StartedAt)

	reverse, _, err := MergeGrokCatalogObservation(nil, sameInstant)
	require.NoError(t, err)
	merged, changed, err = MergeGrokCatalogObservation(reverse, first)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, []string{"other"}, merged.Models)
	require.Equal(t, "2026-10-25T03:00:05+01:00", merged.FetchedAt)
}

func TestGrokCatalogMergeRejectsIllegalObservation(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	scope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	prevModels := []string{"new"}
	prev := &GrokObservedModelsSnapshot{
		Models: prevModels, FetchedAt: t0.Add(2 * time.Second).Format(time.RFC3339Nano),
		SuccessStartedAt: t0.Add(time.Second).Format(time.RFC3339Nano), Scope: scope,
	}
	validStart := t0.Format(time.RFC3339Nano)
	validFinish := t0.Add(time.Second).Format(time.RFC3339Nano)
	cases := []GrokCatalogObservation{
		{Models: []string{"new"}, Attempt: GrokCatalogAttempt{StartedAt: validStart, FinishedAt: validFinish, Status: "ok", Scope: scope}},
		{Models: []string{"new"}, Attempt: GrokCatalogAttempt{StartedAt: validStart, FinishedAt: validFinish, Status: "success", ErrorCode: "dial tcp: connection refused", Scope: scope}},
		{Models: []string{"new"}, Attempt: GrokCatalogAttempt{StartedAt: "yesterday", FinishedAt: validFinish, Status: "failed"}},
		{Models: []string{"new"}, Attempt: GrokCatalogAttempt{StartedAt: validStart, FinishedAt: "nope", Status: "failed"}},
		{Models: []string{"new"}, Attempt: GrokCatalogAttempt{StartedAt: validFinish, FinishedAt: validStart, Status: "failed"}},
		{Models: []string{"new"}, Attempt: GrokCatalogAttempt{StartedAt: "2026-10-25T02:00:00Z", FinishedAt: "2026-10-25T03:00:00+02:00", Status: "failed"}},
		{Models: nil, Attempt: GrokCatalogAttempt{StartedAt: validStart, FinishedAt: validFinish, Status: "success", Scope: scope}},
		{Models: []string{" ", ""}, Attempt: GrokCatalogAttempt{StartedAt: validStart, FinishedAt: validFinish, Status: "success", Scope: scope}},
		{Models: []string{"grok-4.7"}, Attempt: GrokCatalogAttempt{StartedAt: validStart, FinishedAt: validFinish, Status: "success"}},
	}
	for _, observation := range cases {
		got, changed, err := MergeGrokCatalogObservation(prev, observation)
		require.Error(t, err)
		require.Nil(t, got)
		require.False(t, changed)
		require.NotContains(t, err.Error(), "connection refused")
		require.Equal(t, []string{"new"}, prev.Models)
	}
	prevModels[0] = "mutated"
	require.Equal(t, "mutated", prev.Models[0])
}

func TestGrokCatalogMergeLegacySnapshotAcceptsSuccess(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 10, 25, 2, 0, 0, 0, time.UTC)
	scope, err := NewGrokCatalogScope(25, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1/models")
	require.NoError(t, err)
	legacy := &GrokObservedModelsSnapshot{
		Models: []string{"legacy"}, FetchedAt: t0.Format(time.RFC3339Nano), Source: "upstream_v1_models",
	}
	success := GrokCatalogObservation{Models: []string{"new"}, Attempt: GrokCatalogAttempt{
		StartedAt: t0.Add(time.Second).Format(time.RFC3339Nano), FinishedAt: t0.Add(2 * time.Second).Format(time.RFC3339Nano), Status: "success", Scope: scope,
	}}
	merged, changed, err := MergeGrokCatalogObservation(legacy, success)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{"new"}, merged.Models)
	require.Equal(t, success.Attempt.FinishedAt, merged.FetchedAt)
	require.Equal(t, success.Attempt.StartedAt, merged.SuccessStartedAt)
	require.Equal(t, scope.Fingerprint, merged.Scope.Fingerprint)
	require.Equal(t, "upstream_v1_models", merged.Source)
	require.Equal(t, []string{"legacy"}, legacy.Models)
	require.Nil(t, legacy.Scope)
}

func TestGrokCatalogParseReadsLegacySnapshotWithoutScope(t *testing.T) {
	t.Parallel()

	inner := map[string]any{
		"models":     []any{"grok-4.7", "grok-4.6"},
		"fetched_at": "2026-10-25T02:00:00Z",
		"source":     "upstream_v1_models",
	}
	extra := map[string]any{grokObservedModelsExtraKey: inner}
	snap := parseGrokObservedModels(extra)
	require.NotNil(t, snap)
	require.Equal(t, []string{"grok-4.7", "grok-4.6"}, snap.Models)
	require.Equal(t, "2026-10-25T02:00:00Z", snap.FetchedAt)
	require.Equal(t, "upstream_v1_models", snap.Source)
	require.Nil(t, snap.Scope)
	require.Nil(t, snap.LastAttempt)
	require.Empty(t, snap.SuccessStartedAt)
	require.Equal(t, "upstream_v1_models", inner["source"])
	_, ok := inner["models"]
	require.True(t, ok)
}

func TestGrokCatalogParseReadsLastAttemptWithoutModels(t *testing.T) {
	t.Parallel()

	extra := map[string]any{
		grokObservedModelsExtraKey: map[string]any{
			"last_attempt": map[string]any{
				"started_at":  "2026-10-25T02:00:01Z",
				"finished_at": "2026-10-25T02:00:02Z",
				"status":      "failed",
				"error_code":  "transport",
			},
		},
	}
	snap := parseGrokObservedModels(extra)
	require.NotNil(t, snap)
	require.Empty(t, snap.Models)
	require.NotNil(t, snap.LastAttempt)
	require.Equal(t, "failed", snap.LastAttempt.Status)
	require.Equal(t, "transport", snap.LastAttempt.ErrorCode)
	require.Nil(t, snap.Scope)

	require.Nil(t, parseGrokObservedModels(map[string]any{
		grokObservedModelsExtraKey: map[string]any{},
	}))
	require.Nil(t, parseGrokObservedModels(map[string]any{
		grokObservedModelsExtraKey: map[string]any{"models": []any{}},
	}))
}

func TestGrokCatalogParserDoesNotStringifyObjectEntries(t *testing.T) {
	t.Parallel()

	body := []byte(`{"data":[{"id":{"nested":true},"name":{"label":"nope"}},1,true,null]}`)
	for _, parse := range []func([]byte) ([]string, error){
		parseGrokCatalogModelIDs,
		extractGrokUpstreamModelIDs,
		extractGrokModelIDsFromModelsBody,
	} {
		ids, err := parse(body)
		require.Error(t, err)
		require.Empty(t, ids)
		require.NotContains(t, err.Error(), "nested")
		require.NotContains(t, err.Error(), "nope")
		for _, id := range ids {
			require.NotContains(t, id, "{")
			require.NotContains(t, id, "nested")
		}
	}
}

func TestExtractUpstreamModelIDsIgnoresGrokCatalogRules(t *testing.T) {
	t.Parallel()

	got, err := extractUpstreamModelIDs([]byte(`{"error":{"message":"denied"}}`))
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = extractUpstreamModelIDs([]byte(`{"data":[]}`))
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = extractUpstreamModelIDs([]byte(`{"data":[{"id":"canonical-id","model":"display-model"}]}`))
	require.NoError(t, err)
	require.Equal(t, []string{"canonical-id"}, got)
}
