ALTER TABLE accounts ADD COLUMN IF NOT EXISTS owner_user_id BIGINT REFERENCES users(id);
ALTER TABLE groups ADD COLUMN IF NOT EXISTS owner_user_id BIGINT REFERENCES users(id);
ALTER TABLE groups ADD COLUMN IF NOT EXISTS supplier_rate NUMERIC(10,8) NOT NULL DEFAULT 0.9
    CHECK (supplier_rate >= 0 AND supplier_rate <= 1);
CREATE INDEX IF NOT EXISTS accounts_owner_user_id_idx ON accounts(owner_user_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS groups_owner_user_id_idx ON groups(owner_user_id) WHERE deleted_at IS NULL;

ALTER TABLE groups ADD CONSTRAINT groups_private_relay_check CHECK (
    owner_user_id IS NULL OR (
        is_exclusive AND subscription_type = 'standard' AND rate_multiplier = 0
        AND fallback_group_id IS NULL AND fallback_group_id_on_invalid_request IS NULL
        AND NOT profit_control_enabled
    )
);

-- Ownership is permanent; retaining it after soft deletion preserves settlement attribution.
CREATE OR REPLACE FUNCTION relay_owner_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.owner_user_id IS DISTINCT FROM OLD.owner_user_id THEN
        RAISE EXCEPTION 'relay ownership is immutable';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER accounts_relay_owner_immutable BEFORE UPDATE OF owner_user_id ON accounts
    FOR EACH ROW EXECUTE FUNCTION relay_owner_immutable();
CREATE TRIGGER groups_relay_owner_immutable BEFORE UPDATE OF owner_user_id ON groups
    FOR EACH ROW EXECUTE FUNCTION relay_owner_immutable();

-- Guard every writer, including admin bulk binding and direct repository callers.
CREATE OR REPLACE FUNCTION relay_account_group_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE account_owner BIGINT; group_owner BIGINT; exclusive_group BOOLEAN; group_type VARCHAR;
BEGIN
    SELECT owner_user_id INTO account_owner FROM accounts WHERE id = NEW.account_id;
    SELECT owner_user_id, is_exclusive, subscription_type INTO group_owner, exclusive_group, group_type
        FROM groups WHERE id = NEW.group_id;
    IF group_owner IS NOT NULL AND account_owner IS DISTINCT FROM group_owner THEN
        RAISE EXCEPTION 'private relay accepts only its owner accounts';
    END IF;
    IF account_owner IS NOT NULL AND group_owner IS NULL AND (exclusive_group OR group_type <> 'standard') THEN
        RAISE EXCEPTION 'user accounts may be shared only with public balance groups';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER account_groups_relay_guard BEFORE INSERT OR UPDATE ON account_groups
    FOR EACH ROW EXECUTE FUNCTION relay_account_group_guard();

CREATE OR REPLACE FUNCTION relay_group_update_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.owner_user_id IS NULL AND (NEW.is_exclusive OR NEW.subscription_type <> 'standard')
        AND EXISTS (SELECT 1 FROM account_groups ag JOIN accounts a ON a.id=ag.account_id
                    WHERE ag.group_id=NEW.id AND a.owner_user_id IS NOT NULL AND a.deleted_at IS NULL) THEN
        RAISE EXCEPTION 'remove user supplied accounts before changing the public group type';
    END IF;
    IF EXISTS (SELECT 1 FROM groups g WHERE g.owner_user_id IS NOT NULL
               AND g.id IN (NEW.fallback_group_id,NEW.fallback_group_id_on_invalid_request)) THEN
        RAISE EXCEPTION 'private relays cannot be fallback targets';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER groups_relay_guard BEFORE INSERT OR UPDATE ON groups
    FOR EACH ROW EXECUTE FUNCTION relay_group_update_guard();

CREATE OR REPLACE FUNCTION relay_api_key_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE group_owner BIGINT;
BEGIN
    SELECT owner_user_id INTO group_owner FROM groups WHERE id=NEW.group_id;
    IF group_owner IS NOT NULL AND group_owner IS DISTINCT FROM NEW.user_id THEN
        RAISE EXCEPTION 'private relay API keys must belong to the group owner';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER api_keys_relay_guard BEFORE INSERT OR UPDATE OF group_id,user_id ON api_keys
    FOR EACH ROW EXECUTE FUNCTION relay_api_key_guard();

CREATE TABLE IF NOT EXISTS relay_earnings (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    consumer_user_id BIGINT NOT NULL REFERENCES users(id),
    supplier_user_id BIGINT NOT NULL REFERENCES users(id),
    account_id BIGINT NOT NULL REFERENCES accounts(id),
    group_id BIGINT NOT NULL REFERENCES groups(id),
    charged_amount NUMERIC(20,8) NOT NULL CHECK (charged_amount >= 0),
    supplier_rate NUMERIC(10,8) NOT NULL CHECK (supplier_rate >= 0 AND supplier_rate <= 1),
    amount NUMERIC(20,8) NOT NULL CHECK (amount >= 0 AND amount <= charged_amount),
	credited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (request_id, api_key_id)
);
CREATE INDEX IF NOT EXISTS relay_earnings_supplier_time_idx ON relay_earnings(supplier_user_id, id DESC);
CREATE INDEX IF NOT EXISTS relay_earnings_pending_idx ON relay_earnings(supplier_user_id, id) WHERE credited_at IS NULL;
ALTER TABLE batch_image_jobs ADD COLUMN IF NOT EXISTS relay_group_id BIGINT REFERENCES groups(id);
ALTER TABLE batch_image_jobs ADD COLUMN IF NOT EXISTS supplier_user_id BIGINT REFERENCES users(id);
ALTER TABLE batch_image_jobs ADD COLUMN IF NOT EXISTS supplier_rate NUMERIC(10,8) NOT NULL DEFAULT 0;

CREATE OR REPLACE FUNCTION relay_user_deleted() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE resource RECORD;
BEGIN
    IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
        FOR resource IN UPDATE accounts SET deleted_at=NEW.deleted_at,status='disabled',schedulable=false,updated_at=NOW()
            WHERE owner_user_id=NEW.id AND deleted_at IS NULL RETURNING id
        LOOP
            INSERT INTO scheduler_outbox(event_type,account_id,payload)
                VALUES('account_changed',resource.id,jsonb_build_object('group_ids',
                    (SELECT COALESCE(jsonb_agg(group_id),'[]'::jsonb) FROM account_groups WHERE account_id=resource.id)));
        END LOOP;
        FOR resource IN UPDATE groups SET deleted_at=NEW.deleted_at,status='disabled',updated_at=NOW()
            WHERE owner_user_id=NEW.id AND deleted_at IS NULL RETURNING id
        LOOP
            INSERT INTO scheduler_outbox(event_type,group_id) VALUES('group_changed',resource.id);
        END LOOP;
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER users_relay_deleted AFTER UPDATE OF deleted_at ON users
    FOR EACH ROW EXECUTE FUNCTION relay_user_deleted();
