package schema

import (
	"fmt"
	"regexp"
	"strings"
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
	web3DepositHashPattern             = regexp.MustCompile(`^0x[0-9a-f]{64}$`)
	web3DepositCanonicalAddressPattern = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
	web3DepositPositiveIntegerPattern  = regexp.MustCompile(`^[1-9][0-9]*$`)
	web3DepositPositiveDecimalPattern  = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
)

type Web3Deposit struct {
	ent.Schema
}

func (Web3Deposit) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "web3_deposits"},
	}
}

func (Web3Deposit) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Web3Deposit) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			Immutable(),
		field.Int64("user_id").
			Immutable(),
		field.Int64("deposit_address_id").
			Immutable(),
		field.Int64("chain_id").
			Positive().
			Immutable(),
		field.String("token_contract").
			MaxLen(42).
			Immutable().
			Validate(validateWeb3DepositCanonicalAddress),
		field.String("tx_hash").
			MaxLen(66).
			Immutable().
			Validate(validateWeb3DepositHash),
		field.Int64("log_index").
			NonNegative().
			Immutable(),
		field.Int64("block_number").
			NonNegative().
			Immutable(),
		field.String("block_hash").
			MaxLen(66).
			Immutable().
			Validate(validateWeb3DepositHash),
		field.String("from_address").
			MaxLen(42).
			Immutable().
			Validate(validateWeb3DepositCanonicalAddress),
		field.String("to_address").
			MaxLen(42).
			Immutable().
			Validate(validateWeb3DepositCanonicalAddress),
		field.String("raw_amount").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "numeric(78,0)"}).
			Validate(validateWeb3DepositRawAmount),
		field.Int16("token_decimals").
			Immutable().
			Validate(validateWeb3DepositTokenDecimals),
		field.String("token_amount").
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "numeric(38,18)"}).
			Validate(func(value string) error {
				return validateWeb3DepositDecimal(value, 38, 18, "token amount")
			}),
		field.String("credited_amount").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Validate(func(value string) error {
				return validateWeb3DepositDecimal(value, 20, 8, "credited amount")
			}),
		field.String("status").
			MaxLen(32).
			Default(string(web3deposit.DepositStatusDetected)).
			Validate(validateWeb3DepositStatus),
		field.String("review_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("failure_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int32("retry_count").
			Default(0).
			NonNegative(),
		field.Time("next_retry_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("detected_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("finalized_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("credited_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (Web3Deposit) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("chain_id", "tx_hash", "log_index").Unique(),
		index.Fields("status", "next_retry_at", "id"),
		index.Fields("user_id", "created_at", "id"),
		index.Fields("block_number", "id"),
		index.Fields("deposit_address_id", "created_at"),
	}
}

func validateWeb3DepositHash(value string) error {
	if !web3DepositHashPattern.MatchString(value) {
		return fmt.Errorf("invalid web3 deposit hash")
	}
	return nil
}

func validateWeb3DepositCanonicalAddress(value string) error {
	if !web3DepositCanonicalAddressPattern.MatchString(value) {
		return fmt.Errorf("invalid canonical web3 deposit address")
	}
	return nil
}

func validateWeb3DepositRawAmount(value string) error {
	if !web3DepositPositiveIntegerPattern.MatchString(value) || len(value) > 78 {
		return fmt.Errorf("invalid web3 deposit raw amount")
	}
	return nil
}

func validateWeb3DepositTokenDecimals(value int16) error {
	if value < 0 || value > 255 {
		return fmt.Errorf("web3 deposit token decimals must be between 0 and 255")
	}
	return nil
}

func validateWeb3DepositDecimal(value string, precision, scale int, fieldName string) error {
	if !web3DepositPositiveDecimalPattern.MatchString(value) {
		return fmt.Errorf("invalid web3 deposit %s", fieldName)
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
		return fmt.Errorf("web3 deposit %s exceeds decimal(%d,%d)", fieldName, precision, scale)
	}
	allZero := strings.Trim(value, "0.") == ""
	if allZero {
		return fmt.Errorf("web3 deposit %s must be positive", fieldName)
	}
	return nil
}

func validateWeb3DepositStatus(status string) error {
	if !web3deposit.DepositStatus(status).IsValid() {
		return fmt.Errorf("invalid web3 deposit status %q", status)
	}
	return nil
}
