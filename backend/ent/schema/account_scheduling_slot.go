package schema

import (
 "time"
 "entgo.io/ent"
 "entgo.io/ent/dialect"
 "entgo.io/ent/dialect/entsql"
 "entgo.io/ent/schema"
 "entgo.io/ent/schema/field"
 "entgo.io/ent/schema/index"
 "github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

type AccountSchedulingSlot struct { ent.Schema }
func (AccountSchedulingSlot) Annotations() []schema.Annotation { return []schema.Annotation{entsql.Annotation{Table:"account_scheduling_slots"}} }
func (AccountSchedulingSlot) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }
func (AccountSchedulingSlot) Fields() []ent.Field { return []ent.Field{
 field.String("identity").Unique().MaxLen(64),field.Int64("account_id"),field.String("exit_group").MaxLen(100),
 field.String("state").Default("standby").MaxLen(20),field.String("reason").Default("ready").MaxLen(100),
 field.Time("queued_at").Default(time.Now).SchemaType(map[string]string{dialect.Postgres:"timestamptz"}),
 } }
func (AccountSchedulingSlot) Indexes() []ent.Index { return []ent.Index{index.Fields("exit_group","state")} }
