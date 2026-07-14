package service

import "strings"

// AccountExtraJShandlerScriptID is the legacy single-script account binding key.
const AccountExtraJShandlerScriptID = "jshandler_script_id"

// AccountExtraJShandlerScriptIDs is the ordered multi-script account binding key.
const AccountExtraJShandlerScriptIDs = "jshandler_script_ids"

// JShandlerScriptIDFromAccount returns the first configured script id (legacy helper).
func JShandlerScriptIDFromAccount(account *Account) string {
	ids := JShandlerScriptIDsFromAccount(account)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// JShandlerScriptIDsFromAccount returns ordered unique script library ids for the account.
// Prefers jshandler_script_ids ([]string); falls back to legacy jshandler_script_id string.
func JShandlerScriptIDsFromAccount(account *Account) []string {
	if account == nil || account.Extra == nil {
		return nil
	}
	if raw, ok := account.Extra[AccountExtraJShandlerScriptIDs]; ok {
		if ids := parseExtraStringList(raw); len(ids) > 0 {
			return ids
		}
	}
	if id := strings.TrimSpace(account.GetExtraString(AccountExtraJShandlerScriptID)); id != "" {
		return []string{id}
	}
	return nil
}

func parseExtraStringList(raw any) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	switch v := raw.(type) {
	case []string:
		for _, s := range v {
			add(s)
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				add(s)
			}
		}
	case string:
		// Allow comma-separated fallback for accidental string storage.
		for _, part := range strings.Split(v, ",") {
			add(part)
		}
	}
	return out
}
