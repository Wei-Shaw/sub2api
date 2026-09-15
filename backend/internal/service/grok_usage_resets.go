package service

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
)

type GrokUsageResetCards struct {
	Cards []xai.UsageResetCard `json:"cards"`
}

// Web SSO is used only for this operation. In particular it is never persisted
// in account credentials, quota snapshots, logs or a shared request cache.
func (s *GrokQuotaService) QueryUsageResetCards(ctx context.Context, accountID int64, sso string) (*GrokUsageResetCards, error) {
	return s.callUsageResetCards(ctx, accountID, sso, "", false)
}

func (s *GrokQuotaService) RedeemUsageResetCard(ctx context.Context, accountID int64, sso, cardID string) (*GrokUsageResetCards, error) {
	// Re-query with the supplied session so expired, already-used, or different
	// session cards cannot be submitted from a stale browser view.
	current, err := s.QueryUsageResetCards(ctx, accountID, sso)
	if err != nil {
		return nil, err
	}
	for _, card := range current.Cards {
		if card.TokenID == cardID {
			return s.callUsageResetCards(ctx, accountID, sso, cardID, true)
		}
	}
	return nil, infraerrors.New(http.StatusConflict, "GROK_RESET_CARD_UNAVAILABLE", "This reset card is no longer available; query the cards again")
}

func (s *GrokQuotaService) callUsageResetCards(ctx context.Context, accountID int64, sso, cardID string, redeem bool) (*GrokUsageResetCards, error) {
	account, err := s.loadGrokOAuthAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if s.httpUpstream == nil {
		return nil, infraerrors.New(http.StatusServiceUnavailable, "GROK_RESET_NOT_CONFIGURED", "Grok reset service is unavailable")
	}
	proxyURL := s.resolveProxyURL(ctx, account)
	if account.ProxyID != nil && account.Proxy == nil {
		return nil, infraerrors.New(http.StatusBadGateway, "GROK_RESET_PROXY_UNAVAILABLE", "The account's configured proxy is unavailable")
	}
	ctx, cancel := context.WithTimeout(WithHTTPUpstreamRedirectsDisabled(ctx), 20*time.Second)
	defer cancel()
	req, err := xai.NewUsageResetRequest(ctx, sso, cardID, redeem)
	if err != nil {
		return nil, infraerrors.New(http.StatusBadRequest, "GROK_RESET_INVALID_INPUT", err.Error())
	}
	// Always call the official Web host. Never apply account URL/header
	// overrides or send this Web session to an OAuth/custom upstream.
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil || resp == nil || resp.Body == nil {
		return nil, grokUsageResetCallError(redeem)
	}
	defer func() { _ = resp.Body.Close() }()
	cards, err := xai.ParseUsageResetResponse(resp)
	if err != nil {
		var rpcErr *xai.UsageResetRPCError
		if errors.As(err, &rpcErr) && (rpcErr.HTTPStatus == 401 || rpcErr.HTTPStatus == 403 || rpcErr.GRPCStatus == 7 || rpcErr.GRPCStatus == 16) {
			return nil, infraerrors.New(http.StatusBadGateway, "GROK_RESET_WEB_SESSION_REJECTED", "Grok rejected the Web session or proxy; check the SSO session or redeem on grok.com")
		}
		return nil, grokUsageResetCallError(redeem)
	}
	available := make([]xai.UsageResetCard, 0, len(cards))
	now := time.Now()
	for _, card := range cards {
		if redeem && card.TokenID == cardID {
			return nil, grokUsageResetCallError(true)
		}
		if card.Available(now) {
			available = append(available, card)
		}
	}
	sort.Slice(available, func(i, j int) bool { return available[i].ExpiresAt.Before(available[j].ExpiresAt) })
	// A Web SSO can belong to a different account from the selected OAuth
	// credential. Do not clear scheduling cooldowns or rewrite its quota here.
	return &GrokUsageResetCards{Cards: available}, nil
}

func grokUsageResetCallError(redeem bool) error {
	if redeem {
		return infraerrors.New(http.StatusBadGateway, "GROK_RESET_RESULT_UNKNOWN", "Could not confirm redemption; query the cards again before retrying")
	}
	return infraerrors.New(http.StatusBadGateway, "GROK_RESET_QUERY_FAILED", "Could not query Grok reset cards; try again or open grok.com")
}
