-- Split user wallet into cash (recharged) and gift (bonus/rebate) balances.
-- Keep legacy balance as total available balance for backward compatibility during rollout.
ALTER TABLE users ADD COLUMN IF NOT EXISTS cash_balance DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS gift_balance DECIMAL(20,8) NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN IF NOT EXISTS frozen_gift_balance DECIMAL(20,8) NOT NULL DEFAULT 0;

-- Existing production balance includes both paid recharge and previous rewards.
-- For safety, classify the current available balance as cash so existing users do not lose
-- subscription purchase ability during migration. New bonuses/rebates go to gift_balance.
UPDATE users
SET cash_balance = balance
WHERE cash_balance = 0
  AND gift_balance = 0
  AND balance <> 0;
