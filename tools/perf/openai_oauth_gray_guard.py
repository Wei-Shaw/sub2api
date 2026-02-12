#!/usr/bin/env python3
"""OpenAI OAuth 灰度阈值守护脚本。

用途：
- 拉取 Ops 指标阈值配置与 Dashboard Overview 实时数据
- 对比 P99 TTFT / 错误率 / SLA
- 作为 6.2 灰度守护的自动化门禁（退出码可直接用于 CI/CD）

退出码：
- 0: 指标通过
- 1: 请求失败/参数错误
- 2: 指标超阈值（建议停止扩量并回滚）
"""

from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any, Dict, List, Optional


@dataclass
class GuardThresholds:
    sla_percent_min: Optional[float]
    ttft_p99_ms_max: Optional[float]
    request_error_rate_percent_max: Optional[float]
    upstream_error_rate_percent_max: Optional[float]


@dataclass
class GuardSnapshot:
    sla: Optional[float]
    ttft_p99_ms: Optional[float]
    request_error_rate_percent: Optional[float]
    upstream_error_rate_percent: Optional[float]


def build_headers(token: str) -> Dict[str, str]:
    headers = {"Accept": "application/json"REDACTED
    if token.strip():
        headers["Authorization"] = f"Bearer {token.strip()REDACTED"
    return headers


def request_json(url: str, headers: Dict[str, str]) -> Dict[str, Any]:
    req = urllib.request.Request(url=url, method="GET", headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            raw = resp.read().decode("utf-8")
            return json.loads(raw)
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"HTTP {e.codeREDACTED: {bodyREDACTED") from e
    except urllib.error.URLError as e:
        raise RuntimeError(f"request failed: {eREDACTED") from e


def parse_envelope_data(payload: Dict[str, Any]) -> Dict[str, Any]:
    if not isinstance(payload, dict):
        raise RuntimeError("invalid response payload")
    if payload.get("code") != 0:
        raise RuntimeError(f"api error: code={payload.get('code')REDACTED message={payload.get('message')REDACTED")
    data = payload.get("data")
    if not isinstance(data, dict):
        raise RuntimeError("invalid response data")
    return data


def parse_thresholds(data: Dict[str, Any]) -> GuardThresholds:
    return GuardThresholds(
        sla_percent_min=to_float_or_none(data.get("sla_percent_min")),
        ttft_p99_ms_max=to_float_or_none(data.get("ttft_p99_ms_max")),
        request_error_rate_percent_max=to_float_or_none(data.get("request_error_rate_percent_max")),
        upstream_error_rate_percent_max=to_float_or_none(data.get("upstream_error_rate_percent_max")),
    )


def parse_snapshot(data: Dict[str, Any]) -> GuardSnapshot:
    ttft = data.get("ttft") if isinstance(data.get("ttft"), dict) else {REDACTED
    return GuardSnapshot(
        sla=to_float_or_none(data.get("sla")),
        ttft_p99_ms=to_float_or_none(ttft.get("p99_ms")),
        request_error_rate_percent=to_float_or_none(data.get("error_rate")),
        upstream_error_rate_percent=to_float_or_none(data.get("upstream_error_rate")),
    )


def to_float_or_none(v: Any) -> Optional[float]:
    if v is None:
        return None
    try:
        return float(v)
    except (TypeError, ValueError):
        return None


def evaluate(snapshot: GuardSnapshot, thresholds: GuardThresholds) -> List[str]:
    violations: List[str] = []

    if thresholds.sla_percent_min is not None and snapshot.sla is not None:
        if snapshot.sla < thresholds.sla_percent_min:
            violations.append(
                f"SLA 低于阈值: actual={snapshot.sla:.2fREDACTED% threshold={thresholds.sla_percent_min:.2fREDACTED%"
            )

    if thresholds.ttft_p99_ms_max is not None and snapshot.ttft_p99_ms is not None:
        if snapshot.ttft_p99_ms > thresholds.ttft_p99_ms_max:
            violations.append(
                f"TTFT P99 超阈值: actual={snapshot.ttft_p99_ms:.2fREDACTEDms threshold={thresholds.ttft_p99_ms_max:.2fREDACTEDms"
            )

    if (
        thresholds.request_error_rate_percent_max is not None
        and snapshot.request_error_rate_percent is not None
        and snapshot.request_error_rate_percent > thresholds.request_error_rate_percent_max
    ):
        violations.append(
            "请求错误率超阈值: "
            f"actual={snapshot.request_error_rate_percent:.2fREDACTED% "
            f"threshold={thresholds.request_error_rate_percent_max:.2fREDACTED%"
        )

    if (
        thresholds.upstream_error_rate_percent_max is not None
        and snapshot.upstream_error_rate_percent is not None
        and snapshot.upstream_error_rate_percent > thresholds.upstream_error_rate_percent_max
    ):
        violations.append(
            "上游错误率超阈值: "
            f"actual={snapshot.upstream_error_rate_percent:.2fREDACTED% "
            f"threshold={thresholds.upstream_error_rate_percent_max:.2fREDACTED%"
        )

    return violations


def main() -> int:
    parser = argparse.ArgumentParser(description="OpenAI OAuth 灰度阈值守护")
    parser.add_argument("--base-url", required=True, help="服务地址，例如 http://127.0.0.1:5231")
    parser.add_argument("--admin-token", default="", help="Admin JWT（可选，按部署策略）")
    parser.add_argument("--platform", default="openai", help="平台过滤，默认 openai")
    parser.add_argument("--time-range", default="30m", help="时间窗口: 5m/30m/1h/6h/24h/7d/30d")
    parser.add_argument("--group-id", default="", help="可选 group_id")
    args = parser.parse_args()

    base = args.base_url.rstrip("/")
    headers = build_headers(args.admin_token)

    try:
        threshold_url = f"{baseREDACTED/api/v1/admin/ops/settings/metric-thresholds"
        thresholds_raw = request_json(threshold_url, headers)
        thresholds = parse_thresholds(parse_envelope_data(thresholds_raw))

        query = {"platform": args.platform, "time_range": args.time_rangeREDACTED
        if args.group_id.strip():
            query["group_id"] = args.group_id.strip()
        overview_url = (
            f"{baseREDACTED/api/v1/admin/ops/dashboard/overview?"
            + urllib.parse.urlencode(query)
        )
        overview_raw = request_json(overview_url, headers)
        snapshot = parse_snapshot(parse_envelope_data(overview_raw))

        print("[OpenAI OAuth Gray Guard] 当前快照:")
        print(
            json.dumps(
                {
                    "sla": snapshot.sla,
                    "ttft_p99_ms": snapshot.ttft_p99_ms,
                    "request_error_rate_percent": snapshot.request_error_rate_percent,
                    "upstream_error_rate_percent": snapshot.upstream_error_rate_percent,
                REDACTED,
                ensure_ascii=False,
                indent=2,
            )
        )
        print("[OpenAI OAuth Gray Guard] 阈值配置:")
        print(
            json.dumps(
                {
                    "sla_percent_min": thresholds.sla_percent_min,
                    "ttft_p99_ms_max": thresholds.ttft_p99_ms_max,
                    "request_error_rate_percent_max": thresholds.request_error_rate_percent_max,
                    "upstream_error_rate_percent_max": thresholds.upstream_error_rate_percent_max,
                REDACTED,
                ensure_ascii=False,
                indent=2,
            )
        )

        violations = evaluate(snapshot, thresholds)
        if violations:
            print("[OpenAI OAuth Gray Guard] 检测到阈值违例：")
            for idx, line in enumerate(violations, start=1):
                print(f"  {idxREDACTED. {lineREDACTED")
            print("[OpenAI OAuth Gray Guard] 建议：停止扩量并执行回滚。")
            return 2

        print("[OpenAI OAuth Gray Guard] 指标通过，可继续观察或按计划扩量。")
        return 0

    except Exception as exc:
        print(f"[OpenAI OAuth Gray Guard] 执行失败: {excREDACTED", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
