#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

APP_BIN="${ROOT_DIR}/app/build/linux/x64/debug/bundle/keenbench"
ENGINE_BIN="${ROOT_DIR}/engine/bin/keenbench-engine"
TOOL_WORKER_BIN="${ROOT_DIR}/engine/bin/keenbench-tool-worker"

if [[ ! -x "${APP_BIN}" ]]; then
  echo "KeenBench debug app binary was not found at ${APP_BIN}." >&2
  echo "Run 'make run' once to build it, then launch from GNOME again." >&2
  exit 1
fi

exec env \
  KEENBENCH_ENV_PATH="${ROOT_DIR}/.env" \
  KEENBENCH_ENGINE_PATH="${ENGINE_BIN}" \
  KEENBENCH_TOOL_WORKER_PATH="${TOOL_WORKER_BIN}" \
  "${APP_BIN}" "$@"
