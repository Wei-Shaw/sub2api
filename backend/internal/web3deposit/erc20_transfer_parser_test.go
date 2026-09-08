package web3deposit

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"
)

func TestParseERC20TransferLog(t *testing.T) {
	tokenAddress := common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff")
	from := common.HexToAddress("0x1111111111111111111111111111111111111111")
	to := common.HexToAddress("0x2222222222222222222222222222222222222222")
	maximumUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	log := validERC20TransferLog(tokenAddress, from, to, maximumUint256)

	event, err := ParseERC20TransferLog(1030, tokenAddress, log)
	require.NoError(t, err)
	require.Equal(t, uint64(1030), event.ID.ChainID)
	require.Equal(t, log.TxHash, event.ID.TxHash)
	require.Equal(t, uint64(log.Index), event.ID.LogIndex)
	require.Equal(t, log.BlockNumber, event.BlockNumber)
	require.Equal(t, uint64(log.TxIndex), event.TransactionIndex)
	require.Equal(t, log.BlockHash, event.BlockHash)
	require.Equal(t, from, event.From)
	require.Equal(t, to, event.To)
	require.Zero(t, maximumUint256.Cmp(event.RawAmount()))
}

func TestParseERC20TransferLogAllowsZeroAmount(t *testing.T) {
	tokenAddress := common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff")
	log := validERC20TransferLog(tokenAddress, common.Address{}, common.Address{}, big.NewInt(0))

	event, err := ParseERC20TransferLog(1030, tokenAddress, log)
	require.NoError(t, err)
	require.Zero(t, event.RawAmount().Sign())
}

func TestParseERC20TransferLogRejectsMalformedLogsWithoutPanic(t *testing.T) {
	tokenAddress := common.HexToAddress("0xaf37e8b6c9ed7f6318979f56fc287d76c30847ff")
	base := validERC20TransferLog(
		tokenAddress,
		common.HexToAddress("0x1111111111111111111111111111111111111111"),
		common.HexToAddress("0x2222222222222222222222222222222222222222"),
		big.NewInt(1_000_000),
	)

	tests := []struct {
		name    string
		mutate  func(*types.Log)
		wantErr error
	}{
		{name: "removed", mutate: func(log *types.Log) { log.Removed = true }, wantErr: ErrTransferLogRemoved},
		{name: "wrong token", mutate: func(log *types.Log) {
			log.Address = common.HexToAddress("0x3333333333333333333333333333333333333333")
		}, wantErr: ErrTransferTokenMismatch},
		{name: "missing topics", mutate: func(log *types.Log) { log.Topics = nil }, wantErr: ErrTransferTopicCountInvalid},
		{name: "too few topics", mutate: func(log *types.Log) { log.Topics = log.Topics[:2] }, wantErr: ErrTransferTopicCountInvalid},
		{name: "too many topics", mutate: func(log *types.Log) { log.Topics = append(log.Topics, common.Hash{}) }, wantErr: ErrTransferTopicCountInvalid},
		{name: "wrong signature", mutate: func(log *types.Log) { log.Topics[0] = common.HexToHash("0x01") }, wantErr: ErrTransferSignatureInvalid},
		{name: "from padding", mutate: func(log *types.Log) { log.Topics[1][0] = 1 }, wantErr: ErrTransferAddressInvalid},
		{name: "to padding", mutate: func(log *types.Log) { log.Topics[2][11] = 1 }, wantErr: ErrTransferAddressInvalid},
		{name: "empty data", mutate: func(log *types.Log) { log.Data = nil }, wantErr: ErrTransferDataInvalid},
		{name: "short data", mutate: func(log *types.Log) { log.Data = make([]byte, 31) }, wantErr: ErrTransferDataInvalid},
		{name: "long data", mutate: func(log *types.Log) { log.Data = make([]byte, 33) }, wantErr: ErrTransferDataInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			log := cloneERC20TransferLog(base)
			test.mutate(&log)

			require.NotPanics(t, func() {
				_, err := ParseERC20TransferLog(1030, tokenAddress, log)
				require.Error(t, err)
				require.True(t, errors.Is(err, test.wantErr))
			})
		})
	}
}

func validERC20TransferLog(tokenAddress, from, to common.Address, rawAmount *big.Int) types.Log {
	data := make([]byte, common.HashLength)
	rawAmount.FillBytes(data)
	return types.Log{
		Address: tokenAddress,
		Topics: []common.Hash{
			erc20TransferTopic,
			common.BytesToHash(from.Bytes()),
			common.BytesToHash(to.Bytes()),
		},
		Data:        data,
		BlockNumber: 123,
		TxHash:      common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		TxIndex:     4,
		BlockHash:   common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
		Index:       7,
	}
}

func cloneERC20TransferLog(log types.Log) types.Log {
	cloned := log
	cloned.Topics = append([]common.Hash(nil), log.Topics...)
	cloned.Data = append([]byte(nil), log.Data...)
	return cloned
}
