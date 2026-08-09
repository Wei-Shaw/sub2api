-- 为模型介绍表回补 result_field / result_type 两列（v2 语义）。
--
-- 与 206 之前的历史列不同：此次两列不再是"独立结果字段"，而是与
-- output_fields 组合使用的"主结果指示器"：
--   * result_field   指向 output_fields[].key 中的某一项（若为空则演练台按
--                    output_fields 顺序取第一个 video/image 字段作为主结果）；
--   * result_type    仅取 "video" | "image"（默认 "video"），决定用 <video>
--                    还是 <img> 渲染主结果媒体。
-- 其余附属字段仍按各自 output_fields[].type 渲染。
--
-- 同时移除 output_fields[].primary 字段：primary 语义已被 result_field 取代，
-- 避免双份配置冲突。已存在的数据里，jsonb 数组元素中的 "primary" 键会被剥离。
--
-- forward-only；ADD COLUMN 使用 IF NOT EXISTS 保证幂等。

ALTER TABLE model_intros
    ADD COLUMN IF NOT EXISTS result_field VARCHAR(255) NOT NULL DEFAULT '';

ALTER TABLE model_intros
    ADD COLUMN IF NOT EXISTS result_type VARCHAR(16) NOT NULL DEFAULT 'video';

-- 剥离 output_fields 数组中每个元素的 primary 键。
-- 当 output_fields 为空数组 / 非数组时，jsonb_array_elements 会跳过，
-- 因此下面用条件 WHERE 保证只处理数组类型 jsonb。
UPDATE model_intros SET output_fields = COALESCE((
    SELECT jsonb_agg(elem - 'primary')
    FROM jsonb_array_elements(output_fields) AS elem
), '[]'::jsonb)
WHERE jsonb_typeof(output_fields) = 'array' AND output_fields <> '[]'::jsonb;
