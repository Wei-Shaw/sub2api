#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/verify-release-manifest.sh MANIFEST.json [--project NAME] [--image IMAGE_REF]
                                             [--require-ci] [--print-digest|--print-image-ref]
USAGE
}

MANIFEST=""
PROJECT=""
IMAGE_REF=""
REQUIRE_CI=0
PRINT_MODE=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --project)
      PROJECT="${2:-}"
      shift 2
      ;;
    --image)
      IMAGE_REF="${2:-}"
      shift 2
      ;;
    --require-ci)
      REQUIRE_CI=1
      shift
      ;;
    --print-digest)
      PRINT_MODE="digest"
      shift
      ;;
    --print-image-ref)
      PRINT_MODE="image_ref"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      if [ -z "$MANIFEST" ]; then
        MANIFEST="$1"
        shift
      else
        echo "[verify-release-manifest] ERROR: unknown argument: $1" >&2
        exit 2
      fi
      ;;
  esac
done

[ -n "$MANIFEST" ] || { usage >&2; exit 2; }
[ -f "$MANIFEST" ] || { echo "[verify-release-manifest] ERROR: missing manifest: $MANIFEST" >&2; exit 2; }

python3 - "$MANIFEST" "$PROJECT" "$IMAGE_REF" "$REQUIRE_CI" "$PRINT_MODE" <<'PY'
import json
import re
import sys

path, expected_project, expected_image, require_ci, print_mode = sys.argv[1:6]
require_ci = require_ci == "1"

with open(path, "r", encoding="utf-8") as fh:
    manifest = json.load(fh)

errors = []

def need(name):
    value = manifest.get(name)
    if value in (None, "", [], {}):
        errors.append(f"missing required field: {name}")
    return value

project = need("project")
git_sha = need("git_sha")
image = need("image")
image_ref = need("image_ref")
image_digest = need("image_digest")
created_at = need("created_at")
source = need("source")
marker_matrix = need("marker_matrix")
tests = need("tests")
attestations = need("attestations")

if expected_project and project != expected_project:
    errors.append(f"project mismatch: manifest={project!r} expected={expected_project!r}")

if isinstance(git_sha, str) and not re.fullmatch(r"[0-9a-fA-F]{40}", git_sha):
    errors.append("git_sha must be a full 40-character commit SHA")

if isinstance(image_digest, str) and not re.fullmatch(r"sha256:[0-9a-fA-F]{64}", image_digest):
    errors.append("image_digest must be sha256:<64 hex chars>")

if isinstance(image_ref, str):
    if "@sha256:" not in image_ref:
        errors.append("image_ref must be immutable and contain @sha256:")
    if isinstance(image_digest, str) and not image_ref.endswith(image_digest):
        errors.append("image_ref must end with image_digest")
else:
    errors.append("image_ref must be a string")

if expected_image and image_ref != expected_image:
    errors.append(f"image_ref mismatch: manifest={image_ref!r} expected={expected_image!r}")

if require_ci:
    if source != "github-actions":
        errors.append("source must be github-actions when --require-ci is set")
    if not manifest.get("ci_run_url"):
        errors.append("ci_run_url is required when --require-ci is set")

if isinstance(marker_matrix, dict):
    regressions = marker_matrix.get("regressions", 0)
    if isinstance(regressions, list) and regressions:
        errors.append("marker_matrix.regressions must be empty")
    if isinstance(regressions, int) and regressions != 0:
        errors.append("marker_matrix.regressions must be 0")
    if marker_matrix.get("status") not in ("success", "passed"):
        errors.append("marker_matrix.status must be success or passed")
else:
    errors.append("marker_matrix must be an object")

if isinstance(tests, list):
    for item in tests:
        if not isinstance(item, dict):
            errors.append("tests entries must be objects")
            continue
        if item.get("status") != "success":
            errors.append(f"test did not succeed: {item.get('name', '<unnamed>')}")
elif isinstance(tests, dict):
    for name, status in tests.items():
        if status != "success":
            errors.append(f"test did not succeed: {name}")
else:
    errors.append("tests must be a list or object")

if isinstance(attestations, dict):
    if attestations.get("sbom") is not True:
        errors.append("attestations.sbom must be true")
    if attestations.get("provenance") is not True:
        errors.append("attestations.provenance must be true")
else:
    errors.append("attestations must be an object")

if errors:
    for err in errors:
        print(f"[verify-release-manifest] ERROR: {err}", file=sys.stderr)
    sys.exit(3)

if print_mode == "digest":
    print(image_digest)
elif print_mode == "image_ref":
    print(image_ref)
else:
    print(f"[verify-release-manifest] ok project={project} image_ref={image_ref} created_at={created_at}")
PY
