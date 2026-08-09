-- 为模型介绍增加"输出字段"声明，并移除旧的 result_field / result_type 单字段模型。
--
-- 结构：output_fields JSONB DEFAULT '[]'::jsonb
--   数组形式，每个元素形如：
--     {
--       "key":         "video.url",       // 从 fal 原生 result payload 中提取的字段路径
--       "label":       "生成的视频",       // 展示名（可留空）
--       "type":        "video",           // video | image | text | url | json | number
--       "description": "点击可下载",       // 字段说明（可留空）
--       "primary":     true,              // 是否为主结果，主结果在演练台里居中/大尺寸展示
--       "default":     ""                 // 默认值（预留，暂只用于文档/复制）
--     }
--
-- 演练台的展示逻辑：
--   * primary=true 的项，若 type=video 则渲染 <video>，type=image 则渲染 <img>；
--   * 其余项按各自 type 渲染为小图/短视频/文本/链接/JSON 块；
--   * 未配置 output_fields 或数组为空时，演练台不展示专门的结果字段区，
--     仅展示原始 payload（保底），无兜底提取逻辑。
--
-- 同步删除历史列 result_field / result_type（不再使用；数据不做保留迁移）。
--
-- forward-only；使用 IF NOT EXISTS / IF EXISTS 保证幂等。

ALTER TABLE model_intros
    ADD COLUMN IF NOT EXISTS output_fields JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE model_intros
    DROP COLUMN IF EXISTS result_field;

ALTER TABLE model_intros
    DROP COLUMN IF EXISTS result_type;
