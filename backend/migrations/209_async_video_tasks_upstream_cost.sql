-- 209_async_video_tasks_upstream_cost.sql
-- 为 async_video_tasks 表新增 upstream_cost 列，记录本次视频任务在**上游**产生的真实费用（USD）。
--
-- 背景：
--   - 之前视频链路的"上游成本"是按 rate_multiplier × total_cost 估算的（沿用文本/图片
--     写 cost_center 的通用逻辑）。这个估算对 apiz 这类"每次请求单独返回价格"的平台
--     不准确——apiz 回包顶层的 price 是**本次任务真实成本**，除以 100 即为美元金额。
--   - 引入 upstream_cost 列，让 apiz/未来支持真实成本回传的平台把上游侧真实成本
--     持久化到任务行，成本中心据此写 upstream expense 事件。fal/atlascloud 平台
--     不回传时保留 0，仍走旧的 rate_multiplier 估算。
--
-- 单位与精度：与 held_cost/final_cost 同款 decimal(20,10)，与账本口径对齐。
ALTER TABLE async_video_tasks
    ADD COLUMN IF NOT EXISTS upstream_cost DECIMAL(20, 10) NOT NULL DEFAULT 0;
