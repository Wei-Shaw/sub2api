# sub2api code knowledge graph

Generated with `/Users/wille/projects/graphify` for this repository.

Files:

- `GRAPH_REPORT.md` — human-readable graph summary: god nodes, communities, suggested questions.
- `graph.json` — queryable graphify knowledge graph for code structure.

Automation:

- `tools/graphify/rebuild-code-graph.py` rebuilds this graph deterministically from code ASTs.
- `tools/graphify/install-hooks.sh` installs a local pre-commit hook so code commits refresh and stage the graph automatically.

Generation notes:

- Scope: code AST extraction only.
- Excludes: `.git`, `node_modules`, generated frontend dist, Ent generated code, migrations, media, and other heavy/build artifacts via `.graphifyignore`.
- The graph intentionally does not include semantic LLM extraction for docs, to avoid token cost and keep the checked-in artifact focused on code navigation.

Useful commands:

```bash
# Query the graph from repo root
/Users/wille/projects/graphify/.venv/bin/graphify query "show the auth flow" --graph graphify-out/graph.json

# Regenerate code graph after major architecture changes
/Users/wille/projects/graphify/.venv/bin/python - <<'PY'
# See graphify README / skill for full pipeline.
PY
```
