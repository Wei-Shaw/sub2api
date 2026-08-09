-- 208_add_model_intro_description_en.sql
--
-- 背景：model_intros.description 原本只承载单语（纯文本）"模型介绍"，
-- 用户端渲染时无法区分中英文界面。现新增 description_en 字段以支持
-- 中英双文：description 表示中文文案（保持向后兼容），description_en
-- 表示英文文案，前端按当前 locale 挑选并做互相兜底。
--
-- 兼容策略：
--   - 字段 NOT NULL DEFAULT ''：旧记录读出时 description_en 为空串，
--     前端会自动回落到 description 展示，不影响历史行为；
--   - 无需回填任何数据，管理员在"模型介绍"编辑页手工补录或用页面上
--     的"翻译"按钮通过大模型一键翻译。
--
-- forward-only；使用 IF NOT EXISTS 保证幂等。
ALTER TABLE model_intros
    ADD COLUMN IF NOT EXISTS description_en TEXT NOT NULL DEFAULT '';
