#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/deploy-by-digest.sh --manifest release-manifest.json --project NAME
                              [--compose FILE --service SERVICE]
                              [--container CONTAINER] [--lock FILE]
                              [--apply] [--restart] [--skip-marker-compare] [--skip-pull]

This script refuses mutable tag-only deployments. The manifest must be produced by CI
and image_ref must contain @sha256:<digest>.
USAGE
}

fail() {
  echo "[deploy-by-digest] ERROR: $*" >&2
  exit 2
}

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MANIFEST=""
PROJECT=""
COMPOSE=""
SERVICE=""
CONTAINER=""
LOCK_FILE=""
APPLY=0
RESTART=0
SKIP_MARKER_COMPARE=0
SKIP_PULL=0

while [ "$#" -gt 0 ]; do
  case "$1" in
    --manifest)
      MANIFEST="${2:-}"
      shift 2
      ;;
    --project)
      PROJECT="${2:-}"
      shift 2
      ;;
    --compose)
      COMPOSE="${2:-}"
      shift 2
      ;;
    --service)
      SERVICE="${2:-}"
      shift 2
      ;;
    --container)
      CONTAINER="${2:-}"
      shift 2
      ;;
    --lock)
      LOCK_FILE="${2:-}"
      shift 2
      ;;
    --apply)
      APPLY=1
      shift
      ;;
    --restart)
      RESTART=1
      shift
      ;;
    --skip-marker-compare)
      SKIP_MARKER_COMPARE=1
      shift
      ;;
    --skip-pull)
      SKIP_PULL=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

[ -n "$MANIFEST" ] || fail "missing --manifest"
[ -n "$PROJECT" ] || fail "missing --project"

VERIFY="$ROOT/scripts/verify-release-manifest.sh"
MATRIX="$ROOT/scripts/marker-matrix.sh"
[ -x "$VERIFY" ] || fail "missing executable verifier: $VERIFY"
[ -x "$MATRIX" ] || fail "missing executable marker matrix: $MATRIX"

IMAGE_REF="$("$VERIFY" "$MANIFEST" --project "$PROJECT" --require-ci --print-image-ref)"
case "$IMAGE_REF" in
  *@sha256:*) ;;
  *) fail "manifest image_ref is not immutable: $IMAGE_REF" ;;
esac

echo "[deploy-by-digest] manifest ok"
echo "[deploy-by-digest] image_ref=$IMAGE_REF"

if [ "$SKIP_PULL" -eq 0 ] && { [ "$APPLY" -eq 1 ] || [ "$RESTART" -eq 1 ]; }; then
  docker pull "$IMAGE_REF"
fi

if [ "$SKIP_MARKER_COMPARE" -eq 0 ] && [ -n "$CONTAINER" ]; then
  PREV_IMAGE="$(docker inspect "$CONTAINER" --format '{{.Config.Image}}')"
  echo "[deploy-by-digest] previous_image=$PREV_IMAGE"
  "$MATRIX" --project "$PROJECT" --compare "$PREV_IMAGE" "$IMAGE_REF"
fi

if [ "$APPLY" -eq 0 ]; then
  echo "[deploy-by-digest] dry-run only; pass --apply to edit compose/lock files"
  exit 0
fi

[ -n "$COMPOSE" ] || fail "--apply requires --compose"
[ -n "$SERVICE" ] || fail "--apply requires --service"
[ -f "$COMPOSE" ] || fail "compose file not found: $COMPOSE"

TS="$(date -u +%Y%m%dT%H%M%SZ)"
cp -a "$COMPOSE" "$COMPOSE.bak-$TS-before-digest-deploy"
if [ -n "$LOCK_FILE" ] && [ -f "$LOCK_FILE" ]; then
  cp -a "$LOCK_FILE" "$LOCK_FILE.bak-$TS-before-digest-deploy"
fi

python3 - "$COMPOSE" "$SERVICE" "$IMAGE_REF" <<'PY'
import re
import sys

path, service, image = sys.argv[1:4]
with open(path, "r", encoding="utf-8") as fh:
    lines = fh.readlines()

service_re = re.compile(rf"^(\s*){re.escape(service)}:\s*(?:#.*)?$")
image_re = re.compile(r"^(\s*)image:\s*.*$")
in_service = False
service_indent = None
replaced = False

for idx, line in enumerate(lines):
    if not in_service:
        m = service_re.match(line)
        if m:
            in_service = True
            service_indent = len(m.group(1))
        continue

    stripped = line.strip()
    if stripped and not line.startswith(" ") and not line.startswith("\t"):
        break
    indent = len(line) - len(line.lstrip(" "))
    if stripped and indent <= service_indent and re.match(r"^[A-Za-z0-9_.-]+:", stripped):
        break
    m = image_re.match(line)
    if m:
        lines[idx] = f"{m.group(1)}image: {image}\n"
        replaced = True
        break

if not replaced:
    raise SystemExit(f"service {service!r} image line not found in {path}")

with open(path, "w", encoding="utf-8") as fh:
    fh.writelines(lines)
PY

if [ -n "$LOCK_FILE" ]; then
  printf '%s\n' "$IMAGE_REF" > "$LOCK_FILE"
fi

echo "[deploy-by-digest] updated compose=$COMPOSE service=$SERVICE"
[ -z "$LOCK_FILE" ] || echo "[deploy-by-digest] updated lock=$LOCK_FILE"

if [ "$RESTART" -eq 1 ]; then
  docker compose -f "$COMPOSE" up -d --no-deps "$SERVICE"
  echo "[deploy-by-digest] restarted service=$SERVICE"
else
  echo "[deploy-by-digest] restart skipped; pass --restart to run docker compose up"
fi
