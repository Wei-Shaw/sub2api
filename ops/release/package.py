#!/usr/bin/env python3
"""Export a clean, tagged BUAGA commit, never a mutable source directory."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile
import tarfile

ROOT = Path(__file__).resolve().parents[2]


def git(*args):
    return subprocess.check_output(['git', '-C', str(ROOT), *args], text=True).strip()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('tag')
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    if not re.fullmatch(r'rel/buaga-[a-z0-9-]+', args.tag):
        raise SystemExit('Only an explicit rel/buaga-* tag is accepted')
    if git('status', '--porcelain'):
        raise SystemExit('Commit reviewed changes first; working tree must be clean')
    commit = git('rev-parse', 'refs/tags/' + args.tag + '^{commit}')
    subprocess.run(['git', '-C', str(ROOT), 'merge-base', '--is-ancestor', commit, 'production'], check=True)
    if commit != git('rev-parse', 'HEAD'):
        raise SystemExit('Check out the exact release tag before packaging')
    archive = args.output.resolve()
    metadata = archive.with_suffix(archive.suffix + '.json')
    if archive.exists() or metadata.exists():
        raise SystemExit('Never overwrite an existing release package')
    archive.parent.mkdir(parents=True, exist_ok=True)
    # Verification precedes publication of either package or metadata.
    with tempfile.TemporaryDirectory(prefix='buaga-export-') as temp:
        packed = Path(temp) / 'source.tar.gz'
        subprocess.run(['git', '-C', str(ROOT), 'archive', '--format=tar.gz', '-o', str(packed), args.tag], check=True)
        extracted = Path(temp) / 'source'
        extracted.mkdir()
        with tarfile.open(packed) as source:
            for member in source.getmembers():
                path = Path(member.name)
                if path.is_absolute() or '..' in path.parts or not (member.isfile() or member.isdir()):
                    raise SystemExit('Unsafe or unsupported source archive entry')
            source.extractall(extracted)
        subprocess.run(['python3', str(ROOT / 'ops/release/verify_customizations.py'), str(extracted)], check=True)
        data = packed.read_bytes()
        archive.write_bytes(data)
        info = {'source_git_commit': commit, 'source_git_tag': args.tag,
                'source_archive_sha256': hashlib.sha256(data).hexdigest(),
                'archive_format': 'git archive --format=tar.gz', 'customization_guard': True}
        metadata.write_text(json.dumps(info, indent=2) + '\n')
        print(json.dumps(info, indent=2))


if __name__ == '__main__':
    main()
