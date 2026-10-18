-- Store only non-secret request conditions; legacy errors have unknown scope.
ALTER TABLE ops_error_logs ADD COLUMN quality_reasoning_effort TEXT;
ALTER TABLE ops_error_logs ADD COLUMN quality_service_tier TEXT;
ALTER TABLE ops_error_logs ADD COLUMN quality_scope_known BOOLEAN NOT NULL DEFAULT false;
