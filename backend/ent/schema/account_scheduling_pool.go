package schema

import (
 "entgo.io/ent"
 "entgo.io/ent/dialect/entsql"
 "entgo.io/ent/schema"
 "entgo.io/ent/schema/field"
 "github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

type AccountSchedulingPool struct { ent.Schema }
func (AccountSchedulingPool) Annotations() []schema.Annotation { return []schema.Annotation{entsql.Annotation{Table:"account_scheduling_pools"}} }
func (AccountSchedulingPool) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }
func (AccountSchedulingPool) Fields() []ent.Field { return []ent.Field{field.String("name").NotEmpty().MaxLen(100),field.Bool("enabled").Default(false)} }
