#!/usr/bin/env python3
"""Online backup and read-only configuration verification for the v0.2.9 upgrade."""
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path('/home/serveradmin/releases/buaga-v029-custom-20260928')
PG = 'sub2api-return-postgres'
TABLES = ('groups', 'channels', 'channel_groups', 'channel_model_pricing',
          'channel_pricing_intervals', 'channel_account_stats_model_pricing',
          'channel_account_stats_pricing_intervals', 'channel_account_stats_pricing_rules',
          'user_group_rate_multipliers', 'schema_migrations')


def sql(content):
    result = subprocess.run(['docker', 'exec', '-i', PG, 'psql', '-X', '-qAt',
                             '-v', 'ON_ERROR_STOP=1', '-U', 'sub2api', '-d', 'sub2api'],
                            input=content, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError('Read-only database verification failed: ' + result.stderr[-1000:])
    return result.stdout.strip()


def snapshot():
    statements = ["BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY; SET LOCAL statement_timeout='10s';"]
    for table in TABLES:
        statements.append("SELECT json_build_object('name','" + table + "','count',count(*),'hash',"
                          "md5(COALESCE(string_agg((to_jsonb(t)-'updated_at')::text, E'\\n' "
                          "ORDER BY (to_jsonb(t)-'updated_at')::text),''))) FROM " + table + " t;")
    statements.append("SELECT json_build_object('name','settings:'||key,'hash',md5(value)) FROM settings ORDER BY key;")
    statements.append("SELECT json_build_object('name','rollup_state','count',count(*)) FROM usage_group_rollup_state;")
    statements.append('COMMIT;')
    return {row['name']: row for row in map(json.loads, sql('\n'.join(statements)).splitlines())}


def record(name, data):
    (ROOT / (name + '.json')).write_text(json.dumps(data, indent=2) + '\n')


def baseline():
    ROOT.mkdir(parents=True, exist_ok=True)
    path = ROOT / 'config-before.json'
    assert not path.exists(), 'Never overwrite the configuration baseline'
    record('config-before', snapshot())
    print('Configuration baseline captured; values and credentials are not printed.')


def backup():
    os.umask(0o077)
    path = ROOT / 'database-before.dump'
    assert not path.exists(), 'Never overwrite a database backup'
    with path.open('xb') as stream:
        subprocess.run(['docker', 'exec', PG, 'pg_dump', '-U', 'sub2api', '-d', 'sub2api',
                        '--lock-wait-timeout=2s', '-Fc', '-Z', '1'], stdout=stream, check=True)
    with path.open('rb') as stream:
        subprocess.run(['docker', 'exec', '-i', PG, 'pg_restore', '--list'],
                       stdin=stream, stdout=subprocess.DEVNULL, check=True)
    record('database-backup', {'passed': True, 'path': str(path), 'bytes': path.stat().st_size})
    print('Online database backup completed and archive catalog verified.')


def compare():
    before = json.loads((ROOT / 'config-before.json').read_text())
    after = snapshot()
    changed = [name for name in sorted(before.keys() | after.keys()) if before.get(name) != after.get(name)]
    assert not changed, 'Configuration changed; investigate before proceeding: ' + ', '.join(changed)
    assert after['rollup_state']['count'] == 1
    record('config-verified', {'passed': True, 'unchanged_sections': len(before),
                              'pricing_groups_rates_settings_unchanged': True, 'schema_unchanged': True})
    print('Pricing, groups, multipliers, settings, migrations and rollup state verified unchanged.')


if __name__ == '__main__':
    {'baseline': baseline, 'backup': backup, 'compare': compare}[sys.argv[1]]()
