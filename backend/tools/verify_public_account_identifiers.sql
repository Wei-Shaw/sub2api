SELECT
    COUNT(*) AS total_users,
    COUNT(*) FILTER (WHERE account_id IS NULL OR external_user_id IS NULL) AS missing_ids,
    COUNT(*) FILTER (WHERE account_id !~ '^[1-9][0-9]{15}$') AS invalid_account_ids,
    COUNT(*) FILTER (WHERE identity_type = 'root' AND account_id IS DISTINCT FROM external_user_id) AS invalid_root_ids,
    COUNT(*) FILTER (WHERE identity_type = 'iam' AND external_user_id !~ '^[1-9][0-9]{17}$') AS invalid_iam_ids,
    COUNT(*) - COUNT(DISTINCT external_user_id) AS duplicate_external_ids
FROM users;
