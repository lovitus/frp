#!/usr/bin/env bash
set -euo pipefail

REPO="${FRP_REPO:-lovitus/frp}"
RELEASE_TAG="${FRP_RELEASE_TAG:-}"
RAW_BASE="${FRP_RAW_BASE:-}"
MODE=""

usage() {
  cat <<'EOF'
Usage:
  install.sh [--type frps|frpc] [--repo owner/repo] [--release-tag vX.Y.Z] [--raw-base URL]

Environment overrides:
  FRP_REPO, FRP_RELEASE_TAG, FRP_RAW_BASE
EOF
}

http_get() {
  local url="$1"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url"
    return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -qO- "$url"
    return 0
  fi
  echo "Error: curl or wget is required." >&2
  return 1
}

while (($# > 0)); do
  case "$1" in
    --type)
      MODE="${2:-}"
      shift 2
      ;;
    --repo)
      REPO="${2:-}"
      shift 2
      ;;
    --release-tag)
      RELEASE_TAG="${2:-}"
      shift 2
      ;;
    --raw-base)
      RAW_BASE="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$MODE" ]]; then
  while true; do
    read -r -p "Deploy frps(server) or frpc(client)? [frps/frpc]: " MODE
    MODE="$(echo "$MODE" | tr '[:upper:]' '[:lower:]' | xargs)"
    if [[ "$MODE" == "frps" || "$MODE" == "frpc" ]]; then
      break
    fi
    echo "Please enter 'frps' or 'frpc'."
  done
fi

if [[ "$MODE" != "frps" && "$MODE" != "frpc" ]]; then
  echo "Error: --type must be frps or frpc." >&2
  exit 1
fi

if [[ -z "$RAW_BASE" ]]; then
  RAW_BASE="https://raw.githubusercontent.com/${REPO}/dev/hack/quick-deploy"
fi

TARGET_SCRIPT="install-${MODE}.sh"
TARGET_URL="${RAW_BASE}/${TARGET_SCRIPT}"

echo "Using repo: ${REPO}"
echo "Fetching: ${TARGET_URL}"

PASS_ARGS=(--repo "$REPO" --raw-base "$RAW_BASE")
if [[ -n "$RELEASE_TAG" ]]; then
  PASS_ARGS+=(--release-tag "$RELEASE_TAG")
fi

SCRIPT_CONTENT="$(http_get "$TARGET_URL")"
echo "Starting ${TARGET_SCRIPT}..."
printf '%s\n' "$SCRIPT_CONTENT" | bash -s -- "${PASS_ARGS[@]}"
