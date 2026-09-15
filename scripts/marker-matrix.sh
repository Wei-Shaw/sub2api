#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/marker-matrix.sh [--project gpt2api|sub2api] [--markers FILE]...
                           [--target source:PATH|image:REF]... [--output FILE]
  scripts/marker-matrix.sh --compare PREV_IMAGE CAND_IMAGE [--project ...] [--markers FILE]...

Marker file format:
  feature name|literal marker string

The compare mode fails closed when a marker present in PREV is absent from CAND.
USAGE
}

fail() {
  echo "[marker-matrix] ERROR: $*" >&2
  exit 2
}

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT=""
OUTPUT=""
REQUIRE_ALL=0
COMPARE=0
PREV_IMAGE=""
CAND_IMAGE=""
MARKER_FILES=()
TARGETS=()

while [ "$#" -gt 0 ]; do
  case "$1" in
    --project)
      PROJECT="${2:-}"
      shift 2
      ;;
    --markers)
      MARKER_FILES+=("${2:-}")
      shift 2
      ;;
    --target)
      TARGETS+=("${2:-}")
      shift 2
      ;;
    --output)
      OUTPUT="${2:-}"
      shift 2
      ;;
    --require-all)
      REQUIRE_ALL=1
      shift
      ;;
    --compare)
      COMPARE=1
      PREV_IMAGE="${2:-}"
      CAND_IMAGE="${3:-}"
      shift 3
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

if [ -z "$PROJECT" ]; then
  if [ -f "$ROOT/go.mod" ] && [ -d "$ROOT/web" ]; then
    PROJECT="gpt2api"
  elif [ -f "$ROOT/backend/go.mod" ]; then
    PROJECT="sub2api"
  else
    fail "cannot infer project; pass --project"
  fi
fi

case "$PROJECT" in
  gpt2api|sub2api) ;;
  *) fail "unsupported project: $PROJECT" ;;
esac

if [ "${#MARKER_FILES[@]}" -eq 0 ]; then
  if [ ! -d "$ROOT/release/markers" ]; then
    fail "missing marker directory: $ROOT/release/markers"
  fi
  while IFS= read -r file; do
    MARKER_FILES+=("$file")
  done < <(find "$ROOT/release/markers" -type f -name '*.txt' | sort)
fi

FEATURES=()
MARKERS=()
for file in "${MARKER_FILES[@]}"; do
  [ -f "$file" ] || fail "missing marker file: $file"
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"
    [ -z "$line" ] && continue
    case "$line" in \#*) continue ;; esac
    feature="${line%%|*}"
    marker="${line#*|}"
    if [ "$feature" = "$line" ] || [ -z "$feature" ] || [ -z "$marker" ]; then
      fail "invalid marker line in $file: $line"
    fi
    FEATURES+=("$feature")
    MARKERS+=("$marker")
  done < "$file"
done

[ "${#MARKERS[@]}" -gt 0 ] || fail "no markers loaded"

source_has_marker() {
  local src="$1"
  local marker="$2"
  grep -R -I -F -q \
    --exclude-dir=.git \
    --exclude-dir=.codex-backups \
    --exclude-dir=.codex-build \
    --exclude-dir=node_modules \
    --exclude-dir=dist \
    --exclude-dir=vendor \
    -- "$marker" "$src" 2>/dev/null
}

image_payload() {
  local image="$1"
  local out="$2"
  docker image inspect "$image" >/dev/null 2>&1 || docker pull "$image" >/dev/null
  case "$PROJECT" in
    gpt2api)
      docker run --rm --entrypoint sh "$image" -c '
        [ -f /app/gpt2api ] && strings /app/gpt2api 2>/dev/null || true
        if [ -d /app/web/dist ]; then
          find /app/web/dist -maxdepth 30 -type f -exec grep -a -h "" {} + 2>/dev/null || true
        fi
      ' > "$out"
      ;;
    sub2api)
      docker run --rm --entrypoint sh "$image" -c '
        [ -f /app/sub2api ] && strings /app/sub2api 2>/dev/null || true
        for d in /app/resources /app/web/dist /app/backend/internal/web/dist; do
          if [ -d "$d" ]; then
            find "$d" -maxdepth 30 -type f -exec grep -a -h "" {} + 2>/dev/null || true
          fi
        done
      ' > "$out"
      ;;
  esac
}

payload_has_marker() {
  local payload="$1"
  local marker="$2"
  grep -F -q -- "$marker" "$payload"
}

emit_target_matrix() {
  local target="$1"
  local kind="${target%%:*}"
  local value="${target#*:}"
  local payload=""
  local missing=0

  case "$kind" in
    source)
      [ -d "$value" ] || fail "source target is not a directory: $value"
      ;;
    image)
      payload="$(mktemp)"
      image_payload "$value" "$payload"
      ;;
    *)
      fail "target must be source:PATH or image:REF: $target"
      ;;
  esac

  for i in "${!MARKERS[@]}"; do
    status="-"
    if [ "$kind" = "source" ]; then
      if source_has_marker "$value" "${MARKERS[$i]}"; then status="Y"; fi
    else
      if payload_has_marker "$payload" "${MARKERS[$i]}"; then status="Y"; fi
    fi
    if [ "$status" = "-" ] && [ "$REQUIRE_ALL" -eq 1 ]; then missing=1; fi
    printf '%s\t%s\t%s\t%s\n' "$target" "${FEATURES[$i]}" "${MARKERS[$i]}" "$status"
  done

  [ -z "$payload" ] || rm -f "$payload"
  return "$missing"
}

run_compare() {
  [ -n "$PREV_IMAGE" ] || fail "missing previous image"
  [ -n "$CAND_IMAGE" ] || fail "missing candidate image"

  local prev_payload cand_payload regressions
  prev_payload="$(mktemp)"
  cand_payload="$(mktemp)"
  regressions=0
  image_payload "$PREV_IMAGE" "$prev_payload"
  image_payload "$CAND_IMAGE" "$cand_payload"

  printf 'target\tfeature\tmarker\tstatus\n'
  for i in "${!MARKERS[@]}"; do
    prev="-"
    cand="-"
    payload_has_marker "$prev_payload" "${MARKERS[$i]}" && prev="Y"
    payload_has_marker "$cand_payload" "${MARKERS[$i]}" && cand="Y"
    printf 'image:%s\t%s\t%s\t%s\n' "$PREV_IMAGE" "${FEATURES[$i]}" "${MARKERS[$i]}" "$prev"
    printf 'image:%s\t%s\t%s\t%s\n' "$CAND_IMAGE" "${FEATURES[$i]}" "${MARKERS[$i]}" "$cand"
    if [ "$prev" = "Y" ] && [ "$cand" = "-" ]; then
      echo "REGRESSION: candidate lost previous image marker: ${FEATURES[$i]} | ${MARKERS[$i]}" >&2
      regressions=$((regressions + 1))
    fi
  done

  rm -f "$prev_payload" "$cand_payload"
  if [ "$regressions" -gt 0 ]; then
    echo "REGRESSION: candidate lost previous image markers ($regressions)" >&2
    return 42
  fi
}

run_all() {
  if [ "$COMPARE" -eq 1 ]; then
    run_compare
    return $?
  fi

  if [ "${#TARGETS[@]}" -eq 0 ]; then
    TARGETS=("source:$ROOT")
  fi

  printf 'target\tfeature\tmarker\tstatus\n'
  local missing=0
  for target in "${TARGETS[@]}"; do
    if ! emit_target_matrix "$target"; then
      missing=1
    fi
  done
  return "$missing"
}

if [ -n "$OUTPUT" ]; then
  mkdir -p "$(dirname "$OUTPUT")"
  run_all | tee "$OUTPUT"
else
  run_all
fi
