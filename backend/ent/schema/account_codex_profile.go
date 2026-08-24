package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountCodexProfile struct{ ent.Schema }

func (AccountCodexProfile) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.String("os_class").MaxLen(20),
		field.String("canonical_surface").MaxLen(20),
		field.String("architecture").Optional().Nillable().MaxLen(20),
		field.String("proxy_mode").Default("inherit").MaxLen(20),
		field.Int64("proxy_id").Optional().Nillable(),
		field.Int("slot_count"),
		field.Int64("epoch"),
		field.Int64("catalog_version").Default(1),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (AccountCodexProfile) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "os_class", "epoch").Unique(),
		index.Fields("id", "account_id").Unique(),
	}
}
