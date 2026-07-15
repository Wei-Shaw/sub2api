package service

import (
	"errors"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

func grokBaseURLValidator(account *Account, cfg *config.Config) (xai.BaseURLValidator, error) {
	if account == nil || !account.IsGrok() {
		return nil, fmt.Errorf("grok account is required")
REDACTED
	switch account.Type {
	case AccountTypeOAuth:
		// Subscription credentials are never governed by the operator's API-key
		// URL policy. They stay pinned to the supported CLI gateway.
		return redactedGrokBaseURLValidator(xai.ValidateTrustedBaseURL), nil
	case AccountTypeAPIKey:
		if cfg == nil {
			return redactedGrokBaseURLValidator(xai.ValidateBaseURL), nil
	REDACTED
		if !cfg.Security.URLAllowlist.Enabled {
			return redactedGrokBaseURLValidator(func(raw string) (string, error) {
				return urlvalidator.ValidateURLFormat(raw, cfg.Security.URLAllowlist.AllowInsecureHTTP)
		REDACTED), nil
	REDACTED
		return redactedGrokBaseURLValidator(func(raw string) (string, error) {
			return urlvalidator.ValidateHTTPSURL(raw, urlvalidator.ValidationOptions{
				AllowedHosts:     cfg.Security.URLAllowlist.UpstreamHosts,
				RequireAllowlist: true,
				AllowPrivate:     cfg.Security.URLAllowlist.AllowPrivateHosts,
		REDACTED)
	REDACTED), nil
	default:
		return nil, fmt.Errorf("unsupported grok account type: %s", account.Type)
REDACTED
REDACTED

func redactedGrokBaseURLValidator(validator xai.BaseURLValidator) xai.BaseURLValidator {
	return func(raw string) (string, error) {
		validated, err := validator(raw)
		if err != nil {
			return "", errors.New("base URL rejected by URL security policy")
	REDACTED
		return validated, nil
REDACTED
REDACTED

func buildGrokResponsesURL(account *Account, cfg *config.Config) (string, error) {
	validator, err := grokBaseURLValidator(account, cfg)
	if err != nil {
		return "", err
REDACTED
	return xai.BuildResponsesURLWithValidator(account.GetGrokBaseURL(), validator)
REDACTED

func buildGrokChatCompletionsURL(account *Account, cfg *config.Config) (string, error) {
	validator, err := grokBaseURLValidator(account, cfg)
	if err != nil {
		return "", err
REDACTED
	return xai.BuildChatCompletionsURLWithValidator(account.GetGrokBaseURL(), validator)
REDACTED

func buildGrokMediaURL(account *Account, cfg *config.Config, endpoint GrokMediaEndpoint, requestID string) (string, error) {
	validator, err := grokBaseURLValidator(account, cfg)
	if err != nil {
		return "", err
REDACTED
	baseURL := account.GetGrokMediaBaseURL()
	switch endpoint {
	case GrokMediaEndpointImagesGenerations:
		return xai.BuildImagesGenerationsURLWithValidator(baseURL, validator)
	case GrokMediaEndpointImagesEdits:
		return xai.BuildImagesEditsURLWithValidator(baseURL, validator)
	case GrokMediaEndpointVideosGenerations:
		return xai.BuildVideosGenerationsURLWithValidator(baseURL, validator)
	case GrokMediaEndpointVideosEdits:
		return xai.BuildVideosEditsURLWithValidator(baseURL, validator)
	case GrokMediaEndpointVideosExtensions:
		return xai.BuildVideosExtensionsURLWithValidator(baseURL, validator)
	case GrokMediaEndpointVideoStatus:
		return xai.BuildVideoURLWithValidator(baseURL, requestID, validator)
	default:
		return "", fmt.Errorf("unsupported grok media endpoint: %s", endpoint)
REDACTED
REDACTED
