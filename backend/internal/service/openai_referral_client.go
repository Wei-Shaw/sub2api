package service

import "context"

// OpenAIReferralCall carries the account's refreshed authentication and proxy
// settings to the transport adapter. Headers must never be logged or persisted.
type OpenAIReferralCall struct {
	ProxyURL  string
	Headers   map[string]string
	ProgramID string
}

// OpenAIReferralClient isolates invitation transport from eligibility policy.
// QueryEligibility returns a non-nil result on success. SendInvite must not
// retry and returns nil only when the upstream confirms one invitation.
type OpenAIReferralClient interface {
	QueryEligibility(context.Context, OpenAIReferralCall) (*OpenAIReferralEligibility, error)
	SendInvite(context.Context, OpenAIReferralCall, string) error
}
