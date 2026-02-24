#!/usr/bin/env python3
"""OpenAI OAuth 灰度发布演练脚本（本地模拟）。

该脚本会启动本地 mock Ops API，调用 openai_oauth_gray_guard.py，
验证以下场景：
1) A/B/C/D 四个灰度批次均通过
2) 注入异常场景触发阈值告警并返回退出码 2（模拟自动回滚触发）
"""

from __future__ import annotations

import json
import subprocess
import threading
from dataclasses import dataclass
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path
from typing import Dict, Tuple
from urllib.parse import parse_qs, urlparse

ROOT = Path(__file__).resolve().parents[2]
GUARD_SCRIPT = ROOT / "tools" / "perf" / "openai_oauth_gray_guard.py"
REPORT_PATH = ROOT / "docs" / "perf" / "openai-oauth-gray-drill-report.md"


THRESHOLDS = {
    "sla_percent_min": 99.5,
    "ttft_p99_ms_max": 900,
    "request_error_rate_percent_max": 2.0,
    "upstream_error_rate_percent_max": 2.0,
REDACTED

STAGE_SNAPSHOTS: Dict[str, Dict[str, float]] = {
    "A": {"sla": 99.78, "ttft": 780, "error_rate": 1.20, "upstream_error_rate": 1.05REDACTED,
    "B": {"sla": 99.82, "ttft": 730, "error_rate": 1.05, "upstream_error_rate": 0.92REDACTED,
    "C": {"sla": 99.86, "ttft": 680, "error_rate": 0.88, "upstream_error_rate": 0.80REDACTED,
    "D": {"sla": 99.89, "ttft": 640, "error_rate": 0.72, "upstream_error_rate": 0.67REDACTED,
    "rollback": {"sla": 97.10, "ttft": 1550, "error_rate": 6.30, "upstream_error_rate": 5.60REDACTED,
REDACTED


class _MockHandler(BaseHTTPRequestHandler):
    def _write_json(self, payload: dict) -> None:
        raw = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def log_message(self, format: str, *args):  # noqa: A003
        return

    def do_GET(self):  # noqa: N802
        parsed = urlparse(self.path)
        if parsed.path.endswith("/api/v1/admin/ops/settings/metric-thresholds"):
            self._write_json({"code": 0, "message": "success", "data": THRESHOLDSREDACTED)
            return

        if parsed.path.endswith("/api/v1/admin/ops/dashboard/overview"):
            q = parse_qs(parsed.query)
            stage = (q.get("group_id") or ["A"])[0]
            snapshot = STAGE_SNAPSHOTS.get(stage, STAGE_SNAPSHOTS["A"])
            self._write_json(
                {
                    "code": 0,
                    "message": "success",
                    "data": {
                        "sla": snapshot["sla"],
                        "error_rate": snapshot["error_rate"],
                        "upstream_error_rate": snapshot["upstream_error_rate"],
                        "ttft": {"p99_ms": snapshot["ttft"]REDACTED,
                    REDACTED,
                REDACTED
            )
            return

        self.send_response(404)
        self.end_headers()


def run_guard(base_url: str, stage: str) -> Tuple[int, str]:
    cmd = [
        "python",
        str(GUARD_SCRIPT),
        "--base-url",
        base_url,
        "--platform",
        "openai",
        "--time-range",
        "30m",
        "--group-id",
        stage,
    ]
    proc = subprocess.run(cmd, cwd=str(ROOT), capture_output=True, text=True)
    output = (proc.stdout + "\n" + proc.stderr).strip()
    return proc.returncode, output


def main() -> int:
    server = HTTPServer(("127.0.0.1", 0), _MockHandler)
    host, port = server.server_address
    base_url = f"http://{hostREDACTED:{portREDACTED"

    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    lines = [
        "# OpenAI OAuth 灰度守护演练报告",
        "",
        "> 类型：本地 mock 演练（用于验证灰度守护与回滚触发机制）",
        f"> 生成脚本：`tools/perf/openai_oauth_gray_drill.py`",
        "",
        "## 1. 灰度批次结果（6.1）",
        "",
        "| 批次 | 流量比例 | 守护脚本退出码 | 结果 |",
        "|---|---:|---:|---|",
    ]

    batch_plan = [("A", "5%"), ("B", "20%"), ("C", "50%"), ("D", "100%")]
    all_pass = True
    for stage, ratio in batch_plan:
        code, _ = run_guard(base_url, stage)
        ok = code == 0
        all_pass = all_pass and ok
        lines.append(f"| {stageREDACTED | {ratioREDACTED | {codeREDACTED | {'通过' if ok else '失败'REDACTED |")

    lines.extend([
        "",
        "## 2. 回滚触发演练（6.2）",
        "",
    ])

    rollback_code, rollback_output = run_guard(base_url, "rollback")
    rollback_triggered = rollback_code == 2
    lines.append(f"- 注入异常场景退出码：`{rollback_codeREDACTED`")
    lines.append(f"- 是否触发回滚条件：`{'是' if rollback_triggered else '否'REDACTED`")
    lines.append("- 关键信息摘录：")
    excerpt = "\n".join(rollback_output.splitlines()[:8])
    lines.append("```text")
    lines.append(excerpt)
    lines.append("```")

    lines.extend([
        "",
        "## 3. 验收结论（6.3）",
        "",
        f"- 批次灰度结果：`{'通过' if all_pass else '不通过'REDACTED`",
        f"- 回滚触发机制：`{'通过' if rollback_triggered else '不通过'REDACTED`",
        f"- 结论：`{'通过（可进入真实环境灰度）' if all_pass and rollback_triggered else '不通过（需修复后复测）'REDACTED`",
    ])

    REPORT_PATH.parent.mkdir(parents=True, exist_ok=True)
    REPORT_PATH.write_text("\n".join(lines) + "\n", encoding="utf-8")

    server.shutdown()
    server.server_close()

    print(f"drill report generated: {REPORT_PATHREDACTED")
    return 0 if all_pass and rollback_triggered else 1


if __name__ == "__main__":
    raise SystemExit(main())
