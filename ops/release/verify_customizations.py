#!/usr/bin/env python3
"""Fail closed on missing BUAGA files or registered in-file customizations."""
import argparse
import json
from pathlib import Path
import re


def verify(root: Path, registry: Path) -> list[str]:
    failures = []
    text = registry.read_text(encoding="utf-8")
    sections = re.split(r"^### ", text, flags=re.M)[1:]
    if not sections:
        return ["定制清单为空，拒绝构建"]
    seen = set()
    for section in sections:
        title = section.splitlines()[0]
        identity = title.split()[0]
        if identity in seen:
            failures.append(f"重复记录：{identity}")
        seen.add(identity)
        paths = re.findall(r"^- 文件：(.+)$", section, re.M)
        assertions = re.findall(r"^- 断言：(.+)$", section, re.M)
        if len(paths) != 1 or not assertions:
            failures.append(f"{title}：缺少文件或断言")
            continue
        relative = Path(paths[0].strip().strip('`'))
        path = root / relative
        if relative.is_absolute() or '..' in relative.parts or not path.is_file() or not path.resolve().is_relative_to(root.resolve()):
            failures.append(f"{title}：文件缺失或路径越界 {relative}")
            continue
        content = path.read_text(encoding="utf-8")
        for assertion in assertions:
            try:
                marker = json.loads(assertion)
                if not isinstance(marker, str) or not marker:
                    raise ValueError("empty assertion")
            except (ValueError, TypeError):
                failures.append(f"{title}：断言必须为非空 JSON 字符串")
                continue
            if marker not in content:
                failures.append(f"{title}：{relative} 缺失 {marker[:100]}")
    inventory = json.loads(registry.with_name('custom-files.json').read_text())
    if not inventory.get('files'):
        failures.append('定制文件清单为空')
    for filename in inventory['files']:
        path = root / filename
        if not path.is_file() or not path.resolve().is_relative_to(root.resolve()):
            failures.append('定制文件缺失：' + filename)
    # These imported Markdown files live outside frontend/ and were omitted
    # from the historical source archive. Missing them breaks a fresh build.
    if (root / 'frontend/src/views/public/LegalDocumentView.vue').exists():
        for language in ('zh', 'en'):
            document = root / f'docs/legal/admin-compliance.{language}.md'
            if not document.is_file():
                failures.append('前端构建依赖缺失：' + str(document.relative_to(root)))
    return failures


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('source', type=Path)
    parser.add_argument('--registry', type=Path, default=Path(__file__).with_name('CUSTOMIZATIONS.md'))
    args = parser.parse_args()
    try:
        failures = verify(args.source, args.registry)
    except (OSError, ValueError) as error:
        raise SystemExit('守卫无法完成，拒绝构建：' + str(error))
    for failure in failures:
        print('✗ ' + failure)
    if failures:
        raise SystemExit(f'定制守卫：{len(failures)} 项失败，拒绝构建')
    print('定制守卫通过：文件清单和全部内容断言均存在')


if __name__ == '__main__':
    main()
