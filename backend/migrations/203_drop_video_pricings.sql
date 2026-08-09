-- 203_drop_video_pricings: 移除独立的 video_pricings 定价表。
-- 视频定价从此表迁出，后续将并入渠道级 channel_model_pricing（billing_mode='video'），
-- 由渠道视频定价接入完成后统一读取，不再依赖本表。
--
-- 本迁移无损：只丢弃表结构与数据，不修改其它表；旧的 202_video_pricings.sql
-- 迁移保留在历史链条中以便时间线可回放，但 203 之后运行时不再依赖 video_pricings。

DROP INDEX IF EXISTS videopricing_model_slug;
DROP INDEX IF EXISTS videopricing_model_slug_resolution;
DROP TABLE IF EXISTS video_pricings;
