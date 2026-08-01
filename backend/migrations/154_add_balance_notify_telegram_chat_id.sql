ALTER TABLE users
    ADD COLUMN IF NOT EXISTS balance_notify_telegram_chat_id VARCHAR(64) NOT NULL DEFAULT '';
