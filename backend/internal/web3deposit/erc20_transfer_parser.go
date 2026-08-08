package web3deposit

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	erc20TransferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

	ErrTransferLogRemoved        = errors.New("erc20 transfer log is removed")
	ErrTransferTokenMismatch     = errors.New("erc20 transfer token address mismatch")
	ErrTransferTopicCountInvalid = errors.New("erc20 transfer topic count is invalid")
	ErrTransferSignatureInvalid  = errors.New("erc20 transfer signature is invalid")
	ErrTransferAddressInvalid    = errors.New("erc20 transfer address topic is invalid")
	ErrTransferDataInvalid       = errors.New("erc20 transfer data is invalid")
)

func ParseERC20TransferLog(chainID uint64, tokenAddress common.Address, log types.Log) (TransferEvent, error) {
	if log.Removed {
		return TransferEvent{}, ErrTransferLogRemoved
	}
	if log.Address != tokenAddress {
		return TransferEvent{}, ErrTransferTokenMismatch
	}
	if len(log.Topics) != 3 {
		return TransferEvent{}, ErrTransferTopicCountInvalid
	}
	if log.Topics[0] != erc20TransferTopic {
		return TransferEvent{}, ErrTransferSignatureInvalid
	}

	from, err := parseERC20AddressTopic(log.Topics[1])
	if err != nil {
		return TransferEvent{}, fmt.Errorf("parse erc20 transfer from address: %w", err)
	}
	to, err := parseERC20AddressTopic(log.Topics[2])
	if err != nil {
		return TransferEvent{}, fmt.Errorf("parse erc20 transfer to address: %w", err)
	}
	if len(log.Data) != common.HashLength {
		return TransferEvent{}, ErrTransferDataInvalid
	}

	return NewTransferEvent(
		DepositEventID{
			ChainID:  chainID,
			TxHash:   log.TxHash,
			LogIndex: uint64(log.Index),
		},
		log.BlockNumber,
		log.BlockHash,
		from,
		to,
		new(big.Int).SetBytes(log.Data),
	)
}

func parseERC20AddressTopic(topic common.Hash) (common.Address, error) {
	for _, value := range topic[:common.HashLength-common.AddressLength] {
		if value != 0 {
			return common.Address{}, ErrTransferAddressInvalid
		}
	}
	return common.BytesToAddress(topic[common.HashLength-common.AddressLength:]), nil
}
