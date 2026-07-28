package service

import "strings"

const (
	OpsErrorClassificationVersion = 2

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
	message := strings.ToLower(strings.TrimSpace(input.ErrorMessage + " " + input.UpstreamMessage))

	result := OpsErrorClassification{
		FinalOutcome:          OpsFinalOutcomeUnknownFailed,
		Responsibility:        OpsResponsibilityUnknown,
		ErrorCategory:         OpsErrorCategoryUnknown,
		AlertFamily:           OpsAlertFamilyUnknown,
		ClassificationReason:  "unclassified_error",
		ClassificationVersion: OpsErrorClassificationVersion,
	}

	if status > 0 && status < 400 {
		result.FinalOutcome = OpsFinalOutcomeRecovered
		result.ErrorCategory = OpsErrorCategoryRecovered
		result.CountsTowardSLA = false
		result.AlertFamily = recoveredOpsAlertFamily(upstreamStatus, phase, message)
		result.Responsibility = recoveredOpsResponsibility(upstreamStatus, phase, message)
		result.ClassificationReason = "final_request_recovered"
		return result
	}

	if isOpsClientCancellation(status, message) {
		result.FinalOutcome = OpsFinalOutcomeCancelled
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryClientCancelled
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "client_cancel_or_disconnect"
		return result
	}

	if isOpsSecurityRejection(errType, message) {
		result.FinalOutcome = OpsFinalOutcomeSecurityBlocked
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategorySecurityPolicy
		result.AlertFamily = OpsAlertFamilySecurity
		result.ClassificationReason = "security_policy_rejection"
		return result
	}

	if isOpsPlatformCapacityFailure(message) {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryPlatformCapacity
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyCapacity
		result.ClassificationReason = "platform_capacity_unavailable"
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

	if upstreamStatus > 0 || phase == "upstream" || phase == "account_auth" || source == "upstream_http" {
		effectiveUpstreamStatus := upstreamStatus
		// Older and compatibility paths did not always persist a separate upstream
		// status. A preserved upstream 4xx still carries reliable request semantics;
		// synthetic 5xx values remain transport/platform failures without evidence.
		if effectiveUpstreamStatus == 0 && ((status >= 400 && status < 500) || status == 529) {
			effectiveUpstreamStatus = status
		}
		return classifyOpsUpstreamFailure(result, status, effectiveUpstreamStatus, phase, errType, message)
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
	if phase == "account_auth" || upstreamStatus == 401 || upstreamStatus == 403 {
		result.FinalOutcome = OpsFinalOutcomePlatformFailed
		result.Responsibility = OpsResponsibilityPlatform
		result.ErrorCategory = OpsErrorCategoryPlatformCredential
		result.CountsTowardSLA = true
		result.AlertFamily = OpsAlertFamilyCapacity
		result.ClassificationReason = "managed_upstream_credential_rejected"
		return result
	}
	if upstreamStatus >= 400 && upstreamStatus < 500 && upstreamStatus != 429 {
		result.FinalOutcome = OpsFinalOutcomeClientRejected
		result.Responsibility = OpsResponsibilityClient
		result.ErrorCategory = OpsErrorCategoryInvalidRequest
		result.AlertFamily = OpsAlertFamilyClientQuality
		result.ClassificationReason = "upstream_rejected_request_semantics"
		if status >= 500 || status == 0 {
			result.ErrorCategory = OpsErrorCategoryProductCompatibility
			result.AlertFamily = OpsAlertFamilyCompatibility
			result.ClassificationReason = "upstream_4xx_exposed_as_gateway_failure"
		}
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

func recoveredOpsAlertFamily(upstreamStatus int, phase, message string) string {
	if strings.Contains(message, "context canceled") || strings.Contains(message, "client disconnected") {
		return OpsAlertFamilyClientQuality
	}
	if upstreamStatus == 429 || upstreamStatus >= 500 || phase == "upstream" || phase == "account_auth" {
		return OpsAlertFamilyProviderHealth
	}
	if upstreamStatus >= 400 || strings.Contains(message, "invalid") {
		return OpsAlertFamilyCompatibility
	}
	return OpsAlertFamilyNone
}

func recoveredOpsResponsibility(upstreamStatus int, phase, message string) string {
	if strings.Contains(message, "context canceled") || strings.Contains(message, "client disconnected") {
		return OpsResponsibilityClient
	}
	if phase == "account_auth" || upstreamStatus == 401 || upstreamStatus == 403 {
		return OpsResponsibilityPlatform
	}
	if upstreamStatus == 429 || upstreamStatus >= 500 || strings.Contains(message, "overloaded") {
		return OpsResponsibilityProvider
	}
	if upstreamStatus >= 400 && upstreamStatus < 500 {
		return OpsResponsibilityClient
	}
	return OpsResponsibilityUnknown
}

func isOpsClientCancellation(status int, message string) bool {
	return status == 408 || status == 499 || strings.Contains(message, "context canceled") ||
		strings.Contains(message, "client disconnected") || strings.Contains(message, "client closed") ||
		strings.Contains(message, "request canceled") || strings.Contains(message, "broken pipe")
}

func isOpsSecurityRejection(errType, message string) bool {
	return errType == "cyber_policy" || strings.Contains(message, "cyber policy") ||
		strings.Contains(message, "content policy") || strings.Contains(message, "security policy") ||
		strings.Contains(message, "turnstile verification")
}

func isOpsPlatformCapacityFailure(message string) bool {
	return strings.Contains(message, "no available accounts") ||
		strings.Contains(message, "concurrency limit exceeded for account") ||
		strings.Contains(message, "too many pending requests") ||
		strings.Contains(message, "account pool") && strings.Contains(message, "unavailable")
}

func isOpsUnsupportedModel(message string) bool {
	return strings.Contains(message, "model") && (strings.Contains(message, "not supported") ||
		strings.Contains(message, "not in whitelist") || strings.Contains(message, "not configured"))
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
