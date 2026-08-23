SELECT
    COUNT(*) AS total_users,
    COUNT(*) FILTER (WHERE account_id IS NULL) AS missing_account_ids,
    COUNT(*) - COUNT(DISTINCT account_id) AS duplicate_account_ids,
    COUNT(*) FILTER (WHERE account_id !~ '^[1-9][0-9]{15}$') AS invalid_account_ids
FROM users;

SELECT account_id, COUNT(*) AS user_count
FROM users
GROUP BY account_id
HAVING COUNT(*) > 1
ORDER BY account_id;
