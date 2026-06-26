-- 154_add_scheduled_test_client_message.sql
-- Add scheduled test client/message fields for upstream probe customization

ALTER TABLE scheduled_test_plans
    ADD COLUMN IF NOT EXISTS client TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS message TEXT NOT NULL DEFAULT 'hi';
