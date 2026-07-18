-- 邀请返利：邀请人给被邀请用户的私人备注。
--
-- 场景：
--   用户在 /affiliate 页面的"已邀请用户"列表里，需要对每个被邀请人
--   加一段私人备注（例如"张三"、"公司同事"）。备注是"邀请人视角"
--   的私有数据，被邀请人本身不可见。
--
-- 建模：
--   每行 user_affiliates 语义上就是"user_id 这个用户是被 inviter_id
--   邀请的"，(user_id, inviter_id) 天然唯一（user_id 是 PK）。因此
--   把备注直接落在这一行的 inviter_note 字段最简：
--     - 谁写：inviter_id（该行的邀请人）。
--     - 谁看：仅 inviter_id 自己（后端在 handler 层通过
--       `inviter_id = 当前 JWT 用户` 做权限过滤）。
--     - 用户注销 inviter 时（inviter_id ON DELETE SET NULL），备注
--       字段一并变为孤儿数据，仍保留于该行；本次不做联动清理，
--       未来若上升为强合规要求可另加清理 job。
--
--   若未来需要 admin 全局编辑或双向备注（被邀请者也想记邀请人），
--   再抽独立的 user_affiliate_notes 表，双方备注分行存储。
--
-- 长度上限 500 字符：与 support_ticket_notification.excerpt 一致，
-- 覆盖绝大多数备注写法；超过在 service 层截断/报错。
ALTER TABLE user_affiliates
    ADD COLUMN IF NOT EXISTS inviter_note VARCHAR(500);

COMMENT ON COLUMN user_affiliates.inviter_note IS
    '邀请人给该被邀请用户的私人备注（仅邀请人可见，最大 500 字符）';
