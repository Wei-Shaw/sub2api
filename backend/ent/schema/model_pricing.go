package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelPricing struct {
	ent.Schema
}

func (ModelPricing) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "model_pricings"},
	}
}

func (ModelPricing) Fields() []ent.Field {
	return []ent.Field{
		field.String("model").
			MaxLen(200).
			NotEmpty().
			Comment("Model name (lowercase, unique key)"),
		field.Float("input_cost_per_token").
			Default(0).
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("output_cost_per_token").
			Default(0).
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("cache_creation_input_token_cost").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("cache_creation_input_token_cost_above_1hr").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("cache_read_input_token_cost").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("input_cost_per_token_priority").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("output_cost_per_token_priority").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("cache_read_input_token_cost_priority").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("output_cost_per_image").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("output_cost_per_image_token").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Int("long_context_input_token_threshold").
			Default(0).
			Optional().
			Nillable(),
		field.Float("long_context_input_cost_multiplier").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Float("long_context_output_cost_multiplier").
			Default(0).
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "double precision",
			}),
		field.Bool("supports_service_tier").
			Default(false).
			Optional(),
		field.String("litellm_provider").
			Default("").
			Optional().
			MaxLen(100),
		field.String("mode").
			Default("chat").
			Optional().
			MaxLen(50),
		field.Bool("supports_prompt_caching").
			Default(false).
			Optional(),
		field.Bool("locked").
			Default(false).
			Comment("If true, remote sync will skip this entry"),
		field.String("source").
			Default("remote").
			Optional().
			MaxLen(20).
			Comment("remote or manual"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
	}
}

func (ModelPricing) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model").Unique(),
	}
}
