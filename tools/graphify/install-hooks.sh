#!/bin/sh
set -eu

# Install a local git pre-commit hook that keeps graphify-out/ in sync.
# Git hooks live under .git/hooks and are not versioned, so this script is the
# versioned installer.

ROOT=$(git rev-parse --show-toplevel)
HOOK="$ROOT/.git/hooks/pre-commit"
MARKER_START="# sub2api-graphify-hook-start"
MARKER_END="# sub2api-graphify-hook-end"

mkdir -p "$(dirname "$HOOK")"

if [ -f "$HOOK" ] && grep -q "$MARKER_START" "$HOOK"; then
  echo "graphify pre-commit hook already installed: $HOOK"
  exit 0
fi

if [ ! -f "$HOOK" ]; then
  printf '%s\n' '#!/bin/sh' > "$HOOK"
fi

cat >> "$HOOK" <<'EOF'

# sub2api-graphify-hook-start
# Keep graphify-out/graph.json and GRAPH_REPORT.md synced with code commits.
# Installed by: tools/graphify/install-hooks.sh

case "${SKIP_GRAPHIFY:-}" in
  1|true|yes) echo "[graphify] skipped by SKIP_GRAPHIFY=$SKIP_GRAPHIFY"; exit 0 ;;
esac

CHANGED_CODE=$(git diff --cached --name-only --diff-filter=ACMR | grep -E '\.(go|vue|ts|tsx|js|jsx|py|sql|yaml|yml|json|css|scss|html)$' || true)
if [ -z "$CHANGED_CODE" ]; then
  exit 0
fi

if [ ! -x "/Users/wille/projects/graphify/.venv/bin/python" ]; then
  echo "[graphify] /Users/wille/projects/graphify/.venv/bin/python not found; skipping graph rebuild." >&2
  echo "[graphify] Commit continues. Run tools/graphify/install-hooks.sh on Wille's dev machine to enable it." >&2
  exit 0
fi

echo "[graphify] Code changes staged; rebuilding knowledge graph before commit..."
PYTHONPATH="/Users/wille/projects/graphify:${PYTHONPATH:-}" /Users/wille/projects/graphify/.venv/bin/python tools/graphify/rebuild-code-graph.py

git add graphify-out/graph.json graphify-out/GRAPH_REPORT.md
# Do not auto-stage README or hook scripts here; only generated graph artifacts.

# sub2api-graphify-hook-end
EOF

chmod +x "$HOOK"
echo "Installed graphify pre-commit hook: $HOOK"
