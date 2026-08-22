-- IAM login names must start with an ASCII letter. Keep the existing character
-- set and length limit, while rejecting names that begin with a digit or symbol.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_iam_login_name_check;

ALTER TABLE users
    ADD CONSTRAINT users_iam_login_name_check
    CHECK (identity_type <> 'iam' OR login_name ~ '^[A-Za-z][A-Za-z0-9._-]{0,63}$');
