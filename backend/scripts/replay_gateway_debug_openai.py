#!/usr/bin/env python3
"""Replay a captured UPSTREAM_FORWARD_OPENAI request body."""

from __future__ import annotations

import argparse
import os
import sys
import urllib.error
import urllib.request
from pathlib import Path


DEFAULT_AUTH_ENV = "REPLAY_AUTHORIZATION"
DEFAULT_URL = "https://chatgpt.com/backend-api/codex/responses"
DEFAULT_HEADERS = {
    "Accept": "text/event-stream",
    "User-Agent": "codex_vscode/0.142.5 (Windows 10.0.26100; x86_64) unknown (VS Code; 26.623.141536)",
    "Content-Type": "application/json",
    "Session_id": "05fa5563e590ddc3",
    "Conversation_id": "05fa5563e590ddc3",
    "Chatgpt-Account-Id": "f8e1ddb7-c938-4a39-aa2b-92e70ad6a28c",
    "X-Codex-Turn-Metadata": '{"installation_id":"9f0a23b2-ba54-48fd-ba3d-91589aa01812","session_id":"019f02eb-6187-7071-855f-d8b1a4ae1304","thread_id":"019f02eb-6187-7071-855f-d8b1a4ae1304","turn_id":"019f420e-6f8f-7823-a7d1-fbca6cfc8232","window_id":"019f02eb-6187-7071-855f-d8b1a4ae1304:232","request_kind":"turn","thread_source":"user","sandbox":"none","turn_started_at_unix_ms":1783519670161,"workspace_kind":"project"}',
    "Originator": "codex_vscode",
    "Openai-Beta": "responses=experimental",
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Replay a hard-coded OpenAI upstream request with a body file."
    )
    parser.add_argument("--body-file", required=True, help="Request body file to send.")
    parser.add_argument("--url", default=DEFAULT_URL, help="Override request URL.")
    parser.add_argument(
        "--authorization",
        help=(
            "Authorization header value. If omitted, reads "
            f"{DEFAULT_AUTH_ENV}. Example: 'Bearer sk-...'"
        ),
    )
    parser.add_argument(
        "--bearer-token",
        help="Bearer token only. Equivalent to --authorization 'Bearer <token>'.",
    )
    parser.add_argument(
        "--header",
        action="append",
        default=[],
        metavar="NAME: VALUE",
        help="Override/add a request header. May be repeated.",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=300,
        help="Request timeout in seconds (default: 300).",
    )
    parser.add_argument(
        "--no-stream",
        action="store_true",
        help="Read the whole response at once instead of printing chunks as they arrive.",
    )
    return parser.parse_args()


def apply_header_overrides(headers: dict[str, str], overrides: list[str]) -> None:
    for item in overrides:
        if ":" not in item:
            raise ValueError(f"--header must be 'NAME: VALUE', got: {item!r}")
        key, value = item.split(":", 1)
        headers[key.strip()] = value.strip()


def authorization_header(args: argparse.Namespace) -> str:
    if args.bearer_token:
        return f"Bearer {args.bearer_token}"
    auth = args.authorization or os.getenv(DEFAULT_AUTH_ENV)
    if not auth:
        raise ValueError(
            f"missing Authorization; pass --authorization, --bearer-token, or set {DEFAULT_AUTH_ENV}"
        )
    return auth


def replay(url: str, headers: dict[str, str], body: bytes, timeout: float, stream: bool) -> int:
    req = urllib.request.Request(url, data=body, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            print(f"HTTP {resp.status} {resp.reason}")
            for key, value in resp.headers.items():
                print(f"{key}: {value}")
            print()
            if stream:
                while True:
                    chunk = resp.read(8192)
                    if not chunk:
                        break
                    sys.stdout.buffer.write(chunk)
                    sys.stdout.buffer.flush()
            else:
                sys.stdout.buffer.write(resp.read())
                sys.stdout.buffer.flush()
            return 0
    except urllib.error.HTTPError as err:
        print(f"HTTP {err.code} {err.reason}", file=sys.stderr)
        for key, value in err.headers.items():
            print(f"{key}: {value}", file=sys.stderr)
        print(file=sys.stderr)
        sys.stderr.buffer.write(err.read())
        sys.stderr.buffer.flush()
        return 1


def main() -> int:
    args = parse_args()
    body_path = Path(args.body_file)
    if not body_path.exists():
        raise ValueError(f"body file does not exist: {body_path}")

    headers = dict(DEFAULT_HEADERS)
    headers["Authorization"] = authorization_header(args)
    apply_header_overrides(headers, args.header)

    print(f"url: {args.url}")
    print(f"using body: {body_path}")
    body = body_path.read_bytes()
    return replay(args.url, headers, body, timeout=args.timeout, stream=not args.no_stream)


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        raise SystemExit(2)
