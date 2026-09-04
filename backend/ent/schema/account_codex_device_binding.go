package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountCodexDeviceBinding struct{ ent.Schema }

func (AccountCodexDeviceBinding) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.Int64("api_key_id"),
		field.String("os_class").MaxLen(20),
		field.String("canonical_surface").MaxLen(20),
		field.Int64("slot_id"),
		field.Int64("policy_version"),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (AccountCodexDeviceBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "api_key_id", "os_class", "canonical_surface").Unique(),
		index.Fields("api_key_id", "os_class", "canonical_surface"),
	}
}
