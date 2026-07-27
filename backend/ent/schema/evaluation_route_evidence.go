package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// EvaluationRouteEvidence stores redacted routing, transport, usage, and billing evidence.
type EvaluationRouteEvidence struct {
	ent.Schema
}

func (EvaluationRouteEvidence) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "evaluation_route_evidence"},
	}
}

func (EvaluationRouteEvidence) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").StorageKey("route_trace_id").MaxLen(64).Immutable(),
		field.String("evaluation_run_id").SchemaType(map[string]string{dialect.Postgres: "uuid"}).Immutable(),
		field.String("sample_id").SchemaType(map[string]string{dialect.Postgres: "uuid"}).Immutable(),
		field.Int64("api_key_id").Immutable(),
		field.String("request_id").MaxLen(128).Optional().Nillable(),
		field.String("requested_model").MaxLen(200).Immutable(),
		field.String("resolved_model").MaxLen(200).Optional().Nillable(),
		field.String("route_profile_version").MaxLen(100).Immutable(),
		field.String("provider").MaxLen(32).Optional().Nillable(),
		field.String("channel_ref").MaxLen(64).Optional().Nillable(),
		field.String("account_pool_ref").MaxLen(64).Optional().Nillable(),
		field.String("region").MaxLen(64).Immutable(),
		field.Int("attempts").Default(0).NonNegative(),
		field.JSON("fallback_chain", []map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("finish_reason").MaxLen(64).Optional().Nillable(),
		field.Int("input_tokens").Optional().Nillable().NonNegative(),
		field.Int("output_tokens").Optional().Nillable().NonNegative(),
		field.Int("ttft_ms").Optional().Nillable().NonNegative(),
		field.Int("latency_ms").Optional().Nillable().NonNegative(),
		field.Float("billed_amount").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.String("transport_status").MaxLen(24).Default("started"),
		field.String("error_code").MaxLen(100).Optional().Nillable(),
		field.Time("started_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Immutable(),
		field.Time("finished_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Optional().Nillable(),
		field.Time("created_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Default(time.Now).Immutable(),
		field.Time("updated_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).Default(time.Now).UpdateDefault(time.Now),
	}
}

func (EvaluationRouteEvidence) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("api_key", APIKey.Type).
			Ref("evaluation_route_evidence").
			Field("api_key_id").
			Unique().
			Required().
			Immutable().
			Annotations(entsql.OnDelete(entsql.NoAction)),
	}
}

func (EvaluationRouteEvidence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("evaluation_run_id", "sample_id"),
		index.Fields("requested_model", "finished_at"),
	}
}
