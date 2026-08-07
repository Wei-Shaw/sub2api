package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DynamicProxyPool holds the schema for a dynamic IP extraction pool.
// It periodically fetches IPs from an extraction API and creates short-lived
// Proxy records that the gateway can use for outgoing requests.
type DynamicProxyPool struct {
	ent.Schema
}

func (DynamicProxyPool) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "dynamic_proxy_pools"},
	}
}

func (DynamicProxyPool) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (DynamicProxyPool) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			MaxLen(100).
			Comment("Pool display name"),
		field.Bool("enabled").
			Default(true),
		field.String("source_type").
			Default("extract_api").
			MaxLen(20).
			Comment("Source type: extract_api | subscription"),
		field.Int64("subscription_id").
			Optional().Nillable().
			Comment("Linked ProxySubscription ID when source_type=subscription"),
		field.String("extract_url").
			Optional().Default("").
			MaxLen(2000).
			Sensitive().
			Comment("IP extraction API URL (when source_type=extract_api)"),
		field.String("protocol").
			Default("http").
			MaxLen(20).
			Comment("Protocol for extracted IPs: http|https|socks5|socks5h"),
		field.String("auth_mode").
			Default("none").
			MaxLen(20).
			Comment("Auth mode: none|fixed|from_response"),
		field.String("username").
			Optional().Default("").
			MaxLen(200).
			Comment("Fixed auth username (when auth_mode=fixed)"),
		field.String("password").
			Optional().Default("").
			MaxLen(200).
			Sensitive().
			Comment("Fixed auth password (when auth_mode=fixed)"),
		field.String("response_format").
			Default("txt").
			MaxLen(20).
			Comment("Response format: txt|json"),
		field.String("line_separator").
			Default("\\r\\n").
			MaxLen(20).
			Comment("Line separator for txt format"),
		field.String("ip_field_path").
			Optional().Default("").
			MaxLen(200).
			Comment("JSONPath for IP field (json format)"),
		field.String("port_field_path").
			Optional().Default("").
			MaxLen(200).
			Comment("JSONPath for port field (json format)"),
		field.Int("refresh_interval_sec").
			Default(300).
			Comment("How often to fetch new IPs (seconds)"),
		field.Int("ip_duration_sec").
			Default(300).
			Comment("How long each extracted IP is valid (seconds)"),
		field.Int("extract_count").
			Default(1).
			Comment("Number of IPs to extract per request"),
		field.Int("min_alive").
			Default(1).
			Comment("Minimum alive proxies; triggers extraction when below"),
		field.String("name_prefix").
			Default("dpool-").
			MaxLen(40).
			Comment("Name prefix for owned proxy records"),
		field.Time("last_extract_at").
			Optional().Nillable(),
		field.String("last_extract_status").
			Optional().Default("").
			MaxLen(40),
		field.Text("last_extract_error").
			Optional().Default(""),
		field.Int("alive_count").
			Default(0).
			Comment("Current number of non-expired proxies in this pool"),
		field.Int("health_check_interval_sec").
			Default(0).
			Comment("Health check interval in seconds (0 = disabled)"),
	}
}

func (DynamicProxyPool) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("name_prefix").Unique(),
	}
}
