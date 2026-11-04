#!/usr/bin/env python3
"""Same-database upgrade/rollback with ephemeral credentials kept in memory."""
import hashlib
import http.client
import http.cookiejar
import json
import os
from pathlib import Path
import secrets
import sys
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent
COMPOSE = ['docker', 'compose', '-f', str(ROOT / 'compose.yaml'), '-p', 'sub2api-sync-20261104']
BASELINE = 'sub2api:sync-baseline-20261104'
CANDIDATE = 'sub2api:upstream-sync-20261104'
ENV = {**os.environ, 'SYNC_DB_PASSWORD': secrets.token_urlsafe(24), 'SYNC_ADMIN_EMAIL': 'admin@fixture.invalid',
       'SYNC_ADMIN_PASSWORD': secrets.token_urlsafe(24), 'SYNC_JWT_SECRET': secrets.token_urlsafe(48),
       'SYNC_ENCRYPTION_KEY': secrets.token_hex(32), 'SYNC_IMAGE': BASELINE}
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))


def compose(*args):
    result = subprocess.run(COMPOSE + list(args), env=ENV, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('compose ' + str(args) + ': ' + result.stderr)
    return result.stdout.strip()


def sql(statement):
    return compose('exec', '-T', 'postgres', 'psql', '-U', 'sub2api', '-d', 'upgrade', '-v', 'ON_ERROR_STOP=1', '-qAtc', statement)


def request(path, body=None, token=None, method=None):
    headers = {'Content-Type': 'application/json'}
    if token:
        headers['Authorization'] = 'Bearer ' + token
    req = urllib.request.Request('http://127.0.0.1:18444' + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
    try:
        response = OPENER.open(req, timeout=20)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        try:
            body = json.loads(raw)
        except ValueError:
            body = raw
        return response.code, body


def start(image, installed=False):
    ENV['SYNC_IMAGE'] = image
    email, password = ENV['SYNC_ADMIN_EMAIL'], ENV['SYNC_ADMIN_PASSWORD']
    if installed:
        ENV['SYNC_ADMIN_EMAIL'], ENV['SYNC_ADMIN_PASSWORD'] = 'invalid-install-email', 'short'
    compose('rm', '-sf', 'provider', 'app')
    compose('up', '-d', 'app')
    compose('up', '-d', '--no-deps', 'provider')
    ENV['SYNC_ADMIN_EMAIL'], ENV['SYNC_ADMIN_PASSWORD'] = email, password
    for _ in range(100):
        try:
            if request('/ready')[0] == 200:
                # Provider startup is independent of application readiness.
                time.sleep(.5)
                return
        except OSError:
            pass
        time.sleep(.5)
    raise AssertionError('app failed readiness')


def login():
    status, key = request('/api/v1/auth/credential-key')
    assert status == 200, ('credential key', status)
    envelope = json.loads(subprocess.check_output(['docker', 'run', '--rm', '-i', '-v', str(ROOT) + ':/fixture:ro',
        'node:24-alpine', 'node', '/fixture/credential_envelope_fixture.mjs'],
        input=json.dumps({'server': key['data'], 'email': ENV['SYNC_ADMIN_EMAIL'], 'password': ENV['SYNC_ADMIN_PASSWORD']}).encode()))
    status, result = request('/api/v1/auth/login', {'credential_envelope': envelope})
    assert status == 200, ('encrypted login', status)
    return result['data']['access_token'], result['data']['user']['id']


def snapshot(uid, kid):
    return sql(f"""SELECT json_build_object('user',(SELECT jsonb_build_array(id,email,password_hash) FROM users WHERE id={uid}),
       'key',(SELECT jsonb_build_array(id,name,quota) FROM api_keys WHERE id={kid}),
       'subscriptions',(SELECT jsonb_agg(jsonb_build_array(user_id,group_id,expires_at,status)) FROM user_subscriptions))""")


def create_key(token, group_id, name):
    status, key = request('/api/v1/keys', {'name': name, 'group_id': group_id, 'quota': 12.34}, token)
    assert status == 200, ('create key', status)
    return key['data']['id'], key['data']['key']


def settled(kid):
    for _ in range(50):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'actual',actual_cost)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.15)
    raise AssertionError('usage not persisted')

def admin_key_request(path, key, body=None):
    req=urllib.request.Request('http://127.0.0.1:18444'+path,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json','x-api-key':key})
    try:response=OPENER.open(req,timeout=20)
    except urllib.error.HTTPError as error:response=error
    with response:return response.code

def new_admin_key(token,name):
    code,value=request('/api/v1/admin/settings/admin-api-keys',{'name':name,'scopes':['admin.read','admin.write']},token)
    assert code==201,('create scoped key',code)
    return value['data']['key']

def usage_row(kid):
    for _ in range(60):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'count',image_count,'total',total_cost,'actual',actual_cost,'mode',billing_mode)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.1)
    raise AssertionError('usage not persisted')

def provider(path,body=None):
    script='import urllib.request,json; r=urllib.request.Request("http://127.0.0.1:8000"+'+repr(path)+',data='+('None' if body is None else repr(json.dumps(body).encode()))+',headers={"Content-Type":"application/json"});print(urllib.request.urlopen(r).read().decode())'
    return json.loads(compose('exec','-T','provider','python','-c',script))

def usage(kid):
    for _ in range(80):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'cost',total_cost,'actual',actual_cost,'response_model',upstream_response_model)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.1)
    raise AssertionError('missing reliable settlement')

def detailed_usage(kid):
    for _ in range(100):
        rows=json.loads(sql(f"SELECT COALESCE(json_agg(json_build_object('input',input_tokens,'output',output_tokens,'cache',cache_read_tokens,'images',image_count,'cost',total_cost,'actual',actual_cost,'mode',billing_mode)),'[]'::json) FROM usage_logs WHERE api_key_id={kid}"))
        if rows:return rows
        time.sleep(.1)
    raise AssertionError('missing reliable settlement')


def create_fixture(name, platform, model, pricing):
    card=json.dumps(pricing)
    gid=int(sql(f"INSERT INTO groups(name,platform,status,rate_multiplier,model_pricing,allow_messages_dispatch) VALUES ('{name}','{platform}','active',1,'{card}',true) RETURNING id"))
    credentials=json.dumps({'api_key':'fixture','base_url':'http://127.0.0.1:8000','api_protocol':'chat_completions','account_mode':'payg'})
    extra=json.dumps({'openai_responses_supported':False})
    aid=int(sql(f"INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,egress_mode) VALUES ('{name}','{platform}','apikey','{credentials}','{extra}','active',true,3,'direct') RETURNING id"))
    sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({aid},{gid})')
    if platform=='openai':sql(f'UPDATE groups SET allow_image_generation=true WHERE id={gid}')
    return gid,aid



def update_extra(aid, value):
    sql(f"UPDATE accounts SET extra='{json.dumps(value)}'::jsonb WHERE id={aid}")


def check_stage(stage, token, uid, fixed):
    cases = []
    gid, low = create_fixture('images-' + stage, 'openai', 'gpt-image-1', [{'platform':'openai','models':['gpt-image-1'],'billing_mode':'image','per_request_price':.1}])
    low_extra = {'openai_responses_supported': True, 'openai_image_size_tiers': ['1K']}
    update_extra(low, low_extra)
    sql(f"UPDATE accounts SET credentials=jsonb_set(credentials,'{{api_key}}','\"fixture-low\"'),priority=0 WHERE id={low}")
    high = int(sql(f"INSERT INTO accounts(name,platform,type,credentials,extra,status,schedulable,concurrency,priority,egress_mode) SELECT 'high-{stage}',platform,type,jsonb_set(credentials,'{{api_key}}','\"fixture-high\"'),'{{\"openai_responses_supported\":true,\"openai_image_size_tiers\":[\"4K\"]}}',status,schedulable,concurrency,9,egress_mode FROM accounts WHERE id={low} RETURNING id"))
    sql(f'INSERT INTO account_groups(account_id,group_id) VALUES ({high},{gid})')
    kid, key = create_key(token, gid, stage + '-images')
    code, body = request('/v1/images/generations', {'model':'gpt-image-1','prompt':'fixture','size':'3840x2160','n':1}, key)
    assert code == 200, (stage, 'image generation', code, body)
    sent = provider('/fixture/last')
    assert sent['label'] == ('fixture-high' if fixed else 'fixture-low'), (stage, 'tier routing', sent['label'])
    rows = usage_row(kid)
    assert len(rows) == 1, (stage, 'image duplicate settlement', rows)
    cases.append({'case':'image-4k', 'selected_fixture':sent['label'], 'settlement':rows})
    for stream in (False, True):
        kid, key = create_key(token, gid, stage + '-responses-' + str(stream))
        code, body = request('/v1/responses', {'model':'gpt-5.4','stream':stream,'instructions':'client guidance','input':'hello'}, key)
        assert code == 200, (stage, 'responses', stream, code, str(body)[:200])
        if stream:
            events = [json.loads(line[6:]) for line in body.splitlines() if line.startswith('data: {')]
            completed = [event['response'] for event in events if event.get('type') == 'response.completed']
            assert completed and completed[-1]['instructions'] == 'client guidance'
        else:
            assert body['instructions'] == 'client guidance', (stage, 'instructions', body)
        rows = usage_row(kid)
        assert len(rows) == 1 and rows[0]['input'] == 1000 and rows[0]['output'] == 100, (stage, 'responses settlement', rows)
        cases.append({'case':'responses', 'stream':stream, 'settlement':rows})
    grok_gid, grok_aid = create_fixture('grok-' + stage, 'grok', 'grok-4.5', [])
    update_extra(grok_aid, {'openai_responses_supported':True})
    kid, key = create_key(token, grok_gid, stage + '-grok')
    code, body = request('/v1/responses', {'model':'grok-4.5','stream':False,'input':[{'type':'agent_message','content':[{'type':'input_text','text':'Task:'},{'type':'encrypted_content','encrypted_content':'plaintext body'}]}]}, key)
    assert code == 200, (stage, 'grok', code, str(body)[:200])
    sent = provider('/fixture/last')['body']['input']
    if fixed:
        assert sent[0]['role'] == 'user' and sent[0]['content'] == 'Task:plaintext body', (stage, 'grok delegation', sent)
    else:
        assert sent[0]['type'] == 'agent_message', (stage, 'baseline grok', sent)
    assert len(usage_row(kid)) == 1
    cases.append({'case':'grok-delegation', 'normalized':fixed})
    kimi_gid, kimi_aid = create_fixture('kimi-' + stage, 'kimi', 'kimi-k2.5', [])
    kid, key = create_key(token, kimi_gid, stage + '-kimi')
    code, _ = request('/v1/chat/completions', {'model':'kimi-k2.5','stream':False,'messages':[{'role':'user','content':'hello'}]}, key)
    state = json.loads(sql(f"SELECT json_build_object('status',status,'rate_limited',rate_limit_reset_at > NOW(),'temporary',temp_unschedulable_until > NOW()) FROM accounts WHERE id={kimi_aid}"))
    assert state['status'] == ('active' if fixed else 'error'), (stage, 'kimi state', code, state)
    if fixed: assert state['rate_limited'] is True, (stage, 'short cooldown', state)
    cases.append({'case':'kimi-concurrency-403', 'http':code, 'state':state})
    return cases


report = []
try:
    compose('up', '-d', 'postgres', 'redis')
    start(BASELINE)
    token, uid = login()
    ack = json.dumps({'version':'v2026.06.10','admin_user_id':uid,'accepted_at':'2026-11-04T00:00:00Z'})
    sql(f"INSERT INTO settings(key,value) VALUES ('admin_compliance_acknowledgement:{uid}','{ack}')")
    sql(f'UPDATE users SET balance=1000 WHERE id={uid}')
    gid, _ = create_fixture('persisted', 'openai', 'gpt-5.4', [])
    kid, _ = create_key(token, gid, 'persisted-key')
    sql(f"INSERT INTO user_subscriptions(user_id,group_id,starts_at,expires_at,status) VALUES ({uid},{gid},NOW(),NOW()+INTERVAL '1 day','active')")
    pristine = snapshot(uid, kid)
    migrations = int(sql('SELECT COUNT(*) FROM schema_migrations'))
    historical = None
    for stage, image, fixed in [('BASELINE',BASELINE,False),('MODIFIED',CANDIDATE,True),('ROLLBACK',BASELINE,False),('RESTORED',CANDIDATE,True)]:
        if stage != 'BASELINE':
            # Old nodes ignore tier configuration: remove restrictions before rollback.
            if not fixed: sql("UPDATE accounts SET extra=extra-'openai_image_size_tiers'")
            start(image, True)
        token, uid = login()
        if historical:
            upper, digest = historical
            assert sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}") == digest
        cases = check_stage(stage, token, uid, fixed)
        assert snapshot(uid, kid) == pristine, (stage, 'persistent identity/key/subscription')
        assert int(sql('SELECT COUNT(*) FROM schema_migrations')) == migrations, (stage, 'migration drift')
        assert sql(f'SELECT ABS(balance+(SELECT COALESCE(SUM(actual_cost),0) FROM usage_logs WHERE user_id={uid})-1000)<0.00000001 FROM users WHERE id={uid}') == 't', (stage, 'balance equation')
        upper = int(sql('SELECT COALESCE(MAX(id),0) FROM usage_logs'))
        historical = (upper, sql(f"SELECT md5(COALESCE(string_agg(row_to_json(u)::text,',' ORDER BY id),'')) FROM usage_logs u WHERE id<={upper}"))
        item = {'stage':stage, 'result':'PASS', 'cases':cases, 'migrations':migrations, 'persistent_identity_key_subscription':'PASS', 'historical_usage_unchanged':'PASS', 'balance_settlement_identity':'PASS'}
        report.append(item)
        print(json.dumps(item), flush=True)
    (ROOT / 'result.json').write_text(json.dumps(report, indent=2) + '\n')
finally:
    compose('down', '--volumes', '--remove-orphans')
