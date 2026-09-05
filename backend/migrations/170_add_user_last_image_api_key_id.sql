ALTER TABLE users
    ADD COLUMN IF NOT EXISTS last_image_api_key_id BIGINT;
