package web3deposit

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestRecipientMatcherMatchesActiveAddressesInEventOrder(t *testing.T) {
	firstRecipient := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	secondRecipient := common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	events := []TransferEvent{
		newRecipientMatcherEvent(t, 1, secondRecipient),
		newRecipientMatcherEvent(t, 2, common.HexToAddress("0x9999999999999999999999999999999999999999")),
		newRecipientMatcherEvent(t, 3, firstRecipient),
	}
	lookup := &depositAddressLookupStub{addresses: []DepositAddress{
		{ID: 11, UserID: 101, NormalizedAddress: strings.ToLower(firstRecipient.Hex()), Status: AddressStatusActive},
		{ID: 12, UserID: 102, NormalizedAddress: strings.ToLower(secondRecipient.Hex()), Status: AddressStatusActive},
	}}

	matches, err := NewRecipientMatcher(lookup, DefaultRecipientLookupChunkSize).Match(context.Background(), events)
	require.NoError(t, err)
	require.Equal(t, []MatchedTransferEvent{
		{Event: events[0], DepositAddressID: 12, UserID: 102},
		{Event: events[2], DepositAddressID: 11, UserID: 101},
	}, matches)
}

func TestRecipientMatcherMatchesDisabledHistoricalAddress(t *testing.T) {
	recipient := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	event := newRecipientMatcherEvent(t, 1, recipient)
	lookup := &depositAddressLookupStub{addresses: []DepositAddress{{
		ID:                11,
		UserID:            101,
		NormalizedAddress: strings.ToLower(recipient.Hex()),
		Status:            AddressStatusDisabled,
	}}}

	matches, err := NewRecipientMatcher(lookup, DefaultRecipientLookupChunkSize).Match(context.Background(), []TransferEvent{event})

	require.NoError(t, err)
	require.Equal(t, []MatchedTransferEvent{{Event: event, DepositAddressID: 11, UserID: 101}}, matches)
}

func TestRecipientMatcherNormalizesDeduplicatesAndChunksRecipients(t *testing.T) {
	recipients := []common.Address{
		common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"),
		common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		common.HexToAddress("0x3333333333333333333333333333333333333333"),
	}
	events := make([]TransferEvent, 0, len(recipients)+1)
	for index, recipient := range recipients {
		events = append(events, newRecipientMatcherEvent(t, uint64(index+1), recipient))
	}
	events = append(events, newRecipientMatcherEvent(t, 6, recipients[0]))
	lookup := &depositAddressLookupStub{}

	matches, err := NewRecipientMatcher(lookup, 2).Match(context.Background(), events)
	require.NoError(t, err)
	require.NotNil(t, matches)
	require.Empty(t, matches)
	require.Equal(t, []int{2, 2, 1}, lookup.callSizes())
	require.Equal(t, strings.ToLower(recipients[0].Hex()), lookup.calls[0][0])
	require.Equal(t, len(recipients), lookup.uniqueRecipientCount())
}

func TestRecipientMatcherFailsWholeBatchOnLookupError(t *testing.T) {
	wantErr := errors.New("database unavailable")
	events := []TransferEvent{
		newRecipientMatcherEvent(t, 1, common.HexToAddress("0x1111111111111111111111111111111111111111")),
		newRecipientMatcherEvent(t, 2, common.HexToAddress("0x2222222222222222222222222222222222222222")),
	}
	lookup := &depositAddressLookupStub{
		addresses: []DepositAddress{{
			ID:                11,
			UserID:            101,
			NormalizedAddress: strings.ToLower(events[0].To.Hex()),
		}},
		errAtCall: 2,
		err:       wantErr,
	}

	matches, err := NewRecipientMatcher(lookup, 1).Match(context.Background(), events)
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, matches)
}

func TestRecipientMatcherSkipsLookupForEmptyEvents(t *testing.T) {
	lookup := &depositAddressLookupStub{}

	matches, err := NewRecipientMatcher(lookup, 0).Match(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, matches)
	require.Empty(t, matches)
	require.Empty(t, lookup.calls)
}

func newRecipientMatcherEvent(t *testing.T, logIndex uint64, recipient common.Address) TransferEvent {
	t.Helper()
	event, err := NewTransferEvent(
		DepositEventID{ChainID: 1030, LogIndex: logIndex},
		100,
		common.Hash{},
		common.Address{},
		recipient,
		big.NewInt(1),
	)
	require.NoError(t, err)
	return event
}

type depositAddressLookupStub struct {
	addresses []DepositAddress
	calls     [][]string
	errAtCall int
	err       error
}

func (s *depositAddressLookupStub) ListByNormalizedAddresses(_ context.Context, normalizedAddresses []string) ([]DepositAddress, error) {
	s.calls = append(s.calls, append([]string(nil), normalizedAddresses...))
	if s.errAtCall == len(s.calls) {
		return nil, s.err
	}
	requested := make(map[string]struct{}, len(normalizedAddresses))
	for _, address := range normalizedAddresses {
		requested[address] = struct{}{}
	}
	addresses := make([]DepositAddress, 0, len(s.addresses))
	for _, address := range s.addresses {
		if _, ok := requested[address.NormalizedAddress]; ok {
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

func (s *depositAddressLookupStub) callSizes() []int {
	sizes := make([]int, 0, len(s.calls))
	for _, call := range s.calls {
		sizes = append(sizes, len(call))
	}
	return sizes
}

func (s *depositAddressLookupStub) uniqueRecipientCount() int {
	recipients := make(map[string]struct{})
	for _, call := range s.calls {
		for _, address := range call {
			recipients[address] = struct{}{}
		}
	}
	return len(recipients)
}
