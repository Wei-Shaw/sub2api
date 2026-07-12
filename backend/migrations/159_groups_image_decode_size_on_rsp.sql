-- 为 groups 表新增「回包图片分辨率自检（base64）」开关。
-- image_decode_size_on_rsp: BOOLEAN，仅 platform='openai' 分组消费；true 时若上游回包某张图
--   缺失 size 字段或返回 size='auto'，系统在异步记账阶段对该张图的 b64_json 内容执行最小代价
--   的头部解码（image.DecodeConfig），用解码出的 {w}x{h} 进入 6 档归档计费；URL 模式不解码。
-- 默认 false 时保持现状默认 2K 档兜底语义；其他平台分组写入该列无效果。
-- 对存量数据零影响：默认 false，等价现状行为。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_decode_size_on_rsp BOOLEAN NOT NULL DEFAULT FALSE;
