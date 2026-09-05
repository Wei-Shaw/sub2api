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

var (
	web3ScannerKeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9:_-]{0,127}$`)
	web3LeaseValuePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

type Web3ScannerCursor struct {
	ent.Schema
}

func (Web3ScannerCursor) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "web3_scanner_cursors"},
	}
}

func (Web3ScannerCursor) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (Web3ScannerCursor) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			Immutable(),
		field.String("scanner_key").
			MaxLen(128).
			Unique().
			Immutable().
			Validate(validateWeb3ScannerKey),
		field.Int64("chain_id").
			Positive().
			Immutable(),
		field.String("token_contract").
			MaxLen(42).
			Immutable().
			Validate(validateWeb3DepositCanonicalAddress),
		field.Int64("scan_start_block").
			NonNegative().
			Immutable(),
		field.Int64("last_scanned_block").
			NonNegative(),
		field.Int64("last_finalized_block").
			NonNegative(),
		field.String("lease_owner").
			MaxLen(128).
			Optional().
			Nillable().
			Validate(validateWeb3LeaseValue),
		field.String("lease_token").
			MaxLen(128).
			Optional().
			Nillable().
			Validate(validateWeb3LeaseValue),
		field.Time("lease_expires_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("last_error").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("last_success_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (Web3ScannerCursor) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("chain_id", "token_contract").Unique(),
		index.Fields("lease_expires_at"),
	}
}

func validateWeb3ScannerKey(value string) error {
	if !web3ScannerKeyPattern.MatchString(value) {
		return fmt.Errorf("invalid web3 scanner key")
	}
	return nil
}

func validateWeb3LeaseValue(value string) error {
	if !web3LeaseValuePattern.MatchString(value) {
		return fmt.Errorf("invalid web3 scanner lease value")
	}
	return nil
}
