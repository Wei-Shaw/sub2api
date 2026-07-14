-- Keep mixed-version writers consistent before backfilling rows that already exist.
CREATE OR REPLACE FUNCTION public.enforce_openai_long_context_billing_extra()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    parent_effective_value JSONB;
BEGIN
    IF NEW.platform IS DISTINCT FROM 'openai' THEN
        RETURN NEW;
    END IF;

    NEW.extra := COALESCE(NEW.extra, '{}'::jsonb);
    IF NEW.parent_account_id IS NOT NULL AND NEW.quota_dimension = 'spark' THEN
        SELECT CASE
            WHEN parent.platform IS DISTINCT FROM 'openai' THEN 'false'::jsonb
            WHEN NOT (COALESCE(parent.extra, '{}'::jsonb) ? 'openai_long_context_billing_enabled') THEN 'false'::jsonb
            WHEN jsonb_typeof(parent.extra->'openai_long_context_billing_enabled') = 'boolean'
                THEN parent.extra->'openai_long_context_billing_enabled'
            ELSE 'false'::jsonb
        END
        INTO parent_effective_value
        FROM accounts AS parent
        WHERE parent.id = NEW.parent_account_id;

        NEW.extra := jsonb_set(
            NEW.extra,
            '{openai_long_context_billing_enabled}',
            COALESCE(parent_effective_value, 'false'::jsonb),
            true
        );
    ELSIF NOT (NEW.extra ? 'openai_long_context_billing_enabled')
        AND TG_OP = 'UPDATE'
        AND OLD.platform = 'openai'
        AND jsonb_typeof(OLD.extra->'openai_long_context_billing_enabled') = 'boolean' THEN
        NEW.extra := jsonb_set(
            NEW.extra,
            '{openai_long_context_billing_enabled}',
            OLD.extra->'openai_long_context_billing_enabled',
            true
        );
    ELSIF NOT (NEW.extra ? 'openai_long_context_billing_enabled') THEN
        NEW.extra := jsonb_set(
            NEW.extra,
            '{openai_long_context_billing_enabled}',
            'false'::jsonb,
            true
        );
    END IF;

    IF jsonb_typeof(NEW.extra->'openai_long_context_billing_enabled') IS DISTINCT FROM 'boolean' THEN
        RAISE EXCEPTION 'openai_long_context_billing_enabled must be a boolean'
            USING ERRCODE = '22023';
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION public.propagate_openai_long_context_billing_extra_to_shadows()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    WITH updated_shadows AS (
        UPDATE accounts AS shadow
        SET extra = jsonb_set(
            COALESCE(shadow.extra, '{}'::jsonb),
            '{openai_long_context_billing_enabled}',
            NEW.extra->'openai_long_context_billing_enabled',
            true
        )
        WHERE shadow.parent_account_id = NEW.id
          AND shadow.platform = 'openai'
          AND shadow.quota_dimension = 'spark'
          AND shadow.extra->'openai_long_context_billing_enabled'
              IS DISTINCT FROM NEW.extra->'openai_long_context_billing_enabled'
        RETURNING shadow.id
    )
    INSERT INTO scheduler_outbox (event_type, account_id)
    SELECT 'account_changed', id
    FROM updated_shadows;
    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS accounts_enforce_openai_long_context_billing_extra ON accounts;
CREATE TRIGGER accounts_enforce_openai_long_context_billing_extra
BEFORE INSERT OR UPDATE OF platform, extra, parent_account_id, quota_dimension
ON accounts
FOR EACH ROW
EXECUTE FUNCTION public.enforce_openai_long_context_billing_extra();

DROP TRIGGER IF EXISTS accounts_propagate_openai_long_context_billing_extra ON accounts;
CREATE TRIGGER accounts_propagate_openai_long_context_billing_extra
AFTER UPDATE OF platform, extra
ON accounts
FOR EACH ROW
WHEN (
    NEW.platform = 'openai'
    AND NEW.parent_account_id IS NULL
    AND (
        OLD.platform IS DISTINCT FROM NEW.platform
        OR OLD.extra->'openai_long_context_billing_enabled'
            IS DISTINCT FROM NEW.extra->'openai_long_context_billing_enabled'
    )
)
EXECUTE FUNCTION public.propagate_openai_long_context_billing_extra_to_shadows();

UPDATE accounts
SET extra = jsonb_set(
    COALESCE(extra, '{}'::jsonb),
    '{openai_long_context_billing_enabled}',
    'false'::jsonb,
    true
)
WHERE platform = 'openai'
  AND COALESCE(extra, '{}'::jsonb) ? 'openai_long_context_billing_enabled'
  AND jsonb_typeof(extra->'openai_long_context_billing_enabled') IS DISTINCT FROM 'boolean';

UPDATE accounts
SET extra = jsonb_set(
    COALESCE(extra, '{}'::jsonb),
    '{openai_long_context_billing_enabled}',
    'false'::jsonb,
    true
)
WHERE platform = 'openai'
  AND parent_account_id IS NULL
  AND NOT (COALESCE(extra, '{}'::jsonb) ? 'openai_long_context_billing_enabled');

WITH shadow_values AS (
    SELECT
        shadow.id,
        CASE
            WHEN parent.platform IS DISTINCT FROM 'openai' THEN 'false'::jsonb
            WHEN NOT (COALESCE(parent.extra, '{}'::jsonb) ? 'openai_long_context_billing_enabled') THEN 'false'::jsonb
            WHEN jsonb_typeof(parent.extra->'openai_long_context_billing_enabled') = 'boolean'
                THEN parent.extra->'openai_long_context_billing_enabled'
            ELSE 'false'::jsonb
        END AS effective_value
    FROM accounts AS shadow
    JOIN accounts AS parent ON parent.id = shadow.parent_account_id
    WHERE shadow.platform = 'openai'
      AND shadow.quota_dimension = 'spark'
),
updated_shadows AS (
    UPDATE accounts AS shadow
    SET extra = jsonb_set(
        COALESCE(shadow.extra, '{}'::jsonb),
        '{openai_long_context_billing_enabled}',
        shadow_values.effective_value,
        true
    )
    FROM shadow_values
    WHERE shadow.id = shadow_values.id
      AND shadow.extra->'openai_long_context_billing_enabled'
          IS DISTINCT FROM shadow_values.effective_value
    RETURNING shadow.id
)
INSERT INTO scheduler_outbox (event_type, account_id)
SELECT 'account_changed', id
FROM updated_shadows;
