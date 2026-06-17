-- Allow subscription plan validity to store fractional days such as 0.5.
ALTER TABLE subscription_plans
    ALTER COLUMN validity_days TYPE numeric(20,8) USING validity_days::numeric;
