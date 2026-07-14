package service

// JShandlerScriptIDsFromGroup returns ordered unique script library IDs for
// group-bound on_before_request hooks.
func JShandlerScriptIDsFromGroup(group *Group) []string {
	if group == nil || len(group.JSHandlerScriptIDs) == 0 {
		return nil
	}
	return parseExtraStringList(group.JSHandlerScriptIDs)
}

// JShandlerScriptIDsFromAPIKeyGroup resolves before-request scripts from the
// API key's bound group (apiKey.Group). Prefer the hydrated Group object from
// auth cache / DB load.
func JShandlerScriptIDsFromAPIKeyGroup(apiKey *APIKey) []string {
	if apiKey == nil {
		return nil
	}
	return JShandlerScriptIDsFromGroup(apiKey.Group)
}
