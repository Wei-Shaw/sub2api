package schema

import (
	"fmt"
	"regexp"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/web3deposit"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

var (
	web3DepositAddressPattern           = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	web3DepositNormalizedAddressPattern = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
)

type Web3DepositAddress struct {
	ent.Schema
}

func (Web3DepositAddress) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "web3_deposit_addresses"},
	}
}

func (Web3DepositAddress) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Web3DepositAddress) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.String("wallet_id").
			MaxLen(64).
			Immutable().
			Validate(validateWeb3DepositWalletID),
		field.Int64("derivation_index").
			Immutable().
			Validate(validateWeb3DepositDerivationIndex),
		field.String("address").
			MaxLen(42).
			Immutable().
			Validate(validateWeb3DepositAddress),
		field.String("normalized_address").
			MaxLen(42).
			Immutable().
			Validate(validateWeb3DepositNormalizedAddress),
		field.String("status").
			MaxLen(20).
			Default(string(web3deposit.AddressStatusActive)).
			Validate(validateWeb3DepositAddressStatus),
		field.Time("allocated_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("disabled_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_deposit_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (Web3DepositAddress) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "wallet_id").Unique(),
		index.Fields("wallet_id", "derivation_index").Unique(),
		index.Fields("normalized_address").Unique(),
		index.Fields("user_id", "created_at"),
	}
}

func validateWeb3DepositAddress(address string) error {
	if !web3DepositAddressPattern.MatchString(address) {
		return fmt.Errorf("invalid web3 deposit address")
	}
	return nil
}

func validateWeb3DepositNormalizedAddress(address string) error {
	if !web3DepositNormalizedAddressPattern.MatchString(address) {
		return fmt.Errorf("invalid normalized web3 deposit address")
	}
	return nil
}

func validateWeb3DepositAddressStatus(status string) error {
	if !web3deposit.AddressStatus(status).IsValid() {
		return fmt.Errorf("invalid web3 deposit address status %q", status)
	}
	return nil
}
