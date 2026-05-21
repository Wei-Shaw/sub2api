#!/usr/bin/env python3
"""Rebuild the checked-in graphify code knowledge graph for sub2api.

This script is intentionally AST-only: it updates graphify-out/graph.json and
GRAPH_REPORT.md without LLM calls, so it is safe to run from a git hook.
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "graphify-out"
INPUT_PATH = ROOT


def main() -> int:
    sys.path.insert(0, str(ROOT))
    try:
        from graphify.detect import detect
        from graphify.extract import extract
        from graphify.build import build_from_json
        from graphify.cluster import cluster, score_all
        from graphify.analyze import god_nodes, surprising_connections, suggest_questions
        from graphify.report import generate
        from graphify.export import to_json
    except Exception as exc:  # pragma: no cover - hook environment fallback
        print(f"[graphify] Python cannot import graphify: {exc}", file=sys.stderr)
        print("[graphify] Install graphify or run tools/graphify/install-hooks.sh on Wille's dev machine.", file=sys.stderr)
        return 1

    OUT.mkdir(exist_ok=True)

    detected = detect(INPUT_PATH)
    code_files = [Path(f) for f in detected.get("files", {}).get("code", [])]
    if not code_files:
        print("[graphify] No code files detected; graph not updated.", file=sys.stderr)
        return 1

    print(f"[graphify] Rebuilding code graph from {len(code_files)} code files...")
    extraction = extract(code_files)
    graph = build_from_json(extraction)
    communities = cluster(graph)
    cohesion = score_all(graph, communities)
    gods = god_nodes(graph)
    surprises = surprising_connections(graph, communities)
    labels = {cid: f"Community {cid}" for cid in communities}
    questions = suggest_questions(graph, communities, labels)

    report = generate(
        graph,
        communities,
        cohesion,
        labels,
        gods,
        surprises,
        detected,
        {"input": 0, "output": 0},
        str(INPUT_PATH),
        suggested_questions=questions,
    )
    (OUT / "GRAPH_REPORT.md").write_text(report, encoding="utf-8")
    to_json(graph, communities, str(OUT / "graph.json"))

    print(
        f"[graphify] Updated graphify-out/graph.json and GRAPH_REPORT.md "
        f"({graph.number_of_nodes()} nodes, {graph.number_of_edges()} edges, {len(communities)} communities)"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
