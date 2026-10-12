#!/usr/bin/env python3
"""Bounded, rehearsed v0.2.8 migration; never restore production data on rollback."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path('/home/serveradmin/releases/buaga-v028-custom-20260923')
MANIFEST = Path('/home/serveradmin/releases/buaga-current.json')
PG = 'sub2api-return-postgres'
PILOT = 'buaga_upgrade_v028_20260923'
NAMES = ['238b_content_moderation_engine_meta.sql',
         '239_channel_reasoning_effort_multipliers.sql',
         '240_affiliate_ledger_operation_id.sql']
TABLES = ['groups', 'channels', 'channel_model_pricing',
          'channel_account_stats_model_pricing', 'content_moderation_logs',
          'user_affiliate_ledger', 'schema_migrations', 'settings']

def command(*args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)

def sql(db, content):
    result = subprocess.run(['docker', 'exec', '-i', PG, 'psql', '-X', '-qAt',
                             '-v', 'ON_ERROR_STOP=1', '-U', 'sub2api', '-d', db],
                            input=content, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError('Database operation failed: ' + result.stderr[-1800:])
    return result.stdout.strip()

def record(name, data):
    (ROOT / (name + '.json')).write_text(json.dumps(data, indent=2) + '\n')

def migration_files():
    current = json.loads(MANIFEST.read_text())
    before = Path(current['remote_source']) / 'backend/migrations'
    after = ROOT / 'source/backend/migrations'
    old = {p.name: p.read_bytes() for p in before.glob('*.sql')}
    new = {p.name: p.read_bytes() for p in after.glob('*.sql')}
    assert sorted(set(new) - set(old)) == NAMES
    assert all(new.get(name) == content for name, content in old.items())
    return {name: new[name].decode().strip() for name in NAMES}

def dump(path, *args):
    assert not path.exists(), 'Never overwrite a backup'
    with path.open('xb') as stream:
        command('docker', 'exec', PG, 'pg_dump', '-U', 'sub2api', '-d', 'sub2api',
                '--lock-wait-timeout=2s', *args, stdout=stream)
    path.chmod(0o600)
    assert path.stat().st_size > 0

def backup():
    ROOT.mkdir(parents=True, exist_ok=True)
    os.umask(0o077)
    dump(ROOT / 'database-before.dump', '-Fc', '-Z', '1')
    with (ROOT / 'database-before.dump').open('rb') as stream:
        command('docker', 'exec', '-i', PG, 'pg_restore', '--list',
                stdin=stream, stdout=subprocess.DEVNULL)
    record('database-backup', {'passed': True, 'path': str(ROOT / 'database-before.dump'),
                              'bytes': (ROOT / 'database-before.dump').stat().st_size})
    print('Online database backup completed and archive catalog verified.')

# Canonicalize only the documented legacy effort-key conversion. Every other
# field, price, time tier, group multiplier and setting must remain identical.
SNAPSHOT = """
CREATE OR REPLACE FUNCTION pg_temp.effort_price(j jsonb) RETURNS jsonb
LANGUAGE sql IMMUTABLE AS $$
 SELECT (j - 'max_reasoning_effort_multiplier' - 'reasoning_effort_multipliers') ||
 jsonb_build_object('reasoning_effort_multipliers',
   CASE WHEN j ? 'reasoning_effort_multipliers' THEN j->'reasoning_effort_multipliers'
        WHEN jsonb_typeof(j->'max_reasoning_effort_multiplier')='number'
          THEN jsonb_build_object('max', j->'max_reasoning_effort_multiplier')
        ELSE '{}'::jsonb END)
$$;
CREATE OR REPLACE FUNCTION pg_temp.group_price(j jsonb) RETURNS jsonb
LANGUAGE sql IMMUTABLE AS $$
 SELECT CASE WHEN jsonb_typeof(j->'model_pricing')='array' THEN
 jsonb_set(j, '{model_pricing}', COALESCE((SELECT jsonb_agg(pg_temp.effort_price(e) ORDER BY n)
 FROM jsonb_array_elements(j->'model_pricing') WITH ORDINALITY a(e,n)), '[]'::jsonb))
 ELSE j END
$$;
CREATE TEMP VIEW preserved_config AS
SELECT 'groups' AS name, COALESCE(jsonb_agg(pg_temp.group_price(to_jsonb(g)) ORDER BY g.id),'[]'::jsonb) AS value FROM groups g
UNION ALL SELECT 'channels', COALESCE(jsonb_agg(to_jsonb(c) ORDER BY c.id),'[]'::jsonb) FROM channels c
UNION ALL SELECT 'channel_prices', COALESCE(jsonb_agg(pg_temp.effort_price(to_jsonb(c)) ORDER BY c.id),'[]'::jsonb) FROM channel_model_pricing c
UNION ALL SELECT 'stats_prices', COALESCE(jsonb_agg(pg_temp.effort_price(to_jsonb(c)) ORDER BY c.id),'[]'::jsonb) FROM channel_account_stats_model_pricing c
UNION ALL SELECT 'settings', COALESCE(jsonb_agg(to_jsonb(s) ORDER BY s.key),'[]'::jsonb) FROM settings s;
CREATE TEMP TABLE config_before AS SELECT * FROM preserved_config;
"""

def migrate(db, files):
    # NOWAIT avoids a queued DDL lock blocking incoming requests. The transaction
    # timeout also bounds the entire lock-holding period, including assertions.
    parts = ["BEGIN; SET LOCAL lock_timeout='500ms'; SET LOCAL statement_timeout='3s'; SET LOCAL transaction_timeout='3s';",
             "SET LOCAL idle_in_transaction_session_timeout='3s';",
             'LOCK TABLE ' + ','.join(TABLES) + ' IN ACCESS EXCLUSIVE MODE NOWAIT;',
             SNAPSHOT]
    for name, content in files.items():
        checksum = hashlib.sha256(content.encode()).hexdigest()
        parts += [content, "INSERT INTO schema_migrations(filename,checksum) VALUES ('" +
                  name + "','" + checksum + "');"]
    parts += ["""DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM config_before b FULL JOIN preserved_config a USING(name)
            WHERE b.value IS DISTINCT FROM a.value) THEN
   RAISE EXCEPTION 'Configuration or effective price changed: refusing migration';
 END IF;
END $$;
COMMIT;"""]
    started = time.monotonic()
    sql(db, '\n'.join(parts))
    return round(time.monotonic() - started, 3)

def rehearse():
    files = migration_files()
    assert sql('sub2api', "SELECT count(*) FROM pg_database WHERE datname='" + PILOT + "'") == '0'
    dump(ROOT / 'schema-before.sql', '--schema-only', '--no-owner', '--no-privileges')
    args = [arg for table in TABLES for arg in ('-t', table)]
    dump(ROOT / 'configuration-before.dump', '-Fc', '--data-only', *args)
    sql('sub2api', 'CREATE DATABASE ' + PILOT + ' TEMPLATE template0;')
    with (ROOT / 'schema-before.sql').open('rb') as stream:
        command('docker', 'exec', '-i', PG, 'psql', '-X', '-q', '-v', 'ON_ERROR_STOP=1',
                '-U', 'sub2api', '-d', PILOT, stdin=stream, stdout=subprocess.DEVNULL)
    with (ROOT / 'configuration-before.dump').open('rb') as stream:
        command('docker', 'exec', '-i', PG, 'pg_restore', '-U', 'sub2api', '-d', PILOT,
                '--data-only', '--disable-triggers', '--exit-on-error', '--no-owner',
                stdin=stream, stdout=subprocess.DEVNULL)
    elapsed = migrate(PILOT, files)
    record('migration-rehearsal', {'passed': True, 'database': PILOT, 'seconds': elapsed,
                                  'effective_pricing_and_settings_unchanged': True,
                                  'files': {k: hashlib.sha256(v.encode()).hexdigest() for k,v in files.items()}})
    print('Isolated migration passed; all pricing and settings preserved. Seconds:', elapsed)

def apply():
    files = migration_files()
    proof = json.loads((ROOT / 'migration-rehearsal.json').read_text())
    assert proof['passed'] and proof['files'] == {k: hashlib.sha256(v.encode()).hexdigest() for k,v in files.items()}
    assert json.loads((ROOT / 'database-backup.json').read_text())['passed']
    assert sql('sub2api', 'SELECT count(*) FROM usage_group_rollup_state') == '1'
    elapsed = migrate('sub2api', files)
    record('migration-applied', {**proof, 'database': 'sub2api', 'seconds': elapsed,
                               'rollback': 'Keep additive schema; switch application only. Never restore database.'})
    print('Production migration committed; pricing and settings unchanged. Seconds:', elapsed)

if __name__ == '__main__':
    {'backup': backup, 'rehearse': rehearse, 'apply': apply}[sys.argv[1]]()
