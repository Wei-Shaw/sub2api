INSERT INTO settings (key, value)
VALUES
    ('openai_round_robin_scheduler_enabled', 'false')
ON CONFLICT (key) DO NOTHING;
