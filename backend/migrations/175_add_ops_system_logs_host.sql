-- Track the application host that emitted each indexed system log.
ALTER TABLE ops_system_logs
  ADD COLUMN IF NOT EXISTS host VARCHAR(255);
