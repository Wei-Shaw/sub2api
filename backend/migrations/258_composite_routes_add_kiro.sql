-- Migration: 258_composite_routes_add_kiro (ported from nianzs 229; renumbered for this fork sequence)
-- 把 kiro 加入 composite_model_routes.target_platform 的 CHECK 约束。
--
-- 背景：227_composite_routes_add_cn_providers 把国产 3 家加入该约束时采用
-- DROP + 重建的写法，重建的列表里没有本 fork 独有的 kiro。约束缺 kiro 时，
-- 管理端保存 target_platform='kiro' 的 composite 路由会违约失败。
--
-- 分发链路本身已支持：转发按账号而非平台分派，
-- isKiroDirectModeAccount(account) → forwardKiroMessages
-- （internal/service/gateway_forward.go），chat_completions / responses 两个
-- 入口同构；/v1/messages、chat_completions、responses 的 composite 门禁走
-- compositeTargetPlatformResolved，只要求解析出目标平台，不校验平台白名单。
--
-- 注意 kiro 只能通过显式路由行命中：DetectModelPlatform 推断不出 kiro，
-- 因为 kiro 的模型名是 claude-* / gpt-*，与 anthropic/openai 前缀冲突。
--
-- DROP ... IF EXISTS 保证可重入；新约束是旧约束的超集，存量行瞬时校验通过。
ALTER TABLE composite_model_routes
    DROP CONSTRAINT IF EXISTS composite_model_routes_target_platform_check;

ALTER TABLE composite_model_routes
    ADD CONSTRAINT composite_model_routes_target_platform_check
    CHECK (target_platform IN ('anthropic', 'openai', 'gemini', 'antigravity', 'kiro',
                               'grok', 'kimi', 'zhipu', 'deepseek'));
