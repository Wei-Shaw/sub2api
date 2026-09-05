package schema

import (
	"fmt"
	"regexp"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

var web3AssetKeyPattern = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

type Web3UserBalance struct {
	ent.Schema
}

func (Web3UserBalance) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "web3_user_balances"},
	}
}

func (Web3UserBalance) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Web3UserBalance) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.String("asset_key").
			MaxLen(64).
			Immutable().
			Validate(validateWeb3AssetKey),
		field.String("available_amount").
			Default("0").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validateWeb3BalanceAmount),
		field.String("total_deposited").
			Default("0").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validateWeb3BalanceAmount),
		field.String("total_transferred").
			Default("0").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validateWeb3BalanceAmount),
		field.Int64("balance_version").
			Default(0).
			NonNegative(),
	}
}

func (Web3UserBalance) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "asset_key").Unique(),
	}
}

func validateWeb3AssetKey(value string) error {
	if !web3AssetKeyPattern.MatchString(value) {
		return fmt.Errorf("invalid web3 asset key")
	}
	return nil
}

func validateWeb3BalanceAmount(value string) error {
	return validateWeb3NonNegativeDecimal(value, 20, 8, "balance amount")
}
