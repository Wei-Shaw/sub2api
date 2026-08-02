-- Scope company-upgrade submission idempotency to the authenticated applicant.
-- This allows independent clients to reuse opaque idempotency keys without
-- exposing or replaying another user's application.
ALTER TABLE company_upgrade_applications
    DROP CONSTRAINT IF EXISTS company_upgrade_applications_idempotency_key_key;

CREATE UNIQUE INDEX IF NOT EXISTS company_upgrade_applicant_idempotency_unique
    ON company_upgrade_applications(applicant_user_id, idempotency_key);
