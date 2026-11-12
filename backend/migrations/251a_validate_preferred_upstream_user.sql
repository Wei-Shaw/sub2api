SET LOCAL lock_timeout = '5s';

ALTER TABLE accounts VALIDATE CONSTRAINT fk_accounts_preferred_user_id;
