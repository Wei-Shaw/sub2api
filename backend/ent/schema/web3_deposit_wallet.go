package schema

import (
	"fmt"
	"regexp"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

const maxWeb3DepositDerivationIndex = int64(1 << 31)

var (
	web3DepositWalletIDPattern          = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)
	web3DepositWalletAccountPathPattern = regexp.MustCompile(`^m/44'/60'/(0|[1-9][0-9]*)'$`)
	web3DepositWalletFingerprintPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Web3DepositWallet struct {
	ent.Schema
}

func (Web3DepositWallet) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "web3_deposit_wallets"},
	}
}

func (Web3DepositWallet) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Web3DepositWallet) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			Immutable(),
		field.String("wallet_id").
			MaxLen(64).
			Unique().
			Immutable().
			Validate(validateWeb3DepositWalletID),
		field.String("account_path").
			MaxLen(64).
			Immutable().
			Validate(validateWeb3DepositWalletAccountPath),
		field.String("xpub_fingerprint").
			MaxLen(64).
			Immutable().
			Validate(validateWeb3DepositWalletFingerprint),
		field.Int64("next_derivation_index").
			Default(0).
			Validate(validateWeb3DepositDerivationIndex),
		field.String("status").
			MaxLen(20).
			Default("active").
			Validate(validateWeb3DepositWalletStatus),
	}
}

func validateWeb3DepositWalletID(walletID string) error {
	if !web3DepositWalletIDPattern.MatchString(walletID) {
		return fmt.Errorf("invalid web3 deposit wallet ID")
	}
	return nil
}

func validateWeb3DepositWalletAccountPath(accountPath string) error {
	if !web3DepositWalletAccountPathPattern.MatchString(accountPath) {
		return fmt.Errorf("invalid web3 deposit wallet account path")
	}
	return nil
}

func validateWeb3DepositWalletFingerprint(fingerprint string) error {
	if !web3DepositWalletFingerprintPattern.MatchString(fingerprint) {
		return fmt.Errorf("invalid web3 deposit wallet fingerprint")
	}
	return nil
}

func validateWeb3DepositDerivationIndex(index int64) error {
	if index < 0 || index >= maxWeb3DepositDerivationIndex {
		return fmt.Errorf("web3 deposit derivation index must be between 0 and %d", maxWeb3DepositDerivationIndex-1)
	}
	return nil
}

func validateWeb3DepositWalletStatus(status string) error {
	switch status {
	case "active", "disabled":
		return nil
	default:
		return fmt.Errorf("invalid web3 deposit wallet status %q", status)
	}
}
