#!/usr/bin/env python3
"""Monitor model pricing data and notify a WeCom group robot on changes."""

from __future__ import annotations

import base64
import hmac
import hashlib
import json
import logging
import os
import re
import signal
import sys
import time
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit
from urllib.request import Request, urlopen


LOG = logging.getLogger("sub2api-price-monitor")
PRICE_FIELDS = (
    "input_cost_per_token",
    "input_cost_per_token_priority",
    "output_cost_per_token",
    "output_cost_per_token_priority",
    "cache_creation_input_token_cost",
    "cache_creation_input_token_cost_priority",
    "cache_creation_input_token_cost_above_1hr",
    "cache_read_input_token_cost",
    "cache_read_input_token_cost_priority",
    "output_cost_per_image",
    "output_cost_per_image_token",
)
DEFAULT_URL = (
    "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/"
    "model_prices_and_context_window.json"
)
DEFAULT_HASH_URL = (
    "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/"
    "model_prices_and_context_window.sha256"
)
DEFAULT_UPSTREAM_PRICE_PATHS = (
    "/api/pricing",
    "/api/model-pricing",
    "/api/prices",
    "/v1/pricing",
)
DEFAULT_UPSTREAM_AUTH_LOGIN_PATH = "/api/v1/auth/login"
DEFAULT_UPSTREAM_AUTH_REFRESH_PATH = "/api/v1/auth/refresh"
DEFAULT_UPSTREAM_AUTH_KEYS_PATH = "/api/v1/keys"
DEFAULT_SUB2API_API_BASE_URL = "http://sub2api:8080"
DEFAULT_TRANSITHUB_INTERNAL_URL = "http://transithub:10621"
UPSTREAM_PRICING_STATE_VERSION = 4
BROWSER_PRICING_SNAPSHOT_VERSION = 1
UPSTREAM_TOKEN_STATE_VERSION = 1
ACCOUNT_RENAME_STATE_VERSION = 1
ACCOUNT_PRIORITY_STATE_VERSION = 2
MANUAL_PRIORITY_LOCK_STATE_VERSION = 1
ACCOUNT_SAFETY_STATE_VERSION = 2
ACCOUNT_AUTO_GROUP_STATE_VERSION = 1
IMAGE2_MANAGED_GROUP_NAME = "gpt-image-2"
SAFETY_EXCLUDED_GROUP_NAMES = {"\u81ea\u7528pro"}
WECOM_MARKDOWN_MAX_BYTES = 4096
WECOM_MARKDOWN_CHUNK_BYTES = 3000
WECOM_SEND_MAX_ATTEMPTS = 3
WECOM_SEND_RETRY_DELAY_SECONDS = 1
UPSTREAM_PRICE_FIELDS = (
    "model_ratio",
    "model_price",
    "input_price",
    "output_price",
    "cache_write_price",
    "cache_read_price",
    "image_output_price",
    "per_request_price",
    "input_cost_per_token",
    "output_cost_per_token",
    "billing_mode",
    "billing_expr",
    "description",
    "tags",
)


class UpstreamHTTPError(RuntimeError):
    def __init__(self, url: str, status: int):
        super().__init__(f"upstream returned HTTP {status} for {url}")
        self.url = url
        self.status = status


def utc_now() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat()


def env_int(name: str, default: int, minimum: int) -> int:
    raw = os.getenv(name, str(default)).strip()
    try:
        value = int(raw)
    except ValueError:
        LOG.warning("Invalid %s=%r; using %s", name, raw, default)
        return default
    return max(value, minimum)


def env_decimal_percent(
    name: str, default: Decimal, minimum: Decimal, maximum: Decimal
) -> Decimal:
    raw = os.getenv(name, str(default)).strip()
    try:
        value = Decimal(raw)
    except Exception:
        LOG.warning("Invalid %s=%r; using %s", name, raw, default)
        return default
    if not value.is_finite() or value < minimum or value > maximum:
        LOG.warning(
            "%s must be between %s and %s; using %s",
            name,
            minimum,
            maximum,
            default,
        )
        return default
    return value


def env_group_ids(name: str) -> list[int]:
    result: list[int] = []
    for raw_id in os.getenv(name, "").split(","):
        raw_id = raw_id.strip()
        if not raw_id:
            continue
        try:
            group_id = int(raw_id)
        except ValueError:
            LOG.warning("Ignoring invalid group id %r in %s", raw_id, name)
            continue
        if group_id > 0 and group_id not in result:
            result.append(group_id)
    return result


def env_account_ids(name: str) -> set[str]:
    result: set[str] = set()
    for raw_id in os.getenv(name, "").split(","):
        raw_id = raw_id.strip()
        if not raw_id:
            continue
        try:
            account_id = int(raw_id)
        except ValueError:
            LOG.warning("Ignoring invalid account id %r in %s", raw_id, name)
            continue
        if account_id > 0:
            result.add(str(account_id))
    return result


def fetch_bytes(url: str, timeout: int = 30, headers: dict[str, str] | None = None) -> bytes:
    request_headers = {"User-Agent": "sub2api-price-monitor/1.0"}
    if headers:
        request_headers.update(headers)
    request = Request(url, headers=request_headers)
    try:
        with urlopen(request, timeout=timeout) as response:
            return response.read()
    except (HTTPError, URLError, TimeoutError) as exc:
        raise RuntimeError(f"download failed for {url}: {exc}") from exc


def fetch_json(url: str, headers: dict[str, str] | None = None, timeout: int = 15) -> Any:
    headers = headers or {}
    request = Request(
        url,
        headers={
            "Accept": "application/json",
            "User-Agent": "sub2api-price-monitor/1.0",
            **headers,
        },
    )
    try:
        with urlopen(request, timeout=timeout) as response:
            body = response.read()
    except HTTPError as exc:
        raise UpstreamHTTPError(url, exc.code) from exc
    except (URLError, TimeoutError) as exc:
        raise RuntimeError(f"upstream request failed for {url}: {exc}") from exc
    try:
        return json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"upstream returned non-JSON data for {url}") from exc


def post_json(
    url: str,
    payload: dict[str, Any],
    headers: dict[str, str] | None = None,
    timeout: int = 15,
    return_headers: bool = False,
) -> Any:
    request_headers = {
        "Accept": "application/json",
        "Content-Type": "application/json",
        "User-Agent": "sub2api-price-monitor/1.0",
        **(headers or {}),
    }
    request = Request(
        url,
        data=canonical_json(payload),
        headers=request_headers,
        method="POST",
    )
    try:
        with urlopen(request, timeout=timeout) as response:
            body = response.read()
            response_headers = response.headers
    except HTTPError as exc:
        raise UpstreamHTTPError(url, exc.code) from exc
    except (URLError, TimeoutError) as exc:
        raise RuntimeError(f"upstream request failed for {url}: {exc}") from exc
    try:
        result = json.loads(body.decode("utf-8"))
        return (result, response_headers) if return_headers else result
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"upstream returned non-JSON data for {url}") from exc


def put_json(
    url: str, payload: dict[str, Any], headers: dict[str, str] | None = None, timeout: int = 15
) -> Any:
    request_headers = {
        "Accept": "application/json",
        "Content-Type": "application/json",
        "User-Agent": "sub2api-price-monitor/1.0",
        **(headers or {}),
    }
    request = Request(
        url,
        data=canonical_json(payload),
        headers=request_headers,
        method="PUT",
    )
    try:
        with urlopen(request, timeout=timeout) as response:
            body = response.read()
    except HTTPError as exc:
        raise UpstreamHTTPError(url, exc.code) from exc
    except (URLError, TimeoutError) as exc:
        raise RuntimeError(f"Sub2API request failed for {url}: {exc}") from exc
    try:
        return json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"Sub2API returned non-JSON data for {url}") from exc


def parse_sha256(body: bytes) -> str | None:
    text = body.decode("utf-8", errors="replace")
    for token in text.replace("\n", " ").split():
        token = token.strip().lower()
        if len(token) == 64 and all(c in "0123456789abcdef" for c in token):
            return token
    return None


def normalize_number(value: Any) -> float | int | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    return value


def normalize_prices(raw: dict[str, Any]) -> dict[str, dict[str, float | int | None]]:
    result: dict[str, dict[str, float | int | None]] = {}
    for model, entry in raw.items():
        if model == "sample_spec" or not isinstance(entry, dict):
            continue
        prices = {
            field: normalize_number(entry.get(field))
            for field in PRICE_FIELDS
            if field in entry
        }
        if any(value is not None for value in prices.values()):
            result[model] = prices
    return dict(sorted(result.items()))


def canonical_json(value: Any) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode(
        "utf-8"
    )


def canonical_ratio(value: Any) -> str | None:
    """Return one plain decimal ratio, rejecting compound or ambiguous values."""
    if isinstance(value, bool) or value is None:
        return None
    text = str(value).strip().lower()
    if text.endswith("x"):
        text = text[:-1].strip()
    if not re.fullmatch(r"(?:0|[1-9]\d*)(?:\.\d+)?", text):
        return None
    try:
        from decimal import Decimal, InvalidOperation

        ratio = Decimal(text)
    except (InvalidOperation, ValueError):
        return None
    if not ratio.is_finite() or ratio < 0:
        return None
    normalized = format(ratio, "f").rstrip("0").rstrip(".")
    return normalized or "0"


def account_name_for_ratio(name: Any, ratio: Any) -> str | None:
    if not isinstance(name, str):
        return None
    canonical = canonical_ratio(ratio)
    if canonical is None:
        return None
    current_name = name.strip()
    match = re.fullmatch(r"(.+)-(\d+(?:\.\d+)?)", current_name)
    if not match:
        return None
    prefix, previous_ratio = match.groups()
    # Only an explicit '-ratio' suffix is eligible. Compact numbers such as
    # seedance2.0, hx0.04 and PRO012 are ambiguous and must remain untouched.
    # A bare model/version name is not an account price label either.
    if re.search(r"(?:^|[-_\s])(?:seedance|wan|sd|sdxl|version|ver|v)$", prefix, re.I):
        return None
    if Decimal(previous_ratio) >= 2 and re.search(
        r"(?:^|[-_\s])(?:gpt|claude|sonnet|opus|haiku|gemini|deepseek)$", prefix, re.I
    ):
        return None
    return f"{prefix}-{canonical}"


def account_priority_category(name: Any) -> str:
    """Classify an account name so priority is ranked only within its product type."""
    text = str(name or "").strip().lower()
    tokens = [token for token in re.split(r"[^a-z0-9]+", text) if token]
    has_chinese_image = "生图" in text
    has_chinese_video = "视频" in text

    def has_token_or_prefix(*values: str) -> bool:
        return any(token in values or any(token.startswith(value) for value in values) for token in tokens)

    def has_compound_label(*values: str) -> bool:
        return any(
            any(token.startswith("gpt") and token.endswith(value) for token in values for token in tokens)
            for value in values
        )

    # Check the more specific product labels before the generic GPT/other buckets.
    if has_chinese_image or has_token_or_prefix("image", "iamge", "mage", "img"):
        return "image"
    if has_token_or_prefix("claude"):
        return "claude"
    if has_token_or_prefix("plus") or has_compound_label("plus"):
        return "plus"
    if has_token_or_prefix("pro") or has_compound_label("pro"):
        return "pro"
    if has_token_or_prefix("gpt"):
        return "gpt"
    if has_token_or_prefix("grok", "gork"):
        return "grok"
    if has_chinese_video or has_token_or_prefix("video", "vid"):
        return "video"
    return "other"


def is_image2_account(name: Any) -> bool:
    """Exclude image2 accounts from account-level upstream price automation."""
    return account_priority_category(name) == "image"


def is_account_automation_excluded(name: Any) -> bool:
    """Exclude image2 and Grok accounts from account mutations only."""
    return account_priority_category(name) in {"image", "grok"}


def is_domestic_model_account(account: dict[str, Any]) -> bool:
    """Identify accounts currently bound to a domestic-model group."""
    marker = "\u56fd\u6a21"
    return any(
        marker in str(group.get("name") or "")
        for group in account.get("managed_groups", [])
        if isinstance(group, dict)
    )


def is_guomo_collection_account(account: dict[str, Any]) -> bool:
    """Exclude accounts in the Guomo collection from automatic renaming."""
    return any(
        "国模合集" in str(group.get("name") or "")
        for group in account.get("managed_groups", [])
        if isinstance(group, dict)
    )


def single_group_ratio(pricing: dict[str, Any]) -> str | None:
    groups = pricing.get("groups", {})
    if not isinstance(groups, dict) or len(groups) != 1:
        return None
    return canonical_ratio(next(iter(groups.values())))


def is_image2_managed_group(group: Any) -> bool:
    """Exclude only the gpt-image-2 binding from account safety checks."""
    if not isinstance(group, dict):
        return False
    name = str(group.get("name") or "").strip().casefold()
    compact_name = re.sub(r"[^a-z0-9\u4e00-\u9fff]+", "", name)
    return (
        name == IMAGE2_MANAGED_GROUP_NAME
        or "image2" in compact_name
        or "iamge2" in compact_name
        or "生图" in name
    )


def is_safety_excluded_group(group: Any) -> bool:
    """Leave explicitly protected groups out of cost-based binding changes."""
    if not isinstance(group, dict):
        return False
    name = re.sub(r"\s+", "", str(group.get("name") or "").strip().casefold())
    return name in SAFETY_EXCLUDED_GROUP_NAMES or "\u56fd\u6a21" in name


def load_json(path: Path, default: Any) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return default
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"cannot read {path}: {exc}") from exc


def write_json_atomic(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_bytes(canonical_json(value) + b"\n")
    temporary.replace(path)


def write_private_json_atomic(path: Path, value: Any) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = path.with_suffix(path.suffix + ".tmp")
    temporary.write_bytes(canonical_json(value) + b"\n")
    temporary.chmod(0o600)
    temporary.replace(path)


def diff_prices(
    previous: dict[str, dict[str, Any]], current: dict[str, dict[str, Any]]
) -> dict[str, list[dict[str, Any]]]:
    added = []
    removed = []
    changed = []
    for model in sorted(set(current) - set(previous)):
        added.append({"model": model, "after": current[model]})
    for model in sorted(set(previous) - set(current)):
        removed.append({"model": model, "before": previous[model]})
    for model in sorted(set(previous) & set(current)):
        if previous[model] != current[model]:
            changed.append({"model": model, "before": previous[model], "after": current[model]})
    return {"added": added, "removed": removed, "changed": changed}


def fetch_sub2api_accounts(db_config: dict[str, Any], timeout: int = 15) -> dict[str, dict[str, Any]]:
    try:
        import psycopg
    except ImportError as exc:
        raise RuntimeError("psycopg is not installed") from exc

    query = """
        SELECT id, name, platform, type, status, schedulable, model_mapping, base_url, api_key,
               priority
        FROM public.sub2api_price_monitor_accounts
        WHERE deleted_at IS NULL
        ORDER BY id
    """
    try:
        with psycopg.connect(
            host=db_config["host"],
            port=db_config["port"],
            dbname=db_config["name"],
            user=db_config["user"],
            password=db_config["password"],
            connect_timeout=timeout,
            sslmode=db_config.get("sslmode", "disable"),
        ) as connection:
            with connection.cursor() as cursor:
                cursor.execute(query)
                rows = cursor.fetchall()
    except Exception as exc:
        raise RuntimeError(f"Sub2API account query failed: {exc}") from exc

    accounts: dict[str, dict[str, Any]] = {}
    for (
        account_id,
        name,
        platform,
        account_type,
        status,
        schedulable,
        model_mapping,
        base_url,
        api_key,
        priority,
    ) in rows:
        mapping = model_mapping if isinstance(model_mapping, dict) else {}
        accounts[str(account_id)] = {
            "name": str(name),
            "platform": str(platform),
            "type": str(account_type),
            "status": str(status),
            "schedulable": bool(schedulable),
            "models": sorted(str(model) for model in mapping),
            "model_mapping": {
                str(local_model): str(upstream_model)
                for local_model, upstream_model in mapping.items()
            },
            "base_url": str(base_url or ""),
            "api_key": str(api_key or ""),
            "priority": int(priority),
            "managed_groups": [],
        }
    return dict(sorted(accounts.items()))


def fetch_manual_priority_locked_account_ids(
    db_config: dict[str, Any], timeout: int = 15
) -> set[str]:
    """Find accounts whose priority was changed from the admin web session."""
    try:
        import psycopg
    except ImportError as exc:
        raise RuntimeError("psycopg is not installed") from exc

    query = """
        SELECT DISTINCT extra->'params'->>'id'
        FROM public.audit_logs
        WHERE method = 'PUT'
          AND path = '/api/v1/admin/accounts/:id'
          AND auth_method IS DISTINCT FROM 'admin_api_key'
          AND request_body IS NOT NULL
          AND request_body::jsonb ? 'priority'
          AND extra->'params'->>'id' IS NOT NULL
    """
    try:
        with psycopg.connect(
            host=db_config["host"],
            port=db_config["port"],
            dbname=db_config["name"],
            user=db_config["user"],
            password=db_config["password"],
            connect_timeout=timeout,
            sslmode=db_config.get("sslmode", "disable"),
        ) as connection:
            with connection.cursor() as cursor:
                cursor.execute(query)
                rows = cursor.fetchall()
    except Exception as exc:
        raise RuntimeError(f"Sub2API manual priority audit query failed: {exc}") from exc

    return {str(account_id) for (account_id,) in rows if account_id is not None}


def fetch_sub2api_account_groups(
    db_config: dict[str, Any], timeout: int = 15
) -> dict[str, list[dict[str, Any]]]:
    try:
        import psycopg
    except ImportError as exc:
        raise RuntimeError("psycopg is not installed") from exc

    query = """
        SELECT ag.account_id, ag.group_id, g.name, g.rate_multiplier, g.status, ag.priority
        FROM public.account_groups AS ag
        JOIN public.groups AS g ON g.id = ag.group_id
        WHERE g.deleted_at IS NULL
        ORDER BY ag.account_id, ag.priority, ag.group_id
    """
    try:
        with psycopg.connect(
            host=db_config["host"],
            port=db_config["port"],
            dbname=db_config["name"],
            user=db_config["user"],
            password=db_config["password"],
            connect_timeout=timeout,
            sslmode=db_config.get("sslmode", "disable"),
        ) as connection:
            with connection.cursor() as cursor:
                cursor.execute(query)
                rows = cursor.fetchall()
    except Exception as exc:
        raise RuntimeError(f"Sub2API account group query failed: {exc}") from exc

    result: dict[str, list[dict[str, Any]]] = {}
    for account_id, group_id, group_name, rate_multiplier, group_status, group_priority in rows:
        result.setdefault(str(account_id), []).append(
            {
                "group_id": int(group_id),
                "name": str(group_name),
                "rate_multiplier": str(rate_multiplier),
                "status": str(group_status),
                "priority": int(group_priority),
            }
        )
    return result


def fetch_sub2api_groups(
    db_config: dict[str, Any], timeout: int = 15
) -> dict[int, dict[str, Any]]:
    try:
        import psycopg
    except ImportError as exc:
        raise RuntimeError("psycopg is not installed") from exc

    query = """
        SELECT id, name, rate_multiplier, status
        FROM public.groups
        WHERE deleted_at IS NULL
        ORDER BY id
    """
    try:
        with psycopg.connect(
            host=db_config["host"],
            port=db_config["port"],
            dbname=db_config["name"],
            user=db_config["user"],
            password=db_config["password"],
            connect_timeout=timeout,
            sslmode=db_config.get("sslmode", "disable"),
        ) as connection:
            with connection.cursor() as cursor:
                cursor.execute(query)
                rows = cursor.fetchall()
    except Exception as exc:
        raise RuntimeError(f"Sub2API group query failed: {exc}") from exc

    return {
        int(group_id): {
            "group_id": int(group_id),
            "name": str(name),
            "rate_multiplier": str(rate_multiplier),
            "status": str(status),
        }
        for group_id, name, rate_multiplier, status in rows
    }


def attach_sub2api_account_groups(
    accounts: dict[str, dict[str, Any]],
    account_groups: dict[str, list[dict[str, Any]]],
) -> None:
    for account_id, account in accounts.items():
        account["managed_groups"] = account_groups.get(account_id, [])


def build_upstream_metadata_rows(
    accounts: dict[str, dict[str, Any]],
    upstream_pricing: dict[str, dict[str, Any]],
) -> list[dict[str, Any]]:
    """Build display metadata while keeping API keys out of the database."""
    rows: list[dict[str, Any]] = []
    for account_id, account in accounts.items():
        try:
            numeric_account_id = int(account_id)
        except (TypeError, ValueError):
            continue
        pricing = upstream_pricing.get(str(account_id), {})
        if not isinstance(pricing, dict):
            pricing = {}
        host = normalize_host(str(pricing.get("host") or account.get("base_url", "")))
        if not host:
            continue
        groups = pricing.get("groups")
        if not isinstance(groups, dict):
            groups = {}
        group_names = [str(name).strip() for name in groups if str(name).strip()]
        ratio = None
        if len(groups) == 1:
            ratio = canonical_ratio(next(iter(groups.values())))
        rows.append(
            {
                "account_id": numeric_account_id,
                "account_name": str(account.get("name") or ""),
                "host": host,
                "api_name": str(pricing.get("api_name") or "").strip(),
                "group_name": ", ".join(group_names),
                "rate_multiplier": ratio,
                "status": str(pricing.get("status") or "unknown")[:64],
                "error": str(pricing.get("error") or ""),
            }
        )
    return rows


def sync_upstream_metadata(
    db_config: dict[str, Any],
    accounts: dict[str, dict[str, Any]],
    upstream_pricing: dict[str, dict[str, Any]],
    timeout: int = 15,
) -> None:
    """Persist the latest account-to-upstream view for the admin read API."""
    try:
        import psycopg
    except ImportError as exc:
        raise RuntimeError("psycopg is not installed") from exc

    rows = build_upstream_metadata_rows(accounts, upstream_pricing)
    account_ids: list[int] = []
    for account_id in accounts:
        try:
            account_ids.append(int(account_id))
        except (TypeError, ValueError):
            continue

    try:
        with psycopg.connect(
            host=db_config["host"],
            port=db_config["port"],
            dbname=db_config["name"],
            user=db_config["user"],
            password=db_config["password"],
            connect_timeout=timeout,
            sslmode=db_config.get("sslmode", "disable"),
        ) as connection:
            with connection.cursor() as cursor:
                if account_ids:
                    cursor.execute(
                        "DELETE FROM public.account_upstream_metadata WHERE account_id = ANY(%s)",
                        (account_ids,),
                    )
                for row in rows:
                    cursor.execute(
                        """
                        INSERT INTO public.upstream_sources (host)
                        VALUES (%s)
                        ON CONFLICT (host) DO UPDATE SET updated_at = NOW()
                        RETURNING id
                        """,
                        (row["host"],),
                    )
                    source_id = cursor.fetchone()[0]
                    cursor.execute(
                        """
                        INSERT INTO public.account_upstream_metadata (
                            upstream_source_id, account_id, api_name, group_name,
                            rate_multiplier, status, error, synced_at, updated_at
                        )
                        VALUES (%s, %s, %s, %s, %s, %s, %s, NOW(), NOW())
                        ON CONFLICT (upstream_source_id, account_id) DO UPDATE SET
                            api_name = EXCLUDED.api_name,
                            group_name = EXCLUDED.group_name,
                            rate_multiplier = EXCLUDED.rate_multiplier,
                            status = EXCLUDED.status,
                            error = EXCLUDED.error,
                            synced_at = EXCLUDED.synced_at,
                            updated_at = EXCLUDED.updated_at
                        """,
                        (
                            source_id,
                            row["account_id"],
                            row["api_name"],
                            row["group_name"],
                            row["rate_multiplier"],
                            row["status"],
                            row["error"],
                        ),
                    )
    except Exception as exc:
        raise RuntimeError(f"Sub2API upstream metadata sync failed: {exc}") from exc


def update_sub2api_account_name(
    account_id: str,
    name: str,
    api_base_url: str,
    admin_api_key: str,
    timeout: int = 15,
) -> None:
    if not api_base_url or not admin_api_key:
        raise RuntimeError("Sub2API admin API authorization is not configured")
    url = f"{api_base_url.rstrip('/')}/api/v1/admin/accounts/{account_id}"
    response = put_json(url, {"name": name}, {"x-api-key": admin_api_key}, timeout)
    if isinstance(response, dict) and response.get("code") not in (None, 0, "0"):
        raise RuntimeError(f"Sub2API account rename failed: {response.get('message') or response.get('code')}")
    verified = fetch_json(url, {"x-api-key": admin_api_key}, timeout)
    verified_data = verified.get("data") if isinstance(verified, dict) else None
    verified_name = verified_data.get("name") if isinstance(verified_data, dict) else None
    if verified_name != name:
        raise RuntimeError("Sub2API account rename response verification failed")


def update_sub2api_account_priority(
    account_id: str,
    priority: int,
    api_base_url: str,
    admin_api_key: str,
    timeout: int = 15,
) -> None:
    if not api_base_url or not admin_api_key:
        raise RuntimeError("Sub2API admin API authorization is not configured")
    url = f"{api_base_url.rstrip('/')}/api/v1/admin/accounts/{account_id}"
    response = put_json(url, {"priority": priority}, {"x-api-key": admin_api_key}, timeout)
    if isinstance(response, dict) and response.get("code") not in (None, 0, "0"):
        raise RuntimeError(
            f"Sub2API account priority update failed: {response.get('message') or response.get('code')}"
        )
    verified = fetch_json(url, {"x-api-key": admin_api_key}, timeout)
    verified_data = verified.get("data") if isinstance(verified, dict) else None
    verified_priority = verified_data.get("priority") if isinstance(verified_data, dict) else None
    if verified_priority != priority:
        raise RuntimeError("Sub2API account priority response verification failed")


def update_sub2api_account_groups(
    account_id: str,
    group_ids: list[int],
    api_base_url: str,
    admin_api_key: str,
    timeout: int = 15,
) -> None:
    if not api_base_url or not admin_api_key:
        raise RuntimeError("Sub2API admin API authorization is not configured")
    normalized_group_ids = [int(group_id) for group_id in group_ids]
    url = f"{api_base_url.rstrip('/')}/api/v1/admin/accounts/{account_id}"
    response = put_json(
        url,
        {"group_ids": normalized_group_ids},
        {"x-api-key": admin_api_key},
        timeout,
    )
    if isinstance(response, dict) and response.get("code") not in (None, 0, "0"):
        raise RuntimeError(
            f"Sub2API account group update failed: {response.get('message') or response.get('code')}"
        )
    verified = fetch_json(url, {"x-api-key": admin_api_key}, timeout)
    verified_data = verified.get("data") if isinstance(verified, dict) else None
    verified_group_ids: list[int] | None = None
    if isinstance(verified_data, dict):
        raw_group_ids = verified_data.get("group_ids")
        if isinstance(raw_group_ids, list):
            verified_group_ids = [int(group_id) for group_id in raw_group_ids]
        else:
            raw_account_groups = verified_data.get("account_groups")
            if isinstance(raw_account_groups, list):
                verified_group_ids = [
                    int(item["group_id"])
                    for item in raw_account_groups
                    if isinstance(item, dict) and item.get("group_id") is not None
                ]
    if verified_group_ids is None:
        if normalized_group_ids:
            raise RuntimeError("Sub2API account group response verification failed")
        return
    if set(verified_group_ids) != set(normalized_group_ids):
        raise RuntimeError("Sub2API account group response verification failed")


def update_sub2api_account_schedulable(
    account_id: str,
    schedulable: bool,
    api_base_url: str,
    admin_api_key: str,
    timeout: int = 15,
) -> None:
    if not api_base_url or not admin_api_key:
        raise RuntimeError("Sub2API admin API authorization is not configured")
    url = f"{api_base_url.rstrip('/')}/api/v1/admin/accounts/{account_id}/schedulable"
    response = post_json(url, {"schedulable": schedulable}, {"x-api-key": admin_api_key}, timeout)
    if isinstance(response, dict) and response.get("code") not in (None, 0, "0"):
        raise RuntimeError(
            f"Sub2API account schedulable update failed: {response.get('message') or response.get('code')}"
        )
    verified_data = response.get("data") if isinstance(response, dict) else None
    verified_schedulable = (
        verified_data.get("schedulable") if isinstance(verified_data, dict) else None
    )
    if verified_schedulable != schedulable:
        raise RuntimeError("Sub2API account schedulable response verification failed")


def fetch_sub2api_account_extras(
    api_base_url: str, admin_api_key: str, timeout: int = 15
) -> dict[str, dict[str, Any]]:
    """Read non-secret runtime metadata, including Sub2API billing probes."""
    if not api_base_url or not admin_api_key:
        return {}
    url = f"{api_base_url.rstrip('/')}/api/v1/admin/accounts?page=1&page_size=1000"
    response = fetch_json(url, {"x-api-key": admin_api_key}, timeout)
    data = response.get("data") if isinstance(response, dict) else None
    items = data.get("items") if isinstance(data, dict) else None
    if not isinstance(items, list):
        return {}
    result: dict[str, dict[str, Any]] = {}
    for item in items:
        if not isinstance(item, dict) or item.get("id") is None:
            continue
        extra = item.get("extra")
        if isinstance(extra, dict):
            result[str(item["id"])] = extra
    return result


def probe_sub2api_billing_pricing(
    account: dict[str, Any], extra: dict[str, Any] | None
) -> dict[str, Any] | None:
    """Use a fresh Sub2API billing probe as the authoritative account ratio."""
    if not isinstance(extra, dict):
        return None
    probe = extra.get("upstream_billing_probe")
    if not isinstance(probe, dict) or probe.get("status") != "ok":
        return None
    fresh_until = str(probe.get("fresh_until", "")).strip()
    try:
        if not fresh_until or datetime.fromisoformat(fresh_until.replace("Z", "+00:00")) <= datetime.now(timezone.utc):
            return None
    except ValueError:
        return None
    data = probe.get("data")
    if not isinstance(data, dict):
        return None
    canonical = None
    for key in (
        "effective_rate_multiplier",
        "resolved_rate_multiplier",
        "group_rate_multiplier",
    ):
        ratio = data.get(key)
        if isinstance(ratio, bool) or not isinstance(ratio, (int, float, str)):
            continue
        try:
            canonical = canonical_ratio(ratio)
        except (TypeError, ValueError):
            continue
        if canonical is not None:
            break
    if canonical is None:
        return None
    base_url = str(account.get("base_url", ""))
    mapping = account.get("model_mapping", {})
    if not isinstance(mapping, dict):
        mapping = {}
    return {
        "name": account.get("name", ""),
        "host": upstream_host(base_url),
        "api_name": str(
            data.get("api_name")
            or data.get("name")
            or data.get("key_name")
            or data.get("title")
            or data.get("label")
            or ""
        ).strip(),
        "models": sorted(str(model) for model in mapping),
        "status": "authorized",
        "scope": "account_authorized",
        "source_url": "sub2api_billing_probe",
        "pricing_version": None,
        "prices": {},
        "groups": {"Sub2API upstream billing probe": canonical},
        "error": "",
    }


def _safety_candidate_matches(
    previous: dict[str, Any], current: dict[str, Any]
) -> bool:
    return {
        key: value for key, value in previous.items() if key != "account_name"
    } == {key: value for key, value in current.items() if key != "account_name"}


def _ordered_group_ids(
    current_group_ids: list[int], group_order: list[int], extra_group_ids: list[int]
) -> list[int]:
    ordered: list[int] = []
    for group_id in group_order + current_group_ids + extra_group_ids:
        if group_id not in ordered:
            ordered.append(group_id)
    return ordered


def prune_deleted_safety_groups(
    detached_groups: dict[str, dict[str, Any]], active_group_ids: set[int]
) -> dict[str, dict[str, Any]]:
    """Drop persisted safety bindings for groups deleted from Sub2API."""
    sanitized: dict[str, dict[str, Any]] = {}

    def group_is_active(raw_group_id: Any) -> bool:
        try:
            return int(raw_group_id) in active_group_ids
        except (TypeError, ValueError):
            return False

    for account_id, raw_state in detached_groups.items():
        if not isinstance(raw_state, dict):
            continue
        state = dict(raw_state)
        raw_groups = state.get("groups", [])
        if isinstance(raw_groups, list):
            state["groups"] = [
                group
                for group in raw_groups
                if isinstance(group, dict)
                and group_is_active(group.get("group_id"))
            ]
        raw_group_order = state.get("group_order", [])
        if isinstance(raw_group_order, list):
            state["group_order"] = [
                int(group_id)
                for group_id in raw_group_order
                if group_is_active(group_id)
            ]
        sanitized[str(account_id)] = state
    return sanitized


def plan_account_safety_stops(
    accounts: dict[str, dict[str, Any]],
    upstream_pricing: dict[str, dict[str, Any]],
    previous_candidates: dict[str, dict[str, Any]],
    previous_detached_groups: dict[str, dict[str, Any]] | None = None,
    excluded_account_ids: set[str] | None = None,
) -> tuple[dict[str, dict[str, Any]], list[dict[str, Any]]]:
    """Plan per-group detaches/restores without changing account schedulability."""
    previous_detached_groups = previous_detached_groups or {}
    excluded_account_ids = excluded_account_ids or set()
    candidates: dict[str, dict[str, Any]] = {}
    stable: list[dict[str, Any]] = []
    for account_id, account in sorted(accounts.items()):
        if str(account_id).strip() in excluded_account_ids:
            continue
        if is_account_automation_excluded(account.get("name")):
            continue
        pricing = upstream_pricing.get(account_id, {})
        if pricing.get("scope") != "account_authorized":
            continue
        upstream_ratio = single_group_ratio(pricing)
        if upstream_ratio is None:
            continue
        try:
            upstream_value = Decimal(upstream_ratio)
        except Exception:
            continue

        current_groups = [
            group
            for group in account.get("managed_groups", [])
            if isinstance(group, dict) and group.get("group_id") is not None
        ]
        current_group_ids = [int(group["group_id"]) for group in current_groups]
        previous_detached = previous_detached_groups.get(account_id, {})
        if not isinstance(previous_detached, dict):
            previous_detached = {}
        stored_groups = previous_detached.get("groups", [])
        if not isinstance(stored_groups, list):
            stored_groups = []
        stored_by_id = {
            int(group["group_id"]): group
            for group in stored_groups
            if isinstance(group, dict) and group.get("group_id") is not None
        }
        raw_group_order = previous_detached.get("group_order", [])
        previous_group_order = (
            [int(group_id) for group_id in raw_group_order]
            if isinstance(raw_group_order, list)
            else []
        )
        group_order = _ordered_group_ids(
            current_group_ids,
            previous_group_order,
            list(stored_by_id),
        )

        violations: list[dict[str, Any]] = []
        for group in current_groups:
            if (
                is_image2_managed_group(group)
                or is_safety_excluded_group(group)
                or group.get("status") != "active"
            ):
                continue
            managed_ratio = canonical_ratio(group.get("rate_multiplier"))
            if managed_ratio is None or upstream_value <= Decimal(managed_ratio):
                continue
            violations.append(
                {
                    "group_id": int(group["group_id"]),
                    "group_name": group.get("name", group["group_id"]),
                    "managed_ratio": managed_ratio,
                    "priority": int(group.get("priority", 0)),
                }
            )

        restorations: list[dict[str, Any]] = []
        for group_id in group_order:
            stored_group = stored_by_id.get(group_id)
            if stored_group is None or group_id in current_group_ids:
                continue
            if is_image2_managed_group(stored_group):
                continue
            if stored_group.get("status") != "active":
                continue
            managed_ratio = canonical_ratio(stored_group.get("managed_ratio"))
            if not is_safety_excluded_group(stored_group) and (
                managed_ratio is None or upstream_value > Decimal(managed_ratio)
            ):
                continue
            restorations.append(
                {
                    "group_id": group_id,
                    "group_name": stored_group.get("name", group_id),
                    "managed_ratio": managed_ratio,
                    "priority": int(stored_group.get("priority", 0)),
                }
            )

        detach_group_ids = [item["group_id"] for item in violations]
        restore_group_ids = [item["group_id"] for item in restorations]
        if not detach_group_ids and not restore_group_ids:
            continue

        candidate = {
            "account_name": account.get("name", account_id),
            "upstream_ratio": upstream_ratio,
            "detach_group_ids": detach_group_ids,
            "restore_group_ids": restore_group_ids,
            "violations": violations,
            "restorations": restorations,
            "group_order": group_order,
        }
        candidates[account_id] = candidate
        previous = previous_candidates.get(account_id, {})
        if isinstance(previous, dict) and _safety_candidate_matches(previous, candidate):
            stable.append({"account_id": account_id, **candidate})
    return candidates, stable


def plan_account_auto_groups(
    accounts: dict[str, dict[str, Any]],
    groups: dict[int, dict[str, Any]],
    upstream_pricing: dict[str, dict[str, Any]],
    previous_candidates: dict[str, dict[str, Any]],
    previous_ownership: dict[str, dict[str, Any]] | None,
    category_group_ids: dict[str, list[int]],
    minimum_profit_percent: Decimal,
    excluded_account_ids: set[str] | None = None,
) -> tuple[
    dict[str, dict[str, Any]],
    list[dict[str, Any]],
    dict[str, dict[str, Any]],
]:
    """Plan ratio-eligible GPT/Plus group changes and track monitor-owned bindings."""
    candidates: dict[str, dict[str, Any]] = {}
    stable: list[dict[str, Any]] = []
    previous_ownership = previous_ownership or {}
    excluded_account_ids = excluded_account_ids or set()
    ownership: dict[str, dict[str, Any]] = {
        str(account_id): {
            "owned_group_ids": [int(group_id) for group_id in state.get("owned_group_ids", [])],
            "suppressed_group_ids": [
                int(group_id) for group_id in state.get("suppressed_group_ids", [])
            ],
        }
        for account_id, state in previous_ownership.items()
        if str(account_id) in accounts and isinstance(state, dict)
    }

    for account_id, account in sorted(accounts.items()):
        if str(account_id).strip() in excluded_account_ids:
            continue
        if is_domestic_model_account(account):
            continue
        category = account_priority_category(account.get("name"))
        target_group_ids = category_group_ids.get(category, [])
        if category not in {"gpt", "plus"} or not target_group_ids:
            continue
        pricing = upstream_pricing.get(account_id, {})
        if pricing.get("scope") != "account_authorized":
            continue
        upstream_ratio = single_group_ratio(pricing)
        if upstream_ratio is None:
            continue
        try:
            upstream_value = Decimal(upstream_ratio)
        except Exception:
            continue

        current_group_ids = [
            int(group["group_id"])
            for group in account.get("managed_groups", [])
            if isinstance(group, dict) and group.get("group_id") is not None
        ]
        previous_account_ownership = previous_ownership.get(account_id, {})
        if not isinstance(previous_account_ownership, dict):
            previous_account_ownership = {}
        owned_group_ids = {
            int(group_id)
            for group_id in previous_account_ownership.get("owned_group_ids", [])
        }
        suppressed_group_ids = {
            int(group_id)
            for group_id in previous_account_ownership.get("suppressed_group_ids", [])
        }

        # A monitor-owned binding removed outside this process is treated as an
        # administrator override and will not be re-added automatically.
        externally_removed = owned_group_ids - set(current_group_ids)
        owned_group_ids -= externally_removed
        suppressed_group_ids |= externally_removed
        suppressed_group_ids -= set(current_group_ids)

        additions: list[dict[str, Any]] = []
        removals: list[dict[str, Any]] = []
        for group_id in target_group_ids:
            group = groups.get(group_id)
            if (
                not isinstance(group, dict)
                or group.get("status") != "active"
                or is_image2_managed_group(group)
            ):
                continue
            managed_ratio = canonical_ratio(group.get("rate_multiplier"))
            if managed_ratio is None:
                continue
            managed_value = Decimal(managed_ratio)
            if managed_value <= 0:
                continue
            profit_percent = (
                (managed_value - upstream_value) / managed_value * Decimal("100")
            )
            detail = {
                "group_id": group_id,
                "group_name": group.get("name", group_id),
                "managed_ratio": managed_ratio,
                "profit_percent": format(profit_percent.quantize(Decimal("0.01")), "f"),
            }
            # A group is eligible whenever its public multiplier covers the
            # upstream multiplier. No additional profit margin is required.
            eligible = upstream_value <= managed_value
            if (
                group_id not in current_group_ids
                and group_id not in suppressed_group_ids
                and eligible
            ):
                additions.append(detail)
            elif (
                group_id in current_group_ids
                and group_id in owned_group_ids
                and not eligible
            ):
                removals.append(detail)

        if owned_group_ids or suppressed_group_ids:
            ownership[account_id] = {
                "owned_group_ids": sorted(owned_group_ids),
                "suppressed_group_ids": sorted(suppressed_group_ids),
            }
        else:
            ownership.pop(account_id, None)
        if not additions and not removals:
            continue

        candidate = {
            "account_name": account.get("name", account_id),
            "category": category,
            "upstream_ratio": upstream_ratio,
            "minimum_profit_percent": format(minimum_profit_percent, "f"),
            "add_group_ids": [item["group_id"] for item in additions],
            "remove_group_ids": [item["group_id"] for item in removals],
            "additions": additions,
            "removals": removals,
        }
        candidates[account_id] = candidate
        previous = previous_candidates.get(account_id, {})
        if isinstance(previous, dict) and _safety_candidate_matches(previous, candidate):
            stable.append({"account_id": account_id, **candidate})

    return candidates, stable, ownership


def plan_account_renames(
    accounts: dict[str, dict[str, Any]],
    upstream_pricing: dict[str, dict[str, Any]],
    previous_candidates: dict[str, dict[str, str]],
    immediate_account_ids: set[str] | None = None,
) -> tuple[dict[str, dict[str, str]], list[dict[str, str]]]:
    candidates: dict[str, dict[str, str]] = {}
    stable: list[dict[str, str]] = []
    immediate_account_ids = immediate_account_ids or set()
    for account_id, account in accounts.items():
        if is_account_automation_excluded(account.get("name")) or is_guomo_collection_account(account):
            continue
        pricing = upstream_pricing.get(account_id, {})
        if pricing.get("scope") != "account_authorized":
            continue
        desired_name = account_name_for_ratio(account.get("name"), single_group_ratio(pricing))
        current_name = str(account.get("name", ""))
        if not desired_name or desired_name == current_name:
            continue
        candidate = {
            "current_name": current_name,
            "desired_name": desired_name,
            "ratio": str(single_group_ratio(pricing)),
        }
        candidates[account_id] = candidate
        previous = previous_candidates.get(account_id, {})
        if account_id in immediate_account_ids or (
            previous.get("current_name") == current_name
            and previous.get("desired_name") == desired_name
        ):
            stable.append({"account_id": account_id, **candidate})
    return candidates, stable


def plan_account_priorities(
    accounts: dict[str, dict[str, Any]],
    upstream_pricing: dict[str, dict[str, Any]],
    previous_candidates: dict[str, dict[str, Any]],
    immediate_categories: set[str] | None = None,
    manual_priority_locked_ids: set[str] | None = None,
) -> tuple[dict[str, dict[str, Any]], list[dict[str, Any]]]:
    ratios_by_account: dict[str, tuple[str, str]] = {}
    immediate_categories = immediate_categories or set()
    manual_priority_locked_ids = manual_priority_locked_ids or set()
    for account_id in accounts:
        account = accounts[account_id]
        if not account.get("schedulable", False):
            continue
        if account_id in manual_priority_locked_ids:
            continue
        if is_domestic_model_account(account):
            continue
        account_name = account.get("name")
        category = account_priority_category(account_name)
        if is_account_automation_excluded(account_name) or category == "pro":
            continue
        pricing = upstream_pricing.get(account_id, {})
        if pricing.get("scope") != "account_authorized":
            continue
        ratio = single_group_ratio(pricing)
        if ratio is not None:
            ratios_by_account[account_id] = (category, ratio)

    ratios_by_category: dict[str, set[str]] = {}
    for category, ratio in ratios_by_account.values():
        ratios_by_category.setdefault(category, set()).add(ratio)
    priority_by_category_and_ratio = {
        (category, ratio): index + 1
        for category, ratios in ratios_by_category.items()
        for index, ratio in enumerate(sorted(ratios, key=Decimal))
    }
    candidates: dict[str, dict[str, Any]] = {}
    stable: list[dict[str, Any]] = []
    for account_id, (category, ratio) in sorted(ratios_by_account.items()):
        desired_priority = priority_by_category_and_ratio[(category, ratio)]
        current_priority = accounts[account_id].get("priority")
        candidate = {
            "category": category,
            "ratio": ratio,
            "current_priority": current_priority,
            "desired_priority": desired_priority,
        }
        candidates[account_id] = candidate
        previous = previous_candidates.get(account_id, {})
        if category in immediate_categories or (
            previous.get("category") == category
            and previous.get("ratio") == ratio
            and previous.get("desired_priority") == desired_priority
            and current_priority != desired_priority
        ):
            stable.append({
                "account_id": account_id,
                "account_name": accounts[account_id].get("name", account_id),
                **candidate,
            })
    return candidates, stable


def public_account_snapshot(accounts: dict[str, dict[str, Any]]) -> dict[str, dict[str, Any]]:
    fields = ("name", "platform", "type", "status", "schedulable", "models")
    return {
        account_id: {field: account.get(field) for field in fields}
        for account_id, account in accounts.items()
    }


def upstream_host(base_url: str) -> str:
    parsed = urlsplit(base_url)
    return parsed.netloc or base_url.split("/", 1)[0]


def normalize_host(value: str) -> str:
    candidate = value.strip()
    if not candidate:
        return ""
    if "://" not in candidate:
        candidate = f"https://{candidate}"
    parsed = urlsplit(candidate)
    return parsed.netloc.lower()


def upstream_origin(base_url: str) -> str:
    parsed = urlsplit(base_url.rstrip("/"))
    if not parsed.scheme or not parsed.netloc:
        return ""
    return urlunsplit((parsed.scheme, parsed.netloc, "", "", ""))


def append_query(url: str, **params: int) -> str:
    parsed = urlsplit(url)
    query = dict(parse_qsl(parsed.query, keep_blank_values=True))
    query.update({key: str(value) for key, value in params.items()})
    return urlunsplit(
        (parsed.scheme, parsed.netloc, parsed.path, urlencode(query), parsed.fragment)
    )


def append_pagination_query(
    url: str, page: int, page_size: int, config: dict[str, str]
) -> str:
    page_param = config.get("keys_page_param", "page").strip() or "page"
    page_size_param = config.get("keys_page_size_param", "page_size").strip() or "page_size"
    return append_query(url, **{page_param: page, page_size_param: page_size})


def normalize_upstream_auth_config(raw: Any) -> dict[str, dict[str, str]]:
    if not isinstance(raw, dict):
        return {}
    entries = raw.get("upstreams", raw)
    if isinstance(entries, list):
        iterable = []
        for entry in entries:
            if isinstance(entry, dict):
                key = entry.get("host") or entry.get("base_url") or entry.get("login_url")
                iterable.append((key, entry))
    elif isinstance(entries, dict):
        iterable = list(entries.items())
    else:
        return {}

    result: dict[str, dict[str, str]] = {}
    for key, entry in iterable:
        if not isinstance(entry, dict):
            if isinstance(entry, str):
                entry = {"access_token": entry}
            else:
                continue
        host = normalize_host(
            str(entry.get("host") or entry.get("base_url") or entry.get("login_url") or key or "")
        )
        if not host:
            continue
        config = {
            str(config_key): str(config_value)
            for config_key, config_value in entry.items()
            if config_value is not None and isinstance(config_value, (str, int, float))
        }
        result[host] = config
    return result


def normalize_upstream_account_auth_config(raw: Any) -> dict[str, dict[str, str]]:
    """Normalize credentials that override the default login for one account."""
    if not isinstance(raw, dict):
        return {}
    entries = raw.get("account_overrides", {})
    if isinstance(entries, list):
        iterable = []
        for entry in entries:
            if isinstance(entry, dict):
                iterable.append((entry.get("account_id"), entry))
    elif isinstance(entries, dict):
        iterable = list(entries.items())
    else:
        return {}

    result: dict[str, dict[str, str]] = {}
    for key, entry in iterable:
        if not isinstance(entry, dict):
            continue
        account_id = str(entry.get("account_id") or key or "").strip()
        host = normalize_host(
            str(entry.get("host") or entry.get("base_url") or entry.get("login_url") or "")
        )
        if not account_id or not host:
            continue
        result[account_id] = {
            str(config_key): str(config_value)
            for config_key, config_value in entry.items()
            if config_value is not None and isinstance(config_value, (str, int, float))
        }
    return result


def load_upstream_auth_raw() -> Any:
    raw_config = os.getenv("UPSTREAM_AUTH_CONFIG_JSON", "").strip()
    config_file = os.getenv("UPSTREAM_AUTH_CONFIG_FILE", "").strip()
    config_b64 = os.getenv("UPSTREAM_AUTH_CONFIG_B64", "").strip()
    if not raw_config and config_file:
        try:
            raw_config = Path(config_file).read_text(encoding="utf-8")
        except OSError as exc:
            LOG.warning("Cannot read UPSTREAM_AUTH_CONFIG_FILE: %s", exc)
    if not raw_config and config_b64:
        try:
            import base64

            raw_config = base64.b64decode(config_b64).decode("utf-8")
        except (ValueError, UnicodeDecodeError) as exc:
            LOG.warning("Cannot decode UPSTREAM_AUTH_CONFIG_B64: %s", exc)
    if not raw_config:
        return {}
    try:
        return json.loads(raw_config)
    except json.JSONDecodeError as exc:
        LOG.warning("UPSTREAM_AUTH_CONFIG is not valid JSON: %s", exc)
        return {}


def load_upstream_auth_config() -> dict[str, dict[str, str]]:
    return normalize_upstream_auth_config(load_upstream_auth_raw())


def load_upstream_account_auth_config() -> dict[str, dict[str, str]]:
    return normalize_upstream_account_auth_config(load_upstream_auth_raw())


def normalize_upstream_account_rate_override_config(raw: Any) -> dict[str, dict[str, str]]:
    """Load one explicit, verified rate override per Sub2API account."""
    if not isinstance(raw, dict):
        return {}
    entries = raw.get("account_rate_overrides", {})
    if isinstance(entries, list):
        iterable = []
        for entry in entries:
            if isinstance(entry, dict):
                iterable.append((entry.get("account_id"), entry))
    elif isinstance(entries, dict):
        iterable = list(entries.items())
    else:
        return {}

    result: dict[str, dict[str, str]] = {}
    for key, entry in iterable:
        if not isinstance(entry, dict):
            continue
        account_id = str(entry.get("account_id") or key or "").strip()
        host = normalize_host(str(entry.get("host") or entry.get("base_url") or ""))
        key_suffix = str(entry.get("key_suffix") or "").strip().lower()
        group = str(entry.get("group") or "").strip()
        ratio = canonical_ratio(entry.get("rate_multiplier"))
        if not account_id or not host or len(key_suffix) < 4 or not group or ratio is None:
            continue
        result[account_id] = {
            "host": host,
            "key_suffix": key_suffix[-4:],
            "group": group,
            "rate_multiplier": ratio,
        }
    return result


def load_upstream_account_rate_override_config() -> dict[str, dict[str, str]]:
    return normalize_upstream_account_rate_override_config(load_upstream_auth_raw())


def load_browser_pricing_snapshot(path: Path | None) -> dict[str, list[dict[str, Any]]]:
    if path is None:
        return {}
    raw = load_json(path, {})
    if not isinstance(raw, dict):
        return {}
    version = raw.get("version", BROWSER_PRICING_SNAPSHOT_VERSION)
    if version != BROWSER_PRICING_SNAPSHOT_VERSION:
        LOG.warning("Unsupported browser pricing snapshot version: %r", version)
        return {}

    entries = raw.get("entries", [])
    if not isinstance(entries, list):
        return {}
    result: dict[str, list[dict[str, Any]]] = {}
    for entry in entries:
        if not isinstance(entry, dict):
            continue
        host = normalize_host(str(entry.get("host", "")))
        suffix = str(entry.get("key_suffix", "")).strip().lower()
        group = str(entry.get("group", "")).strip()
        ratio = entry.get("ratio")
        if not host or len(suffix) < 4 or not group or ratio in (None, ""):
            continue
        normalized = {
            "key_suffix": suffix[-4:],
            "group": group,
            "ratio": ratio,
            "api_name": extract_api_name(entry),
        }
        result.setdefault(host, []).append(normalized)
    return result


def upstream_price_urls(base_url: str, paths: tuple[str, ...]) -> list[str]:
    parsed = urlsplit(base_url.rstrip("/"))
    if not parsed.scheme or not parsed.netloc:
        return []
    origin = urlunsplit((parsed.scheme, parsed.netloc, "", "", ""))
    base = urlunsplit((parsed.scheme, parsed.netloc, parsed.path.rstrip("/"), "", ""))
    urls = []
    for path in paths:
        normalized_path = "/" + path.lstrip("/")
        for prefix in (base, origin):
            url = prefix + normalized_path
            if url not in urls:
                urls.append(url)
    return urls


def _scalar(value: Any) -> Any:
    if isinstance(value, (str, int, float)) and not isinstance(value, bool):
        return value
    return None


def normalize_group_ratios(raw: Any) -> dict[str, float | int]:
    if not isinstance(raw, dict):
        return {}
    result: dict[str, float | int] = {}
    for group, ratio in raw.items():
        if isinstance(ratio, bool) or not isinstance(ratio, (int, float)):
            continue
        result[str(group)] = ratio
    return dict(sorted(result.items()))


def normalize_upstream_prices(raw: Any) -> dict[str, Any]:
    if not isinstance(raw, dict):
        return {"models": {}, "groups": {}, "pricing_version": None, "scope": "unverified"}

    entries = raw.get("data")
    if isinstance(entries, dict):
        entries = [
            {"model_name": str(model), **(entry if isinstance(entry, dict) else {"model_price": entry})}
            for model, entry in entries.items()
        ]
    elif not isinstance(entries, list):
        entries = raw.get("models")
    if not isinstance(entries, list):
        entries = []

    models: dict[str, dict[str, Any]] = {}
    for entry in entries:
        if not isinstance(entry, dict):
            continue
        model = entry.get("model_name") or entry.get("model") or entry.get("id")
        if not model:
            continue
        fields = {
            field: _scalar(entry.get(field))
            for field in UPSTREAM_PRICE_FIELDS
            if _scalar(entry.get(field)) is not None
        }
        if fields:
            models[str(model)] = dict(sorted(fields.items()))

    reseller = raw.get("reseller_pricing")
    if isinstance(reseller, dict):
        if reseller.get("applied") is True or reseller.get("source") not in {
            None,
            "platform_default",
        }:
            scope = "account_authorized"
        else:
            scope = "public_default"
    else:
        scope = "unverified"
    return {
        "models": dict(sorted(models.items())),
        "groups": normalize_group_ratios(raw.get("group_ratio")),
        "pricing_version": _scalar(raw.get("pricing_version")),
        "scope": scope,
    }


def extract_access_token(raw: Any, path: str = "") -> str:
    if not isinstance(raw, dict):
        return ""
    if path:
        value = extract_response_path(raw, path)
        if isinstance(value, (str, int, float)) and not isinstance(value, bool):
            return str(value).strip()
        return ""
    candidates = [raw.get("access_token"), raw.get("token")]
    data = raw.get("data")
    if isinstance(data, dict):
        candidates.extend([data.get("access_token"), data.get("token")])
    for candidate in candidates:
        if isinstance(candidate, str) and candidate.strip():
            return candidate.strip()
    return ""


def extract_auth_tokens(raw: Any) -> dict[str, Any]:
    if not isinstance(raw, dict):
        return {}
    data = raw.get("data")
    payload = data if isinstance(data, dict) else raw
    access_token = payload.get("access_token") or payload.get("token")
    refresh_token = payload.get("refresh_token")
    expires_in = payload.get("expires_in")
    access_expires_at = payload.get("access_expires_at")
    result: dict[str, Any] = {}
    if isinstance(access_token, str) and access_token.strip():
        result["access_token"] = access_token.strip()
    if isinstance(refresh_token, str) and refresh_token.strip():
        result["refresh_token"] = refresh_token.strip()
    if isinstance(expires_in, (int, float)) and not isinstance(expires_in, bool):
        result["expires_in"] = expires_in
    if isinstance(access_expires_at, (int, float)) and not isinstance(access_expires_at, bool):
        result["access_expires_at"] = datetime.fromtimestamp(
            float(access_expires_at), tz=timezone.utc
        ).replace(microsecond=0).isoformat()
    elif isinstance(access_expires_at, str) and access_expires_at.strip():
        result["access_expires_at"] = access_expires_at.strip()
    elif isinstance(expires_in, (int, float)) and not isinstance(expires_in, bool):
        result["access_expires_at"] = (
            datetime.now(timezone.utc) + timedelta(seconds=float(expires_in))
        ).replace(microsecond=0).isoformat()
    return result


def jwt_expiration(token: str) -> int | None:
    try:
        encoded_payload = token.split(".")[1]
        encoded_payload += "=" * (-len(encoded_payload) % 4)
        payload = json.loads(base64.urlsafe_b64decode(encoded_payload).decode("utf-8"))
        expiration = payload.get("exp")
        return int(expiration) if isinstance(expiration, (int, float)) else None
    except (IndexError, ValueError, UnicodeDecodeError, json.JSONDecodeError):
        return None


def access_token_needs_refresh(token: str, leeway_seconds: int = 300) -> bool:
    expiration = jwt_expiration(token)
    return expiration is not None and expiration <= int(time.time()) + leeway_seconds


def access_token_state_needs_refresh(
    token: str, state_entry: dict[str, Any], leeway_seconds: int = 300
) -> bool:
    if access_token_needs_refresh(token, leeway_seconds):
        return True
    raw_expiration = state_entry.get("access_expires_at")
    if not isinstance(raw_expiration, str) or not raw_expiration.strip():
        return False
    try:
        expiration = datetime.fromisoformat(raw_expiration.replace("Z", "+00:00"))
    except ValueError:
        return False
    return expiration <= datetime.now(timezone.utc).replace(microsecond=0) + timedelta(
        seconds=leeway_seconds
    )


def can_refresh_upstream_access_token(
    config: dict[str, str], token_state: dict[str, Any], state_key: str
) -> bool:
    entry = token_state.get("upstreams", {}).get(state_key, {})
    if not isinstance(entry, dict):
        entry = {}
    if config.get("refresh_token") or entry.get("refresh_token"):
        return True
    return (
        config.get("cookie_refresh", "").strip().lower() in {"1", "true", "yes", "on"}
        and bool(config.get("session_cookie") or entry.get("session_cookie"))
    )


def normalize_upstream_token_state(raw: Any) -> dict[str, Any]:
    if not isinstance(raw, dict) or raw.get("version") != UPSTREAM_TOKEN_STATE_VERSION:
        return {"version": UPSTREAM_TOKEN_STATE_VERSION, "upstreams": {}}
    upstreams = raw.get("upstreams")
    if not isinstance(upstreams, dict):
        upstreams = {}
    return {"version": UPSTREAM_TOKEN_STATE_VERSION, "upstreams": upstreams}


def refresh_upstream_access_token(
    config: dict[str, str],
    base_url: str,
    refresh_token: str,
    timeout: int = 15,
) -> dict[str, Any]:
    origin = upstream_origin(config.get("base_url") or base_url)
    path = config.get("refresh_path", DEFAULT_UPSTREAM_AUTH_REFRESH_PATH).strip()
    refresh_url = config.get("refresh_url") or (origin + "/" + path.lstrip("/"))
    tokens = extract_auth_tokens(
        post_json(refresh_url, {"refresh_token": refresh_token}, timeout=timeout)
    )
    if not tokens.get("access_token") or not tokens.get("refresh_token"):
        raise RuntimeError("upstream token refresh did not return rotated tokens")
    return tokens


def refresh_upstream_access_token_with_cookie(
    config: dict[str, str],
    base_url: str,
    session_cookie: str,
    timeout: int = 15,
) -> tuple[dict[str, Any], str]:
    origin = upstream_origin(config.get("base_url") or base_url)
    path = config.get("cookie_refresh_path", "").strip()
    refresh_url = config.get("cookie_refresh_url") or (
        origin + "/" + path.lstrip("/")
    )
    if not refresh_url:
        raise RuntimeError("upstream cookie refresh URL is not configured")
    cookie_name = config.get("session_cookie_name", "session").strip() or "session"
    response = post_json(
        refresh_url,
        {},
        {"Cookie": session_cookie if "=" in session_cookie else f"{cookie_name}={session_cookie}"},
        timeout=timeout,
        return_headers=True,
    )
    raw, response_headers = response
    token = extract_access_token(raw, config.get("token_path", "").strip())
    if not token:
        raise RuntimeError("upstream cookie refresh did not return access_token")
    refreshed_cookie = session_cookie
    for set_cookie in response_headers.get_all("Set-Cookie", []):
        cookie = set_cookie.split(";", 1)[0].strip()
        if cookie.startswith(f"{cookie_name}="):
            refreshed_cookie = cookie
            break
    tokens = extract_auth_tokens(raw)
    tokens["access_token"] = token
    return tokens, refreshed_cookie


def auth_headers(token: str, config: dict[str, str]) -> dict[str, str]:
    header_name = config.get("token_header", "Authorization").strip() or "Authorization"
    if header_name.lower() == "authorization":
        prefix = config.get("token_prefix", "Bearer").strip()
        value = f"{prefix} {token}".strip() if prefix else token
    else:
        value = token
    headers = {header_name: value}
    session_cookie = config.get("session_cookie", "").strip()
    if session_cookie:
        cookie_name = config.get("session_cookie_name", "session").strip() or "session"
        headers["Cookie"] = (
            session_cookie if "=" in session_cookie else f"{cookie_name}={session_cookie}"
        )
    return headers


def extract_response_path(raw: Any, path: str) -> Any:
    value = raw
    for field in (part.strip() for part in path.split(".") if part.strip()):
        if not isinstance(value, dict) or field not in value:
            return None
        value = value[field]
    return value


def login_upstream(
    config: dict[str, str],
    base_url: str,
    timeout: int = 15,
    token_state: dict[str, Any] | None = None,
    token_state_path: Path | None = None,
    force_refresh: bool = False,
    token_state_key: str | None = None,
) -> str:
    host = normalize_host(config.get("base_url") or base_url)
    state_key = token_state_key or host
    state_entry: dict[str, Any] = {}
    if token_state is not None:
        upstreams = token_state.setdefault("upstreams", {})
        existing = upstreams.get(state_key)
        if isinstance(existing, dict):
            state_entry = existing

    if not config.get("session_cookie") and state_entry.get("session_cookie"):
        config["session_cookie"] = str(state_entry["session_cookie"])

    static_token = state_entry.get("access_token") or config.get("access_token") or config.get("token")
    refresh_token = state_entry.get("refresh_token") or config.get("refresh_token")
    session_cookie = state_entry.get("session_cookie") or config.get("session_cookie")
    cookie_refresh_enabled = (
        config.get("cookie_refresh", "").strip().lower() in {"1", "true", "yes", "on"}
    )
    if session_cookie and cookie_refresh_enabled and (
        force_refresh
        or not static_token
        or access_token_state_needs_refresh(str(static_token), state_entry)
    ):
        tokens, refreshed_cookie = refresh_upstream_access_token_with_cookie(
            config, base_url, str(session_cookie), timeout
        )
        if token_state is not None:
            state = {
                "access_token": tokens["access_token"],
                "session_cookie": refreshed_cookie,
                "updated_at": utc_now(),
            }
            if tokens.get("refresh_token"):
                state["refresh_token"] = tokens["refresh_token"]
            if tokens.get("access_expires_at"):
                state["access_expires_at"] = tokens["access_expires_at"]
            token_state["upstreams"][state_key] = state
            if token_state_path is not None:
                write_private_json_atomic(token_state_path, token_state)
        return str(tokens["access_token"])
    if refresh_token and (
        force_refresh or not static_token or access_token_needs_refresh(str(static_token))
    ):
        tokens = refresh_upstream_access_token(config, base_url, str(refresh_token), timeout)
        if token_state is not None:
            state = {
                "access_token": tokens["access_token"],
                "refresh_token": tokens["refresh_token"],
                "updated_at": utc_now(),
            }
            if tokens.get("access_expires_at"):
                state["access_expires_at"] = tokens["access_expires_at"]
            token_state["upstreams"][state_key] = state
            if token_state_path is not None:
                write_private_json_atomic(token_state_path, token_state)
        return str(tokens["access_token"])
    if static_token:
        return str(static_token).strip()

    email = config.get("email") or config.get("username")
    password = config.get("password")
    if not email or not password:
        raise RuntimeError("missing upstream monitoring email/password or access token")

    origin = upstream_origin(config.get("base_url") or base_url)
    path = config.get("login_path", DEFAULT_UPSTREAM_AUTH_LOGIN_PATH).strip()
    login_url = config.get("login_url") or (origin + "/" + path.lstrip("/"))
    login_field = config.get("login_field", "email").strip() or "email"
    session_cookie_name = config.get("session_cookie_name", "").strip()
    login_response = post_json(
        login_url,
        {login_field: email, "password": password},
        timeout=timeout,
        return_headers=bool(session_cookie_name),
    )
    response_headers = None
    if isinstance(login_response, tuple) and len(login_response) == 2:
        response, response_headers = login_response
    else:
        response = login_response
    if session_cookie_name:
        set_cookie_headers = (
            response_headers.get_all("Set-Cookie", []) if response_headers is not None else []
        )
        for set_cookie in set_cookie_headers:
            cookie = set_cookie.split(";", 1)[0].strip()
            if cookie.startswith(f"{session_cookie_name}="):
                config["session_cookie"] = cookie
                break
        if not config.get("session_cookie"):
            raise RuntimeError("upstream login did not return the configured session cookie")
    token_path = config.get("token_path", "").strip()
    tokens = extract_auth_tokens(response)
    token = extract_access_token(response, token_path) or str(tokens.get("access_token") or "")
    if not token:
        raise RuntimeError("upstream login did not return access_token")
    if token_state is not None:
        state = {"access_token": token, "updated_at": utc_now()}
        if config.get("session_cookie"):
            state["session_cookie"] = config["session_cookie"]
        if tokens.get("refresh_token"):
            state["refresh_token"] = tokens["refresh_token"]
        if tokens.get("access_expires_at"):
            state["access_expires_at"] = tokens["access_expires_at"]
        token_state["upstreams"][state_key] = state
        if token_state_path is not None:
            write_private_json_atomic(token_state_path, token_state)
    return token


def extract_key_entries(raw: Any) -> list[dict[str, Any]]:
    if isinstance(raw, list):
        return [entry for entry in raw if isinstance(entry, dict)]
    if not isinstance(raw, dict):
        return []
    for field in ("items", "keys", "results", "data"):
        value = raw.get(field)
        entries = extract_key_entries(value)
        if entries:
            return entries
    if any(field in raw for field in ("key", "api_key", "token")):
        return [raw]
    return []


def extract_api_name(entry: Any) -> str:
    """Read a display name without retaining API key material."""
    if not isinstance(entry, dict):
        return ""
    containers = [entry]
    for field in ("api", "application", "metadata"):
        nested = entry.get(field)
        if isinstance(nested, dict):
            containers.append(nested)
    for container in containers:
        for field in ("name", "api_name", "key_name", "title", "label"):
            value = container.get(field)
            if isinstance(value, str) and value.strip():
                return value.strip()
    return ""


def extract_ratio(value: Any) -> float | int | None:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        return None
    return value


def extract_group_rates(raw: Any) -> dict[str, float | int]:
    if not isinstance(raw, dict):
        return {}
    values = raw.get("data", raw)
    if not isinstance(values, dict):
        return {}
    rates: dict[str, float | int] = {}
    for group_name, value in values.items():
        ratio = extract_ratio(value)
        if isinstance(value, dict):
            ratio = next(
                (
                    extract_ratio(value.get(field))
                    for field in ("rate_multiplier", "ratio", "group_ratio", "model_ratio")
                    if extract_ratio(value.get(field)) is not None
                ),
                None,
            )
        if ratio is not None:
            rates[str(group_name)] = ratio
    return rates


def api_key_matches(candidate: Any, api_key: str) -> bool:
    if not isinstance(candidate, str) or not api_key:
        return False
    if candidate == api_key:
        return True
    if len(candidate) >= len(api_key) or not candidate.casefold().endswith(api_key[-4:].casefold()):
        return False
    return "..." in candidate or "…" in candidate or "*" in candidate


def match_upstream_key(raw: Any, api_key: str) -> dict[str, Any] | None:
    if not api_key:
        return None
    for entry in extract_key_entries(raw):
        candidate = entry.get("key") or entry.get("api_key") or entry.get("token")
        if not api_key_matches(candidate, api_key):
            continue
        group = entry.get("group")
        group_name = ""
        if isinstance(group, dict):
            group_name = (
                group.get("name")
                or group.get("group_name")
                or entry.get("group_name")
                or entry.get("granted_group_name")
            )
        elif isinstance(group, str):
            group_name = group
        else:
            group_name = entry.get("group_name") or entry.get("granted_group_name")
        group = group if isinstance(group, dict) else {}
        ratio = next(
            (
                extract_ratio(group.get(field))
                for field in ("rate_multiplier", "ratio", "group_ratio", "model_ratio")
                if extract_ratio(group.get(field)) is not None
            ),
            None,
        )
        if ratio is None:
            ratio = next(
                (
                    extract_ratio(entry.get(field))
                    for field in ("rate_multiplier", "ratio", "group_ratio", "model_ratio")
                    if extract_ratio(entry.get(field)) is not None
                ),
                None,
            )
        if ratio is None and group_name:
            group_rates = entry.get("__monitor_group_rates")
            if isinstance(group_rates, dict):
                ratio = extract_ratio(group_rates.get(str(group_name)))
        matched = {"group": str(group_name or ""), "ratio": ratio}
        api_name = extract_api_name(entry)
        if api_name:
            matched["api_name"] = api_name
        return matched
    return None


def probe_authorized_account_pricing(
    account: dict[str, Any],
    config: dict[str, str],
    token: str,
    timeout: int = 15,
    key_entries: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    base_url = str(account.get("base_url", ""))
    mapping = account.get("model_mapping", {})
    if not isinstance(mapping, dict):
        mapping = {}
    origin = upstream_origin(config.get("base_url") or base_url)
    path = config.get("keys_path", DEFAULT_UPSTREAM_AUTH_KEYS_PATH).strip()
    keys_url = config.get("keys_url") or (origin + "/" + path.lstrip("/"))
    headers = auth_headers(token, config)
    if key_entries is None:
        key_entries = fetch_authorized_key_entries(config, base_url, token, timeout)

    matched = match_upstream_key(key_entries, str(account.get("api_key", "")))
    result = {
        "name": account.get("name", ""),
        "host": upstream_host(base_url),
        "api_name": "",
        "models": sorted(str(model) for model in mapping),
        "status": "authorized_key_not_found",
        "scope": "auth_ok_key_not_found",
        "source_url": keys_url,
        "pricing_version": None,
        "prices": {},
        "groups": {},
        "error": "authenticated, but the configured API key was not found",
    }
    if matched is None:
        return result
    group_name = matched.get("group", "")
    ratio = matched.get("ratio")
    result["api_name"] = matched.get("api_name", "")
    if group_name and ratio is not None:
        result["groups"] = {group_name: ratio}
        result["status"] = "authorized"
        result["scope"] = "account_authorized"
        result["error"] = ""
    else:
        result["status"] = "authorized_no_ratio"
        result["scope"] = "account_authorized"
        result["error"] = "matched API key, but no group rate_multiplier was returned"
    return result


def apply_upstream_account_rate_override(
    account_id: str,
    account: dict[str, Any],
    pricing: dict[str, Any],
    overrides: dict[str, dict[str, str]],
) -> dict[str, Any]:
    """Apply a manual rate only after the account, key, host, and group all match."""
    override = overrides.get(str(account_id))
    if not isinstance(override, dict) or pricing.get("scope") != "account_authorized":
        return pricing
    if normalize_host(str(account.get("base_url", ""))) != override.get("host"):
        return pricing
    api_key = str(account.get("api_key", ""))
    if len(api_key) < 4 or api_key[-4:].casefold() != override.get("key_suffix", "").casefold():
        return pricing
    groups = pricing.get("groups")
    group_name = override.get("group", "")
    if not isinstance(groups, dict) or group_name not in groups:
        return pricing
    rate_multiplier = canonical_ratio(override.get("rate_multiplier"))
    if rate_multiplier is None:
        return pricing
    return {**pricing, "groups": {**groups, group_name: rate_multiplier}}


def probe_browser_snapshot_pricing(
    account: dict[str, Any],
    snapshot: dict[str, list[dict[str, Any]]],
) -> dict[str, Any] | None:
    base_url = str(account.get("base_url", ""))
    api_key = str(account.get("api_key", ""))
    host = normalize_host(base_url)
    suffix = api_key[-4:].lower() if len(api_key) >= 4 else ""
    if not host or not suffix:
        return None

    matches = [entry for entry in snapshot.get(host, []) if entry.get("key_suffix") == suffix]
    if len(matches) != 1:
        return None

    entry = matches[0]
    group = str(entry.get("group", ""))
    ratio = entry.get("ratio")
    result = {
        "name": account.get("name", ""),
        "host": upstream_host(base_url),
        "api_name": entry.get("api_name", ""),
        "models": sorted(str(model) for model in account.get("model_mapping", {}) or {}),
        "status": "authorized",
        "scope": "account_authorized",
        "source_url": "chrome_snapshot",
        "pricing_version": None,
        "prices": {},
        "groups": {group: ratio},
        "error": "",
    }
    if not group or ratio in (None, ""):
        result["status"] = "authorized_no_ratio"
        result["groups"] = {}
        result["error"] = "browser snapshot has no group ratio"
    return result


def fetch_transithub_key_rates(
    account: dict[str, Any],
    transithub_url: str,
    internal_token: str,
    fingerprint_secret: str,
    cache: dict[str, list[dict[str, Any]] | Exception],
    timeout: int = 15,
) -> dict[str, Any] | None:
    """Resolve an account's exact upstream key against TransitHub snapshots.

    The monitor computes the same keyed HMAC as TransitHub; the complete key is
    never sent over the network. A match is accepted only when exactly one
    current record has a concrete group and multiplier.
    """
    api_key = str(account.get("api_key", "")).strip()
    base_url = str(account.get("base_url", "")).strip()
    host = normalize_host(base_url)
    if not api_key or not host or not transithub_url or not internal_token or not fingerprint_secret:
        return None
    digest = hmac.new(fingerprint_secret.encode("utf-8"), api_key.encode("utf-8"), hashlib.sha256).hexdigest()
    matched_host = ""
    entry = None
    for candidate_host in transithub_host_candidates(host):
        if candidate_host not in cache:
            url = (
                f"{transithub_url.rstrip('/')}/api/internal/upstream-key-rates?"
                f"{urlencode({'host': candidate_host})}"
            )
            try:
                raw = fetch_json(url, {"X-Internal-Token": internal_token}, timeout)
                items = raw.get("items") if isinstance(raw, dict) else None
                cache[candidate_host] = items if isinstance(items, list) else []
            except (RuntimeError, UpstreamHTTPError) as exc:
                cache[candidate_host] = exc
        entries = cache.get(candidate_host)
        if isinstance(entries, Exception) or not isinstance(entries, list):
            continue
        valid = [
            item for item in entries
            if isinstance(item, dict)
            and item.get("fingerprint") == digest
            and str(item.get("groupName", "")).strip()
            and extract_ratio(item.get("multiplier")) is not None
            and str(item.get("status", "")).strip() == "authorized"
        ]
        match_method = "hmac"
        if not valid:
            # new-api only exposes masked keys after creation. TransitHub stores
            # their last four characters and only accepts a unique suffix match.
            suffix = api_key[-4:].casefold()
            suffix_matches = [
                item for item in entries
                if isinstance(item, dict)
                and str(item.get("keySuffix", "")).strip().casefold() == suffix
            ]
            valid = [
                item for item in suffix_matches
                if str(item.get("groupName", "")).strip()
                and extract_ratio(item.get("multiplier")) is not None
                and str(item.get("status", "")).strip() == "authorized"
            ] if len(suffix_matches) == 1 else []
            match_method = "suffix"
        if len(valid) == 1:
            entry = valid[0]
            matched_host = candidate_host
            break
        if len(valid) > 1:
            return None
    if entry is None:
        return None
    mapping = account.get("model_mapping", {})
    if not isinstance(mapping, dict):
        mapping = {}
    ratio = extract_ratio(entry.get("multiplier"))
    if ratio is None:
        return None
    return {
        "name": account.get("name", ""),
        "host": upstream_host(base_url),
        "api_name": str(entry.get("keyName") or "").strip(),
        "models": sorted(str(model) for model in mapping),
        "status": "authorized_transithub_key",
        "scope": "account_authorized",
        "source_url": "transithub_key_fallback" if match_method == "hmac" else "transithub_key_suffix_fallback",
        "transithub_host": matched_host,
        "pricing_version": None,
        "prices": {},
        "groups": {str(entry.get("groupName")): ratio},
        "error": "",
    }


def transithub_host_candidates(host: str) -> list[str]:
    """Return exact host first, then parent hosts saved in TransitHub."""
    normalized = normalize_host(host)
    if not normalized:
        return []
    labels = normalized.split(".")
    return [".".join(labels[index:]) for index in range(max(1, len(labels) - 1))]


def fetch_authorized_key_entries(
    config: dict[str, str], base_url: str, token: str, timeout: int = 15
) -> list[dict[str, Any]]:
    origin = upstream_origin(config.get("base_url") or base_url)
    path = config.get("keys_path", DEFAULT_UPSTREAM_AUTH_KEYS_PATH).strip()
    keys_url = config.get("keys_url") or (origin + "/" + path.lstrip("/"))
    headers = auth_headers(token, config)
    page_size = max(int(config.get("page_size", "1000") or 1000), 1)
    entries: list[dict[str, Any]] = []
    for page in range(1, 21):
        raw = fetch_json(
            append_pagination_query(keys_url, page, page_size, config), headers, timeout
        )
        page_entries = extract_key_entries(raw)
        entries.extend(page_entries)
        total = raw.get("total") if isinstance(raw, dict) else None
        if not page_entries or (isinstance(total, int) and len(entries) >= total):
            break
        if len(page_entries) < page_size:
            break
    group_rates_url = config.get("group_rates_url", "").strip()
    if group_rates_url:
        try:
            group_rates = extract_group_rates(fetch_json(group_rates_url, headers, timeout))
        except (RuntimeError, UpstreamHTTPError) as exc:
            LOG.warning("Cannot fetch upstream group rates from %s: %s", group_rates_url, exc)
            group_rates = {}
        if group_rates:
            for entry in entries:
                entry["__monitor_group_rates"] = group_rates
    return entries


def probe_account_pricing(
    account: dict[str, Any],
    paths: tuple[str, ...],
    timeout: int = 15,
    auth_config: dict[str, str] | None = None,
    token_cache: dict[str, str] | None = None,
    token_cache_key: str | None = None,
    authorized_key_entries: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    base_url = str(account.get("base_url", ""))
    api_key = str(account.get("api_key", ""))
    result = {
        "name": account.get("name", ""),
        "host": upstream_host(base_url),
        "api_name": "",
        "models": sorted(str(model) for model in account.get("model_mapping", {}) or {}),
        "status": "missing_credentials",
        "scope": "unverified",
        "source_url": "",
        "pricing_version": None,
        "prices": {},
        "groups": {},
        "error": "",
    }
    if not base_url or not api_key:
        result["error"] = "missing Base URL or API Key"
        return result

    if auth_config is not None:
        cache_key = normalize_host(auth_config.get("base_url") or base_url)
        if token_cache_key:
            cache_key = token_cache_key
        token = (token_cache or {}).get(cache_key, "") if token_cache is not None else ""
        try:
            if not token:
                token = login_upstream(auth_config, base_url, timeout)
                if token_cache is not None:
                    token_cache[cache_key] = token
            return probe_authorized_account_pricing(
                account, auth_config, token, timeout, authorized_key_entries
            )
        except (RuntimeError, UpstreamHTTPError) as exc:
            result.update(
                {
                    "status": "auth_failed",
                    "scope": "unverified",
                    "error": str(exc),
                }
            )
            return result

    mapping = account.get("model_mapping", {})
    if not isinstance(mapping, dict):
        mapping = {}
    errors: list[str] = []
    saw_auth_error = False
    saw_access_block = False
    for url in upstream_price_urls(base_url, paths):
        header_options = [
            {"Authorization": f"Bearer {api_key}"},
            {"x-api-key": api_key},
        ]
        for header_index, headers in enumerate(header_options):
            try:
                raw = fetch_json(url, headers, timeout)
            except UpstreamHTTPError as exc:
                if exc.status in (401, 403):
                    saw_auth_error = True
                    saw_access_block = saw_access_block or exc.status == 403
                    if header_index == 0:
                        continue
                errors.append(f"HTTP {exc.status}")
                break
            except RuntimeError as exc:
                errors.append(str(exc).split(" for ", 1)[0])
                break

            normalized = normalize_upstream_prices(raw)
            if normalized["scope"] == "unverified" and normalized["models"]:
                try:
                    anonymous = normalize_upstream_prices(fetch_json(url, {}, timeout))
                    if (
                        anonymous["models"] == normalized["models"]
                        and anonymous["groups"] == normalized["groups"]
                        and anonymous["pricing_version"] == normalized["pricing_version"]
                    ):
                        normalized["scope"] = "public_default"
                    elif not anonymous["models"]:
                        normalized["scope"] = "account_authorized"
                    else:
                        normalized["scope"] = "account_authorized"
                except (RuntimeError, UpstreamHTTPError):
                    pass
            source_models = normalized["models"]
            prices = {}
            for local_model, upstream_model in mapping.items():
                price = source_models.get(str(upstream_model))
                if price is not None:
                    prices[str(local_model)] = {
                        "upstream_model": str(upstream_model),
                        **price,
                    }
            result.update(
                {
                    "status": "supported" if prices else "supported_no_mapped_models",
                    "scope": normalized["scope"],
                    "source_url": url,
                    "pricing_version": normalized["pricing_version"],
                    "prices": dict(sorted(prices.items())),
                    # A public pricing response describes the whole upstream,
                    # not the API key currently being monitored.
                    "groups": (
                        normalized["groups"]
                        if normalized["scope"] == "account_authorized"
                        else {}
                    ),
                    "error": "",
                }
            )
            return result

    if saw_access_block:
        result["status"] = "access_blocked"
    elif saw_auth_error:
        result["status"] = "auth_failed"
    else:
        result["status"] = "unsupported"
    result["error"] = "; ".join(errors[:3]) or "no pricing endpoint with model price fields"
    return result


def diff_upstream_pricing(
    previous: dict[str, dict[str, Any]], current: dict[str, dict[str, Any]]
) -> list[dict[str, Any]]:
    changes: list[dict[str, Any]] = []
    for account_id in sorted(set(previous) | set(current)):
        before_account = previous.get(account_id, {})
        after_account = current.get(account_id, {})
        if not before_account or not after_account:
            continue
        if (
            before_account.get("scope") != "account_authorized"
            or after_account.get("scope") != "account_authorized"
        ):
            continue

        before_prices = before_account.get("prices", {})
        after_prices = after_account.get("prices", {})
        if not isinstance(before_prices, dict):
            before_prices = {}
        if not isinstance(after_prices, dict):
            after_prices = {}
        for model in sorted(set(before_prices) | set(after_prices)):
            before = before_prices.get(model)
            after = after_prices.get(model)
            if before != after:
                changes.append(
                    {
                        "kind": "price",
                        "account_id": account_id,
                        "account_name": after_account.get("name", account_id),
                        "host": after_account.get("host", ""),
                        "model": model,
                        "before": before or {},
                        "after": after or {},
                    }
                )
        before_groups = before_account.get("groups", {})
        after_groups = after_account.get("groups", {})
        if not isinstance(before_groups, dict):
            before_groups = {}
        if not isinstance(after_groups, dict):
            after_groups = {}
        for group in sorted(set(before_groups) | set(after_groups)):
            before_ratio = before_groups.get(group)
            after_ratio = after_groups.get(group)
            if before_ratio != after_ratio:
                changes.append(
                    {
                        "kind": "group_ratio",
                        "account_id": account_id,
                        "account_name": after_account.get("name", account_id),
                        "host": after_account.get("host", ""),
                        "models": after_account.get("models", []),
                        "group": group,
                        "before": before_ratio,
                        "after": after_ratio,
                    }
                )
    return changes


def format_upstream_change(change: dict[str, Any]) -> str:
    prefix = (
        f"- 账号 `{change.get('account_name')}` | 上游 `{change.get('host')}`"
    )
    if change.get("kind") == "status":
        before = change.get("before", {})
        after = change.get("after", {})
        details = "; ".join(
            f"{field}: {before.get(field, '-')} -> {after.get(field, '-')}"
            for field in sorted(set(before) | set(after))
            if before.get(field) != after.get(field)
        )
        return f"{prefix} | 监控状态：{details}"

    if change.get("kind") == "group_ratio":
        before = change.get("before")
        after = change.get("after")
        action = "分组倍率新增" if before is None else "分组倍率删除" if after is None else "分组倍率变化"
        return (
            f"{prefix} | 分组 `{change.get('group')}` | {action}："
            f"{format_ratio(before)} -> {format_ratio(after)}"
        )

    before = change.get("before", {})
    after = change.get("after", {})
    model = change.get("model", "")
    remote_model = after.get("upstream_model") or before.get("upstream_model") or model
    details = "; ".join(
        f"{field}: {before.get(field, '-')} -> {after.get(field, '-')}"
        for field in sorted(set(before) | set(after))
        if field != "upstream_model" and before.get(field) != after.get(field)
    )
    action = "价格新增" if not before else "价格删除" if not after else "价格变化"
    return f"{prefix} | 模型 `{model}`（上游 `{remote_model}`）| {action}：{details or '-'}"


def summarize_upstream_pricing(pricing: dict[str, dict[str, Any]]) -> dict[str, int]:
    summary = {
        "total": len(pricing),
        "supported": 0,
        "authorized": 0,
        "public": 0,
        "access_blocked": 0,
        "auth_failed": 0,
        "unsupported": 0,
        "group_accounts": 0,
        "group_count": 0,
    }
    for result in pricing.values():
        status = result.get("status")
        scope = result.get("scope")
        if status in {"supported", "supported_no_mapped_models", "authorized", "authorized_no_ratio"}:
            summary["supported"] += 1
        elif status == "access_blocked":
            summary["access_blocked"] += 1
        elif status in {"auth_failed", "authorized_key_not_found"}:
            summary["auth_failed"] += 1
        else:
            summary["unsupported"] += 1
        if scope == "account_authorized":
            summary["authorized"] += 1
        elif scope == "public_default":
            summary["public"] += 1
        groups = result.get("groups", {})
        if isinstance(groups, dict) and groups:
            summary["group_accounts"] += 1
            summary["group_count"] += len(groups)
    return summary


def diff_accounts(
    previous: dict[str, dict[str, Any]], current: dict[str, dict[str, Any]]
) -> dict[str, list[dict[str, Any]]]:
    added = []
    removed = []
    changed = []
    for account_id in sorted(set(current) - set(previous)):
        added.append({"account_id": account_id, "after": current[account_id]})
    for account_id in sorted(set(previous) - set(current)):
        removed.append({"account_id": account_id, "before": previous[account_id]})
    for account_id in sorted(set(previous) & set(current)):
        if previous[account_id] != current[account_id]:
            changed.append(
                {
                    "account_id": account_id,
                    "before": previous[account_id],
                    "after": current[account_id],
                }
            )
    return {"added": added, "removed": removed, "changed": changed}


def format_price(value: Any) -> str:
    if value is None:
        return "-"
    if isinstance(value, float):
        return f"{value:.12g}"
    return str(value)


def format_ratio(value: Any) -> str:
    text = format_price(value)
    return text if text == "-" or text.endswith("x") else f"{text}x"


def format_change(change: dict[str, Any]) -> str:
    model = change["model"]
    before = change.get("before", {})
    after = change.get("after", {})
    fields = sorted(set(before) | set(after))
    if "after" not in change:
        return f"- 删除 `{model}`"
    if "before" not in change:
        return f"- 新增 `{model}`"
    details = []
    for field in fields:
        old = before.get(field)
        new = after.get(field)
        if old != new:
            details.append(f"{field}: {format_price(old)} -> {format_price(new)}")
    return f"- `{model}`: " + "; ".join(details)


def format_account_change(change: dict[str, Any]) -> str:
    before = change.get("before", {})
    after = change.get("after", {})
    account_id = change["account_id"]
    if "after" not in change:
        return f"- 删除 `{before.get('name', account_id)}`"
    if "before" not in change:
        return f"- 新增 `{after.get('name', account_id)}`"

    details = []
    if before.get("name") != after.get("name"):
        details.append(f"账号名：{before.get('name')} -> {after.get('name')}")
    if before.get("models") != after.get("models"):
        details.append(
            f"模型：{', '.join(before.get('models', [])) or '-'} -> "
            f"{', '.join(after.get('models', [])) or '-'}"
        )
    if before.get("status") != after.get("status"):
        details.append(f"状态：{before.get('status')} -> {after.get('status')}")
    if before.get("schedulable") != after.get("schedulable"):
        details.append(f"可调度：{before.get('schedulable')} -> {after.get('schedulable')}")
    return f"- `{after.get('name', account_id)}`：" + "; ".join(details)


def format_message(
    source: str,
    changes: dict[str, list[dict[str, Any]]],
    checked_at: str,
    accounts: dict[str, dict[str, Any]] | None = None,
    account_changes: dict[str, list[dict[str, Any]]] | None = None,
    accounts_available: bool = True,
    upstream_changes: list[dict[str, Any]] | None = None,
    upstream_pricing: dict[str, dict[str, Any]] | None = None,
    account_rename_changes: list[dict[str, str]] | None = None,
    account_priority_changes: list[dict[str, Any]] | None = None,
    account_safety_changes: list[dict[str, Any]] | None = None,
    account_auto_group_changes: list[dict[str, Any]] | None = None,
) -> str:
    changed = changes["changed"]
    added = changes["added"]
    removed = changes["removed"]
    account_changes = account_changes or {"changed": [], "added": [], "removed": []}
    account_total = sum(len(items) for items in account_changes.values())
    upstream_changes = upstream_changes or []
    account_rename_changes = account_rename_changes or []
    account_priority_changes = account_priority_changes or []
    account_safety_changes = account_safety_changes or []
    account_auto_group_changes = account_auto_group_changes or []
    total = len(changed) + len(added) + len(removed)
    accounts = accounts or {}
    lines = [
        "## Sub2API 上游价格与账号变动提醒",
        f"> 来源：{source}",
        f"> 检查时间：{checked_at}",
    ]
    if total:
        lines.append(f"> 价格变动模型：{total} 个（修改 {len(changed)}，新增 {len(added)}，删除 {len(removed)}）")
    if account_total:
        lines.append(f"> 账号配置变动：{account_total} 个")
    lines.append("")
    if upstream_changes:
        lines.append("### 上游账号价格变化")
        for item in upstream_changes:
            lines.append(format_upstream_change(item))
    for item in changed + added + removed:
        lines.append(format_change(item))
        model = item["model"]
        related_accounts = sorted(
            account["name"]
            for account in accounts.values()
            if model in account.get("models", [])
        )
        if related_accounts:
            lines.append(f"  账号：{', '.join(f'`{name}`' for name in related_accounts)}")
        elif not accounts_available:
            lines.append("  账号：当前无法读取 Sub2API 账号映射")
        else:
            lines.append("  账号：当前没有配置该模型的账号")
    if account_total:
        lines.extend(["", "### Sub2API 账号/模型映射变动"])
        for item in (
            account_changes["changed"]
            + account_changes["added"]
            + account_changes["removed"]
        ):
            lines.append(format_account_change(item))
    if account_rename_changes:
        lines.extend(["", "### 自动同步账号名称"])
        for item in account_rename_changes:
            lines.append(
                f"- `{item['current_name']}` -> `{item['desired_name']}`（上游倍率 `{item['ratio']}x`）"
            )
    if account_priority_changes:
        lines.extend(["", "### 自动更新账号优先级"])
        for item in account_priority_changes:
            lines.append(
                f"- `{item['account_name']}`：优先级 "
                f"`{item['current_priority']}` -> `{item['desired_priority']}` "
                f"（上游倍率 `{item['ratio']}x`）"
            )
    if account_safety_changes:
        lines.extend(["", "### 自动调整高成本分组绑定"])
        for item in account_safety_changes:
            violations = ", ".join(
                f"`{violation['group_name']}`（本站 {format_ratio(violation['managed_ratio'])}）"
                for violation in item.get("violations", [])
            )
            if violations:
                lines.append(
                    f"- `{item['account_name']}`：上游倍率 `{format_ratio(item['upstream_ratio'])}`，"
                    f"高于本站分组 {violations}，已解除这些分组绑定，其他分组保留"
                )
            restorations = ", ".join(
                f"`{restoration['group_name']}`（本站 {format_ratio(restoration['managed_ratio'])}）"
                for restoration in item.get("restorations", [])
            )
            if restorations:
                lines.append(
                    f"- `{item['account_name']}`：上游倍率 `{format_ratio(item['upstream_ratio'])}`，"
                    f"已恢复监控自动移出的分组 {restorations}"
                )
    if account_auto_group_changes:
        lines.extend(["", "### 自动调整盈利分组绑定"])
        for item in account_auto_group_changes:
            for addition in item.get("additions", []):
                lines.append(
                    f"- `{item['account_name']}`：已加入 `{addition['group_name']}`，"
                    f"上游倍率 `{format_ratio(item['upstream_ratio'])}`，"
                    f"本站倍率 `{format_ratio(addition['managed_ratio'])}`，"
                    f"利润率 `{addition['profit_percent']}%`"
                )
            for removal in item.get("removals", []):
                lines.append(
                    f"- `{item['account_name']}`：已移出 `{removal['group_name']}`，"
                    f"上游倍率 `{format_ratio(item['upstream_ratio'])}`，"
                    f"本站倍率 `{format_ratio(removal['managed_ratio'])}`，"
                    f"利润率 `{removal['profit_percent']}%`，低于最低 "
                    f"`{item['minimum_profit_percent']}%`"
                )
    return "\n".join(lines)


def send_wecom(webhook_url: str, message: str, timeout: int = 15) -> None:
    chunks = split_wecom_markdown(message, WECOM_MARKDOWN_CHUNK_BYTES)
    for index, chunk in enumerate(chunks, start=1):
        content = chunk
        if len(chunks) > 1:
            content = f"### Sub2API价格监控（第 {index}/{len(chunks)} 条）\n{chunk}"
        if len(content.encode("utf-8")) > WECOM_MARKDOWN_MAX_BYTES:
            raise RuntimeError("WeCom notification chunk exceeds 4096 UTF-8 bytes")
        payload = {"msgtype": "markdown", "markdown": {"content": content}}
        for attempt in range(1, WECOM_SEND_MAX_ATTEMPTS + 1):
            request = Request(
                webhook_url,
                data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
                headers={"Content-Type": "application/json", "User-Agent": "sub2api-price-monitor/1.0"},
                method="POST",
            )
            try:
                with urlopen(request, timeout=timeout) as response:
                    body = response.read().decode("utf-8", errors="replace")
                result = json.loads(body)
                if result.get("errcode") != 0:
                    raise RuntimeError(
                        f"WeCom returned errcode={result.get('errcode')}: {result.get('errmsg')}"
                    )
            except (HTTPError, URLError, TimeoutError, json.JSONDecodeError, RuntimeError) as exc:
                if attempt >= WECOM_SEND_MAX_ATTEMPTS:
                    raise RuntimeError(
                        f"WeCom notification segment {index}/{len(chunks)} failed "
                        f"after {attempt} attempts: {exc}"
                    ) from exc
                LOG.warning(
                    "WeCom notification segment %s/%s attempt %s/%s failed: %s",
                    index,
                    len(chunks),
                    attempt,
                    WECOM_SEND_MAX_ATTEMPTS,
                    exc,
                )
                time.sleep(WECOM_SEND_RETRY_DELAY_SECONDS * attempt)
                continue
            LOG.info(
                "Sent WeCom notification segment %s/%s (%s UTF-8 bytes)",
                index,
                len(chunks),
                len(content.encode("utf-8")),
            )
            break


def split_wecom_markdown(message: str, max_bytes: int = WECOM_MARKDOWN_CHUNK_BYTES) -> list[str]:
    """Split markdown on UTF-8 byte boundaries while preserving readable lines."""
    if max_bytes <= 0:
        raise ValueError("max_bytes must be positive")
    if len(message.encode("utf-8")) <= max_bytes:
        return [message]

    def split_long_text(text: str) -> list[str]:
        parts: list[str] = []
        current: list[str] = []
        current_bytes = 0
        for character in text:
            character_bytes = len(character.encode("utf-8"))
            if current and current_bytes + character_bytes > max_bytes:
                parts.append("".join(current))
                current = []
                current_bytes = 0
            current.append(character)
            current_bytes += character_bytes
        if current:
            parts.append("".join(current))
        return parts

    chunks: list[str] = []
    current: list[str] = []
    current_bytes = 0
    for line in message.splitlines(keepends=True):
        line_bytes = len(line.encode("utf-8"))
        if line_bytes > max_bytes:
            if current:
                chunks.append("".join(current))
                current = []
                current_bytes = 0
            long_parts = split_long_text(line)
            chunks.extend(long_parts[:-1])
            current = [long_parts[-1]]
            current_bytes = len(long_parts[-1].encode("utf-8"))
        elif current and current_bytes + line_bytes > max_bytes:
            chunks.append("".join(current))
            current = [line]
            current_bytes = line_bytes
        else:
            current.append(line)
            current_bytes += line_bytes
    if current:
        chunks.append("".join(current))
    return chunks or split_long_text(message)


class PriceMonitor:
    def __init__(
        self,
        data_dir: Path,
        source: str,
        url: str,
        hash_url: str,
        webhook_url: str,
        db_config: dict[str, Any] | None = None,
        upstream_price_paths: tuple[str, ...] = DEFAULT_UPSTREAM_PRICE_PATHS,
        upstream_auth_config: dict[str, dict[str, str]] | None = None,
        upstream_account_auth_config: dict[str, dict[str, str]] | None = None,
        upstream_account_rate_override_config: dict[str, dict[str, str]] | None = None,
        browser_pricing_snapshot_path: Path | None = None,
        sub2api_api_base_url: str = DEFAULT_SUB2API_API_BASE_URL,
        sub2api_admin_api_key: str = "",
        transithub_internal_url: str = DEFAULT_TRANSITHUB_INTERNAL_URL,
        transithub_internal_token: str = "",
        key_fingerprint_secret: str = "",
        auto_rename_accounts: bool = False,
        auto_update_account_priorities: bool = False,
        auto_stop_overpriced_accounts: bool = False,
        auto_assign_profitable_groups: bool = False,
        auto_group_min_profit_percent: Decimal = Decimal("15"),
        auto_group_ids: dict[str, list[int]] | None = None,
        auto_group_movement_excluded_account_ids: set[str] | None = None,
    ):
        self.data_dir = data_dir
        self.source = source
        self.url = url
        self.hash_url = hash_url
        self.webhook_url = webhook_url
        self.db_config = db_config
        self.upstream_price_paths = upstream_price_paths
        self.upstream_auth_config = upstream_auth_config or {}
        self.upstream_account_auth_config = upstream_account_auth_config or {}
        self.upstream_account_rate_override_config = upstream_account_rate_override_config or {}
        self.browser_pricing_snapshot_path = browser_pricing_snapshot_path
        self.sub2api_api_base_url = sub2api_api_base_url.strip().rstrip("/")
        self.sub2api_admin_api_key = sub2api_admin_api_key.strip()
        self.transithub_internal_url = transithub_internal_url.strip().rstrip("/")
        self.transithub_internal_token = transithub_internal_token.strip()
        self.key_fingerprint_secret = key_fingerprint_secret.strip()
        self.auto_rename_accounts = auto_rename_accounts
        self.auto_update_account_priorities = auto_update_account_priorities
        self.auto_stop_overpriced_accounts = auto_stop_overpriced_accounts
        self.auto_assign_profitable_groups = auto_assign_profitable_groups
        self.auto_group_min_profit_percent = auto_group_min_profit_percent
        self.auto_group_ids = auto_group_ids or {}
        self.auto_group_movement_excluded_account_ids = (
            auto_group_movement_excluded_account_ids or set()
        )
        self.state_path = data_dir / "latest.json"
        self.history_path = data_dir / "history.jsonl"
        self.upstream_token_state_path = data_dir / "upstream-token-state.json"
        self.upstream_token_state = normalize_upstream_token_state(
            load_json(self.upstream_token_state_path, {})
        )

    def check_once(self) -> bool:
        remote_hash = None
        if self.hash_url:
            remote_hash = parse_sha256(fetch_bytes(self.hash_url))
            if not remote_hash:
                raise RuntimeError("hash file does not contain a SHA-256 value")

        body = fetch_bytes(self.url)
        body_hash = hashlib.sha256(body).hexdigest()
        if remote_hash and remote_hash != body_hash:
            raise RuntimeError(
                f"source hash mismatch: expected {remote_hash[:12]}, got {body_hash[:12]}"
            )

        raw = json.loads(body.decode("utf-8"))
        if not isinstance(raw, dict):
            raise RuntimeError("pricing document must be a JSON object")
        current = normalize_prices(raw)
        previous_state = load_json(self.state_path, {})
        previous = previous_state.get("models", {}) if isinstance(previous_state, dict) else {}
        if not isinstance(previous, dict):
            previous = {}
        changes = diff_prices(previous, current)

        previous_accounts = (
            previous_state.get("accounts", {}) if isinstance(previous_state, dict) else {}
        )
        if not isinstance(previous_accounts, dict):
            previous_accounts = {}
        previous_accounts_available = bool(
            previous_state.get("accounts_available") if isinstance(previous_state, dict) else False
        )
        accounts = previous_accounts
        groups: dict[int, dict[str, Any]] = {}
        accounts_available = False
        account_groups_available = False
        if self.db_config:
            try:
                accounts = fetch_sub2api_accounts(self.db_config)
                attach_sub2api_account_groups(
                    accounts, fetch_sub2api_account_groups(self.db_config)
                )
                groups = fetch_sub2api_groups(self.db_config)
                accounts_available = True
                account_groups_available = True
            except RuntimeError as exc:
                LOG.error("Sub2API account/group check failed: %s", exc)

        runtime_account_extras: dict[str, dict[str, Any]] = {}
        if accounts_available and self.sub2api_admin_api_key:
            try:
                runtime_account_extras = fetch_sub2api_account_extras(
                    self.sub2api_api_base_url, self.sub2api_admin_api_key
                )
            except Exception as exc:
                LOG.warning("Sub2API runtime billing metadata unavailable: %s", exc)

        browser_pricing_snapshot = load_browser_pricing_snapshot(
            self.browser_pricing_snapshot_path
        )

        current_upstream_pricing: dict[str, dict[str, Any]] = {}
        previous_upstream_pricing = {}
        if (
            isinstance(previous_state, dict)
            and previous_state.get("upstream_pricing_version") == UPSTREAM_PRICING_STATE_VERSION
        ):
            previous_upstream_pricing = previous_state.get("upstream_pricing", {})
        if not isinstance(previous_upstream_pricing, dict):
            previous_upstream_pricing = {}
        # Probe every account so image2/Grok prices remain visible; mutation plans
        # still apply is_account_automation_excluded() separately.
        monitored_account_ids = set(accounts)
        previous_upstream_pricing = {
            account_id: pricing
            for account_id, pricing in previous_upstream_pricing.items()
            if account_id in monitored_account_ids
        }
        if accounts_available:
            token_cache: dict[str, str] = {}
            authorized_key_cache: dict[str, list[dict[str, Any]] | Exception] = {}
            transithub_key_rate_cache: dict[str, list[dict[str, Any]] | Exception] = {}
            for account_id, account in accounts.items():
                if account_id not in monitored_account_ids:
                    continue
                try:
                    runtime_result = probe_sub2api_billing_pricing(
                        account, runtime_account_extras.get(account_id)
                    )
                    if runtime_result is not None:
                        current_upstream_pricing[account_id] = runtime_result
                        continue
                    browser_result = probe_browser_snapshot_pricing(
                        account, browser_pricing_snapshot
                    )
                    if (
                        browser_result is not None
                        and isinstance(browser_result.get("groups"), dict)
                        and any(extract_ratio(value) is not None for value in browser_result.get("groups", {}).values())
                    ):
                        current_upstream_pricing[account_id] = apply_upstream_account_rate_override(
                            account_id,
                            account,
                            browser_result,
                            self.upstream_account_rate_override_config,
                        )
                        continue
                    account_auth_config = self.upstream_account_auth_config.get(account_id)
                    auth_host = normalize_host(str(account.get("base_url", "")))
                    auth_config = account_auth_config or self.upstream_auth_config.get(auth_host)
                    if auth_config is not None:
                        cache_host = normalize_host(
                            auth_config.get("base_url") or account.get("base_url", "")
                        )
                        cache_key = (
                            f"{cache_host}#account:{account_id}"
                            if account_auth_config is not None
                            else cache_host
                        )
                        if cache_key not in authorized_key_cache:
                            try:
                                token = token_cache.get(cache_key, "")
                                if not token:
                                    token = login_upstream(
                                        auth_config,
                                        str(account.get("base_url", "")),
                                        token_state=self.upstream_token_state,
                                        token_state_path=self.upstream_token_state_path,
                                        token_state_key=cache_key,
                                    )
                                    token_cache[cache_key] = token
                                try:
                                    authorized_key_cache[cache_key] = fetch_authorized_key_entries(
                                        auth_config, str(account.get("base_url", "")), token
                                    )
                                except UpstreamHTTPError as exc:
                                    if exc.status != 401 or not can_refresh_upstream_access_token(
                                        auth_config, self.upstream_token_state, cache_key
                                    ):
                                        raise
                                    token = login_upstream(
                                        auth_config,
                                        str(account.get("base_url", "")),
                                        token_state=self.upstream_token_state,
                                        token_state_path=self.upstream_token_state_path,
                                        force_refresh=True,
                                        token_state_key=cache_key,
                                    )
                                    token_cache[cache_key] = token
                                    authorized_key_cache[cache_key] = fetch_authorized_key_entries(
                                        auth_config, str(account.get("base_url", "")), token
                                    )
                            except Exception as exc:
                                authorized_key_cache[cache_key] = exc
                                LOG.error(
                                    "Upstream authorization check failed for host %s: %s",
                                    cache_key,
                                    exc,
                                )
                        cached_entries = authorized_key_cache[cache_key]
                        if isinstance(cached_entries, Exception):
                            mapping = account.get("model_mapping", {})
                            if not isinstance(mapping, dict):
                                mapping = {}
                            failed_pricing = {
                                "name": account.get("name", account_id),
                                "host": upstream_host(str(account.get("base_url", ""))),
                                "api_name": "",
                                "models": sorted(str(model) for model in mapping),
                                "status": "auth_failed",
                                "scope": "unverified",
                                "source_url": "",
                                "pricing_version": None,
                                "prices": {},
                                "groups": {},
                                "error": str(cached_entries),
                            }
                            fallback = fetch_transithub_key_rates(
                                account,
                                self.transithub_internal_url,
                                self.transithub_internal_token,
                                self.key_fingerprint_secret,
                                transithub_key_rate_cache,
                            )
                            current_upstream_pricing[account_id] = fallback or failed_pricing
                            continue
                        pricing = probe_account_pricing(
                            account,
                            self.upstream_price_paths,
                            auth_config=auth_config,
                            token_cache=token_cache,
                            token_cache_key=cache_key,
                            authorized_key_entries=cached_entries,
                        )
                        if (
                            pricing.get("scope") != "account_authorized"
                            or not isinstance(pricing.get("groups"), dict)
                            or not any(extract_ratio(value) is not None for value in pricing.get("groups", {}).values())
                        ):
                            fallback = fetch_transithub_key_rates(
                                account,
                                self.transithub_internal_url,
                                self.transithub_internal_token,
                                self.key_fingerprint_secret,
                                transithub_key_rate_cache,
                            )
                            if fallback is not None:
                                pricing = fallback
                        current_upstream_pricing[account_id] = apply_upstream_account_rate_override(
                            account_id,
                            account,
                            pricing,
                            self.upstream_account_rate_override_config,
                        )
                    else:
                        pricing = probe_account_pricing(account, self.upstream_price_paths)
                        if (
                            pricing.get("scope") != "account_authorized"
                            or not isinstance(pricing.get("groups"), dict)
                            or not any(extract_ratio(value) is not None for value in pricing.get("groups", {}).values())
                        ):
                            fallback = fetch_transithub_key_rates(
                                account,
                                self.transithub_internal_url,
                                self.transithub_internal_token,
                                self.key_fingerprint_secret,
                                transithub_key_rate_cache,
                            )
                            if fallback is not None:
                                pricing = fallback
                        current_upstream_pricing[account_id] = pricing
                except Exception as exc:
                    LOG.error("Upstream pricing check failed for account %s: %s", account_id, exc)
                    current_upstream_pricing[account_id] = {
                        "name": account.get("name", account_id),
                        "host": upstream_host(str(account.get("base_url", ""))),
                        "api_name": "",
                        "models": sorted(
                            str(model) for model in account.get("model_mapping", {}) or {}
                        ),
                        "status": "probe_failed",
                        "scope": "unverified",
                        "source_url": "",
                        "pricing_version": None,
                        "prices": {},
                        "groups": {},
                        "error": "probe failed",
                    }
        if accounts_available and self.db_config:
            try:
                sync_upstream_metadata(self.db_config, accounts, current_upstream_pricing)
            except RuntimeError as exc:
                LOG.warning("Sub2API upstream metadata unavailable: %s", exc)
        upstream_changes = (
            diff_upstream_pricing(previous_upstream_pricing, current_upstream_pricing)
            if previous_upstream_pricing and current_upstream_pricing
            else []
        )
        changed_account_ids = {
            str(change.get("account_id"))
            for change in upstream_changes
            if change.get("kind") == "group_ratio"
        }
        changed_categories = {
            account_priority_category(accounts[account_id].get("name"))
            for account_id in changed_account_ids
            if account_id in accounts
            and not is_account_automation_excluded(accounts[account_id].get("name"))
        }

        previous_rename_candidates = (
            previous_state.get("account_rename_candidates", {})
            if isinstance(previous_state, dict)
            else {}
        )
        if not isinstance(previous_rename_candidates, dict):
            previous_rename_candidates = {}
        rename_candidates, stable_renames = plan_account_renames(
            accounts,
            current_upstream_pricing,
            previous_rename_candidates,
            changed_account_ids,
        )
        rename_changes: list[dict[str, str]] = []
        if self.auto_rename_accounts and self.sub2api_admin_api_key:
            for item in stable_renames:
                try:
                    update_sub2api_account_name(
                        item["account_id"],
                        item["desired_name"],
                        self.sub2api_api_base_url,
                        self.sub2api_admin_api_key,
                    )
                except Exception as exc:
                    LOG.error(
                        "Automatic rename failed for account %s: %s",
                        item["account_id"],
                        exc,
                    )
                    continue
                account = accounts.get(item["account_id"])
                if account is not None:
                    account["name"] = item["desired_name"]
                pricing = current_upstream_pricing.get(item["account_id"])
                if pricing is not None:
                    pricing["name"] = item["desired_name"]
                rename_changes.append(item)
                rename_candidates.pop(item["account_id"], None)
                LOG.info(
                    "Renamed Sub2API account %s: %s -> %s",
                    item["account_id"],
                    item["current_name"],
                    item["desired_name"],
                )
        elif rename_candidates and not self.sub2api_admin_api_key:
            LOG.warning(
                "Detected %s account rename candidate(s), but SUB2API_ADMIN_API_KEY is not configured; read-only mode",
                len(rename_candidates),
            )

        previous_priority_candidates = {}
        if (
            isinstance(previous_state, dict)
            and previous_state.get("account_priority_state_version")
            == ACCOUNT_PRIORITY_STATE_VERSION
        ):
            previous_priority_candidates = previous_state.get(
                "account_priority_candidates", {}
            )
        if not isinstance(previous_priority_candidates, dict):
            previous_priority_candidates = {}
        previous_manual_priority_locks = set()
        if (
            isinstance(previous_state, dict)
            and previous_state.get("manual_priority_lock_state_version")
            == MANUAL_PRIORITY_LOCK_STATE_VERSION
        ):
            stored_locks = previous_state.get("manual_priority_locked_account_ids", [])
            if isinstance(stored_locks, list):
                previous_manual_priority_locks = {
                    str(account_id) for account_id in stored_locks
                }
        manual_priority_locks = previous_manual_priority_locks
        if accounts_available and self.db_config:
            try:
                manual_priority_locks |= fetch_manual_priority_locked_account_ids(
                    self.db_config
                )
            except RuntimeError as exc:
                LOG.error("Sub2API manual priority audit check failed: %s", exc)
        priority_candidates, stable_priorities = plan_account_priorities(
            accounts,
            current_upstream_pricing,
            previous_priority_candidates,
            changed_categories,
            manual_priority_locks,
        )
        priority_changes: list[dict[str, Any]] = []
        if self.auto_update_account_priorities and self.sub2api_admin_api_key:
            for item in stable_priorities:
                try:
                    update_sub2api_account_priority(
                        item["account_id"],
                        item["desired_priority"],
                        self.sub2api_api_base_url,
                        self.sub2api_admin_api_key,
                    )
                except Exception as exc:
                    LOG.error(
                        "Automatic priority update failed for account %s: %s",
                        item["account_id"],
                        exc,
                    )
                    continue
                account = accounts.get(item["account_id"])
                if account is not None:
                    account["priority"] = item["desired_priority"]
                priority_changes.append(item)
                priority_candidates.pop(item["account_id"], None)
                LOG.info(
                    "Updated Sub2API account %s priority: %s -> %s (ratio %sx)",
                    item["account_name"],
                    item["current_priority"],
                    item["desired_priority"],
                    item["ratio"],
                )
        elif (
            self.auto_update_account_priorities
            and priority_candidates
            and not self.sub2api_admin_api_key
        ):
            LOG.warning(
                "Detected %s account priority candidate(s), but SUB2API_ADMIN_API_KEY is not configured; read-only mode",
                len(priority_candidates),
            )

        previous_safety_candidates = {}
        previous_detached_groups: dict[str, dict[str, Any]] = {}
        if (
            isinstance(previous_state, dict)
            and previous_state.get("account_safety_state_version")
            == ACCOUNT_SAFETY_STATE_VERSION
        ):
            previous_safety_candidates = previous_state.get(
                "account_safety_candidates", {}
            )
            previous_detached_groups = previous_state.get(
                "account_safety_detached_groups", {}
            )
        if not isinstance(previous_safety_candidates, dict):
            previous_safety_candidates = {}
        if not isinstance(previous_detached_groups, dict):
            previous_detached_groups = {}
        if account_groups_available:
            previous_detached_groups = prune_deleted_safety_groups(
                previous_detached_groups, set(groups)
            )
        detached_groups = dict(previous_detached_groups)
        safety_candidates, stable_safety_stops = (
            plan_account_safety_stops(
                accounts,
                current_upstream_pricing,
                previous_safety_candidates,
                previous_detached_groups,
                self.auto_group_movement_excluded_account_ids,
            )
            if account_groups_available
            else ({}, [])
        )
        safety_changes: list[dict[str, Any]] = []
        if self.auto_stop_overpriced_accounts and self.sub2api_admin_api_key:
            for item in stable_safety_stops:
                account = accounts.get(item["account_id"])
                if account is None:
                    continue
                current_group_ids = [
                    int(group["group_id"])
                    for group in account.get("managed_groups", [])
                    if isinstance(group, dict) and group.get("group_id") is not None
                ]
                detach_group_ids = {
                    int(group_id) for group_id in item.get("detach_group_ids", [])
                }
                restore_group_ids = [
                    int(group_id) for group_id in item.get("restore_group_ids", [])
                ]
                ordered_group_ids = _ordered_group_ids(
                    current_group_ids,
                    [int(group_id) for group_id in item.get("group_order", [])],
                    restore_group_ids,
                )
                desired_group_ids = [
                    group_id
                    for group_id in ordered_group_ids
                    if (
                        group_id in current_group_ids and group_id not in detach_group_ids
                    )
                    or group_id in restore_group_ids
                ]
                try:
                    update_sub2api_account_groups(
                        item["account_id"],
                        desired_group_ids,
                        self.sub2api_api_base_url,
                        self.sub2api_admin_api_key,
                    )
                except Exception as exc:
                    LOG.error(
                        "Automatic account group safety update failed for account %s: %s",
                        item["account_id"],
                        exc,
                    )
                    continue

                current_groups_by_id = {
                    int(group["group_id"]): group
                    for group in account.get("managed_groups", [])
                    if isinstance(group, dict) and group.get("group_id") is not None
                }
                account["managed_groups"] = [
                    current_groups_by_id.get(group_id)
                    or {
                        **groups.get(group_id, {"group_id": group_id}),
                        "priority": 0,
                    }
                    for group_id in desired_group_ids
                ]

                detached_by_id = {
                    int(group["group_id"]): group
                    for group in detached_groups.get(item["account_id"], {}).get("groups", [])
                    if isinstance(group, dict) and group.get("group_id") is not None
                }
                for restoration in item.get("restorations", []):
                    detached_by_id.pop(int(restoration["group_id"]), None)
                for violation in item.get("violations", []):
                    detached_by_id[int(violation["group_id"])] = {
                        "group_id": int(violation["group_id"]),
                        "name": violation["group_name"],
                        "managed_ratio": violation["managed_ratio"],
                        "status": "active",
                        "priority": int(violation.get("priority", 0)),
                    }
                if detached_by_id:
                    detached_groups[item["account_id"]] = {
                        "account_name": item["account_name"],
                        "group_order": ordered_group_ids,
                        "groups": [
                            detached_by_id[group_id]
                            for group_id in ordered_group_ids
                            if group_id in detached_by_id
                        ],
                    }
                else:
                    detached_groups.pop(item["account_id"], None)
                safety_changes.append(item)
                safety_candidates.pop(item["account_id"], None)
                LOG.warning(
                    "Updated safety group bindings for Sub2API account %s (upstream ratio %sx): detached=%s restored=%s",
                    item["account_name"],
                    item["upstream_ratio"],
                    sorted(detach_group_ids),
                    restore_group_ids,
                )
        elif (
            self.auto_stop_overpriced_accounts
            and safety_candidates
            and not self.sub2api_admin_api_key
        ):
            LOG.warning(
                "Detected %s overpriced account group candidate(s), but SUB2API_ADMIN_API_KEY is not configured; read-only mode",
                len(safety_candidates),
            )

        previous_auto_group_candidates: dict[str, dict[str, Any]] = {}
        previous_auto_group_ownership: dict[str, dict[str, Any]] = {}
        if (
            isinstance(previous_state, dict)
            and previous_state.get("account_auto_group_state_version")
            == ACCOUNT_AUTO_GROUP_STATE_VERSION
        ):
            previous_auto_group_candidates = previous_state.get(
                "account_auto_group_candidates", {}
            )
            previous_auto_group_ownership = previous_state.get(
                "account_auto_group_ownership", {}
            )
        if not isinstance(previous_auto_group_candidates, dict):
            previous_auto_group_candidates = {}
        if not isinstance(previous_auto_group_ownership, dict):
            previous_auto_group_ownership = {}
        auto_group_candidates, stable_auto_group_changes, auto_group_ownership = (
            plan_account_auto_groups(
                accounts,
                groups,
                current_upstream_pricing,
                previous_auto_group_candidates,
                previous_auto_group_ownership,
                self.auto_group_ids,
                self.auto_group_min_profit_percent,
                self.auto_group_movement_excluded_account_ids,
            )
            if account_groups_available and self.auto_assign_profitable_groups
            else ({}, [], previous_auto_group_ownership)
        )
        auto_group_changes: list[dict[str, Any]] = []
        if self.auto_assign_profitable_groups and self.sub2api_admin_api_key:
            for item in stable_auto_group_changes:
                account = accounts.get(item["account_id"])
                if account is None:
                    continue
                current_group_ids = [
                    int(group["group_id"])
                    for group in account.get("managed_groups", [])
                    if isinstance(group, dict) and group.get("group_id") is not None
                ]
                add_group_ids = [
                    int(group_id) for group_id in item.get("add_group_ids", [])
                ]
                remove_group_ids = {
                    int(group_id) for group_id in item.get("remove_group_ids", [])
                }
                ordered_group_ids = _ordered_group_ids(
                    current_group_ids, current_group_ids, add_group_ids
                )
                desired_group_ids = [
                    group_id
                    for group_id in ordered_group_ids
                    if group_id not in remove_group_ids
                ]
                try:
                    update_sub2api_account_groups(
                        item["account_id"],
                        desired_group_ids,
                        self.sub2api_api_base_url,
                        self.sub2api_admin_api_key,
                    )
                except Exception as exc:
                    LOG.error(
                        "Automatic eligible group update failed for account %s: %s",
                        item["account_id"],
                        exc,
                    )
                    continue

                account_ownership = auto_group_ownership.get(
                    item["account_id"],
                    {"owned_group_ids": [], "suppressed_group_ids": []},
                )
                owned_group_ids = {
                    int(group_id)
                    for group_id in account_ownership.get("owned_group_ids", [])
                }
                owned_group_ids |= set(add_group_ids)
                owned_group_ids -= remove_group_ids
                if owned_group_ids or account_ownership.get("suppressed_group_ids"):
                    auto_group_ownership[item["account_id"]] = {
                        "owned_group_ids": sorted(owned_group_ids),
                        "suppressed_group_ids": sorted(
                            int(group_id)
                            for group_id in account_ownership.get(
                                "suppressed_group_ids", []
                            )
                        ),
                    }
                else:
                    auto_group_ownership.pop(item["account_id"], None)
                auto_group_changes.append(item)
                auto_group_candidates.pop(item["account_id"], None)
                LOG.warning(
                    "Updated eligible group bindings for Sub2API account %s (upstream ratio %sx): added=%s removed=%s",
                    item["account_name"],
                    item["upstream_ratio"],
                    add_group_ids,
                    sorted(remove_group_ids),
                )
        elif (
            self.auto_assign_profitable_groups
            and auto_group_candidates
            and not self.sub2api_admin_api_key
        ):
            LOG.warning(
                "Detected %s eligible account group candidate(s), but SUB2API_ADMIN_API_KEY is not configured; read-only mode",
                len(auto_group_candidates),
            )

        empty_account_changes = {"changed": [], "added": [], "removed": []}
        public_accounts = public_account_snapshot(accounts)
        account_changes = (
            diff_accounts(previous_accounts, public_accounts)
            if accounts_available and previous_accounts_available
            else empty_account_changes
        )
        checked_at = utc_now()
        state = {
            "source": self.source,
            "source_url": self.url,
            "checked_at": checked_at,
            "data_sha256": body_hash,
            "model_count": len(current),
            "models": current,
            "account_count": len(accounts),
            "accounts_available": accounts_available,
            "accounts": public_accounts,
            "upstream_pricing_version": UPSTREAM_PRICING_STATE_VERSION,
            "upstream_pricing": current_upstream_pricing,
            "account_rename_state_version": ACCOUNT_RENAME_STATE_VERSION,
            "account_rename_candidates": rename_candidates,
            "account_priority_state_version": ACCOUNT_PRIORITY_STATE_VERSION,
            "account_priority_candidates": priority_candidates,
            "manual_priority_lock_state_version": MANUAL_PRIORITY_LOCK_STATE_VERSION,
            "manual_priority_locked_account_ids": sorted(manual_priority_locks),
            "account_safety_state_version": ACCOUNT_SAFETY_STATE_VERSION,
            "account_safety_candidates": safety_candidates,
            "account_safety_detached_groups": detached_groups,
            "account_auto_group_state_version": ACCOUNT_AUTO_GROUP_STATE_VERSION,
            "account_auto_group_candidates": auto_group_candidates,
            "account_auto_group_ownership": auto_group_ownership,
        }
        write_json_atomic(self.state_path, state)

        total_changes = sum(len(items) for items in changes.values())
        total_account_changes = sum(len(items) for items in account_changes.values())
        upstream_baseline_ready = bool(previous_upstream_pricing and current_upstream_pricing)
        should_alert = bool(rename_changes) or bool(priority_changes) or bool(safety_changes) or bool(auto_group_changes) or (bool(previous) and total_changes > 0) or (
            previous_accounts_available and total_account_changes > 0
        ) or (upstream_baseline_ready and bool(upstream_changes))
        if should_alert:
            event = {
                "source": self.source,
                "checked_at": checked_at,
                "data_sha256": body_hash,
                "changes": changes,
                "account_changes": account_changes,
                "upstream_changes": upstream_changes,
                "account_rename_changes": rename_changes,
                "account_priority_changes": priority_changes,
                "account_safety_changes": safety_changes,
                "account_auto_group_changes": auto_group_changes,
            }
            self.data_dir.mkdir(parents=True, exist_ok=True)
            with self.history_path.open("a", encoding="utf-8") as history:
                history.write(json.dumps(event, ensure_ascii=False, sort_keys=True) + "\n")
            message = format_message(
                self.source,
                changes,
                checked_at,
                accounts,
                account_changes,
                accounts_available,
                upstream_changes,
                current_upstream_pricing,
                rename_changes,
                priority_changes,
                safety_changes,
                auto_group_changes,
            )
            if self.webhook_url:
                send_wecom(self.webhook_url, message)
                LOG.info(
                    "Sent WeCom alert for %s price changes and %s account changes",
                    total_changes,
                    total_account_changes + len(rename_changes) + len(priority_changes),
                )
            else:
                LOG.warning(
                    "Detected %s price changes and %s account changes, but WECOM_WEBHOOK_URL is empty",
                    total_changes,
                    total_account_changes,
                )
        elif not previous or (accounts_available and not previous_accounts_available):
            LOG.info(
                "Pricing/account baseline created: %s models, %s accounts, upstream pricing: %s",
                len(current),
                len(accounts),
                summarize_upstream_pricing(current_upstream_pricing),
            )
        elif accounts_available and not previous_upstream_pricing:
            LOG.info(
                "Upstream pricing baseline created: %s",
                summarize_upstream_pricing(current_upstream_pricing),
            )
        else:
            LOG.info(
                "No price or account changes: %s models, %s accounts, upstream pricing: %s",
                len(current),
                len(accounts),
                summarize_upstream_pricing(current_upstream_pricing),
            )
        return should_alert


def main() -> int:
    logging.basicConfig(
        level=os.getenv("LOG_LEVEL", "INFO").upper(),
        format="%(asctime)s %(levelname)s %(message)s",
    )
    db_host = os.getenv("SUB2API_DB_HOST", "").strip()
    db_config = None
    if db_host:
        db_config = {
            "host": db_host,
            "port": env_int("SUB2API_DB_PORT", 5432, 1),
            "name": os.getenv("SUB2API_DB_NAME", "sub2api").strip(),
            "user": os.getenv("SUB2API_DB_USER", "").strip(),
            "password": os.getenv("SUB2API_DB_PASSWORD", ""),
            "sslmode": os.getenv("SUB2API_DB_SSLMODE", "disable").strip(),
        }
    monitor = PriceMonitor(
        data_dir=Path(os.getenv("DATA_DIR", "/app/data")),
        source=os.getenv("PRICE_SOURCE_NAME", "Wei-Shaw/model-price-repo"),
        url=os.getenv("PRICE_SOURCE_URL", DEFAULT_URL).strip(),
        hash_url=os.getenv("PRICE_SOURCE_HASH_URL", DEFAULT_HASH_URL).strip(),
        webhook_url=os.getenv("WECOM_WEBHOOK_URL", "").strip(),
        db_config=db_config,
        upstream_auth_config=load_upstream_auth_config(),
        upstream_account_auth_config=load_upstream_account_auth_config(),
        upstream_account_rate_override_config=load_upstream_account_rate_override_config(),
        browser_pricing_snapshot_path=(
            Path(os.getenv("BROWSER_PRICING_SNAPSHOT_FILE", "").strip())
            if os.getenv("BROWSER_PRICING_SNAPSHOT_FILE", "").strip()
            else None
        ),
        sub2api_api_base_url=os.getenv(
            "SUB2API_API_BASE_URL", DEFAULT_SUB2API_API_BASE_URL
        ).strip(),
        sub2api_admin_api_key=os.getenv("SUB2API_ADMIN_API_KEY", "").strip(),
        transithub_internal_url=os.getenv(
            "TRANSITHUB_INTERNAL_URL", DEFAULT_TRANSITHUB_INTERNAL_URL
        ).strip(),
        transithub_internal_token=os.getenv("TRANSITHUB_INTERNAL_TOKEN", "").strip(),
        key_fingerprint_secret=os.getenv("KEY_FINGERPRINT_SECRET", "").strip(),
        auto_rename_accounts=os.getenv("AUTO_RENAME_ACCOUNTS", "false").strip().lower()
        in {"1", "true", "yes", "on"},
        auto_update_account_priorities=os.getenv(
            "AUTO_UPDATE_ACCOUNT_PRIORITIES", "false"
        ).strip().lower()
        in {"1", "true", "yes", "on"},
        auto_stop_overpriced_accounts=os.getenv(
            "AUTO_STOP_OVERPRICED_ACCOUNTS", "false"
        ).strip().lower()
        in {"1", "true", "yes", "on"},
        auto_assign_profitable_groups=os.getenv(
            "AUTO_ASSIGN_PROFITABLE_GROUPS", "false"
        ).strip().lower()
        in {"1", "true", "yes", "on"},
        auto_group_min_profit_percent=env_decimal_percent(
            "AUTO_GROUP_MIN_PROFIT_PERCENT",
            Decimal("15"),
            Decimal("0"),
            Decimal("20"),
        ),
        auto_group_ids={
            "gpt": env_group_ids("AUTO_GROUP_GPT_GROUP_IDS"),
            "plus": env_group_ids("AUTO_GROUP_PLUS_GROUP_IDS"),
        },
        auto_group_movement_excluded_account_ids=env_account_ids(
            "AUTO_GROUP_MOVEMENT_EXCLUDED_ACCOUNT_IDS"
        ),
        upstream_price_paths=tuple(
            path.strip()
            for path in os.getenv(
                "UPSTREAM_PRICE_PATHS", ",".join(DEFAULT_UPSTREAM_PRICE_PATHS)
            ).split(",")
            if path.strip()
        ),
    )
    interval = env_int("CHECK_INTERVAL_MINUTES", 60, 5) * 60
    stop = False

    def request_stop(_signum: int, _frame: Any) -> None:
        nonlocal stop
        stop = True

    signal.signal(signal.SIGTERM, request_stop)
    signal.signal(signal.SIGINT, request_stop)

    while not stop:
        try:
            monitor.check_once()
        except Exception as exc:  # keep the independent monitor alive on transient failures
            LOG.error("Price check failed: %s", exc)
        for _ in range(interval):
            if stop:
                break
            time.sleep(1)
    LOG.info("Price monitor stopped")
    return 0


if __name__ == "__main__":
    sys.exit(main())
