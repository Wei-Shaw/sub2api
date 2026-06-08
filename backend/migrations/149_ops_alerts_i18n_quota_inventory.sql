-- Ops alerts: Vietnamese default names + inventory/quota coverage.
-- Idempotent; safe to run on existing deployments.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '10min';

-- Translate legacy default rule names/descriptions that were seeded in Chinese.
WITH translations(old_name, new_name, new_description) AS (
    VALUES
        ('错误率过高', 'Tỷ lệ lỗi cao', 'Cảnh báo khi tỷ lệ lỗi vượt 5% trong 5 phút.'),
        ('成功率过低', 'Tỷ lệ thành công thấp', 'Cảnh báo khi tỷ lệ thành công dưới 95% trong 5 phút.'),
        ('P99延迟过高', 'Độ trễ P99 cao', 'Cảnh báo khi độ trễ P99 vượt 3000ms trong 10 phút.'),
        ('P95延迟过高', 'Độ trễ P95 cao', 'Cảnh báo khi độ trễ P95 vượt 2000ms trong 10 phút.'),
        ('CPU使用率过高', 'CPU sử dụng cao', 'Cảnh báo khi CPU vượt 85% trong 10 phút.'),
        ('内存使用率过高', 'Bộ nhớ sử dụng cao', 'Cảnh báo khi bộ nhớ vượt 90% trong 10 phút, có nguy cơ OOM.'),
        ('并发队列积压', 'Hàng đợi đồng thời bị backlog', 'Cảnh báo khi hàng đợi đồng thời vượt 100 trong 5 phút.'),
        ('错误率极高', 'Tỷ lệ lỗi cực cao', 'Cảnh báo nghiêm trọng khi tỷ lệ lỗi vượt 20% trong 1 phút.')
)
UPDATE ops_alert_rules r
SET
    name = CASE
        WHEN EXISTS (SELECT 1 FROM ops_alert_rules existing WHERE existing.name = t.new_name AND existing.id <> r.id)
            THEN t.new_name || ' #' || r.id
        ELSE t.new_name
    END,
    description = t.new_description,
    updated_at = NOW()
FROM translations t
WHERE r.name = t.old_name;

-- Clean historical event text so the "Alert events" table no longer shows Chinese titles.
WITH translations(old_name, new_name) AS (
    VALUES
        ('错误率过高', 'Tỷ lệ lỗi cao'),
        ('成功率过低', 'Tỷ lệ thành công thấp'),
        ('P99延迟过高', 'Độ trễ P99 cao'),
        ('P95延迟过高', 'Độ trễ P95 cao'),
        ('CPU使用率过高', 'CPU sử dụng cao'),
        ('内存使用率过高', 'Bộ nhớ sử dụng cao'),
        ('并发队列积压', 'Hàng đợi đồng thời bị backlog'),
        ('错误率极高', 'Tỷ lệ lỗi cực cao')
)
UPDATE ops_alert_events e
SET
    title = replace(e.title, t.old_name, t.new_name),
    description = replace(e.description, t.old_name, t.new_name)
FROM translations t
WHERE e.title LIKE '%' || t.old_name || '%'
   OR e.description LIKE '%' || t.old_name || '%';

-- Seed missing default inventory/quota rules.
INSERT INTO ops_alert_rules (
    name, description, enabled, metric_type, operator, threshold,
    window_minutes, sustained_minutes, severity, notify_email, cooldown_minutes,
    created_at, updated_at
) VALUES
    (
        'Không còn tài khoản khả dụng',
        'Cảnh báo khi số tài khoản có thể điều phối còn dưới 1. Bao gồm tài khoản lỗi, hết hạn, rate-limit, quá tải hoặc hết quota.',
        true, 'account_available_count', '<', 1.0, 1, 1, 'P0', true, 15, NOW(), NOW()
    ),
    (
        'Tỷ lệ tài khoản khả dụng thấp',
        'Cảnh báo sớm khi tỷ lệ tài khoản khả dụng toàn hệ thống dưới 30%.',
        true, 'account_available_ratio', '<', 30.0, 5, 2, 'P1', true, 20, NOW(), NOW()
    ),
    (
        'Quota tài khoản đạt 70%',
        'Cảnh báo khi bất kỳ tài khoản API Key/Bedrock nào dùng từ 70% quota trở lên.',
        true, 'account_quota_usage_ratio', '>=', 70.0, 5, 1, 'P2', true, 30, NOW(), NOW()
    ),
    (
        'Quota tài khoản đạt 80%',
        'Cảnh báo khi bất kỳ tài khoản API Key/Bedrock nào dùng từ 80% quota trở lên.',
        true, 'account_quota_usage_ratio', '>=', 80.0, 5, 1, 'P1', true, 30, NOW(), NOW()
    ),
    (
        'Có tài khoản hết quota',
        'Cảnh báo khi có ít nhất một tài khoản API Key/Bedrock đã hết quota ở tổng/ngày/tuần.',
        true, 'account_quota_exhausted_count', '>', 0.0, 1, 1, 'P1', true, 15, NOW(), NOW()
    )
ON CONFLICT (name) DO NOTHING;
