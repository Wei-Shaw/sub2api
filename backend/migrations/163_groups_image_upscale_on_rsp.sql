-- 为 groups 表新增「回包分辨率不足时同步 upscale 交付」开关。
-- image_upscale_on_rsp: BOOLEAN，仅 platform='openai' 分组消费，且依赖 image_decode_size_on_rsp。
--   true 时：若请求归一目标档位 ≥ 2K，而回包 b64_json 解码出的真实档位更低，
--   系统在把图返回客户端（及转存 COS）之前调用 fal-ai/seedvr/upscale/image 放大到目标档位；
--   放大失败/超时则原图兜底并按原图档位计费。fal upscale 的 endpoint/token/超时为系统配置。
-- 默认 false 时保持现状（不放大）；其他平台分组写入该列无效果。
-- 对存量数据零影响：默认 false，等价现状行为。
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS image_upscale_on_rsp BOOLEAN NOT NULL DEFAULT FALSE;
