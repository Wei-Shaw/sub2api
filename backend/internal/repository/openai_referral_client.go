package repository

import (
	"context"
	"encoding/json"
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/imroc/req/v3"
)

// Desktop 26.908.40834, app-primary-44ec287874b7.js: SIt, EIt, PIt, FIt.
// Referrals use /backend-api/referrals, not /backend-api/wham.
const openAIReferralURL = "https://chatgpt.com/backend-api/referrals/invite"
const openAIReferralEntrypoint = "persistent"

type openAIReferralClient struct {
	clientFactory service.PrivacyClientFactory
	baseURL       string
}

func NewOpenAIReferralClient(factory service.PrivacyClientFactory) service.OpenAIReferralClient {
	return &openAIReferralClient{clientFactory: factory, baseURL: openAIReferralURL}
}

func (c *openAIReferralClient) request(ctx context.Context, call service.OpenAIReferralCall) (*req.Request, error) {
	client, err := c.clientFactory(call.ProxyURL)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_REFERRAL_CLIENT_ERROR", "failed to build referral client")
	}
	// Sending has no documented idempotency key. Override any factory retries.
	return client.R().SetContext(ctx).SetHeaders(call.Headers).SetRetryCount(0), nil
}

func (c *openAIReferralClient) QueryEligibility(ctx context.Context, call service.OpenAIReferralCall) (*service.OpenAIReferralEligibility, error) {
	r, err := c.request(ctx, call)
	if err != nil {
		return nil, err
	}
	resp, err := r.SetQueryParams(map[string]string{"program_id": call.ProgramID, "entrypoint": openAIReferralEntrypoint}).Get(c.baseURL + "/eligibility")
	if err != nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_REFERRAL_QUERY_FAILED", "failed to query invitation eligibility")
	}
	if !resp.IsSuccessState() {
		return nil, referralHTTPError(resp.StatusCode)
	}
	var result *service.OpenAIReferralEligibility
	if err := json.Unmarshal(resp.Bytes(), &result); err != nil || result == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "OPENAI_REFERRAL_INVALID_RESPONSE", "invalid invitation eligibility response")
	}
	return result, nil
}

func (c *openAIReferralClient) SendInvite(ctx context.Context, call service.OpenAIReferralCall, email string) error {
	r, err := c.request(ctx, call)
	if err != nil {
		return err
	}
	resp, err := r.SetBody(map[string]any{
		"program_id": call.ProgramID, "entrypoint": openAIReferralEntrypoint, "emails": []string{email},
	}).Post(c.baseURL)
	if err != nil {
		return referralSendUnknown()
	}
	if !resp.IsSuccessState() {
		return referralHTTPError(resp.StatusCode)
	}
	var payload struct {
		Invites []json.RawMessage `json:"invites"`
	}
	if err := json.Unmarshal(resp.Bytes(), &payload); err != nil || len(payload.Invites) != 1 || string(payload.Invites[0]) == "null" {
		return referralSendUnknown()
	}
	return nil
}

func referralSendUnknown() error {
	return infraerrors.New(http.StatusBadGateway, "OPENAI_REFERRAL_SEND_UNKNOWN", "invitation outcome is unknown; check the invitation status in Codex before sending again")
}

// Upstream bodies can contain personal data or credentials. Return only stable
// error codes distinguishing validation failures, duplicates and limits.
func referralHTTPError(status int) error {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return infraerrors.New(http.StatusBadRequest, "OPENAI_REFERRAL_REJECTED", "the invitation request was rejected; check the email and eligibility")
	case http.StatusUnauthorized:
		return infraerrors.New(http.StatusBadGateway, "OPENAI_REFERRAL_AUTH_ERROR", "upstream authentication failed; refresh the account credentials")
	case http.StatusForbidden:
		return infraerrors.New(http.StatusForbidden, "OPENAI_REFERRAL_FORBIDDEN", "this account is not eligible for invitations")
	case http.StatusConflict:
		return infraerrors.New(http.StatusConflict, "OPENAI_REFERRAL_ALREADY_EXISTS", "an invitation already exists for this recipient")
	case http.StatusTooManyRequests:
		return infraerrors.New(http.StatusTooManyRequests, "OPENAI_REFERRAL_RATE_LIMITED", "invitation limit reached; try again later")
	default:
		return infraerrors.New(http.StatusBadGateway, "OPENAI_REFERRAL_UPSTREAM_ERROR", "invitation service is unavailable")
	}
}
