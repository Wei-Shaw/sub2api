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

// BatchImageItem holds indexed output rows for a batch image job.
type BatchImageItem struct {
	ent.Schema
}

func (BatchImageItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "batch_image_items"},
	}
}

func (BatchImageItem) Fields() []ent.Field {
	return []ent.Field{
		field.String("job_id").MaxLen(64),
		field.String("custom_id").MaxLen(255),
		field.String("status").MaxLen(32),
		field.String("request_hash").Optional().Nillable().MaxLen(128),
		field.String("prompt_preview").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("provider_source_object").Optional().Nillable().MaxLen(1024),
		field.Int("source_line_number").Optional().Nillable(),
		field.Int64("source_byte_offset").Optional().Nillable(),
		field.Int64("source_byte_length").Optional().Nillable(),
		field.String("mime_type").Optional().Nillable().MaxLen(128),
		field.String("file_extension").Optional().Nillable().MaxLen(32),
		field.Int("image_count").Default(0),
		field.String("error_code").Optional().Nillable().MaxLen(128),
		field.String("error_message").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Float("billed_amount").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("indexed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (BatchImageItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("job_id", "custom_id").Unique(),
		index.Fields("job_id", "status"),
		index.Fields("provider_source_object"),
	}
}
