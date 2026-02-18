#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd)

OPENAI_KEY_OVERRIDE=${KEENBENCH_OPENAI_API_KEY-}
if [ -f "${REPO_ROOT}/.env" ]; then
  set -a
  # shellcheck disable=SC1090
  source "${REPO_ROOT}/.env"
  set +a
fi
if [ -n "${OPENAI_KEY_OVERRIDE}" ]; then
  export KEENBENCH_OPENAI_API_KEY="${OPENAI_KEY_OVERRIDE}"
fi

# Real models only — fake AI is not permitted in tests.
if [ "${KEENBENCH_FAKE_OPENAI:-}" = "1" ]; then
  echo "ERROR: KEENBENCH_FAKE_OPENAI=1 is not permitted. All AI tests must use real models." >&2
  echo "See CLAUDE.md for the testing policy." >&2
  exit 1
fi

OUTPUT_DIR=${KEENBENCH_E2E_SCREENSHOTS_DIR:-"${REPO_ROOT}/artifacts/screenshots"}
CAPTURE_SCRIPT=${KEENBENCH_E2E_CAPTURE_SCRIPT:-"${REPO_ROOT}/scripts/e2e/capture_window.sh"}
DEVICE=${KEENBENCH_E2E_DEVICE:-"linux"}
DATA_DIR=${KEENBENCH_DATA_DIR:-"${REPO_ROOT}/artifacts/e2e_data/$(date +%s)"}

if ! command -v flutter >/dev/null 2>&1; then
  echo "flutter not found in PATH." >&2
  exit 1
fi

if ! command -v import >/dev/null 2>&1; then
  echo "Missing dependency: ImageMagick 'import'." >&2
  exit 1
fi

if ! command -v xdotool >/dev/null 2>&1 && ! command -v wmctrl >/dev/null 2>&1; then
  echo "Missing dependency: xdotool or wmctrl." >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
mkdir -p "$DATA_DIR"

export KEENBENCH_E2E_SCREENSHOTS_DIR="$OUTPUT_DIR"
export KEENBENCH_E2E_CAPTURE_SCRIPT="$CAPTURE_SCRIPT"
export KEENBENCH_DATA_DIR="$DATA_DIR"
export KEENBENCH_E2E=1

if [ "${KEENBENCH_E2E_SINGLE:-0}" != "1" ]; then
  bash "${SCRIPT_DIR}/run_e2e_serial.sh"
  exit $?
fi

cd "$REPO_ROOT/app"
flutter test integration_test -d "$DEVICE"
