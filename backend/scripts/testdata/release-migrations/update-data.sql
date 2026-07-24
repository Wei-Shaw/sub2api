UPDATE users
SET status = 'active'
WHERE status IS NULL;
