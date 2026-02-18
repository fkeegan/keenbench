#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "ERROR: package_macos.sh must be run on macOS (Darwin)" >&2
  exit 1
fi

parse_bool() {
  local v="${1:-}"
  v="$(echo "$v" | tr '[:upper:]' '[:lower:]' | xargs)"
  [[ "$v" == "1" || "$v" == "true" || "$v" == "yes" || "$v" == "y" || "$v" == "on" ]]
}

if ! command -v hdiutil >/dev/null 2>&1; then
  echo "ERROR: hdiutil not found (required to build .dmg)" >&2
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

DIST_ROOT="$ROOT/dist/macos"
if parse_bool "${KEENBENCH_MACOS_UNIVERSAL:-}"; then
  DMG_NAME_DEFAULT="KeenBench-macos-universal2.dmg"
else
  DMG_NAME_DEFAULT="KeenBench-macos.dmg"
fi
DMG_NAME="${KEENBENCH_DMG_NAME:-$DMG_NAME_DEFAULT}"
DMG_PATH="$ROOT/dist/$DMG_NAME"

mkdir -p "$DIST_ROOT"

assert_universal2() {
  local file="$1"
  if ! command -v lipo >/dev/null 2>&1; then
    echo "ERROR: lipo not found (required for universal2 verification)" >&2
    exit 1
  fi
  local info
  info="$(lipo -info "$file" 2>/dev/null || true)"
  if [[ "$info" != *"arm64"* || "$info" != *"x86_64"* ]]; then
    echo "ERROR: expected universal2 binary but got: $info" >&2
    echo "  file: $file" >&2
    exit 1
  fi
}

build_engine() {
  if parse_bool "${KEENBENCH_MACOS_UNIVERSAL:-}"; then
    if ! command -v lipo >/dev/null 2>&1; then
      echo "ERROR: lipo not found (required for universal2 engine)" >&2
      exit 1
    fi
    echo "Building engine (universal2)..."
    local out_arm="$DIST_ROOT/keenbench-engine-arm64"
    local out_amd="$DIST_ROOT/keenbench-engine-amd64"
    local out="$DIST_ROOT/keenbench-engine"
    rm -f "$out_arm" "$out_amd" "$out"
    (cd "$ROOT/engine" && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o "$out_arm" ./cmd/keenbench-engine)
    (cd "$ROOT/engine" && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o "$out_amd" ./cmd/keenbench-engine)
    lipo -create -output "$out" "$out_arm" "$out_amd"
    chmod +x "$out"
    assert_universal2 "$out"
    return
  fi

  echo "Building engine..."
  (cd "$ROOT/engine" && go build -o ../engine/bin/keenbench-engine ./cmd/keenbench-engine)
}

build_flutter_app() {
  if parse_bool "${KEENBENCH_MACOS_UNIVERSAL:-}"; then
    if ! command -v xcodebuild >/dev/null 2>&1; then
      echo "ERROR: xcodebuild not found (required for universal2 app build)" >&2
      exit 1
    fi
    echo "Building Flutter macOS app (release, universal2)..."
	    (cd "$ROOT/app" && "$FLUTTER_BIN" build macos --release --config-only)
	    (cd "$ROOT/app/macos" && \
	      xcodebuild \
	        -workspace Runner.xcworkspace \
	        -scheme Runner \
	        -configuration Release \
	        -destination 'generic/platform=macOS' \
	        ARCHS='arm64 x86_64' \
	        ONLY_ACTIVE_ARCH=NO \
	        CODE_SIGNING_ALLOWED=NO \
	        CODE_SIGNING_REQUIRED=NO \
	        CONFIGURATION_BUILD_DIR="$ROOT/app/build/macos/Build/Products/Release" \
	        build)
	    return
	  fi

  echo "Building Flutter macOS app (release)..."
  (cd "$ROOT/app" && "$FLUTTER_BIN" build macos --release)
}

build_tool_worker() {
  echo "Building standalone tool worker (macOS)..."
  if parse_bool "${KEENBENCH_MACOS_UNIVERSAL:-}"; then
    "$ROOT/scripts/build_toolworker_macos.sh" universal2
    return
  fi
  "$ROOT/scripts/build_toolworker_macos.sh" native
}

ENGINE_BIN_SRC="$ROOT/engine/bin/keenbench-engine"
if parse_bool "${KEENBENCH_MACOS_UNIVERSAL:-}"; then
  ENGINE_BIN_SRC="$DIST_ROOT/keenbench-engine"
fi

TOOL_WORKER_BIN_SRC="$DIST_ROOT/keenbench-tool-worker"

build_engine
build_flutter_app
build_tool_worker

APP_BUILD_DIR="$ROOT/app/build/macos/Build/Products/Release"
APP_SRC="$(find "$APP_BUILD_DIR" -maxdepth 1 -name "*.app" -print | head -n 1 || true)"
if [[ -z "$APP_SRC" ]]; then
  echo "ERROR: no .app found in $APP_BUILD_DIR" >&2
  exit 1
fi

APP_NAME="$(basename "$APP_SRC")"
APP_DST="$DIST_ROOT/$APP_NAME"

echo "Staging app bundle..."
rm -rf "$APP_DST"
cp -R "$APP_SRC" "$APP_DST"

if [[ ! -x "$ENGINE_BIN_SRC" ]]; then
  echo "ERROR: engine binary not found or not executable: $ENGINE_BIN_SRC" >&2
  exit 1
fi
if [[ ! -x "$TOOL_WORKER_BIN_SRC" ]]; then
  echo "ERROR: tool worker binary not found or not executable: $TOOL_WORKER_BIN_SRC" >&2
  exit 1
fi

if parse_bool "${KEENBENCH_MACOS_UNIVERSAL:-}"; then
  app_exec="$APP_DST/Contents/MacOS/app"
  if [[ ! -f "$app_exec" ]]; then
    app_exec="$(find "$APP_DST/Contents/MacOS" -maxdepth 1 -type f -print | head -n 1 || true)"
  fi
  if [[ -z "${app_exec:-}" || ! -f "$app_exec" ]]; then
    echo "ERROR: could not locate main app executable under $APP_DST/Contents/MacOS" >&2
    exit 1
  fi
  assert_universal2 "$ENGINE_BIN_SRC"
  assert_universal2 "$TOOL_WORKER_BIN_SRC"
  assert_universal2 "$app_exec"
fi

echo "Bundling engine + tool worker into .app..."
install -m 0755 "$ENGINE_BIN_SRC" "$APP_DST/Contents/MacOS/keenbench-engine"
install -m 0755 "$TOOL_WORKER_BIN_SRC" "$APP_DST/Contents/MacOS/keenbench-tool-worker"

if [[ -n "${KEENBENCH_CODESIGN_IDENTITY:-}" ]]; then
  IDENTITY="$KEENBENCH_CODESIGN_IDENTITY"
  echo "Codesigning (optional): $IDENTITY"

  codesign_one() {
    local path="$1"
    if [[ ! -e "$path" ]]; then
      echo "ERROR: codesign target not found: $path" >&2
      exit 1
    fi
    codesign --force --options runtime --timestamp --sign "$IDENTITY" "$path"
  }

  # Sign nested code first (inner -> outer) to avoid invalidating signatures.
  if [[ -d "$APP_DST/Contents/Frameworks" ]]; then
    while IFS= read -r -d '' f; do
      codesign_one "$f"
    done < <(find "$APP_DST/Contents/Frameworks" -type f \( -name "*.dylib" -o -name "*.so" \) -print0)

    while IFS= read -r -d '' d; do
      codesign_one "$d"
    done < <(find "$APP_DST/Contents/Frameworks" -type d -name "*.framework" -print0)
  fi

  for maybe_dir in "$APP_DST/Contents/PlugIns" "$APP_DST/Contents/XPCServices"; do
    if [[ -d "$maybe_dir" ]]; then
      while IFS= read -r -d '' bundle; do
        codesign_one "$bundle"
      done < <(find "$maybe_dir" -maxdepth 1 -type d \( -name "*.appex" -o -name "*.xpc" \) -print0)
    fi
  done

  if [[ -d "$APP_DST/Contents/MacOS" ]]; then
    while IFS= read -r -d '' exe; do
      codesign_one "$exe"
    done < <(find "$APP_DST/Contents/MacOS" -maxdepth 1 -type f -perm -111 -print0)
  fi

  # Finally sign the app bundle itself.
  codesign_one "$APP_DST"
  codesign --verify --strict --verbose=2 "$APP_DST"
else
  echo "Skipping codesign (set KEENBENCH_CODESIGN_IDENTITY to enable)."
fi

echo "Creating DMG..."
STAGE="$DIST_ROOT/dmg-stage"
rm -rf "$STAGE"
mkdir -p "$STAGE"
cp -R "$APP_DST" "$STAGE/"
ln -s /Applications "$STAGE/Applications"

rm -f "$DMG_PATH"
hdiutil create -volname "KeenBench" -srcfolder "$STAGE" -ov -format UDZO "$DMG_PATH" >/dev/null

if [[ -n "${KEENBENCH_CODESIGN_IDENTITY:-}" ]] && parse_bool "${KEENBENCH_CODESIGN_DMG:-}"; then
  echo "Codesigning DMG: $IDENTITY"
  codesign --force --timestamp --sign "$IDENTITY" "$DMG_PATH"
  codesign --verify --verbose=2 "$DMG_PATH"
fi

echo "OK: $DMG_PATH"
