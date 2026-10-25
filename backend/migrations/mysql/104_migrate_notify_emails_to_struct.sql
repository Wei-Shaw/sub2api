-- MySQL 方言版本（翻译自 ../104_migrate_notify_emails_to_struct.sql）
--
-- Migrate notification email lists from old []string format to new []NotifyEmailEntry format
-- Old: ["a@x.com", "b@x.com"]
-- New: [{"email":"a@x.com","disabled":false,"verified":true}, ...]
-- Existing emails are marked as verified=false (unverified), disabled=false (enabled)
--
-- 翻译说明：
--   - PG 的 UPDATE ... SET col = (SELECT jsonb_agg(...) FROM jsonb_array_elements_text(col))
--     → 多表 UPDATE（目标表 JOIN 派生表）+ JSON_TABLE 拆元素 + JSON_ARRAYAGG 重组，
--     派生表按 id 分组后 JOIN 回目标表，仅命中 PG 守卫通过的行。
--   - PG 的全部 WHERE 守卫（IS NOT NULL / <> '[]' / <> '' / (x::jsonb -> 0) IS NOT NULL /
--     jsonb_typeof(x::jsonb -> 0) = 'string'）收敛进 JSON_TABLE 输入的 CASE 表达式：
--     守卫全过 → 原数组；否则 → '[]'（拆出 0 行 → 不进 GROUP BY → JOIN 不命中 → 不更新）。
--     这样 JSON_TABLE 绝不会解析到非法 JSON（users 行为 ''、settings 其它 key 的非 JSON
--     值等），与 PG「守卫不通过则不执行子查询」的短路行为对齐。
--   - jsonb_build_object('email', e, 'disabled', false, 'verified', false)
--     → JSON_OBJECT('email', e, 'disabled', CAST('false' AS JSON), 'verified', CAST('false' AS JSON))
--     （MySQL 的 FALSE 字面量是整数 0，必须 CAST 为 JSON 才能得到 JSON 布尔 false；
--       已是 JSON 的值在 JSON_OBJECT 中按原样嵌入）。
--   - (x::jsonb -> 0) IS NOT NULL AND jsonb_typeof(x::jsonb -> 0) = 'string'
--     → 增加文本预检 LIKE '[%'（排除 JSON 标量/对象文档，MySQL 对这类文档取 '$[0]'
--       可能报错而 PG 同样会报错，预检使行为更稳健）+ JSON_VALID
--       + JSON_TYPE(JSON_EXTRACT(x, '$[0]')) = 'STRING'（JSON_TYPE 对 JSON 字符串返回
--       大写 'STRING'；空数组时 JSON_EXTRACT 返回 SQL NULL，条件联合排除空数组）。
-- TODO-REVIEW: PG 的 jsonb_array_elements_text 会把数组中的非字符串元素转为其文本表示；
--   MySQL JSON_TABLE 的 VARCHAR 列对非标量元素取 NULL。仅当历史数据出现「首元素为字符串、
--   后续元素混入对象/数组」的畸形异构数组时两者才有差异（应用写入的都是同构字符串数组
--   或已迁移的对象数组，正常数据不受影响；若确认存在异构数据需改用 JSON 读取后文本化）。

-- 1. User balance notification emails
UPDATE users u
JOIN (
    SELECT u2.id AS uid,
           COALESCE(
               JSON_ARRAYAGG(JSON_OBJECT('email', jt.elem,
                                         'disabled', CAST('false' AS JSON),
                                         'verified', CAST('false' AS JSON))),
               CAST('[]' AS JSON)
           ) AS new_value
    FROM users u2
    JOIN JSON_TABLE(
             CASE
                 WHEN u2.balance_notify_extra_emails IS NOT NULL
                      AND u2.balance_notify_extra_emails <> '[]'
                      AND u2.balance_notify_extra_emails <> ''
                      AND u2.balance_notify_extra_emails LIKE '[%'
                      AND JSON_VALID(u2.balance_notify_extra_emails)
                      AND JSON_TYPE(JSON_EXTRACT(u2.balance_notify_extra_emails, '$[0]')) = 'STRING'
                 THEN u2.balance_notify_extra_emails
                 ELSE '[]'
             END,
             '$[*]' COLUMNS (elem VARCHAR(1000) PATH '$')
         ) jt
    GROUP BY u2.id
) x ON x.uid = u.id
SET u.balance_notify_extra_emails = x.new_value;

-- 2. Admin account quota notification emails
UPDATE settings s
JOIN (
    SELECT s2.id AS sid,
           COALESCE(
               JSON_ARRAYAGG(JSON_OBJECT('email', jt.elem,
                                         'disabled', CAST('false' AS JSON),
                                         'verified', CAST('false' AS JSON))),
               CAST('[]' AS JSON)
           ) AS new_value
    FROM settings s2
    JOIN JSON_TABLE(
             CASE
                 WHEN s2.`key` = 'account_quota_notify_emails'
                      AND s2.value IS NOT NULL
                      AND s2.value <> '[]'
                      AND s2.value <> ''
                      AND s2.value LIKE '[%'
                      AND JSON_VALID(s2.value)
                      AND JSON_TYPE(JSON_EXTRACT(s2.value, '$[0]')) = 'STRING'
                 THEN s2.value
                 ELSE '[]'
             END,
             '$[*]' COLUMNS (elem VARCHAR(1000) PATH '$')
         ) jt
    WHERE s2.`key` = 'account_quota_notify_emails'
    GROUP BY s2.id
) x ON x.sid = s.id
SET s.value = x.new_value;
