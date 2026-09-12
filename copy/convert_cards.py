"""Convert card export TXT files to sub2api JSON format.

Usage:
    python convert_cards.py <input1.txt> [input2.txt ...] [-o output.json]
"""
import json
import sys
import base64
import argparse
from datetime import datetime, timezone


def decode_jwt_payload(token: str) -> dict:
    """Decode JWT payload without verification."""
    parts = token.split(".")
    if len(parts) < 2:
        return {}
    payload = parts[1]
    padding = 4 - len(payload) % 4
    if padding != 4:
        payload += "=" * padding
    try:
        decoded = base64.urlsafe_b64decode(payload)
        return json.loads(decoded)
    except Exception:
        return {}


def convert_record(record: dict, prefix: str = "") -> dict:
    """Convert a single card record to the target format."""
    email = record.get("email", "")
    access_token = record.get("access_token", "")
    id_token = record.get("id_token", "")

    at_payload = decode_jwt_payload(access_token)
    it_payload = decode_jwt_payload(id_token)

    expires_at = at_payload.get("exp", 0)

    auth_claim = it_payload.get("https://api.openai.com/auth", {})
    plan_type = auth_claim.get("chatgpt_plan_type", "free")

    return {
        "name": f"[{prefix}]{email}",
        "platform": "openai",
        "type": "oauth",
        "credentials": {
            "access_token": access_token,
            "chatgpt_account_id": record.get("chatgpt_account_id", ""),
            "chatgpt_user_id": record.get("chatgpt_user_id", ""),
            "client_id": record.get("client_id", ""),
            "email": email,
            "expires_at": expires_at,
            "id_token": id_token,
            "organization_id": record.get("organization_id", ""),
            "plan_type": plan_type,
            "refresh_token": record.get("refresh_token", ""),
        },
        "extra": {
            "email": email,
            "openai_oauth_responses_websockets_v2_enabled": False,
            "openai_oauth_responses_websockets_v2_mode": "off",
            "privacy_mode": "training_off",
        },
        "concurrency": 10,
        "priority": 1,
        "rate_multiplier": 1,
        "auto_pause_on_expired": True,
    }


def parse_txt_file(filepath: str) -> list[dict]:
    """Parse a TXT file, returning list of JSON records."""
    records = []
    with open(filepath, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("\ufeff"):
                continue
            try:
                obj = json.loads(line)
                records.append(obj)
            except json.JSONDecodeError:
                continue
    return records


def main():
    parser = argparse.ArgumentParser(description="Convert card export TXT to sub2api JSON")
    parser.add_argument("inputs", nargs="+", help="Input TXT file(s)")
    parser.add_argument("-o", "--output", default="accounts_export.json", help="Output JSON file")
    parser.add_argument("-p", "--prefix", default="lslscz", help="Prefix to prepend to account name")
    args = parser.parse_args()

    all_records = []
    for fp in args.inputs:
        all_records.extend(parse_txt_file(fp))

    accounts = [convert_record(r, args.prefix) for r in all_records]

    result = {
        "exported_at": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.") + f"{datetime.now(timezone.utc).microsecond // 1000:03d}Z",
        "proxies": [],
        "accounts": accounts,
    }

    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(result, f, indent=2, ensure_ascii=False)

    print(f"Converted {len(accounts)} accounts -> {args.output}")


if __name__ == "__main__":
    main()
