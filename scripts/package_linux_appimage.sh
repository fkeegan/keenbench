#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "ERROR: package_linux_appimage.sh must be run on Linux" >&2
  exit 1
fi

FLUTTER_BIN="${FLUTTER_BIN:-flutter}"
if ! command -v "$FLUTTER_BIN" >/dev/null 2>&1; then
  echo "ERROR: flutter not found in PATH (set FLUTTER_BIN=...)" >&2
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "ERROR: go not found in PATH" >&2
  exit 1
fi

arch_raw="$(uname -m)"
appimage_arch="$arch_raw"
go_arch="$arch_raw"
case "$arch_raw" in
  x86_64)
    appimage_arch="x86_64"
    go_arch="amd64"
    ;;
  aarch64 | arm64)
    appimage_arch="aarch64"
    go_arch="arm64"
    ;;
esac

APP_ID="${LINUX_APP_ID:-com.keenbench.app}"
APP_BINARY="${LINUX_APP_BINARY:-keenbench}"
APP_DISPLAY_NAME="${LINUX_APP_DISPLAY_NAME:-KeenBench}"
APP_ICON_NAME="${LINUX_APP_ICON_NAME:-keenbench}"

DIST_ROOT="$ROOT/dist/linux"
APPDIR="$DIST_ROOT/AppDir"
APPIMAGE_NAME="${KEENBENCH_APPIMAGE_NAME:-KeenBench-linux-${appimage_arch}.AppImage}"
APPIMAGE_PATH="$ROOT/dist/$APPIMAGE_NAME"
ENGINE_BIN="$DIST_ROOT/keenbench-engine"
TOOL_WORKER_BIN="$DIST_ROOT/keenbench-tool-worker"

mkdir -p "$DIST_ROOT"

resolve_appimagetool() {
  local requested="${APPIMAGETOOL_BIN:-appimagetool}"
  if command -v "$requested" >/dev/null 2>&1; then
    command -v "$requested"
    return
  fi

  if [[ -x "$requested" ]]; then
    echo "$requested"
    return
  fi

  local tool_dir="$DIST_ROOT/tools"
  local tool_path="$tool_dir/appimagetool-$appimage_arch.AppImage"
  local tool_url="${KEENBENCH_APPIMAGETOOL_URL:-https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-$appimage_arch.AppImage}"
  mkdir -p "$tool_dir"

  if [[ ! -x "$tool_path" ]]; then
    echo "appimagetool not found; downloading $tool_url" >&2
    if command -v curl >/dev/null 2>&1; then
      curl -fL "$tool_url" -o "$tool_path"
    elif command -v wget >/dev/null 2>&1; then
      wget -O "$tool_path" "$tool_url"
    else
      echo "ERROR: neither curl nor wget is available to download appimagetool" >&2
      exit 1
    fi
    chmod +x "$tool_path"
  fi

  echo "$tool_path"
}

build_engine() {
  echo "Building engine (linux/$go_arch)..."
  rm -f "$ENGINE_BIN"
  (cd "$ROOT/engine" && CGO_ENABLED=0 GOOS=linux GOARCH="$go_arch" go build -o "$ENGINE_BIN" ./cmd/keenbench-engine)
  chmod +x "$ENGINE_BIN"
}

build_flutter_bundle() {
  echo "Building Flutter Linux app (release)..."
  (cd "$ROOT/app" && "$FLUTTER_BIN" build linux --release)
}

find_latest_flutter_bundle() {
  find "$ROOT/app/build/linux" -type d -path '*/release/bundle' -printf '%T@ %p\n' 2>/dev/null \
    | sort -nr \
    | head -n 1 \
    | cut -d' ' -f2-
}

build_tool_worker() {
  "$ROOT/scripts/build_toolworker_linux.sh"
}

prepare_desktop_file() {
  local dst="$1"
  sed \
    -e "s|@APP_DISPLAY_NAME@|$APP_DISPLAY_NAME|g" \
    -e "s|@BINARY_NAME@|$APP_BINARY|g" \
    -e "s|@APP_ICON_NAME@|$APP_ICON_NAME|g" \
    -e "s|@APPLICATION_ID@|$APP_ID|g" \
    "$ROOT/app/linux/keenbench.desktop.in" > "$dst"
}

stage_appdir() {
  local flutter_bundle="$1"

  echo "Staging AppDir..."
  rm -rf "$APPDIR"
  mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications"

  cp -a "$flutter_bundle/." "$APPDIR/usr/bin/"

  if [[ ! -x "$APPDIR/usr/bin/$APP_BINARY" ]]; then
    echo "ERROR: expected Flutter binary missing from bundle: $APPDIR/usr/bin/$APP_BINARY" >&2
    exit 1
  fi

  if [[ ! -x "$ENGINE_BIN" ]]; then
    echo "ERROR: engine binary missing: $ENGINE_BIN" >&2
    exit 1
  fi
  if [[ ! -x "$TOOL_WORKER_BIN" ]]; then
    echo "ERROR: tool worker binary missing: $TOOL_WORKER_BIN" >&2
    exit 1
  fi

  install -m 0755 "$ENGINE_BIN" "$APPDIR/usr/bin/keenbench-engine"
  install -m 0755 "$TOOL_WORKER_BIN" "$APPDIR/usr/bin/keenbench-tool-worker"

  local desktop_inside="$APPDIR/usr/share/applications/$APP_ID.desktop"
  prepare_desktop_file "$desktop_inside"
  ln -sf "usr/share/applications/$APP_ID.desktop" "$APPDIR/$APP_ID.desktop"

  local icon_src=""
  local icon_size=""
  for icon_size in 512 256 128 64 48 32 16; do
    if [[ -f "$ROOT/app/linux/runner/resources/${APP_ICON_NAME}_${icon_size}.png" ]]; then
      icon_src="$ROOT/app/linux/runner/resources/${APP_ICON_NAME}_${icon_size}.png"
      break
    fi
  done
  if [[ -z "$icon_src" ]]; then
    echo "ERROR: could not find app icon for $APP_ICON_NAME under app/linux/runner/resources" >&2
    exit 1
  fi

  mkdir -p "$APPDIR/usr/share/icons/hicolor/${icon_size}x${icon_size}/apps"
  cp "$icon_src" "$APPDIR/usr/share/icons/hicolor/${icon_size}x${icon_size}/apps/${APP_ICON_NAME}.png"
  ln -sf "usr/share/icons/hicolor/${icon_size}x${icon_size}/apps/${APP_ICON_NAME}.png" "$APPDIR/${APP_ICON_NAME}.png"
  ln -sf "${APP_ICON_NAME}.png" "$APPDIR/.DirIcon"

  cat > "$APPDIR/AppRun" <<EOF
#!/usr/bin/env bash
set -euo pipefail
HERE="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
exec "\$HERE/usr/bin/$APP_BINARY" "\$@"
EOF
  chmod +x "$APPDIR/AppRun"
}

APPIMAGETOOL="$(resolve_appimagetool)"

build_engine
build_flutter_bundle
build_tool_worker

FLUTTER_BUNDLE="$(find_latest_flutter_bundle)"
if [[ -z "$FLUTTER_BUNDLE" || ! -d "$FLUTTER_BUNDLE" ]]; then
  echo "ERROR: failed to locate Flutter Linux release bundle under app/build/linux" >&2
  exit 1
fi

stage_appdir "$FLUTTER_BUNDLE"

echo "Building AppImage..."
rm -f "$APPIMAGE_PATH"
ARCH="$appimage_arch" APPIMAGE_EXTRACT_AND_RUN=1 "$APPIMAGETOOL" "$APPDIR" "$APPIMAGE_PATH"

if [[ ! -f "$APPIMAGE_PATH" ]]; then
  echo "ERROR: AppImage build did not produce: $APPIMAGE_PATH" >&2
  exit 1
fi

chmod +x "$APPIMAGE_PATH"
echo "OK: $APPIMAGE_PATH"
