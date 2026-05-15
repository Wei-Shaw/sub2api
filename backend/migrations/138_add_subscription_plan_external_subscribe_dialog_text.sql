ALTER TABLE subscription_plans
    ADD COLUMN IF NOT EXISTS external_subscribe_dialog_text TEXT NOT NULL DEFAULT '';
