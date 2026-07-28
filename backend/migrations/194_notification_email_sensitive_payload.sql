-- Sensitive template variables, such as verification codes and password-reset
-- URLs, are encrypted by the application before they enter this column.
ALTER TABLE notification_email_deliveries
    ADD COLUMN IF NOT EXISTS sensitive_variables_ciphertext TEXT;
