package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

type AccountCodexIdentityPolicy struct{ ent.Schema }

func (AccountCodexIdentityPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id").Unique(),
		field.String("mode").Default("off").MaxLen(40),
		field.String("binding_scope").Default("api_key_os_surface").MaxLen(40),
		field.JSON("session_policy", map[string]any{}).
			Default(func() map[string]any { return map[string]any{"mode": "conversation_isolated"} }),
		field.Int("affinity_ttl_seconds").Default(3600),
		field.String("unsupported_policy").Default("reject").MaxLen(40),
		field.Int64("version").Default(1),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}
