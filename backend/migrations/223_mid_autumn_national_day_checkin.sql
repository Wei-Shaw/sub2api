-- 中秋国庆 13 天签到活动：兑换过兑换码的用户每日固定获得 10 美元。
ALTER TABLE checkin_settings
    ADD COLUMN IF NOT EXISTS campaign_start DATE NOT NULL DEFAULT '2026-09-25',
    ADD COLUMN IF NOT EXISTS campaign_end DATE NOT NULL DEFAULT '2026-10-07',
    ADD COLUMN IF NOT EXISTS campaign_reward INTEGER NOT NULL DEFAULT 10
        CHECK (campaign_reward > 0);

ALTER TABLE user_checkins DROP CONSTRAINT IF EXISTS user_checkins_reward_mode_check;
ALTER TABLE user_checkins
    ADD CONSTRAINT user_checkins_reward_mode_check
    CHECK (reward_mode IN ('standard', 'reduced', 'campaign'));
