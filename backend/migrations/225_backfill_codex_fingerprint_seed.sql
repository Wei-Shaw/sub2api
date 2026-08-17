-- Codex persists a random installation identity. Backfill one random seed for
-- each convergence-enabled OpenAI OAuth account instead of deriving globally
-- visible identities from the deployment-local accounts.id sequence.
UPDATE accounts
SET extra = jsonb_set(
    COALESCE(extra, '{}'::jsonb),
    '{codex_fingerprint_seed}',
    to_jsonb(gen_random_uuid()::text),
    true
)
WHERE platform = 'openai'
  AND type = 'oauth'
  AND BTRIM(extra->>'codex_fingerprint_mode') IN ('device', 'session', 'full')
  AND NOT (
      COALESCE(extra->>'codex_fingerprint_seed', '') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
      OR COALESCE(extra->>'codex_fingerprint_seed', '') ~* '^[0-9a-f]{32}$'
      OR COALESCE(extra->>'codex_fingerprint_seed', '') ~* '^urn:uuid:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
      OR COALESCE(extra->>'codex_fingerprint_seed', '') ~* '^\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}$'
  );
