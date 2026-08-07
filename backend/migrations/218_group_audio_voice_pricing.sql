-- Grok Voice 显式定价：realtime / TTS / STT。
-- NULL 表示未配置（当前中继不强制计费；字段预留给分组配置与后续计费接线）。
ALTER TABLE groups ADD COLUMN IF NOT EXISTS audio_realtime_price_per_min DECIMAL(20,8);
ALTER TABLE groups ADD COLUMN IF NOT EXISTS audio_tts_price_per_million_chars DECIMAL(20,8);
ALTER TABLE groups ADD COLUMN IF NOT EXISTS audio_stt_price_per_hour DECIMAL(20,8);
