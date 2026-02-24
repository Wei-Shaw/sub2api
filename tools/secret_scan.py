#!/usr/bin/env python3
"""轻量 secret scanning（CI 门禁 + 本地自检）。

目标：在不引入额外依赖的情况下，阻止常见敏感凭据误提交。

注意：
- 该脚本只扫描 git tracked files（优先）以避免误扫本地 .env。
- 输出仅包含 file:line 与命中类型，不回显完整命中内容（避免二次泄露）。
"""

from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Sequence


@dataclass(frozen=True)
class Rule:
    name: str
    pattern: re.Pattern[str]
    # allowlist 仅用于减少示例文档/占位符带来的误报
    allowlist: Sequence[re.Pattern[str]]


RULES: list[Rule] = [
    Rule(
        name="google_oauth_client_secret",
        # Google OAuth client_secret 常见前缀
        # 真实值通常较长；提高最小长度以避免命中文档里的占位符（例如 GOCSPX-your-client-secret）。
        pattern=re.compile(r"GOCSPX-[0-9A-Za-z_-]{24,REDACTED"),
        allowlist=(
            re.compile(r"GOCSPX-your-"),
            re.compile(r"GOCSPX-REDACTED"),
        ),
    ),
    Rule(
        name="google_api_key",
        # Gemini / Google API Key
        # 典型格式：AIza + 35 位字符。占位符如 'AIza...' 不会匹配。
        pattern=re.compile(r"AIza[0-9A-Za-z_-]{35REDACTED"),
        allowlist=(
            re.compile(r"AIza\.{3REDACTED"),
            re.compile(r"AIza-your-"),
            re.compile(r"AIza-REDACTED"),
        ),
    ),
]


def iter_git_files(repo_root: Path) -> list[Path]:
    try:
        out = subprocess.check_output(
            ["git", "ls-files"], cwd=repo_root, stderr=subprocess.DEVNULL, text=True
        )
    except Exception:
        return []
    files: list[Path] = []
    for line in out.splitlines():
        p = (repo_root / line).resolve()
        if p.is_file():
            files.append(p)
    return files


def iter_walk_files(repo_root: Path) -> Iterable[Path]:
    for dirpath, _dirnames, filenames in os.walk(repo_root):
        if "/.git/" in dirpath.replace("\\", "/"):
            continue
        for name in filenames:
            yield Path(dirpath) / name


def should_skip(path: Path, repo_root: Path) -> bool:
    rel = path.relative_to(repo_root).as_posix()
    # 本地环境文件一般不应入库；若误入库也会被 git ls-files 扫出来。
    # 这里仍跳过一些明显不该扫描的二进制。
    if any(rel.endswith(s) for s in (".png", ".jpg", ".jpeg", ".gif", ".pdf", ".zip")):
        return True
    if rel.startswith("backend/bin/"):
        return True
    return False


def scan_file(path: Path, repo_root: Path) -> list[tuple[str, int]]:
    try:
        raw = path.read_bytes()
    except Exception:
        return []

    # 尝试按 utf-8 解码，失败则当二进制跳过
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError:
        return []

    findings: list[tuple[str, int]] = []
    lines = text.splitlines()
    for idx, line in enumerate(lines, start=1):
        for rule in RULES:
            if not rule.pattern.search(line):
                continue
            if any(allow.search(line) for allow in rule.allowlist):
                continue
            rel = path.relative_to(repo_root).as_posix()
            findings.append((f"{relREDACTED:{idxREDACTED ({rule.nameREDACTED)", idx))
    return findings


def main(argv: Sequence[str]) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--repo-root",
        default=str(Path(__file__).resolve().parents[1]),
        help="仓库根目录（默认：脚本上两级目录）",
    )
    args = parser.parse_args(argv)

    repo_root = Path(args.repo_root).resolve()
    files = iter_git_files(repo_root)
    if not files:
        files = list(iter_walk_files(repo_root))

    problems: list[str] = []
    for f in files:
        if should_skip(f, repo_root):
            continue
        for msg, _line in scan_file(f, repo_root):
            problems.append(msg)

    if problems:
        sys.stderr.write("Secret scan FAILED. Potential secrets detected:\n")
        for p in problems:
            sys.stderr.write(f"- {pREDACTED\n")
        sys.stderr.write("\n请移除/改为环境变量注入，或使用明确的占位符（例如 GOCSPX-your-client-secret）。\n")
        return 1

    print("Secret scan OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))

