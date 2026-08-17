-- Stateless session/full rewriting cannot produce a valid Codex session graph.
-- Preserve the enabled state while narrowing legacy modes to installation-only.
UPDATE accounts
SET extra = jsonb_set(extra, '{codex_fingerprint_mode}', '"device"'::jsonb, true),
    updated_at = NOW()
WHERE platform = 'openai'
  AND type = 'oauth'
  AND BTRIM(extra->>'codex_fingerprint_mode') IN ('session', 'full');
