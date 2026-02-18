#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "ERROR: build_toolworker_macos.sh must be run on macOS (Darwin)" >&2
  exit 1
fi

TARGET="${1:-native}" # native | arm64 | x86_64 | universal2

PYWORKER_DIR="$ROOT/engine/tools/pyworker"
REQS="$PYWORKER_DIR/requirements.txt"
OUT_DIR="$ROOT/dist/macos"

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

run_for_arch() {
  local arch="$1"
  shift
  if [[ "$arch" == "$host_arch" ]]; then
    "$@"
    return
  fi
  if [[ "$host_arch" == "arm64" && "$arch" == "x86_64" ]]; then
    arch -x86_64 "$@"
    return
  fi
  echo "ERROR: cannot run $arch commands on host arch $host_arch" >&2
  exit 1
}

build_one() {
  local arch="$1"
  local out="$2"

  local venv_dir="$PYWORKER_DIR/.packaging/venv-$arch"
  local build_dir="$PYWORKER_DIR/.packaging/pyinstaller-$arch"

  echo "Building macOS tool worker with PyInstaller ($arch)..."
  echo "- python: $PYTHON"
  echo "- output: $out"

  rm -rf "$venv_dir" "$build_dir" "$out"
  mkdir -p "$build_dir"

  if ! run_for_arch "$arch" "$PYTHON" -c 'import platform; print(platform.machine())' >/dev/null 2>&1; then
    echo "ERROR: $PYTHON cannot run under arch=$arch." >&2
    echo "Hint (Apple Silicon -> x86_64): install Rosetta and use a universal or x86_64 Python." >&2
    echo "  softwareupdate --install-rosetta --agree-to-license" >&2
    exit 1
  fi

  run_for_arch "$arch" "$PYTHON" -m venv "$venv_dir"
  run_for_arch "$arch" "$venv_dir/bin/python" -m pip install --upgrade pip >/dev/null

  # PyInstaller is a build-time dependency; keep it out of runtime requirements.
  run_for_arch "$arch" "$venv_dir/bin/python" -m pip install -q pyinstaller
  run_for_arch "$arch" "$venv_dir/bin/python" -m pip install -q -r "$REQS"

  run_for_arch "$arch" "$venv_dir/bin/pyinstaller" \
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
}

final_out=""
case "$TARGET" in
  native)
    build_one "$host_arch" "$OUT_DIR/keenbench-tool-worker"
    final_out="$OUT_DIR/keenbench-tool-worker"
    ;;
  arm64)
    build_one "arm64" "$OUT_DIR/keenbench-tool-worker-arm64"
    final_out="$OUT_DIR/keenbench-tool-worker-arm64"
    ;;
  x86_64)
    build_one "x86_64" "$OUT_DIR/keenbench-tool-worker-amd64"
    final_out="$OUT_DIR/keenbench-tool-worker-amd64"
    ;;
  universal2)
    # PyInstaller --onefile binaries cannot be merged with lipo: the outer launcher is a
    # thin Mach-O wrapper but each slice embeds its own arch-specific Python runtime archive.
    # After lipo the x86_64 launcher ends up extracting the arm64 archive and fails to load
    # the Python shared library on Intel hardware.  Ship two separate arch-specific binaries
    # and let the engine select the correct one via runtime.GOARCH at startup.
    build_one "arm64" "$OUT_DIR/keenbench-tool-worker-arm64"
    build_one "x86_64" "$OUT_DIR/keenbench-tool-worker-x86_64"
    final_out="$OUT_DIR/keenbench-tool-worker-arm64 $OUT_DIR/keenbench-tool-worker-x86_64"
    ;;
  *)
    echo "ERROR: unknown target '$TARGET' (expected native|arm64|x86_64|universal2)" >&2
    exit 1
    ;;
esac

echo "OK: built $final_out"
