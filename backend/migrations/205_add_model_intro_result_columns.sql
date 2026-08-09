-- 为模型介绍增加"结果展示"元信息，仅用于用户端演练台正确渲染任务结果。
--
-- 新列：
--   * result_field  结果字段路径（在 fal 原生 result payload 中定位可播放/可展示 URL 的路径，
--                   例如 "video.url"、"images[0].url"、"video"）。为空时演练台走原有的鲁棒
--                   兜底提取（extractVideoUrls），保证历史配置继续工作。
--   * result_type   结果类型，仅两值：'video' | 'image'。默认 'video'，与原行为一致。
--
-- default_params 的 shape 升级（从 {k: v} → {k: {value, required, description, enum, options}}）
-- 由前端负责，不需要建 SQL 侧的 CHECK，历史行内容可以是任意 JSONB，前端读取时按新格式解释，
-- 无法识别的老结构会被视为"无字段声明"（表单模式不展示字段，走纯 JSON 模式）。
--
-- forward-only；使用 IF NOT EXISTS / DEFAULT 保证幂等。

ALTER TABLE model_intros
    ADD COLUMN IF NOT EXISTS result_field VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE model_intros
    ADD COLUMN IF NOT EXISTS result_type VARCHAR(16) NOT NULL DEFAULT 'video';
