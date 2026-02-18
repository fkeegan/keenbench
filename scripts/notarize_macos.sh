#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/notarize_macos.sh <artifact> [--profile <keychain-profile>]

Environment:
  KEENBENCH_NOTARY_PROFILE   Keychain profile name created with:
                          xcrun notarytool store-credentials <name> --apple-id ... --team-id ... --password ...

Examples:
  KEENBENCH_NOTARY_PROFILE=keenbench-notary scripts/notarize_macos.sh dist/KeenBench-macos-universal2.dmg
  scripts/notarize_macos.sh dist/KeenBench-macos.dmg --profile keenbench-notary
EOF
}

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "ERROR: notarize_macos.sh must be run on macOS (Darwin)" >&2
  exit 1
fi

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || -z "${1:-}" ]]; then
  usage
  exit 1
fi

ARTIFACT=""
PROFILE="${KEENBENCH_NOTARY_PROFILE:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --profile)
      PROFILE="${2:-}"
      shift 2
      ;;
    --profile=*)
      PROFILE="${1#*=}"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      if [[ -z "$ARTIFACT" ]]; then
        ARTIFACT="$1"
        shift
      else
        echo "ERROR: unexpected argument: $1" >&2
        usage >&2
        exit 1
      fi
      ;;
  esac
done

if [[ -z "$ARTIFACT" ]]; then
  echo "ERROR: missing artifact path" >&2
  usage >&2
  exit 1
fi
if [[ ! -f "$ARTIFACT" ]]; then
  echo "ERROR: artifact not found: $ARTIFACT" >&2
  exit 1
fi
if [[ -z "$PROFILE" ]]; then
  echo "ERROR: notarytool keychain profile not set (use --profile or KEENBENCH_NOTARY_PROFILE)" >&2
  exit 1
fi

if ! command -v xcrun >/dev/null 2>&1; then
  echo "ERROR: xcrun not found (Xcode Command Line Tools required)" >&2
  exit 1
fi

if ! xcrun notarytool --version >/dev/null 2>&1; then
  echo "ERROR: xcrun notarytool not available (install Xcode Command Line Tools, macOS 12+ recommended)" >&2
  exit 1
fi

echo "Submitting for notarization:"
echo "- artifact: $ARTIFACT"
echo "- profile:  $PROFILE"

tmp_json="$(mktemp -t keenbench-notary-submit.XXXXXX.json)"
cleanup() {
  rm -f "$tmp_json"
}
trap cleanup EXIT

set +e
xcrun notarytool submit "$ARTIFACT" \
  --keychain-profile "$PROFILE" \
  --wait \
  --output-format json >"$tmp_json"
rc=$?
set -e

cat "$tmp_json"

if [[ $rc -ne 0 ]]; then
  echo "ERROR: notarization failed (exit=$rc)" >&2
  if command -v python3 >/dev/null 2>&1; then
    request_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("id",""))' "$tmp_json" 2>/dev/null || true)"
    if [[ -n "${request_id:-}" ]]; then
      echo "Fetching notarization log (id=$request_id)..." >&2
      xcrun notarytool log "$request_id" --keychain-profile "$PROFILE" || true
    fi
  fi
  exit "$rc"
fi

echo "Stapling ticket..."
xcrun stapler staple "$ARTIFACT"

echo "Validating staple..."
xcrun stapler validate "$ARTIFACT"

if command -v spctl >/dev/null 2>&1; then
  echo "Gatekeeper assessment..."
  spctl -a -vvv -t install "$ARTIFACT" || true
fi

echo "OK: notarized + stapled $ARTIFACT"
