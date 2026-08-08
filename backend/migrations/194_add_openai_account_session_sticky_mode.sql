-- Keep existing accounts on the historical sticky behavior while allowing an
-- explicitly configured OpenAI fallback account to yield session_hash affinity.
ALTER TABLE accounts
    ADD COLUMN IF NOT EXISTS openai_session_sticky_mode VARCHAR(32) NOT NULL DEFAULT 'normal';

UPDATE accounts
SET openai_session_sticky_mode = 'normal'
WHERE openai_session_sticky_mode IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'accounts_openai_session_sticky_mode_check'
    ) THEN
        ALTER TABLE accounts
            ADD CONSTRAINT accounts_openai_session_sticky_mode_check
            CHECK (openai_session_sticky_mode IN ('normal', 'fallback_only'));
    END IF;
END $$;
