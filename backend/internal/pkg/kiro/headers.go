package kiro

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

const (
	// SDK versions advertised in the User-Agent header. Match Kiro-Go's
	// values so upstream behaviour mirrors the reference implementation.
	kiroStreamingSDKVersion = "1.0.34"
	defaultSystemVersion    = "darwin"
	defaultNodeVersion      = "v20.11.0"
	defaultKiroVersion      = "0.1.32"
)

// HeaderValues carries the per-request header strings derived from an
// account. Held separately from the *http.Request so it can be reused
// across multiple endpoint attempts in the fallback loop.
type HeaderValues struct {
	UserAgent    string
	AmzUserAgent string
	Host         string
}

// BuildStreamingHeaderValues constructs the headers we send to the
// generateAssistantResponse endpoint for a given account.
//
// machineID is a sticky UUID per account (stored on extra.machine_id).
// host is the URL host of the chosen endpoint.
func BuildStreamingHeaderValues(machineID, host string) HeaderValues {
	const (
		apiName = "codewhispererstreaming"
		mode    = "m/E"
	)
	userAgent := fmt.Sprintf(
		"aws-sdk-js/%s ua/2.1 os/%s lang/js md/nodejs#%s api/%s#%s %s KiroIDE-%s",
		kiroStreamingSDKVersion,
		defaultSystemVersion,
		defaultNodeVersion,
		apiName,
		kiroStreamingSDKVersion,
		mode,
		defaultKiroVersion,
	)
	amzUserAgent := fmt.Sprintf("aws-sdk-js/%s KiroIDE-%s", kiroStreamingSDKVersion, defaultKiroVersion)
	if machineID != "" {
		userAgent += "-" + machineID
		amzUserAgent += "-" + machineID
	}
	return HeaderValues{
		UserAgent:    userAgent,
		AmzUserAgent: amzUserAgent,
		Host:         host,
	}
}

// ApplyBaseHeaders sets the headers every Kiro request needs on an
// *http.Request. The Authorization header is the caller's responsibility
// (the token comes from a TokenProvider that handles caching/refresh).
func ApplyBaseHeaders(req *http.Request, accessToken string, values HeaderValues) {
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("User-Agent", values.UserAgent)
	req.Header.Set("x-amz-user-agent", values.AmzUserAgent)
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
	req.Header.Set("x-amzn-kiro-agent-mode", "vibe")
	req.Header.Set("Amz-Sdk-Request", "attempt=1; max=3")
	req.Header.Set("Amz-Sdk-Invocation-Id", uuid.New().String())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "*/*")
	if values.Host != "" {
		req.Host = values.Host
	}
}

// GenerateMachineID returns a fresh UUID for use as the per-account
// machine identifier. Callers persist this once per account in
// extra.machine_id.
func GenerateMachineID() string {
	return uuid.New().String()
}
