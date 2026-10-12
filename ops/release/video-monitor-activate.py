"""Pin four verified legacy monitor routes. Never writes balances or usage."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path('/home/serveradmin/releases/buaga-video-monitor-r2-20260924')
TASKS = ['c2c0dca2-a457-952e-b9b9-b8cf7379550c', '73109262-c525-9863-a57d-c3c5b458f1cd',
         '60cddfb4-8ee5-9f56-98bc-aa27e51aa610', 'bf9d2295-b274-95f3-af38-030233647b3d']
IDS = ','.join("'" + x + "'" for x in TASKS)

def sql(statement):
    result = subprocess.run(['docker', 'exec', '-i', 'sub2api-return-postgres', 'sh', '-lc',
        'psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],
        input=statement, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError('Database check failed; sensitive output suppressed')
    return result.stdout.strip()

def fingerprint(value):
    return hashlib.sha256(json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode()).hexdigest()

def main():
    # These exact four tasks were independently queried against safeapi.vip on
    # Sep 24 and returned done+video.url. Older fingerprints omitted Grok URLs;
    # do not mass-rebind other historical tasks to their current account route.
    row = json.loads(sql("BEGIN READ ONLY; SELECT json_build_object('platform',platform,'type',type,"
        "'base',credentials->>'base_url','protocol',COALESCE(extra->>'openai_video_protocol',''),"
        "'parent',parent_account_id) FROM accounts WHERE id=555; COMMIT;"))
    assert row['platform'] == 'grok' and row['type'] == 'apikey'
    assert row['base'].rstrip('/') == 'https://safeapi.vip'
    old = fingerprint([row['platform'], row['type'], '', row['protocol'], row['parent']])
    new = fingerprint([row['platform'], row['type'], row['base'].strip(), row['protocol'], row['parent']])
    clause = f"task_id IN ({IDS}) AND user_id=267 AND account_id=555 AND pending->>'tracking_only'='true'"
    before = json.loads(sql(f"BEGIN READ ONLY; SELECT json_agg(json_build_object('task_id',task_id,"
        f"'state',state,'fingerprint',routing_fingerprint)) FROM video_tasks WHERE {clause}; COMMIT;"))
    assert len(before) == 4 and all(x['fingerprint'] in (old,new) for x in before)
    if sys.argv[1] == 'check':
        print(json.dumps({'verified_legacy_routes':len(before),'writes':0}))
        return
    assert sys.argv[1] == 'apply'
    ROOT.mkdir(exist_ok=True)
    backup = ROOT / 'legacy-monitor-routes.before.json'
    if not backup.exists():
        backup.write_text(json.dumps(before,indent=2)+'\n')
    # No state, amount or usage changes. Requeue only route-mismatch observations
    # the new monitoring worker may have made while the candidate was starting.
    changed = sql(f"BEGIN; SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='5s'; "
        f"UPDATE video_tasks SET routing_fingerprint='{new}',"
        "last_error=CASE WHEN last_error='account_route_changed' THEN '' ELSE last_error END,"
        "updated_at=CASE WHEN last_error='account_route_changed' THEN NOW()-INTERVAL '1 minute' ELSE updated_at END "
        f"WHERE {clause} AND routing_fingerprint='{old}' RETURNING task_id; COMMIT;")
    print(json.dumps({'pinned_legacy_routes':len(changed.splitlines()) if changed else 0,
                      'usage_writes':0,'balance_writes':0}))

if __name__ == '__main__':
    main()
