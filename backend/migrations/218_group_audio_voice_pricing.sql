-- Grok Voice 显式定价：realtime / TTS / STT。
-- NULL = 使用代码默认单价；显式 0 = 免费；>0 = 分组覆盖价。
ALTER TABLE groups ADD COLUMN IF NOT EXISTS audio_realtime_price_per_min DECIMAL(20,8);
ALTER TABLE groups ADD COLUMN IF NOT EXISTS audio_tts_price_per_million_chars DECIMAL(20,8);
ALTER TABLE groups ADD COLUMN IF NOT EXISTS audio_stt_price_per_hour DECIMAL(20,8);
