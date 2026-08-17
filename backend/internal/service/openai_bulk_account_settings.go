package service

import (
	"fmt"
	"strconv"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai_compat"
)

type bulkOpenAISettings struct {
	longContextBilling      bool
	endpointCapabilities    bool
	responsesMode           bool
	capabilitiesIncludeChat bool
	forcedResponsesMode     bool
REDACTED

func (s bulkOpenAISettings) any() bool {
	return s.longContextBilling || s.endpointCapabilities || s.responsesMode
REDACTED

func normalizeBulkOpenAISettings(input *BulkUpdateAccountsInput) (bulkOpenAISettings, error) {
	var settings bulkOpenAISettings
	if input == nil {
		return settings, nil
REDACTED

	if _, exists := input.Extra[openAILongContextBillingEnabledKey]; exists {
		settings.longContextBilling = true
		if err := ValidateOpenAILongContextBillingExtra(PlatformOpenAI, input.Extra); err != nil {
			return settings, err
	REDACTED
REDACTED

	if raw, exists := input.Credentials[openAIEndpointCapabilitiesCredentialKey]; exists {
		settings.endpointCapabilities = true
		capabilities, includeChat, err := normalizeBulkOpenAIEndpointCapabilities(raw)
		if err != nil {
			return settings, err
	REDACTED
		settings.capabilitiesIncludeChat = includeChat
		input.Credentials[openAIEndpointCapabilitiesCredentialKey] = capabilities
REDACTED

	if raw, exists := input.Extra[openai_compat.ExtraKeyResponsesMode]; exists {
		settings.responsesMode = true
		mode, forced, err := normalizeBulkOpenAIResponsesMode(raw)
		if err != nil {
			return settings, err
	REDACTED
		settings.forcedResponsesMode = forced
		input.Extra[openai_compat.ExtraKeyResponsesMode] = mode
REDACTED

	if settings.endpointCapabilities && !settings.capabilitiesIncludeChat {
		if settings.forcedResponsesMode {
			return settings, infraerrors.BadRequest(
				"OPENAI_RESPONSES_MODE_INVALID",
				"a forced Responses route requires the chat_completions endpoint capability",
			)
	REDACTED
		if input.Extra == nil {
			input.Extra = make(map[string]any, 1)
	REDACTED
		input.Extra[openai_compat.ExtraKeyResponsesMode] = nil
		settings.responsesMode = true
REDACTED

	return settings, nil
REDACTED

func normalizeBulkOpenAIEndpointCapabilities(raw any) (any, bool, error) {
	if raw == nil {
		return nil, true, nil
REDACTED

	values := make([]string, 0, 2)
	switch typed := raw.(type) {
	case []any:
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, false, invalidBulkOpenAIEndpointCapabilities()
		REDACTED
			values = append(values, value)
	REDACTED
	case []string:
		values = append(values, typed...)
	default:
		return nil, false, invalidBulkOpenAIEndpointCapabilities()
REDACTED

	selected := make(map[string]bool, 2)
	for _, value := range values {
		switch OpenAIEndpointCapability(value) {
		case OpenAIEndpointCapabilityChatCompletions, OpenAIEndpointCapabilityEmbeddings:
			selected[value] = true
		default:
			return nil, false, invalidBulkOpenAIEndpointCapabilities()
	REDACTED
REDACTED
	if len(selected) == 0 {
		return nil, false, invalidBulkOpenAIEndpointCapabilities()
REDACTED

	includeChat := selected[string(OpenAIEndpointCapabilityChatCompletions)]
	if includeChat && selected[string(OpenAIEndpointCapabilityEmbeddings)] {
		return nil, true, nil
REDACTED
	if includeChat {
		return []string{string(OpenAIEndpointCapabilityChatCompletions)REDACTED, true, nil
REDACTED
	return []string{string(OpenAIEndpointCapabilityEmbeddings)REDACTED, false, nil
REDACTED

func invalidBulkOpenAIEndpointCapabilities() error {
	return infraerrors.BadRequest(
		"OPENAI_ENDPOINT_CAPABILITIES_INVALID",
		"openai_capabilities must contain chat_completions, embeddings, or both",
	)
REDACTED

func normalizeBulkOpenAIResponsesMode(raw any) (any, bool, error) {
	if raw == nil {
		return nil, false, nil
REDACTED
	mode, ok := raw.(string)
	if !ok {
		return nil, false, invalidBulkOpenAIResponsesMode()
REDACTED
	switch openai_compat.ResponsesSupportMode(mode) {
	case openai_compat.ResponsesSupportModeAuto:
		return nil, false, nil
	case openai_compat.ResponsesSupportModeForceResponses,
		openai_compat.ResponsesSupportModeForceChatCompletions:
		return mode, true, nil
	default:
		return nil, false, invalidBulkOpenAIResponsesMode()
REDACTED
REDACTED

func invalidBulkOpenAIResponsesMode() error {
	return infraerrors.BadRequest(
		"OPENAI_RESPONSES_MODE_INVALID",
		"openai_responses_mode must be auto, force_responses, force_chat_completions, or null",
	)
REDACTED

func validateBulkOpenAISettingsTargets(
	input *BulkUpdateAccountsInput,
	settings bulkOpenAISettings,
	targetsByID map[int64]*Account,
) (int, error) {
	if input == nil || !settings.any() {
		return 0, nil
REDACTED

	inheritedCount := 0
	for _, accountID := range input.AccountIDs {
		account, ok := targetsByID[accountID]
		if !ok || account == nil {
			return 0, invalidBulkOpenAITarget(accountID, "account does not exist")
	REDACTED

		if settings.longContextBilling {
			if account.Platform != PlatformOpenAI || !supportsOpenAILongContextBilling(account.Type) {
				return 0, invalidBulkOpenAITarget(accountID, "long-context billing requires an OpenAI OAuth, setup-token, or API-key account")
		REDACTED
			if account.IsShadow() {
				inheritedCount++
		REDACTED
	REDACTED

		if settings.endpointCapabilities || settings.responsesMode {
			if account.Platform != PlatformOpenAI || account.Type != AccountTypeAPIKey {
				return 0, invalidBulkOpenAITarget(accountID, "endpoint capabilities and Responses routing require an OpenAI API-key account")
		REDACTED
	REDACTED

		if settings.forcedResponsesMode && !settings.capabilitiesIncludeChat &&
			!settings.endpointCapabilities &&
			!account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions) {
			return 0, invalidBulkOpenAITarget(accountID, "a forced Responses route requires the chat_completions endpoint capability")
	REDACTED
REDACTED

	if settings.longContextBilling && inheritedCount == len(input.AccountIDs) && bulkUpdateOnlyChangesLongContext(input) {
		return 0, infraerrors.BadRequest(
			"OPENAI_LONG_CONTEXT_PARENT_REQUIRED",
			"long-context billing is owned by parent accounts; select at least one parent account",
		)
REDACTED
	return inheritedCount, nil
REDACTED

func supportsOpenAILongContextBilling(accountType string) bool {
	switch accountType {
	case AccountTypeOAuth, AccountTypeSetupToken, AccountTypeAPIKey:
		return true
	default:
		return false
REDACTED
REDACTED

func invalidBulkOpenAITarget(accountID int64, message string) error {
	return infraerrors.BadRequest(
		"OPENAI_BULK_TARGET_INVALID",
		fmt.Sprintf("account %d: %s", accountID, message),
	).WithMetadata(map[string]string{"account_id": strconv.FormatInt(accountID, 10)REDACTED)
REDACTED

func bulkUpdateOnlyChangesLongContext(input *BulkUpdateAccountsInput) bool {
	if input == nil || input.Name != "" || input.ProxyID != nil || input.Concurrency != nil ||
		input.Priority != nil || input.RateMultiplier != nil || input.LoadFactor != nil ||
		input.Status != "" || input.Schedulable != nil || input.GroupIDs != nil ||
		len(input.Credentials) != 0 || input.ProbeEnabled != nil {
		return false
REDACTED
	if len(input.Extra) != 1 {
		return false
REDACTED
	_, ok := input.Extra[openAILongContextBillingEnabledKey]
	return ok
REDACTED
