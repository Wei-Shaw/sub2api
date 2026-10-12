#!/usr/bin/env python3
"""Read-only check of actual frontend delivery, including lazy-loaded chunks."""
import argparse
import hashlib
import json
import re
from urllib.parse import urljoin
from urllib.request import urlopen


def verify(base):
    def fetch(path):
        with urlopen(urljoin(base, path), timeout=30) as response:
            return response.read(), response.headers.get_content_type()

    html, _ = fetch('/')
    pending = set(re.findall(r'(?:src|href)="(/assets/[^" ]+)"', html.decode()))
    if not pending:
        raise ValueError('No frontend entry assets found')
    verified = {}
    while pending:
        path = pending.pop()
        if path in verified:
            continue
        raw, mime = fetch(path)
        if not raw or mime == 'text/html':
            raise ValueError('Missing asset or HTML fallback: ' + path)
        verified[path] = hashlib.sha256(raw).hexdigest()
        if path.endswith('.js'):
            # Only follow actual dynamic-import expressions.  A broad quoted-string
            # scan treats ordinary application strings (for example an asset name
            # shown in a component) as dependencies and creates false failures.
            text = raw.decode('utf-8', errors='replace')
            refs = re.findall(
                r'import\s*\(\s*["\x27]([^"\x27]+\.(?:js|css))["\x27]\s*\)',
                text,
            )
            for ref in refs:
                resolved = urljoin(path, ref)
                if resolved.startswith('/assets/assets/'):
                    resolved = resolved.replace('/assets/assets/', '/assets/', 1)
                if resolved.startswith('/assets/') and resolved not in verified:
                    pending.add(resolved)
    return {'base': base, 'passed': True, 'asset_count': len(verified), 'sha256': verified}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('base', help='Candidate origin or real public HTTPS URL')
    args = parser.parse_args()
    print(json.dumps(verify(args.base), indent=2))
