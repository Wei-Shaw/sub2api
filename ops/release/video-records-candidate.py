"""Prepare/verify the video-records candidate. Never changes Nginx or traffic."""
import copy
import hashlib
import http.client
import json
from pathlib import Path
import socket
import subprocess
import sys
import urllib.request

ROOT = Path('/home/serveradmin/releases/buaga-video-records-20260924')
OLD = 'sub2api-custom-v028-20260923'
OLD_IMAGE = 'sha256:e67ffc9418562fbcec2292aeeae3de2596a4dd0099a85250923cfa7d5215ed73'
NEW = 'sub2api-custom-video-records-20260924'
IMAGE = 'sub2api:custom-video-records-20260924'
PORT = 18196

class Connection(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(45)
        self.sock.connect('/var/run/docker.sock')

def docker(method, path, payload=None):
    conn = Connection('localhost')
    conn.request(method, path, json.dumps(payload).encode() if payload is not None else None,
                 {'Content-Type': 'application/json'})
    res = conn.getresponse()
    body = res.read()
    conn.close()
    if res.status >= 300:
        raise RuntimeError(f'Docker HTTP {res.status} for {path}')
    return json.loads(body) if body else {}

def baseline():
    manifest = json.loads(Path('/home/serveradmin/releases/buaga-current.json').read_text())
    assert manifest['container'] == OLD and manifest['image_id'] == OLD_IMAGE
    old = docker('GET', '/containers/' + OLD + '/json')
    assert old['Image'] == OLD_IMAGE and old['State']['Health']['Status'] == 'healthy'
    return old

def checked_image():
    archive = Path(str(ROOT) + '.tar.gz')
    meta = json.loads(Path(str(archive) + '.json').read_text())
    assert meta['source_git_tag'] == 'rel/buaga-video-records-20260924'
    assert meta['source_archive_sha256'] == hashlib.sha256(archive.read_bytes()).hexdigest()
    image = docker('GET', '/images/' + IMAGE + '/json')
    assert image['Config']['Labels']['buaga.source.commit'] == meta['source_git_commit']
    layers = docker('GET', '/images/' + OLD_IMAGE + '/json')['RootFS']['Layers']
    assert image['RootFS']['Layers'][:len(layers)] == layers
    return image

def candidate():
    source = baseline()
    image = checked_image()
    with socket.socket() as probe:
        probe.bind(('127.0.0.1', PORT))
    payload = copy.deepcopy(source['Config'])
    payload.pop('Hostname', None)
    payload['Image'] = image['Id']
    env = dict(x.split('=', 1) for x in payload['Env'])
    env['GATEWAY_SCHEDULING_SKIP_STARTUP_CLEANUP'] = 'true'
    payload['Env'] = [k + '=' + v for k, v in env.items()]
    payload['HostConfig'] = copy.deepcopy(source['HostConfig'])
    payload['HostConfig']['PortBindings'] = {'8080/tcp': [{'HostIp': '127.0.0.1', 'HostPort': str(PORT)}]}
    payload['HostConfig']['LogConfig'] = {'Type': 'json-file', 'Config': {'max-size': '50m', 'max-file': '3'}}
    payload['NetworkingConfig'] = {'EndpointsConfig': {n: {} for n in source['NetworkSettings']['Networks']}}
    docker('POST', '/containers/create?name=' + NEW, payload)
    docker('POST', '/containers/' + NEW + '/start')
    print('Candidate started on loopback; production traffic unchanged.')

def verify():
    baseline()
    image = checked_image()
    app = docker('GET', '/containers/' + NEW + '/json')
    assert app['Image'] == image['Id'] and app['State']['Health']['Status'] == 'healthy'
    admin_key = subprocess.check_output(['docker', 'exec', 'sub2api-return-postgres', 'psql', '-U', 'sub2api',
        '-d', 'sub2api', '-At', '-c', "SELECT value FROM settings WHERE key='admin_api_key'"], text=True).strip()
    assert admin_key
    paths = ['/health', '/api/v1/admin/video-tasks?page=1&page_size=5',
             '/api/v1/admin/video-tasks?page=1&page_size=5&status=submission_unknown',
             '/api/v1/admin/groups?page=1&page_size=1', '/api/v1/admin/system/version']
    for path in paths:
        req = urllib.request.Request(f'http://127.0.0.1:{PORT}' + path, headers={'x-api-key': admin_key})
        with urllib.request.urlopen(req, timeout=20) as resp:
            assert resp.status == 200
            body = json.load(resp)
            if path.startswith('/api/'):
                assert body['code'] == 0
    proof = {'passed': True, 'candidate': NEW, 'image_id': image['Id'], 'port': PORT,
             'checks': paths, 'production_unchanged': True, 'new_paid_requests': 0}
    ROOT.mkdir(exist_ok=True)
    (ROOT / 'candidate-verified.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))

if __name__ == '__main__':
    {'candidate': candidate, 'verify': verify}[sys.argv[1]]()
