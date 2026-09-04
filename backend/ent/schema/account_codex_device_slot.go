package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AccountCodexDeviceSlot struct{ ent.Schema }

func (AccountCodexDeviceSlot) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.Int64("profile_id"),
		field.Int("slot_index"),
		field.String("proxy_mode").Default("inherit").MaxLen(20),
		field.Int64("proxy_id").Optional().Nillable(),
		field.String("client_version_mode").Default("inherit").MaxLen(20),
		field.String("client_version").Default("").MaxLen(64),
		field.Int64("epoch"),
		field.String("state").Default("active").MaxLen(20),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (AccountCodexDeviceSlot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("profile_id", "slot_index", "epoch").Unique(),
		index.Fields("id", "account_id").Unique(),
		index.Fields("account_id", "state"),
	}
}
