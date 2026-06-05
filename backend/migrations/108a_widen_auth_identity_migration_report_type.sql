DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'auth_identity_migration_reports'
          AND column_name = 'report_type'
          AND COALESCE(character_maximum_length, 0) < 80
    ) THEN
        ALTER TABLE auth_identity_migration_reports
            ALTER COLUMN report_type TYPE VARCHAR(80);
    END IF;
END $$;
