ALTER TABLE minimax_media_tasks DROP CONSTRAINT minimax_media_tasks_kind_check;
ALTER TABLE minimax_media_tasks ADD CONSTRAINT minimax_media_tasks_kind_check CHECK (kind IN ('speech','video','image','voice_clone','voice_design'));

-- Flat per-image pricing has no size-dependent billing tier. Retain the
-- existing image/video checks and give these rows an explicit unit marker.
ALTER TABLE usage_logs DROP CONSTRAINT usage_logs_image_billing_size_check;
ALTER TABLE usage_logs ADD CONSTRAINT usage_logs_image_billing_size_check CHECK (
 image_count <= 0
 OR billing_mode = 'video'
 OR COALESCE(video_count, 0) > 0
 OR (image_size IS NOT NULL AND image_size IN ('1K','2K','4K','mixed'))
 OR (billing_mode = 'image' AND image_size IS NOT NULL AND image_size = 'per_image')
) NOT VALID;

-- Private assets belong to the originating user/key and upstream account.
-- No source audio, prompt, secret, or signed media URL is stored here.
CREATE TABLE minimax_media_assets (
 kind VARCHAR(32) NOT NULL,
 category VARCHAR(32) NOT NULL DEFAULT '',
 asset_id VARCHAR(256) NOT NULL,
 account_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 api_key_id BIGINT NOT NULL,
 activated_task_id VARCHAR(64),
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 PRIMARY KEY (kind, account_id, asset_id)
);
CREATE UNIQUE INDEX minimax_media_asset_owner ON minimax_media_assets(kind,asset_id,api_key_id);
