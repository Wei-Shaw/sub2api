package schema

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

var web3SignedDecimalPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)

type Web3BalanceTransfer struct {
	ent.Schema
}

func (Web3BalanceTransfer) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "web3_balance_transfers"},
	}
}

func (Web3BalanceTransfer) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.Int64("web3_balance_id").
			Immutable(),
		field.String("amount").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(func(value string) error {
				return validateWeb3DepositDecimal(value, 20, 8, "balance transfer amount")
			}),
		field.String("web3_balance_before").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validateWeb3TransferSnapshotAmount),
		field.String("web3_balance_after").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validateWeb3TransferSnapshotAmount),
		field.String("user_balance_before").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validateWeb3UserBalanceSnapshotAmount),
		field.String("user_balance_after").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(validateWeb3UserBalanceSnapshotAmount),
		field.String("idempotency_key").
			MaxLen(180).
			NotEmpty().
			Unique().
			Immutable(),
		field.JSON("metadata", map[string]any{}).
			Default(map[string]any{}).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (Web3BalanceTransfer) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at", "id"),
		index.Fields("web3_balance_id", "created_at", "id"),
	}
}

func validateWeb3TransferSnapshotAmount(value string) error {
	return validateWeb3NonNegativeDecimal(value, 20, 8, "balance transfer snapshot")
}

func validateWeb3UserBalanceSnapshotAmount(value string) error {
	return validateWeb3SignedDecimal(value, 20, 8, "user balance transfer snapshot")
}

func validateWeb3NonNegativeDecimal(value string, precision, scale int, fieldName string) error {
	if !web3DepositPositiveDecimalPattern.MatchString(value) {
		return fmt.Errorf("invalid web3 %s", fieldName)
	}
	parts := strings.SplitN(value, ".", 2)
	integerDigits := len(parts[0])
	if parts[0] == "0" {
		integerDigits = 0
	}
	fractionDigits := 0
	if len(parts) == 2 {
		fractionDigits = len(parts[1])
	}
	if integerDigits > precision-scale || fractionDigits > scale {
		return fmt.Errorf("web3 %s exceeds decimal(%d,%d)", fieldName, precision, scale)
	}
	return nil
}

func validateWeb3SignedDecimal(value string, precision, scale int, fieldName string) error {
	if !web3SignedDecimalPattern.MatchString(value) {
		return fmt.Errorf("invalid web3 %s", fieldName)
	}
	unsignedValue := strings.TrimPrefix(value, "-")
	parts := strings.SplitN(unsignedValue, ".", 2)
	integerDigits := len(parts[0])
	if parts[0] == "0" {
		integerDigits = 0
	}
	fractionDigits := 0
	if len(parts) == 2 {
		fractionDigits = len(parts[1])
	}
	if integerDigits > precision-scale || fractionDigits > scale {
		return fmt.Errorf("web3 %s exceeds decimal(%d,%d)", fieldName, precision, scale)
	}
	return nil
}
