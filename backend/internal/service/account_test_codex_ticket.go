package service

import (
	"context"
	"net/http"
)

type qualityTicketReceiptKey struct{}

func (s *AccountTestService) applyAccountTestTicket(ctx context.Context, account *Account, body []byte, headers http.Header) error {
	receipt, err := s.openaiGatewayService.applyOpenAICodexTicketWithReceipt(ctx, account, extractOpenAICodexTicketModel(body), headers)
	if captured, ok := ctx.Value(qualityTicketReceiptKey{}).(*OpenAICodexTicketReceipt); ok {
		*captured = receipt
		// A ticket-enabled quality check must not silently fall back to a
		// ticketless request, even when normal traffic allows fail-open.
		if qualityTicketBlocked(receipt.Status) {
			return ErrOpenAICodexTicketUnavailable
		}
	}
	return err
}

func qualityTicketBlocked(status string) bool {
	return status == "missing" || status == "expired" || status == "unavailable"
}
