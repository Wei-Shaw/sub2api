package schema

import (
 "entgo.io/ent"
 "entgo.io/ent/dialect"
 "entgo.io/ent/dialect/entsql"
 "entgo.io/ent/schema"
 "entgo.io/ent/schema/field"
 "entgo.io/ent/schema/index"
 "github.com/Wei-Shaw/sub2api/ent/schema/mixins"
)

type AccountSchedulingLease struct { ent.Schema }
func (AccountSchedulingLease) Annotations() []schema.Annotation { return []schema.Annotation{entsql.Annotation{Table:"account_scheduling_leases"}} }
func (AccountSchedulingLease) Mixin() []ent.Mixin { return []ent.Mixin{mixins.TimeMixin{}} }
func (AccountSchedulingLease) Fields() []ent.Field { return []ent.Field{field.String("lease_id").Unique().MaxLen(100),field.String("identity").MaxLen(64),field.Time("expires_at").SchemaType(map[string]string{dialect.Postgres:"timestamptz"})} }
func (AccountSchedulingLease) Indexes() []ent.Index { return []ent.Index{index.Fields("identity","expires_at")} }
