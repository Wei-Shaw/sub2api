-- 允许用户平台维度配额记录 Kiro。
--
-- 142_user_platform_quotas 建表时的平台 check 只包含四个平台；
-- 代码层和设置层已经支持 kiro，运行时会在记账时插入 platform='kiro'。

ALTER TABLE user_platform_quotas
  DROP CONSTRAINT IF EXISTS user_platform_quotas_platform_check;

ALTER TABLE user_platform_quotas
  ADD CONSTRAINT user_platform_quotas_platform_check
  CHECK (platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'kiro'));
