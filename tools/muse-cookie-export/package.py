"""Rebuild the small browser-exporter download served by the account form."""
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile, ZipInfo

ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / 'frontend/public/muse-session-exporter.zip'
SOURCE = ROOT / 'tools/muse-cookie-export'
files = {name: SOURCE / name for name in ('manifest.json', 'popup.html', 'popup.js', 'README.md')}
files['THIRD_PARTY_NOTICES_MUSE.md'] = ROOT / 'THIRD_PARTY_NOTICES_MUSE.md'
with ZipFile(OUTPUT, 'w') as archive:
    for name, path in sorted(files.items()):
        entry = ZipInfo('muse-cookie-export/' + name, date_time=(2026, 1, 1, 0, 0, 0))
        entry.compress_type = ZIP_DEFLATED
        entry.external_attr = 0o644 << 16
        archive.writestr(entry, path.read_bytes())
print(OUTPUT)
