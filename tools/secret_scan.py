#!/usr/bin/env python3
"""High-confidence secret scanner for tracked and untracked repo files.

The scanner intentionally reports only file/line/pattern names, never matched
secret values. It focuses on token formats and private-key blocks that are
unlikely to be legitimate source-code identifiers.
"""
from __future__ import annotations

import os
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MAX_FILE_SIZE = 1_000_000

EXCLUDED_DIR_PARTS = {
    ".git",
    ".idea",
    ".vscode",
    ".next",
    ".nuxt",
    ".turbo",
    ".cache",
    "__pycache__",
    "node_modules",
    "dist",
    "build",
    "coverage",
    "vendor",
    "tmp",
    "temp",
}

EXCLUDED_SUFFIXES = {
    ".png",
    ".jpg",
    ".jpeg",
    ".gif",
    ".webp",
    ".ico",
    ".pdf",
    ".zip",
    ".gz",
    ".tgz",
    ".xz",
    ".7z",
    ".rar",
    ".mp3",
    ".mp4",
    ".mov",
    ".woff",
    ".woff2",
    ".ttf",
    ".eot",
    ".wasm",
    ".sqlite",
    ".db",
    ".lockb",
}

PATTERNS: list[tuple[str, re.Pattern[str]]] = [
    ("private-key-block", re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----")),
    ("aws-access-key-id", re.compile(r"\b(?:AKIA|ASIA)[0-9A-Z]{16}\b")),
    ("github-token", re.compile(r"\bgh[pousr]_[A-Za-z0-9_]{36,255}\b")),
    ("slack-token", re.compile(r"\bxox[baprs]-[A-Za-z0-9-]{10,}\b")),
    ("stripe-secret-key", re.compile(r"\bsk_(?:live|test)_[A-Za-z0-9]{20,}\b")),
    ("openai-api-key", re.compile(r"\bsk-(?:proj-)?[A-Za-z0-9_-]{32,}\b")),
    ("anthropic-api-key", re.compile(r"\bsk-ant-[A-Za-z0-9_-]{32,}\b")),
    ("telegram-bot-token", re.compile(r"\b\d{8,12}:[A-Za-z0-9_-]{30,}\b")),
]

REDACTED_RE = re.compile(r"(?i)(redacted|example|dummy|placeholder|your[_-]?token|changeme|test[_-]?only|fake[_-]?secret)")


def git_files() -> list[Path]:
    proc = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard"],
        cwd=ROOT,
        check=True,
        text=True,
        stdout=subprocess.PIPE,
    )
    return [ROOT / line for line in proc.stdout.splitlines() if line.strip()]


def should_skip(path: Path) -> bool:
    try:
        rel = path.relative_to(ROOT)
    except ValueError:
        return True
    if any(part in EXCLUDED_DIR_PARTS for part in rel.parts):
        return True
    if path.suffix.lower() in EXCLUDED_SUFFIXES:
        return True
    try:
        if path.stat().st_size > MAX_FILE_SIZE:
            return True
    except FileNotFoundError:
        return True
    return False


def is_binary(data: bytes) -> bool:
    return b"\0" in data[:8192]


def is_test_file(path: Path) -> bool:
    rel = path.relative_to(ROOT).as_posix()
    return rel.endswith(("_test.go", ".spec.ts", ".spec.tsx", ".test.ts", ".test.tsx"))


def is_allowed_fixture(path: Path, line: str, pattern_name: str) -> bool:
    if REDACTED_RE.search(line):
        return True
    if is_test_file(path) and pattern_name == "private-key-block":
        # Test-only PEM fixtures are common for crypto/payment parsing tests.
        # Provider tokens in test files are still reported unless explicitly
        # marked fake/redacted by REDACTED_RE above.
        return True
    return False


def scan_file(path: Path) -> list[tuple[str, int, str]]:
    if should_skip(path):
        return []
    data = path.read_bytes()
    if is_binary(data):
        return []
    text = data.decode("utf-8", errors="replace")
    findings: list[tuple[str, int, str]] = []
    for line_no, line in enumerate(text.splitlines(), 1):
        for name, pattern in PATTERNS:
            if pattern.search(line) and not is_allowed_fixture(path, line, name):
                findings.append((path.relative_to(ROOT).as_posix(), line_no, name))
    return findings


def main() -> int:
    findings: list[tuple[str, int, str]] = []
    for path in git_files():
        if path.exists() and path.is_file():
            findings.extend(scan_file(path))

    if findings:
        print("Potential secrets found (values redacted):", file=sys.stderr)
        for rel, line_no, name in findings:
            print(f"  {rel}:{line_no}: {name}", file=sys.stderr)
        return 1

    print("Secret scan passed: no high-confidence secrets found.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
