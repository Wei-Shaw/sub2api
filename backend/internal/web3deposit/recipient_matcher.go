package web3deposit

import (
	"context"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
)

const DefaultRecipientLookupChunkSize = 500

type ActiveDepositAddressLookup interface {
	ListActiveByNormalizedAddresses(ctx context.Context, normalizedAddresses []string) ([]DepositAddress, error)
}

type MatchedTransferEvent struct {
	Event            TransferEvent
	DepositAddressID int64
	UserID           int64
}

type RecipientMatcher struct {
	lookup    ActiveDepositAddressLookup
	chunkSize int
}

var _ ScannerRecipientMatcher = (*RecipientMatcher)(nil)

func NewRecipientMatcher(lookup ActiveDepositAddressLookup, chunkSize int) *RecipientMatcher {
	if chunkSize <= 0 {
		chunkSize = DefaultRecipientLookupChunkSize
	}
	return &RecipientMatcher{
		lookup:    lookup,
		chunkSize: chunkSize,
	}
}

func (m *RecipientMatcher) Match(ctx context.Context, events []TransferEvent) ([]MatchedTransferEvent, error) {
	if len(events) == 0 {
		return []MatchedTransferEvent{}, nil
	}

	normalizedRecipients := uniqueNormalizedRecipients(events)
	addressesByNormalizedAddress := make(map[string]DepositAddress, len(normalizedRecipients))
	for start := 0; start < len(normalizedRecipients); start += m.chunkSize {
		end := min(start+m.chunkSize, len(normalizedRecipients))
		addresses, err := m.lookup.ListActiveByNormalizedAddresses(ctx, normalizedRecipients[start:end])
		if err != nil {
			return nil, fmt.Errorf("look up active web3 deposit recipients: %w", err)
		}
		for _, address := range addresses {
			if address.Status == AddressStatusActive {
				addressesByNormalizedAddress[strings.ToLower(address.NormalizedAddress)] = address
			}
		}
	}

	matches := make([]MatchedTransferEvent, 0, len(events))
	for _, event := range events {
		address, ok := addressesByNormalizedAddress[normalizeEVMAddress(event.To)]
		if !ok {
			continue
		}
		matches = append(matches, MatchedTransferEvent{
			Event:            event,
			DepositAddressID: address.ID,
			UserID:           address.UserID,
		})
	}
	return matches, nil
}

func uniqueNormalizedRecipients(events []TransferEvent) []string {
	recipients := make([]string, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		normalizedAddress := normalizeEVMAddress(event.To)
		if _, ok := seen[normalizedAddress]; ok {
			continue
		}
		seen[normalizedAddress] = struct{}{}
		recipients = append(recipients, normalizedAddress)
	}
	return recipients
}

func normalizeEVMAddress(address common.Address) string {
	return strings.ToLower(address.Hex())
}
