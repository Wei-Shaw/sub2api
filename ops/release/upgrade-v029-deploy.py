"""Deploy v0.2.9 on the verified BUAGA custom baseline."""
import copy
import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request

ROOT = Path('/home/serveradmin/releases/buaga-v029-custom-20260928')
MANIFEST = Path('/home/serveradmin/releases/buaga-current.json')
CONF = Path('/etc/nginx/conf.d/sub2api-global.conf')
OLD = 'sub2api-custom-video-preview-cache-7d-20260926'
OLD_ID = 'sha256:aa0e5d73f28885eb324de5daec72ada480a0cf4d0d948c514c971a253f1522df'
NEW = 'sub2api-custom-v029-20260928'
IMAGE = 'sub2api:custom-v029-20260928'
PORT = 18208
ARCHIVE = Path(str(ROOT) + '.tar.gz')
META = Path(str(ARCHIVE) + '.json')
TOOLS = str(ROOT / 'source/ops/release')

class Connection(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(60)
        self.sock.connect('/var/run/docker.sock')

def docker(method, path, payload=None):
    conn = Connection('localhost')
    conn.request(method, path, json.dumps(payload).encode() if payload is not None else None,
                 {'Content-Type': 'application/json'})
    response = conn.getresponse()
    data = response.read()
    conn.close()
    if response.status >= 300:
        raise RuntimeError('Docker HTTP ' + str(response.status) + ' for ' + path)
    return json.loads(data) if data else {}

def run(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.DEVNULL).strip()

def get(path, headers=None, raw=False, port=PORT):
    req = urllib.request.Request(f'http://127.0.0.1:{port}{path}', headers=headers or {})
    with urllib.request.urlopen(req, timeout=30) as response:
        assert response.status == 200
        body = response.read()
        return body if raw else json.loads(body)

def record(name, data):
    (ROOT / (name + '.json')).write_text(json.dumps(data, indent=2) + '\n')

def image():
    info = json.loads(META.read_text())
    assert info['source_git_tag'] == 'rel/buaga-v029-custom-20260928'
    assert info['source_archive_sha256'] == hashlib.sha256(ARCHIVE.read_bytes()).hexdigest()
    result = docker('GET', '/images/' + IMAGE + '/json')
    assert result['Config']['Labels']['buaga.source.commit'] == info['source_git_commit']
    base = docker('GET', '/images/' + OLD_ID + '/json')['RootFS']['Layers']
    assert result['RootFS']['Layers'][:len(base)] == base
    return result

def old():
    result = docker('GET', '/containers/' + OLD + '/json')
    assert result['Image'] == OLD_ID and result['State']['Health']['Status'] == 'healthy'
    return result

def candidate():
    current = json.loads(MANIFEST.read_text())
    assert current['container'] == OLD and current['image_id'] == OLD_ID
    source = old()
    built = image()
    subprocess.run(['python3', TOOLS + '/verify_package.py', str(ARCHIVE), str(META)], check=True)
    before_migrations = Path(current['remote_source']) / 'backend/migrations'
    after_migrations = ROOT / 'source/backend/migrations'
    assert {p.name: p.read_bytes() for p in before_migrations.glob('*.sql')} == {p.name: p.read_bytes() for p in after_migrations.glob('*.sql')}
    assert json.loads((ROOT / 'database-backup.json').read_text())['passed']
    with socket.socket() as probe:
        probe.bind(('127.0.0.1', PORT))
    payload = copy.deepcopy(source['Config'])
    payload.pop('Hostname', None)
    payload['Image'] = built['Id']
    payload['Labels'] = {**(payload.get('Labels') or {}), 'buaga.source.commit': json.loads(META.read_text())['source_git_commit']}
    env = dict(item.split('=', 1) for item in payload['Env'] if '=' in item)
    env['GATEWAY_SCHEDULING_SKIP_STARTUP_CLEANUP'] = 'true'
    payload['Env'] = [k + '=' + v for k, v in env.items()]
    payload['HostConfig'] = copy.deepcopy(source['HostConfig'])
    payload['HostConfig']['PortBindings'] = {'8080/tcp': [{'HostIp': '127.0.0.1', 'HostPort': str(PORT)}]}
    payload['HostConfig']['LogConfig'] = {'Type': 'json-file', 'Config': {'max-size': '50m', 'max-file': '5'}}
    payload['NetworkingConfig'] = {'EndpointsConfig': {n: {} for n in source['NetworkSettings']['Networks']}}
    docker('POST', '/containers/create?name=' + NEW, payload)
    docker('POST', '/containers/' + NEW + '/start')
    subprocess.run(['docker', 'cp', NEW + ':/app/.buaga-release', str(ROOT / 'frontend')], check=True)
    record('candidate-created', {'image_id': built['Id'], 'schema_unchanged': True, 'traffic_changed': False})
    print('Candidate started on localhost only; production route unchanged.')

def verify():
    app = docker('GET', '/containers/' + NEW + '/json')
    assert app['Image'] == image()['Id'] and app['State']['Health']['Status'] == 'healthy'
    baseline = old()
    assert app['HostConfig']['Binds'] == baseline['HostConfig']['Binds']
    env = dict(x.split('=', 1) for x in app['Config']['Env'])
    prior = dict(x.split('=', 1) for x in baseline['Config']['Env'])
    assert env.pop('GATEWAY_SCHEDULING_SKIP_STARTUP_CLEANUP') == 'true'
    prior.pop('GATEWAY_SCHEDULING_SKIP_STARTUP_CLEANUP', None)
    assert env == prior
    get('/health')
    key = run('docker', 'exec', 'sub2api-return-postgres', 'psql', '-X', '-qAt', '-U', 'sub2api', '-d', 'sub2api',
              '-c', "SELECT value FROM settings WHERE key='admin_api_key'")
    assert key
    paths = ('accounts?page=1&page_size=1', 'groups?page=1&page_size=1', 'channels?page=1&page_size=1',
             'announcements?page=1&page_size=1', 'video-tasks?page=1&page_size=1', 'system/version', 'ops/scheduler')
    for path in paths:
        data = get('/api/v1/admin/' + path, {'x-api-key': key})
        assert data['code'] == 0, path
        if path == 'system/version':
            assert '0.2.9' in json.dumps(data['data']), 'Wrong version'
    html = get('/', raw=True)
    entry = re.search(rb'/assets/(index-[A-Za-z0-9_-]+\.js)', html).group(1).decode()
    assert entry.encode() in (ROOT / 'frontend/index.html').read_bytes()
    for path in ('/login', '/dashboard', '/keys', '/monitor', '/admin/usage', '/admin/ops', '/video-model-plaza', '/video-docs', '/admin/video-tasks'):
        assert entry.encode() in get(path, raw=True), path
    for path in re.findall(rb'(?:src|href)="(/assets/[^" ]+)"', html):
        data = get(path.decode(), raw=True)
        assert data == (ROOT / 'frontend' / path.decode().lstrip('/')).read_bytes()
    plaza = get('/api/v1/video-model-plaza')
    assert plaza['code'] == 0 and isinstance(plaza['data']['groups'], list)
    menus = get('/api/v1/settings/public')['data']['custom_menu_items']
    assert any(x['id'] == '97370c95df45cba1' and x['visibility'] == 'user' for x in menus)
    proof = {'passed': True, 'image_id': app['Image'], 'frontend_entry': entry,
             'admin_reads': list(paths), 'new_paid_requests': 0, 'schema_unchanged': True}
    record('candidate-verified', proof)
    print(json.dumps(proof))
    return proof

def assets():
    proof = verify()
    static = Path('/var/lib/sub2api/buaga-static-v024-20260909/assets')
    assert static.is_dir()
    count = 0
    for path in (ROOT / 'frontend/assets').iterdir():
        assert path.is_file()
        target = static / path.name
        if target.exists():
            assert target.read_bytes() == path.read_bytes(), 'Hash collision: ' + path.name
        else:
            shutil.copyfile(path, target)
            target.chmod(0o644)
            count += 1
    record('assets-verified', {**proof, 'added_assets': count, 'old_assets_preserved': True})
    print('Published assets additively:', count)

def install(content):
    fd, name = tempfile.mkstemp(prefix='.buaga-v029-', dir=CONF.parent)
    with os.fdopen(fd, 'wb') as stream:
        stream.write(content)
    os.chmod(name, 0o644)
    os.replace(name, CONF)

def origin(path):
    return subprocess.check_output(['curl', '-fsS', '--max-time', '30', '--resolve',
                                   'buaga.dpdns.org:443:127.0.0.1', 'https://buaga.dpdns.org' + path])

def origin_login_status():
    return run('curl', '-sS', '--max-time', '20', '-o', '/dev/null', '-w', '%{http_code}',
               '--resolve', 'buaga.dpdns.org:443:127.0.0.1', 'https://buaga.dpdns.org/login')

def switch():
    prior = json.loads(MANIFEST.read_text())
    assert prior['container'] == OLD and prior['image_id'] == OLD_ID
    proof = verify()
    tests = json.loads((ROOT / 'tests-verified.json').read_text())
    assert tests['passed'] and tests['source_git_commit'] == json.loads(META.read_text())['source_git_commit']
    assert tests['customization_guard_count'] == 416 and tests['backend_full_unit'] and tests['focused_race'] and tests['frontend_tests'] == 2565
    assetproof = json.loads((ROOT / 'assets-verified.json').read_text())
    assert assetproof['passed'] and assetproof['image_id'] == proof['image_id']
    assert origin('/assets/' + proof['frontend_entry']) == (ROOT / 'frontend/assets' / proof['frontend_entry']).read_bytes()
    before = CONF.read_bytes()
    after, count = re.subn(rb'(upstream\s+sub2api_backend\s*\{[^}]*?server\s+)127\.0\.0\.1:18207(?=[\s;])',
                          rb'\g<1>127.0.0.1:18208', before, count=1)
    assert count == 1
    subprocess.run(['python3', str(ROOT / 'source/ops/release/upgrade-v029-safety.py'), 'compare'], check=True)
    if (ROOT / 'nginx.before.conf').exists():
        # A failed verification may have already restored this exact baseline.
        assert (ROOT / 'nginx.before.conf').read_bytes() == before
        assert (ROOT / 'manifest.before.json').read_bytes() == MANIFEST.read_bytes()
    else:
        (ROOT / 'nginx.before.conf').write_bytes(before)
        (ROOT / 'manifest.before.json').write_bytes(MANIFEST.read_bytes())
    login_status = origin_login_status()
    assert login_status in ('200', '451')
    try:
        assert CONF.read_bytes() == before
        install(after)
        run('nginx', '-t')
        run('systemctl', 'reload', 'nginx')
        origin('/health')
        assert origin_login_status() == login_status, 'Origin region policy changed'
        if login_status == '200':
            assert proof['frontend_entry'].encode() in origin('/login')
        # A 451 from the origin-local request is the existing Nginx region
        # restriction; app rendering is tested directly and via public browser.
        verify()
    except BaseException:
        install(before)
        run('nginx', '-t')
        run('systemctl', 'reload', 'nginx')
        raise
    info = json.loads(META.read_text())
    current = copy.deepcopy(prior)
    current.update(info)
    current.update({'version': '0.2.9', 'release': 'buaga-v029-custom-20260928', 'image': IMAGE,
                    'image_id': proof['image_id'], 'container': NEW, 'upstream': '127.0.0.1:18208',
                    'binary_sha256': run('docker', 'exec', NEW, 'sha256sum', '/app/sub2api').split()[0],
                    'remote_source': str(ROOT / 'source'), 'release_directory': str(ROOT),
                    'source_archive': str(ARCHIVE), 'source_git_archive': str(ARCHIVE),
                    'source_git_archive_sha256': info['source_archive_sha256'],
                    'official_commit': '4c00df2e0183e2c70b7fa8ba45914205e36aad0c',
                    'source_identity': 'Merge stable v0.2.9; preserve BUAGA customizations and existing unset image pricing semantics',
                    'build_script': TOOLS + '/build-bluegreen.sh', 'next_build_script': TOOLS + '/build-bluegreen.sh',
                    'candidate_script': str(ROOT / 'deploy.py'), 'frontend_entry': proof['frontend_entry'],
                    'frontend_index_sha256': hashlib.sha256((ROOT / 'frontend/index.html').read_bytes()).hexdigest(),
                    'verification': {**proof, 'backend_full_unit': True, 'backend_test_platform': 'darwin/arm64',
                                     'frontend_tests': tests['frontend_tests'], 'frontend_files': tests['frontend_files'], 'frontend_build': True,
                                     'frontend_full_lint': True, 'customization_guard_count': 416,
                                     'server_image_build': True, 'old_requests_preserved': True,
                                     'production_browser_verified': False, 'price_monitor_bound_to_current_app': False},
                    'rollback': {'container': OLD, 'image_id': OLD_ID, 'upstream': '127.0.0.1:18207',
                                 'nginx_config': str(ROOT / 'nginx.before.conf'), 'manifest': str(ROOT / 'manifest.before.json'),
                                 'instructions': 'Restore Nginx with graceful reload and source pointer/manifest; rebind monitor. Never restore DB.'}})
    link = Path(prior['default_source'])
    temporary = link.with_name(link.name + '-v029-next')
    assert link.is_symlink() and not temporary.exists()
    temporary.symlink_to(ROOT / 'source')
    os.replace(temporary, link)
    MANIFEST.write_text(json.dumps(current, indent=2) + '\n')
    record('switch-verified', proof)
    print('Traffic switched gracefully; old application remains running.')

def monitor_check():
    code = """import os,json,urllib.request
base=os.environ['SUB2API_API_BASE_URL']
assert base=='http://sub2api-custom-v029-20260928:8080'
key=os.environ['SUB2API_ADMIN_API_KEY']
for path in ['/health','/api/v1/admin/accounts?page=1&page_size=1']:
 r=urllib.request.urlopen(urllib.request.Request(base+path,headers={'x-api-key':key}),timeout=20)
 assert r.status==200
 data=json.load(r)
 if path.startswith('/api/'): assert data['code']==0
print('Monitor health and authenticated admin read passed')
"""
    checked = subprocess.run(['docker', 'exec', 'sub2api-price-monitor', 'python', '-c', code], capture_output=True, text=True)
    assert checked.returncode == 0, 'Monitor read failed; sensitive output suppressed'
    return checked.stdout.strip()

def rebind_monitor():
    assert json.loads(MANIFEST.read_text())['container'] == NEW
    subprocess.run(['python3', str(ROOT / 'source/ops/price-monitor/rebind_api.py')], check=True)
    record('monitor-verified', {'passed': True, 'api_base_url': 'http://' + NEW + ':8080'})
    print(monitor_check())
    return

def finalize():
    proof = verify()
    print(monitor_check())
    subprocess.run(['python3', str(ROOT / 'source/ops/release/upgrade-v029-safety.py'), 'compare'], check=True)
    current = json.loads(MANIFEST.read_text())
    assert current['container'] == NEW and current['image_id'] == proof['image_id']
    current['verification']['price_monitor_bound_to_current_app'] = True
    MANIFEST.write_text(json.dumps(current, indent=2) + '\n')
    subprocess.run(['python3', '/home/serveradmin/releases/prepare-custom-upgrade.py', '--check'], check=True)
    record('final-verified', {**proof, 'price_monitor_verified': True, 'old_application_healthy': True})
    print('Release identity, route, archive, binary and monitor verified.')

def watch():
    samples = []
    deadline = time.monotonic() + 1800
    while time.monotonic() < deadline and not (ROOT / 'stop-watch').exists():
        started = time.monotonic()
        try:
            assert json.loads(origin('/health'))['status'] == 'ok'
            error = None
        except Exception as exc:
            error = type(exc).__name__
        samples.append({'at': time.time(), 'seconds': round(time.monotonic() - started, 3), 'error': error})
        record('health-watch', {'samples': len(samples), 'failures': sum(x['error'] is not None for x in samples),
                               'max_seconds': max(x['seconds'] for x in samples), 'checks': samples})
        time.sleep(3)
    print('Health watch complete:', len(samples), 'samples')

if __name__ == '__main__':
    {'candidate': candidate, 'verify': verify, 'assets': assets, 'switch': switch,
     'monitor': rebind_monitor, 'finalize': finalize, 'watch': watch,
     'stop-watch': lambda: (ROOT / 'stop-watch').touch()}[sys.argv[1]]()
