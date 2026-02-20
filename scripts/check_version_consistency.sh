#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PUBSPEC_PATH="$ROOT/app/pubspec.yaml"
ENGINE_PATH="$ROOT/engine/internal/engine/engine.go"
CHANGELOG_PATH="$ROOT/CHANGELOG.md"

require_value() {
  local value="$1"
  local label="$2"
  if [[ -z "$value" ]]; then
    echo "ERROR: failed to parse $label" >&2
    exit 1
  fi
}

app_version_raw="$(awk '/^version:[[:space:]]*/ {print $2; exit}' "$PUBSPEC_PATH")"
require_value "$app_version_raw" "$PUBSPEC_PATH version"
app_version_core="${app_version_raw%%+*}"
require_value "$app_version_core" "$PUBSPEC_PATH core version"

engine_version="$(sed -n 's/^.*EngineVersion = "\([^"]*\)".*$/\1/p' "$ENGINE_PATH" | head -n 1)"
require_value "$engine_version" "$ENGINE_PATH EngineVersion"

changelog_version="$(awk '
  /^## \[[0-9]+\.[0-9]+\.[0-9]+\]/ {
    line = $0
    sub(/^## \[/, "", line)
    sub(/\].*$/, "", line)
    print line
    exit
  }
' "$CHANGELOG_PATH")"
require_value "$changelog_version" "$CHANGELOG_PATH latest release section"

echo "Version sources:"
echo "- app/pubspec.yaml: $app_version_raw (core: $app_version_core)"
echo "- engine/internal/engine/engine.go: $engine_version"
echo "- CHANGELOG.md latest release: $changelog_version"

if [[ "$app_version_core" != "$engine_version" || "$app_version_core" != "$changelog_version" ]]; then
  echo "ERROR: version mismatch detected across version sources" >&2
  exit 1
fi

echo "OK: version sources are aligned at $app_version_core"
