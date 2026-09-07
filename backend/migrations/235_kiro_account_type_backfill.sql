-- 回填存量 kiro 账号的 type 字段。
--
-- 背景：CreateAccountModal.vue 建 kiro 账号时，不管 social/builder_id/idc/
-- api_key 里选了哪种，一律硬编码 type='apikey'（Task 22/23 遗留的口径不
-- 一致——建号那一刻 form.type 从没被正确赋过值，credentials.auth_method
-- 才是唯一的真实来源）。这次把 Kiro 的鉴权方式改成跟 Antigravity 一样准确
-- 区分 OAuth（social/builder_id/idc 都是真 OAuth）和 API Key，后端
-- CreateAccount 已经改成落库前按 auth_method 权威改写 type——但这只对新建
-- 账号生效，存量行需要这条迁移一次性回填，否则它们会继续显示成 apikey，
-- 且享受不到 IsOAuth() 门控的通用逻辑（比如 /accounts/:id/available-models
-- 会错误地按 API Key 分支处理 model_mapping）。
--
-- 默认鉴权方式是 social（与 Account.KiroAuthMethod()/kiro.ParseAuthMethod
-- 的缺省值保持一致：auth_method 缺失或无法识别时按 social 处理）。
UPDATE accounts
SET type = 'oauth'
WHERE platform = 'kiro'
  AND type <> 'oauth'
  AND COALESCE(credentials ->> 'auth_method', 'social') <> 'api_key';

UPDATE accounts
SET type = 'apikey'
WHERE platform = 'kiro'
  AND type <> 'apikey'
  AND credentials ->> 'auth_method' = 'api_key';
