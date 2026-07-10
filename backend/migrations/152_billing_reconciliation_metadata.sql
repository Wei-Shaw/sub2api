-- V-3 billing reconciliation metadata.

ALTER TABLE usage_logs
	ADD COLUMN IF NOT EXISTS currency VARCHAR(3) NOT NULL DEFAULT 'USD',
	ADD COLUMN IF NOT EXISTS pricing_source VARCHAR(32) NOT NULL DEFAULT 'fallback',
	ADD COLUMN IF NOT EXISTS pricing_version VARCHAR(64);

ALTER TABLE video_usage_logs
	ADD COLUMN IF NOT EXISTS currency VARCHAR(3) NOT NULL DEFAULT 'USD',
	ADD COLUMN IF NOT EXISTS pricing_source VARCHAR(32) NOT NULL DEFAULT 'fallback',
	ADD COLUMN IF NOT EXISTS pricing_version VARCHAR(64);

INSERT INTO settings (key, value)
VALUES ('usd_cny_rate', '7.20')
ON CONFLICT (key) DO NOTHING;
