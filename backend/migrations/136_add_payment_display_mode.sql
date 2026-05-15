INSERT INTO settings (key, value)
VALUES ('payment_display_mode', 'off')
ON CONFLICT (key) DO NOTHING;
