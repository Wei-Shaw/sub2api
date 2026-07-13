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

// SupportDocChunk represents a chunk of text harvested from the configured doc_url.
//
// 与 SupportFaqItem 同样，`embedding` 列（vector(1536)）由 SQL migration 创建，
// 不在 ent schema 中体现；repository 层通过原生 SQL 读写。
type SupportDocChunk struct {
	ent.Schema
}

func (SupportDocChunk) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_doc_chunks"},
	}
}

func (SupportDocChunk) Fields() []ent.Field {
	return []ent.Field{
		field.String("source_url").
			MaxLen(500).
			NotEmpty().
			Comment("抓取源 URL"),
		field.String("chunk_text").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("切片后的正文段落"),
		field.String("content_hash").
			MaxLen(64).
			Immutable().
			Comment("sha256(chunk_text) 的十六进制；与 source_url 联合唯一"),
		field.Time("fetched_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SupportDocChunk) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_url"),
		index.Fields("source_url", "content_hash").Unique(),
	}
}
