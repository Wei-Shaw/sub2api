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
			name:    "account concurrency is platform capacity",
			input:   OpsErrorClassificationInput{StatusCode: 429, ErrorMessage: "Concurrency limit exceeded for account, please retry later", IsBusinessLimited: true},
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
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryProductCompatibility, family: OpsAlertFamilyCompatibility,
		},
		{
			name:    "upstream invalid request preserved as 400 is client rejected",
			input:   OpsErrorClassificationInput{StatusCode: 400, UpstreamStatusCode: intPtr(400), ErrorPhase: "upstream", ErrorType: "invalid_request_error"},
			outcome: OpsFinalOutcomeClientRejected, responsibility: OpsResponsibilityClient,
			category: OpsErrorCategoryInvalidRequest, family: OpsAlertFamilyClientQuality,
		},
		{
			name:    "managed credential rejection is platform responsibility",
			input:   OpsErrorClassificationInput{StatusCode: 502, UpstreamStatusCode: intPtr(401), ErrorPhase: "upstream", ErrorMessage: "OAuth access token has been revoked"},
			outcome: OpsFinalOutcomePlatformFailed, responsibility: OpsResponsibilityPlatform,
			category: OpsErrorCategoryPlatformCredential, family: OpsAlertFamilyCapacity, sla: true,
		},
		{
			name:    "provider overload is provider failure",
			input:   OpsErrorClassificationInput{StatusCode: 503, UpstreamStatusCode: intPtr(529), ErrorPhase: "upstream", ErrorType: "overloaded_error"},
			outcome: OpsFinalOutcomeProviderFailed, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryProviderOverloaded, family: OpsAlertFamilyProviderHealth, sla: true,
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
			name:    "recovered provider server error is a non SLA signal",
			input:   OpsErrorClassificationInput{StatusCode: 200, UpstreamStatusCode: intPtr(503), ErrorPhase: "upstream"},
			outcome: OpsFinalOutcomeRecovered, responsibility: OpsResponsibilityProvider,
			category: OpsErrorCategoryRecovered, family: OpsAlertFamilyProviderHealth,
		},
		{
			name:    "recovered client cancellation stays client quality",
			input:   OpsErrorClassificationInput{StatusCode: 200, ErrorPhase: "upstream", ErrorMessage: "context canceled"},
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
		PlatformCapacityCount: 4, CompatibilityCount: 8, ClientRejectedCount: 12,
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
