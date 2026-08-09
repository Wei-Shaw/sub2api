//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyOpsError(t *testing.T) {
	t.Parallel()

	intPtr := func(value int) *int { return &value }
	tests := []struct {
		name           string
		input          OpsErrorClassificationInput
		outcome        string
		responsibility string
		category       string
		family         string
		sla            bool
	}{
		{
			name:    "platform capacity is not hidden by legacy business flag",
			input:   OpsErrorClassificationInput{StatusCode: 503, ErrorPhase: "routing", ErrorMessage: "No available accounts", IsBusinessLimited: true},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCapacity, family: OpsAlertFamilyCapacity, sla: true,
		},
		{
			name:    "routing availability failure is not hidden by legacy business flag",
			input:   OpsErrorClassificationInput{StatusCode: 503, ErrorPhase: "routing", ErrorSource: "gateway", ErrorOwner: "platform", ErrorMessage: "Service temporarily unavailable", IsBusinessLimited: true},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformInternal, family: OpsAlertFamilyAvailability, sla: true,
		},
		{
			name:    "account concurrency is platform capacity",
			input:   OpsErrorClassificationInput{StatusCode: 429, ErrorMessage: "Concurrency limit exceeded for account, please retry later", IsBusinessLimited: true},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCapacity, family: OpsAlertFamilyCapacity, sla: true,
		},
		{
			name:    "stale upstream status does not hide account pool capacity",
			input:   OpsErrorClassificationInput{StatusCode: 499, UpstreamStatusCode: intPtr(503), ErrorPhase: "upstream", ErrorOwner: "platform", ErrorMessage: "No available accounts"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCapacity, family: OpsAlertFamilyCapacity, sla: true,
		},
		{
			name:    "stale upstream rate limit does not hide account concurrency",
			input:   OpsErrorClassificationInput{StatusCode: 499, UpstreamStatusCode: intPtr(429), ErrorPhase: "upstream", ErrorOwner: "platform", ErrorMessage: "Concurrency limit exceeded for account"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCapacity, family: OpsAlertFamilyCapacity, sla: true,
		},
		{
			name:    "user concurrency is a business limit",
			input:   OpsErrorClassificationInput{StatusCode: 429, ErrorMessage: "Concurrency limit exceeded for user, please retry later", IsBusinessLimited: true},
			outcome: OpsFinalOutcomeBusinessLimited, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryUserConcurrency, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "upstream invalid request exposed as 502 is compatibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(400), ErrorPhase: "upstream", ErrorType: "invalid_request_error"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryProductCompatibility, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "upstream invalid request preserved as 400 is client rejected",
			input:   OpsErrorClassificationInput{StatusCode: 400, UpstreamStatusCode: intPtr(400), ErrorPhase: "upstream", ErrorType: "invalid_request_error"},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryInvalidRequest, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "empty input exposed as gateway failure is compatibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(400), ErrorPhase: "upstream", ErrorType: "api_error", UpstreamMessage: "Empty input messages"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryProductCompatibility, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "invalid thinking signature is request semantics",
			input:   OpsErrorClassificationInput{StatusCode: 400, UpstreamStatusCode: intPtr(400), ErrorPhase: "upstream", ErrorType: "api_error", UpstreamMessage: "messages.1.content.0: Invalid `signature` in `thinking` block"},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryInvalidRequest, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "upstream workspace failure is provider health not client rejection",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(402), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: `{"code":"deactivated_workspace"}`},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderServer, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "explicit upstream error type is not treated as client request",
			input:   OpsErrorClassificationInput{StatusCode: 502, ErrorPhase: "request", ErrorType: "upstream_error", ErrorSource: "client_request", ErrorMessage: "Upstream request failed"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryNetworkTransport, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "embedded upstream credential rejection is platform responsibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, ErrorType: "upstream_error", ErrorMessage: `codex models manifest upstream error 403: {"code":"INSUFFICIENT_BALANCE"}`},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCapacity, family: OpsAlertFamilyCapacity, sla: true,
		},
		{
			name:    "recovered upstream workspace failure remains provider attributed",
			input:   OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: intPtr(402), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: `{"code":"deactivated_workspace"}`, Recovered: true},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyProviderHealth,
		},
		{
			name:    "gateway 499 does not hide upstream rate limit",
			input:   OpsErrorClassificationInput{StatusCode: 499, UpstreamStatusCode: intPtr(429), ErrorPhase: "upstream", ErrorType: "api_error", UpstreamMessage: "Upstream rate limit exceeded"},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderRateLimit, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "gateway 499 does not hide upstream timeout",
			input:   OpsErrorClassificationInput{StatusCode: 499, UpstreamStatusCode: intPtr(504), ErrorPhase: "upstream", ErrorType: "api_error"},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderServer, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "recovered upstream invalid request is platform compatibility not client fault",
			input:   OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: intPtr(400), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "invalid_request_error", Recovered: true},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "recovered semantic failure with upstream 503 remains platform compatibility",
			input:   OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: intPtr(503), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "maximum context length exceeded", Recovered: true},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "upstream unsupported model is provider failure not local client rejection",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(404), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "model is not supported by this provider"},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderServer, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "recovered upstream unsupported model is compatibility signal",
			input:   OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: intPtr(400), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "model is not supported by this provider", Recovered: true},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "managed credential rejection is platform responsibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(401), ErrorPhase: "upstream", ErrorMessage: "OAuth access token has been revoked"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCredential, family: OpsAlertFamilyCredential, sla: true,
		},
		{
			name:    "Chinese upstream credential rejection is platform responsibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(403), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "认证失败，请重新登录或检查 API Key"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCredential, family: OpsAlertFamilyCredential, sla: true,
		},
		{
			name:    "recovered managed credential rejection remains a credential signal",
			input:   OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: intPtr(403), ErrorPhase: "upstream", ErrorType: "upstream_error", ErrorMessage: "OAuth access token expired", Recovered: true},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyCredential,
		},
		{
			name:    "provider overload is provider failure",
			input:   OpsErrorClassificationInput{StatusCode: 503, UpstreamStatusCode: intPtr(529), ErrorPhase: "upstream", ErrorType: "overloaded_error"},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderOverloaded, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "upstream insufficient balance is capacity not credential",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(403), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "insufficient balance"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCapacity, family: OpsAlertFamilyCapacity, sla: true,
		},
		{
			name:    "Chinese upstream balance rejection is capacity not credential",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(403), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "预扣费额度失败, 用户剩余额度不足"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCapacity, family: OpsAlertFamilyCapacity, sla: true,
		},
		{
			name:    "upstream capability disabled is compatibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(403), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "Image generation is not enabled for this group"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryProductCompatibility, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "unknown upstream 403 remains provider-owned",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(403), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "provider rejected request"},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderServer, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "provider rate limit is provider failure",
			input:   OpsErrorClassificationInput{StatusCode: 429, UpstreamStatusCode: intPtr(429), ErrorPhase: "upstream", ErrorMessage: "Upstream rate limit exceeded"},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderRateLimit, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "client cancellation is excluded",
			input:   OpsErrorClassificationInput{StatusCode: 499, ErrorMessage: "context canceled"},
			outcome: OpsFinalOutcomeCancelled, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryClientCancelled, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "upstream broken pipe is transport failure not client cancellation",
			input:   OpsErrorClassificationInput{StatusCode: 499, ErrorPhase: "upstream", ErrorType: "upstream_error", ErrorMessage: "upstream stream disconnected: broken pipe"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryNetworkTransport, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "recovered provider server error is a non SLA signal",
			input:   OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: intPtr(503), ErrorPhase: "upstream", Recovered: true},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyProviderHealth,
		},
		{
			name:    "recovered upstream phase without status remains provider signal",
			input:   OpsErrorClassificationInput{StatusCode: 200, ErrorPhase: "upstream", ErrorType: "upstream_error", ErrorMessage: "Recovered upstream error: connection reset", Recovered: true},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyProviderHealth,
		},
		{
			name:    "recovered client cancellation stays client quality",
			input:   OpsErrorClassificationInput{StatusCode: 200, ErrorPhase: "upstream", ErrorMessage: "context canceled", Recovered: true},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "unsupported model is compatibility not availability",
			input:   OpsErrorClassificationInput{StatusCode: 404, ErrorMessage: `Model "example" is not supported by any configured account`},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryUnsupportedModel, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "channel model restriction is client policy",
			input:   OpsErrorClassificationInput{StatusCode: 404, ErrorPhase: "routing", ErrorType: "api_error", ErrorMessage: `Model "gpt-5.6-luna" is not available for this group`},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryClientPolicy, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "stale upstream status does not hide local group model configuration",
			input:   OpsErrorClassificationInput{StatusCode: 404, UpstreamStatusCode: intPtr(503), ErrorPhase: "upstream", ErrorType: "model_not_found", ErrorMessage: `Model "example" is not supported by any configured account in this group`},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryUnsupportedModel, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "internal unsupported model remains a client compatibility error",
			input:   OpsErrorClassificationInput{StatusCode: 404, ErrorPhase: "internal", ErrorSource: "gateway", ErrorOwner: "platform", ErrorMessage: `Model "example" is not supported by any configured account`},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryUnsupportedModel, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "local authentication is client rejected",
			input:   OpsErrorClassificationInput{StatusCode: 401, ErrorPhase: "auth", ErrorType: "authentication_error"},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryClientAuth, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "legacy business flag does not turn invalid key into quota",
			input:   OpsErrorClassificationInput{StatusCode: 403, ErrorPhase: "auth", ErrorMessage: "Invalid API key", IsBusinessLimited: true},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryClientAuth, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "balance rejection remains business limited even in auth phase",
			input:   OpsErrorClassificationInput{StatusCode: 403, ErrorPhase: "auth", ErrorType: "billing_error", ErrorMessage: "Insufficient account balance", IsBusinessLimited: true},
			outcome: OpsFinalOutcomeBusinessLimited, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryUserQuota, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "API key auth account balance wording is business limited",
			input:   OpsErrorClassificationInput{StatusCode: 403, ErrorPhase: "auth", ErrorType: "api_error", ErrorMessage: "Insufficient account balance", IsBusinessLimited: true},
			outcome: OpsFinalOutcomeBusinessLimited, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryUserQuota, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "subscription rejection is distinct from quota",
			input:   OpsErrorClassificationInput{StatusCode: 403, ErrorPhase: "auth", ErrorType: "subscription_error", ErrorMessage: "No active subscription found for this group", IsBusinessLimited: true},
			outcome: OpsFinalOutcomeBusinessLimited, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryUserSubscription, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "local feature gate is policy not quota",
			input:   OpsErrorClassificationInput{StatusCode: 403, ErrorPhase: "auth", ErrorMessage: "Images API is not supported for this platform", IsBusinessLimited: true},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryClientPolicy, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "preserved upstream rate limit works without separate status",
			input:   OpsErrorClassificationInput{StatusCode: 429, ErrorPhase: "upstream", ErrorType: "rate_limit_error"},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderRateLimit, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "internal 500 is platform failure",
			input:   OpsErrorClassificationInput{StatusCode: 500, ErrorPhase: "internal"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformInternal, family: OpsAlertFamilyAvailability, sla: true,
		},
		{
			name:    "security rejection is separated",
			input:   OpsErrorClassificationInput{StatusCode: 403, ErrorType: "cyber_policy"},
			outcome: OpsFinalOutcomeSecurityBlocked, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategorySecurityPolicy, family: OpsAlertFamilySecurity,
		},
		{
			name:    "security session block is separated from legacy business policy",
			input:   OpsErrorClassificationInput{StatusCode: 403, ErrorPhase: "request", ErrorType: "cyber_policy_session_blocked", ErrorMessage: "request rejected locally by session block", IsBusinessLimited: true},
			outcome: OpsFinalOutcomeSecurityBlocked, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategorySecurityPolicy, family: OpsAlertFamilySecurity,
		},
		{
			name:    "upstream content policy rejection keeps provider ownership",
			input:   OpsErrorClassificationInput{StatusCode: 400, UpstreamStatusCode: intPtr(400), ErrorPhase: "upstream", ErrorType: "api_error", UpstreamMessage: "content policy rejection"},
			outcome: OpsFinalOutcomeSecurityBlocked, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategorySecurityPolicy, family: OpsAlertFamilySecurity,
		},
		{
			name:    "upstream cyber policy type keeps provider ownership",
			input:   OpsErrorClassificationInput{StatusCode: 502, ErrorPhase: "request", ErrorSource: "upstream_http", ErrorType: "cyber_policy", UpstreamMessage: "request rejected for cybersecurity risk"},
			outcome: OpsFinalOutcomeSecurityBlocked, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategorySecurityPolicy, family: OpsAlertFamilySecurity,
		},
		{
			name:    "http 200 upstream error is not recovered without explicit evidence",
			input:   OpsErrorClassificationInput{StatusCode: 200, ErrorPhase: "upstream", ErrorType: "upstream_error", ErrorMessage: "Upstream request failed"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryNetworkTransport, family: OpsAlertFamilyProviderHealth, sla: true,
		},
		{
			name:    "http 200 cyber session block remains security blocked",
			input:   OpsErrorClassificationInput{StatusCode: 200, ErrorType: "cyber_policy_session_blocked", Recovered: true},
			outcome: OpsFinalOutcomeSecurityBlocked, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategorySecurityPolicy, family: OpsAlertFamilySecurity,
		},
		{
			name:    "upstream context window hidden behind 502 is compatibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(502), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "Your input exceeds the context window"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryProductCompatibility, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "invalid upstream field hidden behind 502 is compatibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(502), ErrorPhase: "upstream", ErrorType: "upstream_error", UpstreamMessage: "Invalid 'messages[0].content'"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryProductCompatibility, family: OpsAlertFamilyCompatibility,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyOpsError(tt.input)
			require.Equal(t, tt.outcome, got.FinalOutcome)
			require.Equal(t, tt.responsibility, got.Responsibility)
			require.Equal(t, tt.category, got.ErrorCategory)
			require.Equal(t, tt.family, got.AlertFamily)
			require.Equal(t, tt.sla, got.CountsTowardSLA)
			require.Equal(t, OpsErrorClassificationVersion, got.ClassificationVersion)
		})
	}
}

func TestOpsClassificationMetricValue(t *testing.T) {
	t.Parallel()
	stats := &OpsErrorClassificationStats{
		SuccessCount: 90, FinalErrorCount: 35, SLAFailureCount: 10,
		PlatformFailureCount: 6, ProviderFailureCount: 3, UnknownFailureCount: 1,
		PlatformCapacityCount: 4, PlatformCredentialCount: 3, CompatibilityCount: 8, ClientRejectedCount: 12,
		BusinessLimitedCount: 5, CancelledCount: 2, SecurityBlockedCount: 1,
		RecoveredProviderCount: 7,
	}

	value, samples, bad, ok := opsClassificationMetricValue("availability_failure_rate", stats)
	require.True(t, ok)
	require.InDelta(t, 10.0, value, 0.0001)
	require.Equal(t, int64(100), samples)
	require.Equal(t, int64(10), bad)

	value, samples, bad, ok = opsClassificationMetricValue("compatibility_error_count", stats)
	require.True(t, ok)
	require.Equal(t, float64(8), value)
	require.Equal(t, int64(125), samples)
	require.Equal(t, int64(8), bad)

	value, samples, bad, ok = opsClassificationMetricValue("platform_credential_failure_count", stats)
	require.True(t, ok)
	require.Equal(t, int64(125), samples)
	require.InDelta(t, 3.0, value, 0.0001)
	require.Equal(t, int64(3), bad)
}

func TestOpsAlertIncidentKey(t *testing.T) {
	t.Parallel()
	groupID := int64(8)
	region := "us-west"
	rule := &OpsAlertRule{ID: 9, IncidentFamily: "availability"}

	require.Equal(t, "availability|anthropic|8|us-west", opsAlertIncidentKey(rule, "anthropic", &groupID, &region))
	require.Equal(t, "custom_rule_9||0|", opsAlertIncidentKey(&OpsAlertRule{ID: 9, IncidentFamily: "custom"}, "", nil, nil))
	require.Less(t, opsAlertSeverityRank("P0"), opsAlertSeverityRank("P1"))
}

func TestBuildOpsAlertDescriptionIncludesEvidence(t *testing.T) {
	t.Parallel()
	description := buildOpsAlertDescription(
		&OpsAlertRule{MetricType: "availability_failure_rate", Operator: ">=", Threshold: 20},
		25, 40, 10, 5, "openai", nil,
	)
	require.Contains(t, description, "bad=10")
	require.Contains(t, description, "samples=40")
	require.Contains(t, description, "platform=openai")
}
