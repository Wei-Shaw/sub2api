import os
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any

import psycopg
from fastapi import Depends, FastAPI, Header, HTTPException, Query
from psycopg.rows import dict_row

app = FastAPI(title='Sub2API Key Status API', version='1.0.0')

API_TOKEN = os.environ.get('KEY_STATUS_API_TOKEN', '')
ACCOUNT_EMAIL = os.environ.get('SUB2API_ACCOUNT_EMAIL', '')


def db_connect():
    return psycopg.connect(
        host=os.environ['DATABASE_HOST'],
        port=int(os.environ.get('DATABASE_PORT', '5432')),
        dbname=os.environ['DATABASE_NAME'],
        user=os.environ['DATABASE_USER'],
        password=os.environ['DATABASE_PASSWORD'],
        row_factory=dict_row,
    )


def require_auth(authorization: str | None = Header(default=None), x_api_key: str | None = Header(default=None)):
    if not API_TOKEN:
        raise HTTPException(status_code=500, detail='KEY_STATUS_API_TOKEN is not configured')
    bearer = ''
    if authorization and authorization.lower().startswith('bearer '):
        bearer = authorization[7:].strip()
    supplied = bearer or x_api_key or ''
    if supplied != API_TOKEN:
        raise HTTPException(status_code=401, detail='unauthorized')


def money(value: Any) -> str:
    if value is None:
        return '0'
    if isinstance(value, Decimal):
        return format(value, 'f')
    return str(value)


def iso(value: Any) -> str | None:
    if value is None:
        return None
    return value.isoformat()


def key_state(row: dict[str, Any]) -> dict[str, bool]:
    now = datetime.now(timezone.utc)
    expires_at = row.get('expires_at')
    deleted = row.get('deleted_at') is not None
    expired = expires_at is not None and expires_at <= now
    status_active = row.get('status') == 'active'
    quota = row.get('quota') or Decimal('0')
    quota_used = row.get('quota_used') or Decimal('0')
    quota_exhausted = quota > 0 and quota_used >= quota
    usable = status_active and not deleted and not expired and not quota_exhausted
    return {
        'status_active': status_active,
        'expired': expired,
        'deleted': deleted,
        'quota_exhausted': quota_exhausted,
        'usable': usable,
    }


def serialize(row: dict[str, Any]) -> dict[str, Any]:
    quota = row.get('quota') or Decimal('0')
    quota_used = row.get('quota_used') or Decimal('0')
    remaining = quota - quota_used
    if remaining < 0:
        remaining = Decimal('0')
    state = key_state(row)
    return {
        'name': row['name'],
        'group': {
            'name': row.get('group_name'),
            'platform': row.get('group_platform'),
            'status': row.get('group_status'),
        },
        'status': row['status'],
        'state': state,
        'active': state['status_active'],
        'usable': state['usable'],
        'expires_at': iso(row.get('expires_at')),
        'last_used_at': iso(row.get('last_used_at')),
        'charged_usd': money(quota),
        'quota_usd': money(quota),
        'consumed_usd': money(quota_used),
        'remaining_usd': money(remaining),
    }


BASE_QUERY = '''
select
    ak.id, ak.user_id, ak.key, ak.name, ak.group_id, ak.status,
    ak.created_at, ak.updated_at, ak.deleted_at, ak.quota, ak.quota_used,
    ak.expires_at, ak.last_used_at, ak.rate_limit_5h, ak.rate_limit_1d,
    ak.rate_limit_7d, ak.usage_5h, ak.usage_1d, ak.usage_7d,
    u.email as user_email, u.username, u.status as user_status,
    g.name as group_name, g.platform as group_platform, g.status as group_status,
    coalesce(usage_totals.total_cost, 0) as usage_total_cost,
    coalesce(usage_totals.actual_cost, 0) as usage_actual_cost,
    coalesce(billing_totals.delta_usd, 0) as billing_delta_usd
from api_keys ak
join users u on u.id = ak.user_id
left join groups g on g.id = ak.group_id
left join lateral (
    select sum(total_cost) as total_cost, sum(actual_cost) as actual_cost
    from usage_logs ul
    where ul.api_key_id = ak.id
) usage_totals on true
left join lateral (
    select sum(delta_usd) as delta_usd
    from billing_usage_entries bue
    where bue.api_key_id = ak.id and bue.applied = true
) billing_totals on true
where (%(account_email)s = '' or u.email = %(account_email)s)
'''


@app.get('/health')
def health():
    with db_connect() as conn:
        conn.execute('select 1')
    return {'ok': True}


@app.get('/api-keys/status', dependencies=[Depends(require_auth)])
def api_key_status(key: str = Query(..., min_length=1)):
    sql = BASE_QUERY + ' and ak.key = %(key)s and ak.deleted_at is null'
    with db_connect() as conn:
        row = conn.execute(sql, {'account_email': ACCOUNT_EMAIL, 'key': key}).fetchone()
    if not row:
        raise HTTPException(status_code=404, detail='api key not found')
    return serialize(row)
