-- Balance notification user preferences
ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_notify_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_notify_threshold DECIMAL(20,8) DEFAULT NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_notify_extra_emails TEXT NOT NULL DEFAULT '[]';
