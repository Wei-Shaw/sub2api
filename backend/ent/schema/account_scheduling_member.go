package schema

import (
 "entgo.io/ent"
 "entgo.io/ent/dialect/entsql"
 "entgo.io/ent/schema"
 "entgo.io/ent/schema/field"
 "entgo.io/ent/schema/index"
 "github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

type AccountSchedulingMember struct { ent.Schema }
func (AccountSchedulingMember) Annotations() []schema.Annotation { return []schema.Annotation{entsql.Annotation{Table:"account_scheduling_members"}} }
func (AccountSchedulingMember) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }
func (AccountSchedulingMember) Fields() []ent.Field { return []ent.Field{field.Int64("account_id").Unique(),field.Int64("pool_id"),field.Int64("position").Default(0)} }
func (AccountSchedulingMember) Indexes() []ent.Index { return []ent.Index{index.Fields("pool_id","position")} }
