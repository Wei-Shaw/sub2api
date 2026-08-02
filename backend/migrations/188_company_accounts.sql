CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS organizations (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(16) NOT NULL UNIQUE,
    owner_user_id BIGINT NOT NULL REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    normalized_name VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    member_limit INTEGER NOT NULL DEFAULT 20 CHECK (member_limit > 0),
    effective_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_organizations_name_trgm ON organizations USING GIN (normalized_name gin_trgm_ops);

CREATE TABLE IF NOT EXISTS organization_memberships (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id),
    role VARCHAR(16) NOT NULL CHECK (role IN ('owner', 'member')),
    status VARCHAR(16) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'archived')),
    authz_generation BIGINT NOT NULL DEFAULT 1,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS organization_single_owner
    ON organization_memberships(organization_id) WHERE role = 'owner';
CREATE INDEX IF NOT EXISTS idx_organization_memberships_org_status
    ON organization_memberships(organization_id, status);

CREATE TABLE IF NOT EXISTS company_upgrade_applications (
    id BIGSERIAL PRIMARY KEY,
    applicant_user_id BIGINT NOT NULL REFERENCES users(id),
    requested_name VARCHAR(255) NOT NULL,
    normalized_name VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn')),
    fee_amount NUMERIC(20,8) NOT NULL CHECK (fee_amount > 0),
    fee_currency VARCHAR(8) NOT NULL DEFAULT 'USD',
    idempotency_key VARCHAR(128) NOT NULL,
    reviewer_user_id BIGINT REFERENCES users(id),
    review_reason TEXT,
    organization_id BIGINT REFERENCES organizations(id),
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS company_upgrade_one_pending_per_user
    ON company_upgrade_applications(applicant_user_id) WHERE status = 'pending';
CREATE UNIQUE INDEX IF NOT EXISTS company_upgrade_applicant_idempotency_unique
    ON company_upgrade_applications(applicant_user_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_company_upgrade_status_created
    ON company_upgrade_applications(status, created_at);
CREATE INDEX IF NOT EXISTS idx_company_upgrade_name_trgm
    ON company_upgrade_applications USING GIN (normalized_name gin_trgm_ops);

CREATE TABLE IF NOT EXISTS organization_name_change_requests (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    applicant_user_id BIGINT NOT NULL REFERENCES users(id),
    old_name VARCHAR(255) NOT NULL,
    new_name VARCHAR(255) NOT NULL,
    normalized_name VARCHAR(255) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'withdrawn')),
    reviewer_user_id BIGINT REFERENCES users(id),
    review_reason TEXT,
    decided_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS organization_name_change_one_pending
    ON organization_name_change_requests(organization_id) WHERE status = 'pending';

CREATE TABLE IF NOT EXISTS managed_policies (
    id BIGSERIAL PRIMARY KEY,
    policy_key VARCHAR(128) NOT NULL UNIQUE,
    display_name VARCHAR(128) NOT NULL,
    policy_type VARCHAR(32) NOT NULL DEFAULT 'system' CHECK (policy_type = 'system'),
    description TEXT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS managed_policy_actions (
    id BIGSERIAL PRIMARY KEY,
    policy_id BIGINT NOT NULL REFERENCES managed_policies(id) ON DELETE CASCADE,
    action VARCHAR(160) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(policy_id, action)
);
CREATE TABLE IF NOT EXISTS member_policy_attachments (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT NOT NULL REFERENCES organizations(id),
    membership_id BIGINT NOT NULL REFERENCES organization_memberships(id),
    policy_id BIGINT NOT NULL REFERENCES managed_policies(id),
    policy_version INTEGER NOT NULL,
    attached_by_user_id BIGINT NOT NULL REFERENCES users(id),
    detached_by_user_id BIGINT REFERENCES users(id),
    detached_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS member_policy_attachment_active_unique
    ON member_policy_attachments(membership_id, policy_id) WHERE detached_at IS NULL;

CREATE TABLE IF NOT EXISTS organization_financial_ledger (
    id BIGSERIAL PRIMARY KEY,
    idempotency_key VARCHAR(160) NOT NULL UNIQUE,
    kind VARCHAR(32) NOT NULL CHECK (kind IN ('upgrade_reserve', 'upgrade_capture', 'upgrade_release', 'allocate', 'reclaim')),
    organization_id BIGINT REFERENCES organizations(id),
    application_id BIGINT REFERENCES company_upgrade_applications(id),
    actor_user_id BIGINT NOT NULL REFERENCES users(id),
    source_user_id BIGINT REFERENCES users(id),
    destination_user_id BIGINT REFERENCES users(id),
    amount NUMERIC(20,8) NOT NULL CHECK (amount > 0),
    currency VARCHAR(8) NOT NULL DEFAULT 'USD',
    source_balance_after NUMERIC(20,8),
    destination_balance_after NUMERIC(20,8),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_org_financial_ledger_org_created
    ON organization_financial_ledger(organization_id, created_at);

CREATE TABLE IF NOT EXISTS organization_audit_events (
    id BIGSERIAL PRIMARY KEY,
    organization_id BIGINT REFERENCES organizations(id),
    actor_user_id BIGINT REFERENCES users(id),
    subject_user_id BIGINT REFERENCES users(id),
    action VARCHAR(128) NOT NULL,
    result VARCHAR(32) NOT NULL,
    correlation_id VARCHAR(128),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_org_audit_org_created ON organization_audit_events(organization_id, created_at);

CREATE TABLE IF NOT EXISTS notification_outbox (
    id BIGSERIAL PRIMARY KEY,
    dedup_key VARCHAR(255) NOT NULL UNIQUE,
    event VARCHAR(128) NOT NULL,
    recipient VARCHAR(255) NOT NULL,
    locale VARCHAR(16) NOT NULL DEFAULT 'en-US',
    variables JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'processing', 'delivered', 'failed')),
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ,
    claimed_by_worker_id VARCHAR(64),
    delivered_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notification_outbox_claim
    ON notification_outbox(status, next_attempt_at, id);
ALTER TABLE notification_outbox ADD COLUMN IF NOT EXISTS claimed_by_worker_id VARCHAR(64);

INSERT INTO managed_policies (policy_key, display_name, policy_type, description, version)
VALUES
    ('CompanyFinanceReadOnly', 'Company finance read only', 'system', 'View the root account available, frozen, and total balance.', 1),
    ('CompanySharedBalanceUse', 'Company shared balance use', 'system', 'Use the root account balance for API consumption without viewing its amount.', 1)
ON CONFLICT (policy_key) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    description = EXCLUDED.description;

INSERT INTO managed_policy_actions (policy_id, action)
SELECT id, 'organization.finance.balance.read' FROM managed_policies WHERE policy_key = 'CompanyFinanceReadOnly'
ON CONFLICT DO NOTHING;
INSERT INTO managed_policy_actions (policy_id, action)
SELECT id, 'organization.balance.shared.use' FROM managed_policies WHERE policy_key = 'CompanySharedBalanceUse'
ON CONFLICT DO NOTHING;
