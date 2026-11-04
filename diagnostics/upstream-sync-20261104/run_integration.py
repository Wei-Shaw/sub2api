#!/usr/bin/env python3
"""Run integration tests against disposable Docker-only PostgreSQL and Redis."""
from pathlib import Path
import subprocess
import uuid

repo = Path(__file__).resolve().parents[2]
name = 'sub2api-sync-it-' + uuid.uuid4().hex[:8]
created = []

def docker(*args, capture=False):
    return subprocess.run(['docker', *args], check=True, text=True,
                          stdout=subprocess.PIPE if capture else subprocess.DEVNULL).stdout

network = docker('network', 'create', '--internal', name, capture=True).strip()
try:
    pg = name + '-postgres'
    redis = name + '-redis'
    created.append(docker('run', '-d', '--name', pg, '--network', name,
                          '-e', 'POSTGRES_PASSWORD=fixture-only', '-e', 'POSTGRES_DB=sub2api_test',
                          'postgres:18.1-alpine3.23', capture=True).strip())
    created.append(docker('run', '-d', '--name', redis, '--network', name,
                          'redis:8.4-alpine', capture=True).strip())
    args = ['run', '--rm', '--network', name, '--cpus=2', '--memory=4g',
            '-v', str(repo) + ':/src:ro', '-v', 'sub2api-gomod:/go/pkg/mod',
            '-v', 'sub2api-gocache:/root/.cache/go-build',
            '-e', 'SUB2API_TEST_EXTERNAL_POSTGRES_DSN=postgres://postgres:fixture-only@' + pg + ':5432/sub2api_test?sslmode=disable',
            '-e', 'SUB2API_TEST_EXTERNAL_REDIS_ADDR=' + redis + ':6379',
            '-e', 'CI=true', '-e', 'CGO_ENABLED=0', '-e', 'GOMAXPROCS=2',
            '-e', 'GOMEMLIMIT=1400MiB', '-e', 'GOGC=50', '-e', 'GOFLAGS=-p=1',
            '-e', 'GOTOOLCHAIN=local', '-e', 'GOPROXY=off', 'golang:1.26.10-alpine',
            'sh', '-c', 'cp -a /src /work && cd /work/backend && go test -tags=integration ./... -count=1 -timeout=12m']
    result = subprocess.run(['docker', *args])
    raise SystemExit(result.returncode)
finally:
    for container in reversed(created):
        subprocess.run(['docker', 'rm', '-f', container], stdout=subprocess.DEVNULL, check=False)
    subprocess.run(['docker', 'network', 'rm', network], stdout=subprocess.DEVNULL, check=False)
