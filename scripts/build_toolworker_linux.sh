#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "ERROR: build_toolworker_linux.sh must be run on Linux" >&2
  exit 1
fi

PYWORKER_DIR="$ROOT/engine/tools/pyworker"
REQS="$PYWORKER_DIR/requirements.txt"
OUT_DIR="$ROOT/dist/linux"

mkdir -p "$OUT_DIR" "$PYWORKER_DIR/.packaging"

PYTHON="${PYTHON:-python3}"
if ! command -v "$PYTHON" >/dev/null 2>&1; then
  echo "ERROR: $PYTHON not found in PATH" >&2
  exit 1
fi

if [[ ! -f "$REQS" ]]; then
  echo "ERROR: requirements file not found: $REQS" >&2
  exit 1
fi

host_arch="$(uname -m)"
venv_dir="$PYWORKER_DIR/.packaging/venv-linux-$host_arch"
build_dir="$PYWORKER_DIR/.packaging/pyinstaller-linux-$host_arch"
out="$OUT_DIR/keenbench-tool-worker"

echo "Building Linux tool worker with PyInstaller ($host_arch)..."
echo "- python: $PYTHON"
echo "- output: $out"

rm -rf "$venv_dir" "$build_dir" "$out"
mkdir -p "$build_dir"

"$PYTHON" -m venv "$venv_dir"
"$venv_dir/bin/python" -m pip install --upgrade pip >/dev/null

# PyInstaller is build-time only; keep it out of runtime requirements.
"$venv_dir/bin/python" -m pip install -q pyinstaller
"$venv_dir/bin/python" -m pip install -q -r "$REQS"

"$venv_dir/bin/pyinstaller" \
  --clean \
  --noconfirm \
  --onefile \
  --name "$(basename "$out")" \
  --distpath "$(dirname "$out")" \
  --workpath "$build_dir/work" \
  --specpath "$build_dir/spec" \
  "$PYWORKER_DIR/worker.py"

if [[ ! -x "$out" ]]; then
  echo "ERROR: tool worker build did not produce an executable at $out" >&2
  exit 1
fi

echo "OK: built $out"
