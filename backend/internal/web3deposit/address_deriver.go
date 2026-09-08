package web3deposit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ErrAccountXPubInvalid        = errors.New("web3 deposit account xpub is invalid")
	ErrDerivationIndexOutOfRange = errors.New("web3 deposit derivation index is out of range")
	ErrAddressDerivationFailed   = errors.New("web3 deposit address derivation failed")
)

type DerivedEVMAddress struct {
	DerivationIndex   int64
	Address           string
	NormalizedAddress string
}

func DeriveEVMAddress(accountXPub string, derivationIndex int64) (DerivedEVMAddress, error) {
	if derivationIndex < 0 || derivationIndex >= MaxDerivationIndexExclusive {
		return DerivedEVMAddress{}, ErrDerivationIndexOutOfRange
	}

	accountKey, err := parseAccountXPub(accountXPub)
	if err != nil {
		return DerivedEVMAddress{}, err
	}
	defer accountKey.Zero()

	externalKey, err := accountKey.Derive(0)
	if err != nil {
		return DerivedEVMAddress{}, fmt.Errorf("%w: derive external branch", ErrAddressDerivationFailed)
	}
	defer externalKey.Zero()
	childKey, err := externalKey.Derive(uint32(derivationIndex))
	if err != nil {
		return DerivedEVMAddress{}, fmt.Errorf("%w: derive child", ErrAddressDerivationFailed)
	}
	defer childKey.Zero()

	publicKey, err := childKey.ECPubKey()
	if err != nil {
		return DerivedEVMAddress{}, fmt.Errorf("%w: read child public key", ErrAddressDerivationFailed)
	}
	uncompressedPublicKey := publicKey.SerializeUncompressed()
	hash := crypto.Keccak256(uncompressedPublicKey[1:])
	address := common.BytesToAddress(hash[12:]).Hex()

	return DerivedEVMAddress{
		DerivationIndex:   derivationIndex,
		Address:           address,
		NormalizedAddress: strings.ToLower(address),
	}, nil
}

func parseAccountXPub(accountXPub string) (*hdkeychain.ExtendedKey, error) {
	accountKey, err := hdkeychain.NewKeyFromString(strings.TrimSpace(accountXPub))
	if err != nil {
		return nil, ErrAccountXPubInvalid
	}
	if accountKey.IsPrivate() || accountKey.Depth() != 3 || accountKey.ChildIndex() < hdkeychain.HardenedKeyStart {
		accountKey.Zero()
		return nil, ErrAccountXPubInvalid
	}
	return accountKey, nil
}
