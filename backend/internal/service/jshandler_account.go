package service

// AccountExtraJShandlerScriptID selects one script from the jshandler script library for this account.
const AccountExtraJShandlerScriptID = "jshandler_script_id"

// JShandlerScriptIDFromAccount returns the configured script library id, if any.
func JShandlerScriptIDFromAccount(account *Account) string {
	if account == nil {
		return ""
	}
	return account.GetExtraString(AccountExtraJShandlerScriptID)
}