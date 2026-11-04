package service

import "strings"

const kimiConcurrentRequestLimitMessage = "You've reached your concurrent request limit. Please wait for your ongoing requests to finish and try again."

func isKimiConcurrencyLimit403(account *Account, message string) bool {
	return account != nil && account.Platform == PlatformKimi &&
		strings.TrimSpace(message) == kimiConcurrentRequestLimitMessage
}
