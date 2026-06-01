package service

import "net/http"

func applyOpenAICompatibleAPIKeyAuth(req *http.Request, account *Account, apiKey string) {
	switch account.OpenAICompatibleAuthHeader() {
	case OpenAICompatibleAuthHeaderAPIKey:
		req.Header.Set(OpenAICompatibleAuthHeaderAPIKey, apiKey)
	case OpenAICompatibleAuthHeaderXAPIKey:
		req.Header.Set(OpenAICompatibleAuthHeaderXAPIKey, apiKey)
	default:
		req.Header.Set(OpenAICompatibleAuthHeaderAuthorization, "Bearer "+apiKey)
	}
}
