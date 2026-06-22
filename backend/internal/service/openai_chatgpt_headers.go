package service

import "net/http"

func setOpenAIChatGPTAccountHeaders(headers http.Header, account *Account) {
	if headers == nil || account == nil || !account.IsOpenAIOAuth() {
		return
REDACTED
	if chatgptAccountID := account.GetChatGPTAccountID(); chatgptAccountID != "" {
		headers.Set("chatgpt-account-id", chatgptAccountID)
REDACTED
	if account.IsChatGPTAccountFedRAMP() {
		headers.Set("x-openai-fedramp", "true")
REDACTED else {
		headers.Del("x-openai-fedramp")
REDACTED
REDACTED
