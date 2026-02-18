#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $(basename "$0") <label>" >&2
  exit 2
fi

label="$1"

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)
REPO_ROOT=$(cd -- "${SCRIPT_DIR}/../.." >/dev/null 2>&1 && pwd)

OUTPUT_DIR=${KEENBENCH_E2E_SCREENSHOTS_DIR:-"${REPO_ROOT}/artifacts/screenshots"}
PID=${KEENBENCH_E2E_PID:-""}
WINDOW_CLASS=${KEENBENCH_E2E_WINDOW_CLASS:-""}
WINDOW_TITLE=${KEENBENCH_E2E_WINDOW_TITLE:-""}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing dependency: $1" >&2
    return 1
  fi
  return 0
}

if ! require_cmd import; then
  echo "Install ImageMagick to provide the 'import' command." >&2
  exit 1
fi

if ! command -v xdotool >/dev/null 2>&1 && ! command -v wmctrl >/dev/null 2>&1; then
  echo "Missing dependency: xdotool or wmctrl (for window lookup)." >&2
  exit 1
fi

if [[ -z "${DISPLAY:-}" ]]; then
  echo "DISPLAY is not set; X11 is required for window screenshots." >&2
  exit 1
fi

mkdir -p "$OUTPUT_DIR"

timestamp=$(date +%Y%m%d_%H%M%S_%3N)
safe_label=$(echo "$label" | tr '[:upper:]' '[:lower:]' | tr -cs 'a-z0-9._-' '_')
output_path="${OUTPUT_DIR}/${timestamp}_${safe_label}.png"

find_window_id() {
  local win_id=""

  if command -v xdotool >/dev/null 2>&1; then
    if [[ -n "$WINDOW_CLASS" ]]; then
      win_id=$(xdotool search --onlyvisible --class "$WINDOW_CLASS" 2>/dev/null | tail -n 1 || true)
    elif [[ -n "$WINDOW_TITLE" ]]; then
      win_id=$(xdotool search --onlyvisible --name "$WINDOW_TITLE" 2>/dev/null | tail -n 1 || true)
    fi
  fi

  if [[ -z "$win_id" ]] && command -v wmctrl >/dev/null 2>&1; then
    if [[ -n "$WINDOW_CLASS" ]]; then
      win_id=$(wmctrl -lx | awk -v cls="$WINDOW_CLASS" 'tolower($3) ~ tolower(cls) {print $1; exit}')
    elif [[ -n "$WINDOW_TITLE" ]]; then
      win_id=$(wmctrl -l | awk -v title="$WINDOW_TITLE" 'tolower($0) ~ tolower(title) {print $1; exit}')
    fi
  fi

  if [[ -z "$win_id" ]] && [[ -n "$PID" ]] && command -v xdotool >/dev/null 2>&1; then
    win_id=$(xdotool search --pid "$PID" --onlyvisible 2>/dev/null | head -n 1 || true)
  fi

  if [[ -z "$win_id" ]] && [[ -n "$PID" ]] && command -v wmctrl >/dev/null 2>&1; then
    win_id=$(wmctrl -lp | awk -v pid="$PID" '$3==pid {print $1; exit}')
  fi

  echo "$win_id"
}

window_id=$(find_window_id)
if [[ -z "$window_id" ]]; then
  echo "Could not find a visible window." >&2
  if command -v wmctrl >/dev/null 2>&1; then
    echo "Open windows (wmctrl -lx):" >&2
    wmctrl -lx >&2 || true
  fi
  exit 1
fi

if command -v wmctrl >/dev/null 2>&1; then
  wmctrl -ia "$window_id" >/dev/null 2>&1 || true
elif command -v xdotool >/dev/null 2>&1; then
  xdotool windowactivate --sync "$window_id" >/dev/null 2>&1 || true
fi

if [[ "$window_id" != 0x* ]]; then
  window_id=$(printf '0x%x' "$window_id")
fi

import -window "$window_id" -silent "$output_path"

echo "$output_path"
