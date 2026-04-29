-- Convert old boolean web_search_emulation to tri-state string
-- true → "enabled", false → remove key (becomes "default")
UPDATE accounts
SET extra = (extra - 'web_search_emulation') || jsonb_build_object('web_search_emulation', 'enabled')
WHERE extra ? 'web_search_emulation'
  AND extra->>'web_search_emulation' = 'true';

UPDATE accounts
SET extra = extra - 'web_search_emulation'
WHERE extra ? 'web_search_emulation'
  AND extra->>'web_search_emulation' = 'false';
