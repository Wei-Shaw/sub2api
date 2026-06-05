package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// Setting holds the schema definition for the Setting entity.
//
// 删除策略：硬删除
// Setting 使用硬删除而非软删除，原因如下：
//   - 系统设置是简单的键值对，删除即意味着恢复默认值
//   - 设置变更通常通过应用日志追踪，无需在数据库层面保留历史
//   - 保持表结构简洁，避免无效数据积累
//
// 如需设置变更审计，建议在更新/删除前将变更记录写入审计日志表。
type Setting struct {
	ent.Schema
}

func (Setting) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "settings"},
	}
}

func (Setting) Fields() []ent.Field {
	return []ent.Field{
		field.String("key").
			MaxLen(100).
			NotEmpty().
			Unique(),
		field.String("value").
			SchemaType(map[string]string{
				dialect.Postgres: "text",
			}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
	}
}

func (Setting) Indexes() []ent.Index {
	// key 字段已在 Fields() 中声明 Unique()，无需额外索引
	return nil
}
