#!/usr/bin/env python3
"""Server-side verification without Git; the manifest must be supplied separately."""
import argparse
import hashlib
import json
import re
from pathlib import Path
import subprocess
import tarfile
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('archive', type=Path)
    parser.add_argument('manifest', type=Path)
    parser.add_argument('--extract', type=Path)
    args = parser.parse_args()
    info = json.loads(args.manifest.read_text())
    if not re.fullmatch(r'rel/buaga-[a-z0-9-]+', info.get('source_git_tag', '')):
        raise SystemExit('Missing explicit BUAGA release tag')
    if not re.fullmatch(r'[0-9a-f]{40}', info.get('source_git_commit', '')):
        raise SystemExit('Invalid source commit')
    if info.get('archive_format') != 'git archive --format=tar.gz' or not info.get('customization_guard'):
        raise SystemExit('Missing verified Git archive provenance')
    digest = hashlib.sha256(args.archive.read_bytes()).hexdigest()
    if digest != info['source_archive_sha256']:
        raise SystemExit('Source archive checksum mismatch')
    with tempfile.TemporaryDirectory(prefix='buaga-package-check-') as temp:
        root = Path(temp)
        with tarfile.open(args.archive) as packed:
            if packed.pax_headers.get('comment') != info['source_git_commit']:
                raise SystemExit('Git archive commit does not match manifest')
            for member in packed.getmembers():
                path = Path(member.name)
                if path.is_absolute() or '..' in path.parts or not (member.isfile() or member.isdir()):
                    raise SystemExit('Unsupported archive path/type')
            packed.extractall(root)
        # Installed verifier + registry are the approval baseline, not supplied code.
        subprocess.run(['python3', str(Path(__file__).with_name('verify_customizations.py')), str(root)], check=True)
        if args.extract:
            if args.extract.exists():
                raise SystemExit('Refuse to overwrite source directory')
            import shutil
            shutil.copytree(root, args.extract)
    print('Verified Git archive, commit, checksum and customizations')


if __name__ == '__main__':
    main()
