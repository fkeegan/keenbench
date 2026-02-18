#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORKER_DIR="$ROOT/engine/tools/pyworker"
VENV_DIR="$WORKER_DIR/.venv"
BIN_DIR="$ROOT/engine/bin"
WRAPPER="$BIN_DIR/keenbench-tool-worker"

PYTHON="${PYTHON:-}"
if [[ -z "${PYTHON}" ]]; then
  if command -v python3 >/dev/null 2>&1; then
    PYTHON="python3"
  elif command -v python >/dev/null 2>&1; then
    PYTHON="python"
  else
    echo "python not found in PATH" >&2
    exit 1
  fi
fi

mkdir -p "$WORKER_DIR" "$BIN_DIR"
"$PYTHON" -m venv "$VENV_DIR"
"$VENV_DIR/bin/pip" install --upgrade pip
"$VENV_DIR/bin/pip" install -r "$WORKER_DIR/requirements.txt"

cat > "$WRAPPER" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

# Navigate from engine/bin/ up to repo root
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VENV="$ROOT/engine/tools/pyworker/.venv"
WORKER="$ROOT/engine/tools/pyworker/worker.py"

exec "$VENV/bin/python" -u "$WORKER" "$@"
EOF

chmod +x "$WRAPPER"

echo "Worker venv created at: $VENV_DIR"
echo "Wrapper installed at: $WRAPPER"
echo "Set KEENBENCH_TOOL_WORKER_PATH=$WRAPPER"
