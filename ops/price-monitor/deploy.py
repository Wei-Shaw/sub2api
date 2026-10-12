#!/usr/bin/env python3
"""Single-instance monitor handover. Never restart or route the main application."""
import argparse
import copy
import hashlib
import http.client
import json
from pathlib import Path
import socket
import subprocess

NAME = 'sub2api-price-monitor'
BACKUP = NAME + '-before-explicit-suffix-20260917'
IMAGE = 'sub2api-price-monitor:explicit-suffix-20260917-r2'
LIVE_SOURCE_SHA = '9ec806a0d9e1d281fb67c29c61fcd3e9f5644f56f4bb3bad55d0a6011a5b9ec4'


class Connection(http.client.HTTPConnection):
    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(45)
        self.sock.connect('/var/run/docker.sock')


def docker(method, path, payload=None):
    connection = Connection('localhost')
    connection.request(method, path, json.dumps(payload).encode() if payload is not None else None, {'Content-Type': 'application/json'})
    response = connection.getresponse()
    raw = response.read()
    connection.close()
    if response.status >= 300:
        raise RuntimeError(f'Docker operation failed: HTTP {response.status} ({method} {path})')
    return json.loads(raw) if raw else {}


def output(*args):
    return subprocess.check_output(args, text=True, stderr=subprocess.DEVNULL).strip()


def prepare(root):
    info = json.loads((root / 'source.tar.gz.json').read_text())
    # Source extraction and its digest must have been verified by installed release tools.
    subprocess.run(['python3', '/home/serveradmin/releases/git-tools/verify_package.py', str(root / 'source.tar.gz'), str(root / 'source.tar.gz.json'), '--extract', str(root / 'verified-source')], check=True)
    live = docker('GET', '/containers/' + NAME + '/json')
    assert live['State']['Running']
    sha = output('docker', 'exec', NAME, 'sha256sum', '/app/monitor.py').split()[0]
    assert sha == LIVE_SOURCE_SHA, 'Live monitor changed; rebase before deploying'
    source = root / 'verified-source/ops/price-monitor'
    assert hashlib.sha256((source / 'monitor.py').read_bytes()).hexdigest() != sha
    base_tag = 'sub2api-price-monitor:verified-base-' + live['Image'].split(':')[1][:16]
    subprocess.run(['docker', 'tag', live['Image'], base_tag], check=True)
    assert docker('GET', '/images/' + base_tag + '/json')['Id'] == live['Image']
    subprocess.run(['docker', 'build', '--pull=false', '--network=none', '--build-arg', 'MONITOR_BASE=' + base_tag,
                    '--label', 'buaga.source.commit=' + info['source_git_commit'],
                    '-f', str(source / 'Dockerfile'), '-t', IMAGE, str(source)], check=True)
    assert docker('GET', '/images/' + base_tag + '/json')['Id'] == live['Image']
    base_layers = docker('GET', '/images/' + live['Image'] + '/json')['RootFS']['Layers']
    assert docker('GET', '/images/' + IMAGE + '/json')['RootFS']['Layers'][:len(base_layers)] == base_layers
    subprocess.run(['docker', 'run', '--rm', '--network=none', '--entrypoint', 'python',
                    '-v', str(source) + ':/tests:ro', '-w', '/tests', IMAGE, '-m', 'unittest', '-q'], check=True)
    subprocess.run(['docker', 'run', '--rm', '--network=none', '--entrypoint', 'python',
                    '-v', '/opt/sub2api-price-monitor/data:/app/data:ro',
                    '-v', str(source) + '/verify_snapshot.py:/verify_snapshot.py:ro',
                    IMAGE, '/verify_snapshot.py'], check=True)
    proof = {'image_id': docker('GET', '/images/' + IMAGE + '/json')['Id'],
             'previous_image_id': live['Image'], 'source_git_commit': info['source_git_commit'],
             'source_git_tag': info['source_git_tag'],
             'source_sha256': hashlib.sha256((source / 'monitor.py').read_bytes()).hexdigest(),
             'passed': True}
    (root / 'monitor-verified.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))


def promote(root):
    proof = json.loads((root / 'monitor-verified.json').read_text())
    deployed_source = root / 'verified-source/ops/price-monitor/monitor.py'
    assert hashlib.sha256(deployed_source.read_bytes()).hexdigest() == proof['source_sha256']
    live = docker('GET', '/containers/' + NAME + '/json')
    assert proof['passed'] and proof['previous_image_id'] == live['Image']
    assert proof['image_id'] == docker('GET', '/images/' + IMAGE + '/json')['Id']
    # --time=-1 waits for the existing cycle to complete instead of killing it.
    subprocess.run(['docker', 'stop', '--time=-1', NAME], check=True)
    assert not docker('GET', '/containers/' + NAME + '/json')['State']['Running']
    docker('POST', '/containers/' + NAME + '/rename?name=' + BACKUP)
    docker('POST', '/containers/' + BACKUP + '/update', {'RestartPolicy': {'Name': 'no'}})
    payload = copy.deepcopy(live['Config'])
    payload.pop('Hostname', None)
    payload['Image'] = proof['image_id']
    payload['HostConfig'] = copy.deepcopy(live['HostConfig'])
    payload['HostConfig']['LogConfig'] = {'Type': 'json-file', 'Config': {'max-size': '10m', 'max-file': '3'}}
    payload['NetworkingConfig'] = {'EndpointsConfig': {
        network: {'Aliases': [alias for alias in (endpoint.get('Aliases') or []) if alias not in (live['Id'], live['Id'][:12])]}
        for network, endpoint in live['NetworkSettings']['Networks'].items()
    }}
    try:
        docker('POST', '/containers/create?name=' + NAME, payload)
        docker('POST', '/containers/' + NAME + '/start')
        current = docker('GET', '/containers/' + NAME + '/json')
        assert current['State']['Running'] and current['Image'] == proof['image_id']
        assert current['Config']['Env'] == live['Config']['Env']
        assert set(current['NetworkSettings']['Networks']) == set(live['NetworkSettings']['Networks'])
        assert current['HostConfig']['Binds'] == live['HostConfig']['Binds']
        assert output('docker', 'exec', NAME, 'sha256sum', '/app/monitor.py').split()[0] == proof['source_sha256']
    except Exception:
        # Retain the failed candidate for inspection, restore original name/runtime.
        try:
            docker('POST', '/containers/' + NAME + '/stop?t=60')
            docker('POST', '/containers/' + NAME + '/rename?name=' + NAME + '-failed-20260917')
        except RuntimeError:
            pass
        docker('POST', '/containers/' + BACKUP + '/rename?name=' + NAME)
        docker('POST', '/containers/' + NAME + '/update', {'RestartPolicy': live['HostConfig']['RestartPolicy']})
        docker('POST', '/containers/' + NAME + '/start')
        raise
    # Keep deployed source and persistent deployment record in agreement.
    path = Path('/opt/sub2api-price-monitor/monitor.py')
    assert hashlib.sha256(path.read_bytes()).hexdigest() == LIVE_SOURCE_SHA
    path.with_name('monitor.before-explicit-suffix-20260917.py').write_bytes(path.read_bytes())
    path.write_bytes(deployed_source.read_bytes())
    proof.update({'container': NAME, 'rollback_container': BACKUP, 'single_instance_handover': True})
    (root / 'monitor-deployed.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['prepare', 'promote'])
    parser.add_argument('root', type=Path)
    args = parser.parse_args()
    (prepare if args.mode == 'prepare' else promote)(args.root)
