package service

import "strings"

const (
	OpsErrorClassificationVersion = 3

	OpsFinalOutcomeRecovered       = "recovered"
	OpsFinalOutcomeClientRejected  = "client_rejected"
	OpsFinalOutcomeBusinessLimited = "business_limited"
	OpsFinalOutcomeSecurityBlocked = "security_blocked"
	OpsFinalOutcomeCancelled       = "cancelled"
	OpsFinalOutcomePlatformFailed  = "platform_failed"
	OpsFinalOutcomeProviderFailed  = "provider_failed"
	OpsFinalOutcomeUnknownFailed   = "unknown_failed"

	OpsResponsibilityClient   = "client"
	OpsResponsibilityPlatform = "platform"
	OpsResponsibilityProvider = "provider"
	OpsResponsibilityUnknown  = "unknown"

	OpsAlertFamilyNone           = "none"
	OpsAlertFamilyAvailability   = "availability"
	OpsAlertFamilyCapacity       = "capacity"
	OpsAlertFamilyCredential     = "credential"
	OpsAlertFamilyProviderHealth = "provider_health"
	OpsAlertFamilyCompatibility  = "compatibility"
	OpsAlertFamilyClientQuality  = "client_quality"
	OpsAlertFamilySecurity       = "security"
	OpsAlertFamilyUnknown        = "unknown_failure"
)

const (
	OpsErrorCategoryRecovered            = "recovered"
	OpsErrorCategoryClientAuth           = "client_auth"
	OpsErrorCategoryClientPolicy         = "client_policy"
	OpsErrorCategoryInvalidRequest       = "invalid_request"
	OpsErrorCategoryUnsupportedModel     = "unsupported_model"
	OpsErrorCategoryUserQuota            = "user_quota"
	OpsErrorCategoryUserSubscription     = "user_subscription"
	OpsErrorCategoryUserConcurrency      = "user_concurrency"
	OpsErrorCategoryClientCancelled      = "client_cancelled"
	OpsErrorCategorySecurityPolicy       = "security_policy"
	OpsErrorCategoryPlatformCapacity     = "platform_capacity"
	OpsErrorCategoryPlatformCredential   = "platform_credential"
	OpsErrorCategoryPlatformInternal     = "platform_internal"
	OpsErrorCategoryProviderRateLimit    = "provider_rate_limit"
	OpsErrorCategoryProviderOverloaded   = "provider_overloaded"
	OpsErrorCategoryProviderServer       = "provider_server"
	OpsErrorCategoryNetworkTransport     = "network_transport"
	OpsErrorCategoryProductCompatibility = "product_compatibility"
	OpsErrorCategoryUnknown              = "unknown"
)

type OpsErrorClassificationInput struct {
	StatusCode         int
	UpstreamStatusCode *int
	ErrorPhase         string
	ErrorType          string
	ErrorSource        string
	ErrorOwner         string
	ErrorMessage       string
	UpstreamMessage    string
	IsBusinessLimited  bool
	Recovered          bool
}

type OpsErrorClassification struct {
	FinalOutcome          string
	Responsibility        string
	ErrorCategory         string
	CountsTowardSLA       bool
	AlertFamily           string
	ClassificationReason  string
	ClassificationVersion int
}

func ClassifyOpsError(input OpsErrorClassificationInput) OpsErrorClassification {
	status := input.StatusCode
	upstreamStatus := 0
	if input.UpstreamStatusCode != nil {
		upstreamStatus = *input.UpstreamStatusCode
	}
	phase := strings.ToLower(strings.TrimSpace(input.ErrorPhase))
	errType := strings.ToLower(strings.TrimSpace(input.ErrorType))
	source := strings.ToLower(strings.TrimSpace(input.ErrorSource))
	owner := strings.ToLower(strings.TrimSpace(input.ErrorOwner))
	message := strings.ToLower(strings.TrimSpace(input.ErrorMessage + " " + input.UpstreamMessage))

	result := OpsErrorClassification{
		FinalOutcome:          OpsFinalOutcomeUnknownFailed,
		Responsibility:        OpsResponsibilityUnknown,
		ErrorCategory:         OpsErrorCategoryUnknown,
		AlertFamily:           OpsAlertFamilyUnknown,
		ClassificationReason:  "unclassified_error",
		ClassificationVersion: OpsErrorClassificationVersion,
	}

	upstreamEvidence := isOpsUpstreamEvidence(upstreamStatus, phase, source, owner, errType) ||
		isOpsUpstreamTransportMessage(message)

	// Local cyber/Turnstile decisions are explicit client-facing security
	// blocks. Upstream responses can use the same error type, so require an
	// absence of upstream evidence before assigning client responsibility.
	if !upstreamEvidence && isOpsExplicitLocalSecurityRejection(errType, message) {
		result.FinalOutcome = OpsFinalOutcomeSecurityBlocked
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategorySecurityPolicy
		result.AlertFamily = OpsAlertFamilySecurity
		result.ClassificationReason = "security_policy_rejection"
		return result
	}

	if input.Recovered {
		result.FinalOutcome = OpsFinalOutcomeRecovered
		result.ErrorCategory = OpsErrorCategoryRecovered
		result.CountsTowardSLA = false
		result.AlertFamily = recoveredOpsAlertFamily(upstreamStatus, phase, errType, message)
		result.Responsibility = recoveredOpsResponsibility(upstreamStatus, phase, errType, message)
		result.ClassificationReason = "final_request_recovered"
		if isOpsSecurityPolicyMessage(message) && upstreamEvidence {
			result.AlertFamily = OpsAlertFamilySecurity
			result.Responsibility = OpsResponsibilityProvider
		}
		return result
	}

	// Account-pool exhaustion and gateway routing capacity are platform-owned
	// even when a previous upstream attempt left status evidence on the request.
	if isOpsPlatformCapacityFailure(message) {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryPlatformCapacity
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyCapacity
		result.ClassificationReason = "platform_capacity_unavailable"
		return result
	}

	// This exact diagnostic is produced locally after inspecting the configured
	// account pool. Stale metadata from an earlier upstream attempt must not turn
	// it into a provider SLA failure.
	if isOpsLocalGroupModelUnavailable(message) {
		result.FinalOutcome = OpsFinalOutcomeClientRejected
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryUnsupportedModel
		result.AlertFamily = OpsAlertFamilyCompatibility
		result.ClassificationReason = "unsupported_or_unconfigured_model"
		return result
	}

	// Explicit gateway ownership must win over stale upstream-attempt metadata.
	if status >= 500 && (phase == "routing" || phase == "internal" ||
		(owner == OpsResponsibilityPlatform && source == "gateway")) {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryPlatformInternal
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyAvailability
		result.ClassificationReason = "platform_internal_or_routing_failure"
		return result
	}

	// A gateway 499 may be emitted while closing a connection after an
	// upstream failure. Prefer the explicit upstream status when it exists.
	if isOpsClientCancellation(status, message) && upstreamStatus == 0 &&
		!isOpsUpstreamTransportMessage(message) &&
		(!upstreamEvidence || !strings.Contains(message, "broken pipe")) {
		result.FinalOutcome = OpsFinalOutcomeCancelled
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryClientCancelled
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "client_cancel_or_disconnect"
		return result
	}

	if upstreamEvidence {
		if isOpsSecurityPolicyMessage(message) {
			result.FinalOutcome = OpsFinalOutcomeSecurityBlocked
			result.Responsibility = OpsResponsibilityProvider
			result.ErrorCategory = OpsErrorCategorySecurityPolicy
			result.AlertFamily = OpsAlertFamilySecurity
			result.ClassificationReason = "upstream_security_policy_rejection"
			return result
		}
		effectiveUpstreamStatus := upstreamStatus
		// Older and compatibility paths did not always persist a separate upstream
		// status. A preserved upstream 4xx still carries reliable request semantics;
		// synthetic 5xx values remain transport/platform failures without evidence.
		if effectiveUpstreamStatus == 0 && !isOpsUpstreamTransportMessage(message) &&
			!strings.Contains(message, "broken pipe") &&
			((status >= 400 && status < 500) || status == 529) {
			effectiveUpstreamStatus = status
		}
		return classifyOpsUpstreamFailure(result, status, effectiveUpstreamStatus, phase, errType, message)
	}

	if isOpsSecurityPolicyMessage(message) {
		result.FinalOutcome = OpsFinalOutcomeSecurityBlocked
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategorySecurityPolicy
		result.AlertFamily = OpsAlertFamilySecurity
		result.ClassificationReason = "security_policy_rejection"
		return result
	}

	if isOpsChannelModelRestricted(message) {
		result.FinalOutcome = OpsFinalOutcomeClientRejected
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryClientPolicy
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "channel_model_not_available"
		return result
	}

	if isOpsUnsupportedModel(message) {
		result.FinalOutcome = OpsFinalOutcomeClientRejected
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryUnsupportedModel
		result.AlertFamily = OpsAlertFamilyCompatibility
		result.ClassificationReason = "unsupported_or_unconfigured_model"
		return result
	}

	if isOpsUserBusinessLimit(errType, status, message) {
		result.FinalOutcome = OpsFinalOutcomeBusinessLimited
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryUserQuota
		if isOpsUserSubscription(errType, message) {
			result.ErrorCategory = OpsErrorCategoryUserSubscription
		} else if strings.Contains(message, "concurrency limit exceeded for user") {
			result.ErrorCategory = OpsErrorCategoryUserConcurrency
		}
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "user_plan_quota_or_concurrency_limit"
		return result
	}

	// The legacy flag also covered local IP, client, group and feature gates.
	// Keep them excluded from infrastructure SLA without calling them quota.
	if input.IsBusinessLimited && !isOpsExplicitClientAuth(errType, status, message) {
		result.FinalOutcome = OpsFinalOutcomeClientRejected
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryClientPolicy
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "local_access_or_feature_policy"
		return result
	}

	if isOpsLocalClientAuth(phase, errType, status, message) {
		result.FinalOutcome = OpsFinalOutcomeClientRejected
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryClientAuth
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "local_authentication_or_permission_rejection"
		return result
	}

	if isOpsClientRequestStatus(status) || errType == "invalid_request_error" {
		result.FinalOutcome = OpsFinalOutcomeClientRejected
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryInvalidRequest
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "client_request_semantics"
		return result
	}

	if phase == "network" {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryNetworkTransport
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyProviderHealth
		result.ClassificationReason = "gateway_upstream_transport_failure"
		return result
	}

	if status >= 500 || phase == "routing" || phase == "internal" {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryPlatformInternal
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyAvailability
		result.ClassificationReason = "platform_internal_or_routing_failure"
		return result
	}

	return result
}

func classifyOpsUpstreamFailure(result OpsErrorClassification, status, upstreamStatus int, phase, errType, message string) OpsErrorClassification {
	if isOpsManagedCapacityMessage(message) {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryPlatformCapacity
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyCapacity
		result.ClassificationReason = "managed_upstream_capacity_rejected"
		return result
	}
	if isOpsCapabilityConfigurationMessage(message) {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryProductCompatibility
		result.AlertFamily = OpsAlertFamilyCompatibility
		result.ClassificationReason = "managed_upstream_capability_not_enabled"
		return result
	}
	if phase == "account_auth" || upstreamStatus == 401 ||
		(upstreamStatus == 403 && isOpsManagedCredentialMessage(message)) {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryPlatformCredential
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyCredential
		result.ClassificationReason = "managed_upstream_credential_rejected"
		return result
	}
	if isOpsExplicitUpstreamRequestRejection(upstreamStatus, errType, message) {
		result.FinalOutcome = OpsFinalOutcomeClientRejected
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryInvalidRequest
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "upstream_rejected_request_semantics"
		if status >= 500 || status == 0 {
			result.FinalOutcome = OpsFinalOutcomePlatformFailed
			result.Responsibility = OpsResponsibilityPlatform
			result.ErrorCategory = OpsErrorCategoryProductCompatibility
			result.AlertFamily = OpsAlertFamilyCompatibility
			result.ClassificationReason = "upstream_request_incompatibility_exposed_as_gateway_failure"
		}
		return result
	}
	// An upstream 4xx is not automatically a client-side mistake. Provider
	// account/workspace states (such as 402 deactivated_workspace), endpoint
	// availability, and undocumented provider gates are all reported as 4xx.
	// Treat only explicit invalid-request evidence as client responsibility.
	if upstreamStatus >= 400 && upstreamStatus < 500 && upstreamStatus != 429 {
		result.FinalOutcome = OpsFinalOutcomeProviderFailed
		result.Responsibility = OpsResponsibilityProvider
		result.ErrorCategory = OpsErrorCategoryProviderServer
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyProviderHealth
		result.ClassificationReason = "upstream_provider_rejected_request"
		return result
	}
	if upstreamStatus == 429 {
		result.FinalOutcome = OpsFinalOutcomeProviderFailed
		result.Responsibility = OpsResponsibilityProvider
		result.ErrorCategory = OpsErrorCategoryProviderRateLimit
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyProviderHealth
		result.ClassificationReason = "upstream_capacity_or_rate_limit"
		return result
	}
	if upstreamStatus == 529 || strings.Contains(message, "overloaded") || errType == "overloaded_error" {
		result.FinalOutcome = OpsFinalOutcomeProviderFailed
		result.Responsibility = OpsResponsibilityProvider
		result.ErrorCategory = OpsErrorCategoryProviderOverloaded
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyProviderHealth
		result.ClassificationReason = "upstream_overloaded"
		return result
	}
	if upstreamStatus >= 500 {
		result.FinalOutcome = OpsFinalOutcomeProviderFailed
		result.Responsibility = OpsResponsibilityProvider
		result.ErrorCategory = OpsErrorCategoryProviderServer
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyProviderHealth
		result.ClassificationReason = "upstream_server_error"
		return result
	}
	if strings.Contains(message, "context canceled") {
		result.FinalOutcome = OpsFinalOutcomeCancelled
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryClientCancelled
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "upstream_call_cancelled_by_client"
		return result
	}
	result.FinalOutcome = OpsFinalOutcomePlatformFailed
	result.Responsibility = OpsResponsibilityPlatform
	result.ErrorCategory = OpsErrorCategoryNetworkTransport
	result.CountsTowardSLA = true
	result.AlertFamily = OpsAlertFamilyProviderHealth
	result.ClassificationReason = "upstream_transport_or_unclassified_failure"
	return result
}

func recoveredOpsAlertFamily(upstreamStatus int, phase, errType, message string) string {
	if strings.Contains(message, "context canceled") || strings.Contains(message, "client disconnected") {
		return OpsAlertFamilyClientQuality
	}
	if isOpsPlatformCapacityFailure(message) || isOpsManagedCapacityMessage(message) {
		return OpsAlertFamilyCapacity
	}
	if isOpsCapabilityConfigurationMessage(message) {
		return OpsAlertFamilyCompatibility
	}
	if phase == "account_auth" || upstreamStatus == 401 ||
		(upstreamStatus == 403 && isOpsManagedCredentialMessage(message)) {
		return OpsAlertFamilyCredential
	}
	if isOpsExplicitUpstreamRequestRejection(upstreamStatus, errType, message) ||
		isOpsUnsupportedModel(message) || strings.Contains(message, "invalid") {
		return OpsAlertFamilyCompatibility
	}
	if upstreamStatus >= 400 || phase == "upstream" || phase == "account_auth" {
		return OpsAlertFamilyProviderHealth
	}
	return OpsAlertFamilyNone
}

func recoveredOpsResponsibility(upstreamStatus int, phase, errType, message string) string {
	if strings.Contains(message, "context canceled") || strings.Contains(message, "client disconnected") {
		return OpsResponsibilityClient
	}
	if isOpsLocalGroupModelUnavailable(message) {
		return OpsResponsibilityClient
	}
	if isOpsPlatformCapacityFailure(message) || isOpsManagedCapacityMessage(message) ||
		isOpsCapabilityConfigurationMessage(message) {
		return OpsResponsibilityPlatform
	}
	if phase == "account_auth" || upstreamStatus == 401 ||
		(upstreamStatus == 403 && isOpsManagedCredentialMessage(message)) {
		return OpsResponsibilityPlatform
	}
	if isOpsExplicitUpstreamRequestRejection(upstreamStatus, errType, message) {
		return OpsResponsibilityPlatform
	}
	if upstreamStatus == 429 || upstreamStatus >= 500 || strings.Contains(message, "overloaded") {
		return OpsResponsibilityProvider
	}
	if upstreamStatus >= 400 && upstreamStatus < 500 {
		return OpsResponsibilityProvider
	}
	if phase == "upstream" {
		return OpsResponsibilityProvider
	}
	return OpsResponsibilityUnknown
}

func isOpsExplicitUpstreamRequestRejection(upstreamStatus int, errType, message string) bool {
	if errType == "invalid_request_error" && (upstreamStatus == 400 || upstreamStatus == 422) {
		return true
	}
	return strings.Contains(message, "context_length_exceeded") ||
		strings.Contains(message, "exceeds the context window") ||
		strings.Contains(message, "maximum context length") ||
		strings.Contains(message, "invalid '") ||
		strings.Contains(message, "invalid request") ||
		strings.Contains(message, "invalid_request_error") ||
		strings.Contains(message, "invalid parameter") ||
		strings.Contains(message, "unknown parameter") ||
		strings.Contains(message, "unsupported parameter") ||
		strings.Contains(message, "malformed request") ||
		strings.Contains(message, "empty input messages") ||
		strings.Contains(message, "user messages must have non-empty content") ||
		(strings.Contains(message, "invalid `signature`") && strings.Contains(message, "`thinking` block")) ||
		strings.Contains(message, "invalid schema for response_format") ||
		strings.Contains(message, "'required' must include every key in properties") ||
		strings.Contains(message, "`temperature` is deprecated for this model") ||
		strings.Contains(message, "`temperature` and `top_p` cannot both be specified") ||
		(strings.Contains(message, "cache_control.ttl") && strings.Contains(message, "must not come after")) ||
		strings.Contains(message, "does not support the effort parameter")
}

func isOpsClientCancellation(status int, message string) bool {
	return status == 408 || status == 499 || strings.Contains(message, "context canceled") ||
		strings.Contains(message, "client disconnected") || strings.Contains(message, "client closed") ||
		strings.Contains(message, "request canceled")
}

func isOpsExplicitLocalSecurityRejection(errType, message string) bool {
	return strings.HasPrefix(errType, "cyber_policy") || errType == "content_policy_violation" ||
		errType == "moderation_blocked" || strings.Contains(message, "turnstile verification")
}

func isOpsSecurityPolicyMessage(message string) bool {
	return strings.Contains(message, "content policy") || strings.Contains(message, "security policy") ||
		strings.Contains(message, "content_policy_violation") || strings.Contains(message, "moderation_blocked") ||
		strings.Contains(message, "cyber policy") || strings.Contains(message, "cyber-security policy") ||
		strings.Contains(message, "cybersecurity risk")
}

func isOpsUpstreamEvidence(upstreamStatus int, phase, source, owner, errType string) bool {
	return upstreamStatus > 0 || phase == "upstream" || phase == "account_auth" ||
		source == "upstream_http" || owner == OpsResponsibilityProvider || errType == "upstream_error"
}

func isOpsUpstreamTransportMessage(message string) bool {
	return strings.Contains(message, "upstream stream disconnected") ||
		strings.Contains(message, "stream read error")
}

func isOpsPlatformCapacityFailure(message string) bool {
	return strings.Contains(message, "no available accounts") ||
		strings.Contains(message, "concurrency limit exceeded for account") ||
		strings.Contains(message, "too many pending requests") ||
		strings.Contains(message, "account pool") && strings.Contains(message, "unavailable")
}

func isOpsManagedCredentialMessage(message string) bool {
	return strings.Contains(message, "access token") || strings.Contains(message, "oauth token") ||
		strings.Contains(message, "credentials") || strings.Contains(message, "credential") ||
		strings.Contains(message, "token has been revoked") || strings.Contains(message, "token expired") ||
		strings.Contains(message, "invalid token") || strings.Contains(message, "authentication failed") ||
		strings.Contains(message, "认证失败") || strings.Contains(message, "请重新登录") ||
		strings.Contains(message, "检查 api key")
}

func isOpsManagedCapacityMessage(message string) bool {
	return strings.Contains(message, "insufficient balance") ||
		strings.Contains(message, "insufficient_account_balance") ||
		strings.Contains(message, "insufficient_balance") ||
		strings.Contains(message, "预扣费额度失败") || strings.Contains(message, "用户剩余额度")
}

func isOpsCapabilityConfigurationMessage(message string) bool {
	return strings.Contains(message, "not enabled for this group") ||
		strings.Contains(message, "disabled for this group") ||
		strings.Contains(message, "capability is not enabled") ||
		strings.Contains(message, "permission is not enabled")
}

func isOpsChannelModelRestricted(message string) bool {
	return strings.Contains(message, "model") && strings.Contains(message, "not available for this group")
}

func isOpsUnsupportedModel(message string) bool {
	return strings.Contains(message, "model") && (strings.Contains(message, "not supported") ||
		strings.Contains(message, "not in whitelist") || strings.Contains(message, "not configured"))
}

func isOpsLocalGroupModelUnavailable(message string) bool {
	return strings.Contains(message, "not supported by any configured account in this group")
}

func isOpsLocalClientAuth(phase, errType string, status int, message string) bool {
	return phase == "auth" || isOpsExplicitClientAuth(errType, status, message)
}

func isOpsExplicitClientAuth(errType string, status int, message string) bool {
	if errType == "authentication_error" || status == 401 {
		return true
	}
	return strings.Contains(message, "invalid api key") ||
		strings.Contains(message, "api key is required") ||
		strings.Contains(message, "api key is expired") ||
		strings.Contains(message, "api key is disabled") ||
		strings.Contains(message, "user associated with api key not found") ||
		strings.Contains(message, "user account is not active") ||
		strings.Contains(message, "api key 所属分组已删除") ||
		strings.Contains(message, "api key 所属分组已停用") ||
		strings.Contains(message, "api key is not assigned to any group")
}

func isOpsUserBusinessLimit(errType string, status int, message string) bool {
	if errType == "billing_error" || errType == "subscription_error" {
		return true
	}
	return strings.Contains(message, "insufficient balance") ||
		strings.Contains(message, "insufficient account balance") ||
		strings.Contains(message, "insufficient_balance") ||
		strings.Contains(message, "insufficient_account_balance") ||
		strings.Contains(message, "quota exhausted") || strings.Contains(message, "usage limit exceeded") ||
		strings.Contains(message, "concurrency limit exceeded for user") ||
		(status == 429 && !strings.Contains(message, "upstream"))
}

func isOpsUserSubscription(errType, message string) bool {
	return errType == "subscription_error" || strings.Contains(message, "subscription")
}

func isOpsClientRequestStatus(status int) bool {
	if status < 400 || status >= 500 || status == 429 {
		return false
	}
	return status != 408
}
