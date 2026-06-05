package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// IdempotencyRecord 幂等请求记录表。
type IdempotencyRecord struct {
	ent.Schema
}

func (IdempotencyRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "idempotency_records"},
	}
}

func (IdempotencyRecord) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (IdempotencyRecord) Fields() []ent.Field {
	return []ent.Field{
		field.String("scope").MaxLen(128),
		field.String("idempotency_key_hash").MaxLen(64),
		field.String("request_fingerprint").MaxLen(64),
		field.String("status").MaxLen(32),
		field.Int("response_status").Optional().Nillable(),
		field.String("response_body").Optional().Nillable(),
		field.String("error_reason").MaxLen(128).Optional().Nillable(),
		field.Time("locked_until").Optional().Nillable(),
		field.Time("expires_at"),
	}
}

func (IdempotencyRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope", "idempotency_key_hash").Unique(),
		index.Fields("expires_at"),
		index.Fields("status", "locked_until"),
	}
}
