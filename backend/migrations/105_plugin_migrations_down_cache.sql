-- P13/C-2 hard purge: cache down-migration SQL on plugin enable so the host
-- can roll back plugin schema during Purge even when the plugin process is no
-- longer alive (e.g. user uninstalled the binary before purging).
--
-- All three columns are NULL-able. NULL down_sql_cached means the plugin did
-- not declare a down file for this entry -- Purge will skip executing SQL for
-- that row but still delete its bookkeeping. down_filename / down_checksum
-- are denormalised copies of MigrationDecl.{DownFilename, DownChecksumSha256}
-- kept here so the host has a self-contained record after the plugin is gone.

ALTER TABLE plugin_migrations
    ADD COLUMN IF NOT EXISTS down_sql_cached       TEXT NULL;
ALTER TABLE plugin_migrations
    ADD COLUMN IF NOT EXISTS down_filename         TEXT NULL;
ALTER TABLE plugin_migrations
    ADD COLUMN IF NOT EXISTS down_checksum_sha256  TEXT NULL;
