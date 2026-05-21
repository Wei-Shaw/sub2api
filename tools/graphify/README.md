# graphify automation

This repository keeps a checked-in code knowledge graph under `graphify-out/`.

## Files

- `graphify-out/graph.json` — queryable code graph.
- `graphify-out/GRAPH_REPORT.md` — readable summary for agents and humans.
- `tools/graphify/rebuild-code-graph.py` — deterministic AST-only rebuild script.
- `tools/graphify/install-hooks.sh` — installs the local pre-commit hook.

## Install the auto-update hook

Run once on Wille's dev machine:

```bash
tools/graphify/install-hooks.sh
```

After installation, every commit that stages code changes will:

1. rebuild `graphify-out/graph.json` and `graphify-out/GRAPH_REPORT.md`,
2. automatically `git add` those two generated files,
3. include the refreshed graph in the same commit.

To skip for an emergency commit:

```bash
SKIP_GRAPHIFY=1 git commit -m "..."
```

## Manual rebuild

```bash
PYTHONPATH=/Users/wille/projects/graphify /Users/wille/projects/graphify/.venv/bin/python tools/graphify/rebuild-code-graph.py
git add graphify-out/graph.json graphify-out/GRAPH_REPORT.md
```

This is AST-only and does not call an LLM.
