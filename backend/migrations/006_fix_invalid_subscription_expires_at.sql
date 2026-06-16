-- Fix legacy subscription records with invalid expires_at (year > 2099).
DO $$
BEGIN
  IF to_regclass('public.user_subscriptions') IS NOT NULL THEN
    UPDATE user_subscriptions
    SET expires_at = TIMESTAMPTZ '2099-12-31 23:59:59+00'
    WHERE expires_at > TIMESTAMPTZ '2099-12-31 23:59:59+00';
  END IF;
END $$;

