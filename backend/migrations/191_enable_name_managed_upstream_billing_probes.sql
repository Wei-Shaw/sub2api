-- OpenAI API Key account probe eligibility is managed by a strict lowercase
-- name marker. Preserve existing snapshots and all unrelated account metadata.
UPDATE accounts
SET extra = jsonb_set(
        COALESCE(extra, '{}'::jsonb),
        '{upstream_billing_probe_enabled}',
        to_jsonb(name NOT LIKE '%free%'),
        true
    ),
    updated_at = NOW()
WHERE deleted_at IS NULL
  AND platform = 'openai'
  AND type = 'apikey';
