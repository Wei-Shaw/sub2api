package service

// sanitizeEndpointCapabilitiesCredentials keeps provider-neutral endpoint
// settings from enabling an endpoint that the account authentication cannot
// support. It intentionally leaves absent settings absent, preserving legacy
// defaults for existing accounts.
func sanitizeEndpointCapabilitiesCredentials(platform, accountType string, credentials map[string]any) map[string]any {
	if credentials == nil || platform != PlatformGemini || accountType == AccountTypeAPIKey {
		return credentials
	}
	raw, exists := credentials[endpointCapabilitiesCredentialKey]
	if !exists || raw == nil {
		return credentials
	}

	selected := credentialCapabilitySet(raw)
	if !selected["gemini_native"] {
		delete(credentials, endpointCapabilitiesCredentialKey)
		return credentials
	}
	// OAuth, service-account, and other Gemini auth forms keep their native
	// generation marker but can never persist OpenAI-compatible embeddings.
	credentials[endpointCapabilitiesCredentialKey] = []string{"gemini_native"}
	return credentials
}

func credentialCapabilitySet(raw any) map[string]bool {
	selected := make(map[string]bool)
	add := func(value string) {
		if value != "" {
			selected[value] = true
		}
	}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			add(value)
		}
	case []any:
		for _, rawValue := range values {
			if value, ok := rawValue.(string); ok {
				add(value)
			}
		}
	case map[string]bool:
		for value, enabled := range values {
			if enabled {
				add(value)
			}
		}
	case map[string]any:
		for value, rawEnabled := range values {
			if enabled, ok := rawEnabled.(bool); ok && enabled {
				add(value)
			}
		}
	}
	return selected
}
