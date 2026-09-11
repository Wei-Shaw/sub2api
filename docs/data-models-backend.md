# Backend 数据模型目录

**生成时间:** 2026-04-28  
**扫描级别:** Quick Scan  
**来源:** `backend/ent/schema` 文件名、字段声明、边声明和 `backend/migrations`

## 概览

Sub2API 使用 Ent schema 作为主要数据模型源，并通过 `backend/migrations` 维护 SQL 迁移历史。当前扫描识别到 32 个 schema 文件和 130+ 个 SQL 迁移文件。字段表来自 schema 字段声明的快速提取，未逐行审计 constraint、index、hook、mixin 和迁移 SQL 细节。

## 核心实体

| 实体 | 文件 | 主要字段 | 关系/说明 |
| --- | --- | --- | --- |
| User | `user.go` | email、password_hash、role、balance、concurrency、status、username、totp、signup_source、last_login_at、balance_notify、total_recharged、rpm_limit | 关联 API keys、redeem codes、subscriptions、usage logs、attributes、payment orders、auth identities |
| API Key | `api_key.go` | key、name、status、last_used_at、IP 黑白名单、quota、expires_at、rate limits、usage windows | 关联 user、group、usage_logs |
| Group | `group.go` | name、rate_multiplier、exclusive、platform、subscription_type、限额、model routing、RPM、messages dispatch | 关联 API keys、redeem codes、subscriptions、usage_logs、accounts、allowed_users |
| Account | `account.go` | name、platform、type、credentials、concurrency、load_factor、priority、status、expires_at、schedulable、rate limit、session window | 关联 groups、proxy、usage_logs |
| Usage Log | `usage_log.go` | request_id、model、requested/upstream model、billing_tier、billing_mode、tokens、costs、duration、first_token、user_agent、ip、image、created_at | 关联 user、api_key、account、group、subscription |
| User Subscription | `user_subscription.go` | starts_at、expires_at、status、daily/weekly/monthly usage、assigned_at、notes | 关联 user、group、assigned_by_user、usage_logs |
| Subscription Plan | `subscription_plan.go` | name、description、price、validity、features、product_name、for_sale、sort_order | 支付购买套餐 |

## 认证与身份

| 实体 | 文件 | 主要字段 | 说明 |
| --- | --- | --- | --- |
| AuthIdentity | `auth_identity.go` | provider_type、provider_key、provider_subject、verified_at、issuer、metadata | 统一第三方身份 |
| AuthIdentityChannel | `auth_identity_channel.go` | provider_type、provider_key、channel、channel_app_id、channel_subject、metadata | 同一身份下多渠道映射 |
| PendingAuthSession | `pending_auth_session.go` | session_token、intent、provider、redirect_to、resolved_email、verification timestamps、expires_at、consumed_at | OAuth pending flow |
| IdentityAdoptionDecision | `identity_adoption_decision.go` | adopt_display_name、adopt_avatar、decided_at | 绑定/采用身份资料 |
| SecuritySecret | `security_secret.go` | key、value | 安全密钥存储 |

## 支付与营销

| 实体 | 文件 | 主要字段 | 说明 |
| --- | --- | --- | --- |
| PaymentOrder | `payment_order.go` | user、amount、pay_amount、out_trade_no、payment_type、provider snapshot、status、refund fields、expires/paid/completed/failed timestamps | 用户支付订单 |
| PaymentProviderInstance | `payment_provider_instance.go` | provider_key、name、config、supported_types、enabled、payment_mode、limits、refund_enabled | 支付渠道实例配置 |
| PaymentAuditLog | `payment_audit_log.go` | order_id、action、detail、operator、created_at | 支付审计 |
| PromoCode | `promo_code.go` | code、bonus_amount、max_uses、used_count、status、expires_at、notes | 优惠码 |
| PromoCodeUsage | `promo_code_usage.go` | bonus_amount、used_at | 优惠码使用记录 |
| RedeemCode | `redeem_code.go` | code、type、value、status、used_at、notes、validity_days | 卡密兑换 |

## 运维、监控与配置

| 实体 | 文件 | 主要字段 | 说明 |
| --- | --- | --- | --- |
| Setting | `setting.go` | key、value、updated_at | 系统设置 KV |
| Proxy | `proxy.go` | protocol、host、port、username、password、status | 上游账号代理 |
| ChannelMonitor | `channel_monitor.go` | provider、endpoint、api_key、primary/extra models、group、interval、headers、body override | 渠道监控配置 |
| ChannelMonitorHistory | `channel_monitor_history.go` | model、status、latency、message、checked_at | 单次检查记录 |
| ChannelMonitorDailyRollup | `channel_monitor_daily_rollup.go` | bucket_date、total/ok/degraded/failed/error count、latency count | 日汇总 |
| ChannelMonitorRequestTemplate | `channel_monitor_request_template.go` | name、provider、headers、body override | 监控请求模板 |
| ErrorPassthroughRule | `error_passthrough_rule.go` | priority、error_codes、keywords、platforms、response_code、custom_message、skip_monitoring | 上游错误透传规则 |
| TLSFingerprintProfile | `tls_fingerprint_profile.go` | cipher_suites、curves、ALPN、versions、extensions | TLS 指纹配置 |
| UsageCleanupTask | `usage_cleanup_task.go` | status、filters、error_message、started/finished/canceled | usage 清理任务 |
| IdempotencyRecord | `idempotency_record.go` | scope、key hash、fingerprint、status、response、locked_until、expires_at | 幂等控制 |

## 公告与用户属性

| 实体 | 文件 | 主要字段 | 说明 |
| --- | --- | --- | --- |
| Announcement | `announcement.go` | title、content、status、notify_mode、targeting、starts_at、ends_at | 公告 |
| AnnouncementRead | `announcement_read.go` | read_at、created_at | 用户已读 |
| UserAllowedGroup | `user_allowed_group.go` | user_id、created_at | 用户可见/可用分组 |
| UserAttributeDefinition | `user_attribute_definition.go` | key、name、type、options、required、validation、display_order、enabled | 自定义用户属性定义 |
| UserAttributeValue | `user_attribute_value.go` | value | 自定义用户属性值 |

## 迁移策略

- SQL 迁移位于 `backend/migrations`，文件名体现从 `001_init.sql` 到 `133_affiliate_rebate_freeze.sql` 的演进。
- 迁移覆盖订阅、用户 allowed groups、usage aggregation、ops monitoring、Sora 历史表、渠道定价、支付订单、auth identity、channel monitors、affiliate rebate 等大块能力。
- 迁移 runner 和校验相关代码位于 `backend/internal/repository/migrations_runner*.go`。

## 开发规则

- 修改数据模型时优先改 `backend/ent/schema/*.go`，不要手改 `backend/ent` 生成文件。
- schema 改动后运行 `cd backend && make generate`，并检查生成的 Ent 与 Wire 变化。
- 数据迁移需要新增 `backend/migrations/*.sql`，并考虑幂等、安全回滚、非事务索引、历史数据 backfill 和 schema parity。
- 涉及金额、余额、usage、billing、订阅限额的变更必须配套测试，失败路径应 fail-closed。

## 后续深挖建议

Quick Scan 没有抽取字段类型、索引、唯一约束、默认值和校验器。若要做 schema 变更或生成 ERD，应 deep scan `backend/ent/schema` 和 `backend/migrations`，并结合 repository 查询路径确认真实读写关系。
