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

// BatchImageJob holds the schema definition for asynchronous image batch jobs.
//
// 删除策略：硬删除
// 这张表是批量生图任务的账务和状态源，不使用软删除；输出清理通过
// output_deleted 状态和删除时间字段表达。
type BatchImageJob struct {
	ent.Schema
REDACTED

func (BatchImageJob) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "batch_image_jobs"REDACTED,
REDACTED
REDACTED

func (BatchImageJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("batch_id").MaxLen(64).Immutable(),
		field.Int64("user_id"),
		field.Int64("api_key_id").Optional().Nillable(),
		field.Int64("account_id").Optional().Nillable(),
		field.String("provider").MaxLen(32),
		field.String("model").MaxLen(128),
		field.String("status").MaxLen(32).Default("created"),
		field.String("provider_job_name").Optional().Nillable().MaxLen(512),
		field.String("provider_input_ref").Optional().Nillable().MaxLen(1024),
		field.String("provider_output_ref").Optional().Nillable().MaxLen(1024),
		field.String("gcs_input_uri").Optional().Nillable().MaxLen(1024),
		field.String("gcs_output_uri").Optional().Nillable().MaxLen(1024),
		field.Int("item_count"),
		field.Int("success_count").Default(0),
		field.Int("fail_count").Default(0),
		field.Int("cancelled_count").Default(0),
		field.Float("estimated_cost").SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"REDACTED).Default(0),
		field.Float("hold_amount").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"REDACTED),
		field.Float("actual_cost").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"REDACTED),
		field.String("currency").MaxLen(16).Default("USD"),
		field.String("hold_id").Optional().Nillable().MaxLen(128),
		field.String("idempotency_key").Optional().Nillable().MaxLen(255),
		field.String("request_hash").Optional().Nillable().MaxLen(128),
		field.String("manifest_hash").Optional().Nillable().MaxLen(128),
		field.Int("retry_count").Default(0),
		field.Int("version").Default(0),
		field.Time("output_expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("input_deleted_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("output_deleted_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.String("last_error_code").Optional().Nillable().MaxLen(128),
		field.String("last_error_message").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"REDACTED),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("submitted_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("started_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("finished_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
		field.Time("settled_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"REDACTED),
REDACTED
REDACTED

func (BatchImageJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("batch_id").Unique(),
		index.Fields("user_id", "created_at"),
		index.Fields("status"),
		index.Fields("provider", "status"),
		index.Fields("idempotency_key").Annotations(entsql.IndexWhere("idempotency_key IS NOT NULL AND idempotency_key <> ''")),
		index.Fields("manifest_hash").Unique().Annotations(entsql.IndexWhere("manifest_hash IS NOT NULL AND manifest_hash <> ''")),
		index.Fields("output_expires_at"),
REDACTED
REDACTED
