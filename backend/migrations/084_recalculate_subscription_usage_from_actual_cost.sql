UPDATE user_subscriptions us
SET
    daily_usage_usd = COALESCE((
        SELECT SUM(ul.actual_cost)
        FROM usage_logs ul
        WHERE ul.subscription_id = us.id
          AND ul.created_at >= us.daily_window_start
    ), 0),
    weekly_usage_usd = COALESCE((
        SELECT SUM(ul.actual_cost)
        FROM usage_logs ul
        WHERE ul.subscription_id = us.id
          AND ul.created_at >= us.weekly_window_start
    ), 0),
    monthly_usage_usd = COALESCE((
        SELECT SUM(ul.actual_cost)
        FROM usage_logs ul
        WHERE ul.subscription_id = us.id
          AND ul.created_at >= us.monthly_window_start
    ), 0),
    updated_at = NOW()
WHERE us.deleted_at IS NULL;
