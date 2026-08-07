-- Soft switch: hide RPM/TPM throughput rates on user-facing Channel Monitor V2.
-- Default true (rates hidden). Admins always see full metrics.
INSERT INTO settings (key, value)
VALUES ('channel_monitor_hide_throughput', 'true')
ON CONFLICT (key) DO NOTHING;
