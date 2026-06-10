package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SupportFaqItem holds the schema for FAQ entries used by the support chat RAG.
//
// 注意：`embedding` 列（PostgreSQL `vector(1536)`）不在 ent schema 中声明，
// 由 `migrations/151_add_support_knowledge_rag.sql` 直接 DDL 创建，
// repository 层通过原生 SQL（pgvector `<=>` 操作符）读写该列。
// 这样可避免 ent 对自定义 PG 类型的有限支持引入的复杂度。
type SupportFaqItem struct {
	ent.Schema
}

func (SupportFaqItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_faq_items"},
	}
}

func (SupportFaqItem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (SupportFaqItem) Fields() []ent.Field {
	return []ent.Field{
		field.String("question").
			MaxLen(200).
			NotEmpty().
			Comment("FAQ 问题"),
		field.String("answer").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty().
			Comment("FAQ 答案（Markdown）"),
		field.Strings("tags").
			SchemaType(map[string]string{dialect.Postgres: "text[]"}).
			Default([]string{}).
			Comment("标签（admin 用，不影响检索）"),
		field.Bool("enabled").
			Default(true).
			Comment("是否启用；false 时不会出现在公开 FAQ 列表与 RAG 检索"),
		field.Int("sort_order").
			Default(0).
			Comment("排序值（升序）"),
	}
}

func (SupportFaqItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sort_order", "id"),
	}
}
