-- Soft switch: hide RPM/TPM throughput rates on user-facing Channel Monitor V2.
-- Default false (rates visible). Admins always see full metrics.
INSERT INTO settings (key, value)
VALUES ('channel_monitor_hide_throughput', 'false')
ON CONFLICT (key) DO NOTHING;
