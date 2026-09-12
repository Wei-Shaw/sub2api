-- Required by the model substring index in migration 243. pg_trgm is a trusted
-- PostgreSQL extension; the migration role must have CREATE on this database.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
