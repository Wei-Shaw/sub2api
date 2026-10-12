#!/usr/bin/env python3
"""Bind monitor to the verified live container without touching application networks."""
import copy
import json
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import urlsplit, urlunsplit

from deploy import docker, NAME

manifest = json.loads(Path('/home/serveradmin/releases/buaga-current.json').read_text())
release = manifest['release']
assert re.fullmatch(r'[a-z0-9-]+', release), 'Invalid release identity'
application = docker('GET', '/containers/' + manifest['container'] + '/json')
assert application['Image'] == manifest['image_id']
assert application['State']['Health']['Status'] == 'healthy'
live = docker('GET', '/containers/' + NAME + '/json')
env = dict(item.split('=', 1) for item in live['Config']['Env'] if '=' in item)
previous_url = env['SUB2API_API_BASE_URL']
parsed = urlsplit(previous_url)
assert parsed.scheme == 'http' and parsed.port == 8080 and not parsed.username
target = urlunsplit((parsed.scheme, manifest['container'] + ':8080', parsed.path, '', ''))
assert target != previous_url, 'Already bound to current production container'

def verify_api(base):
    code = '''import json,os,sys,urllib.request
base=sys.argv[1]
assert urllib.request.urlopen(base+"/health",timeout=10).status==200
req=urllib.request.Request(base+"/api/v1/admin/accounts?page=1&page_size=1",headers={"x-api-key":os.environ["SUB2API_ADMIN_API_KEY"]})
with urllib.request.urlopen(req,timeout=10) as response:
 assert response.status==200 and json.load(response)["code"]==0
print("Monitor health and authenticated admin read passed")
'''
    subprocess.run(['docker', 'exec', NAME, 'python', '-c', code, base], check=True)

verify_api(target)
env_path = Path('/opt/sub2api-price-monitor/.env')
before = env_path.read_text()
updated, count = re.subn(r'^SUB2API_API_BASE_URL=.*$', 'SUB2API_API_BASE_URL=' + target, before, flags=re.M)
assert count == 1, 'Cannot find unique persistent API base URL'
backup_env = env_path.with_name('.env.before-api-rebind-' + release)
assert not backup_env.exists()
backup = NAME + '-before-api-rebind-' + release
payload = copy.deepcopy(live['Config'])
payload.pop('Hostname', None)
payload['Image'] = live['Image']
payload['Env'] = [item if not item.startswith('SUB2API_API_BASE_URL=') else 'SUB2API_API_BASE_URL=' + target for item in payload['Env']]
payload['HostConfig'] = copy.deepcopy(live['HostConfig'])
payload['NetworkingConfig'] = {'EndpointsConfig': {
    network: {'Aliases': [alias for alias in (endpoint.get('Aliases') or []) if alias not in (live['Id'], live['Id'][:12])]}
    for network, endpoint in live['NetworkSettings']['Networks'].items()
}}
if '--check' in sys.argv:
    print('Read-only API validation passed; no container or environment changes')
    raise SystemExit(0)
backup_env.write_text(before)
backup_env.chmod(0o600)
subprocess.run(['docker', 'stop', '--timeout=-1', NAME], check=True)
docker('POST', '/containers/' + NAME + '/rename?name=' + backup)
docker('POST', '/containers/' + backup + '/update', {'RestartPolicy': {'Name': 'no'}})
try:
    docker('POST', '/containers/create?name=' + NAME, payload)
    docker('POST', '/containers/' + NAME + '/start')
    current = docker('GET', '/containers/' + NAME + '/json')
    assert current['State']['Running'] and current['Image'] == live['Image']
    assert current['HostConfig']['Binds'] == live['HostConfig']['Binds']
    verify_api(target)
    env_path.write_text(updated)
    env_path.chmod(0o600)
    result = subprocess.run(['docker', 'compose', '-f', '/opt/sub2api-price-monitor/docker-compose.yml', 'config', '--format', 'json'], capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('Compose validation failed; sensitive details suppressed')
    services = json.loads(result.stdout)['services']
    monitor = next(service for service in services.values() if service.get('container_name') == NAME)
    assert monitor['environment']['SUB2API_API_BASE_URL'] == target
except Exception:
    env_path.write_text(before)
    try:
        docker('POST', '/containers/' + NAME + '/stop?t=60')
        docker('POST', '/containers/' + NAME + '/rename?name=' + NAME + '-failed-api-rebind-' + release)
    except RuntimeError:
        pass
    docker('POST', '/containers/' + backup + '/rename?name=' + NAME)
    docker('POST', '/containers/' + NAME + '/update', {'RestartPolicy': live['HostConfig']['RestartPolicy']})
    docker('POST', '/containers/' + NAME + '/start')
    raise
record_path = Path('/opt/sub2api-price-monitor/CURRENT_RELEASE.json')
record = json.loads(record_path.read_text())
record.update({'api_base_url': target, 'api_binding': 'verified current application container',
               'api_binding_rollback_container': backup, 'admin_read_verified': True})
record_path.write_text(json.dumps(record, indent=2) + '\n')
print('Monitor API binding and persistent startup updated; application network unchanged')
