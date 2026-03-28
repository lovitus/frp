#!/usr/bin/env bash
set -euo pipefail

REPO="${FRP_REPO:-lovitus/frp}"
RELEASE_TAG="${FRP_RELEASE_TAG:-}"
RAW_BASE="${FRP_RAW_BASE:-}"
MODE=""
INPUT_FD=0
HAS_TTY_FD=0

usage() {
  cat <<'EOF'
Usage:
  install.sh [--type frps|frpc] [--repo owner/repo] [--release-tag vX.Y.Z] [--raw-base URL]

Environment overrides:
  FRP_REPO, FRP_RELEASE_TAG, FRP_RAW_BASE
EOF
}

init_input_fd() {
  if { exec 9<>/dev/tty; } 2>/dev/null; then
    INPUT_FD=9
    HAS_TTY_FD=1
  else
    INPUT_FD=0
    HAS_TTY_FD=0
  fi
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

read_interactive_line() {
  local prompt="$1"
  local value
  if [[ "$HAS_TTY_FD" -eq 1 ]]; then
    read -r -p "$prompt" value <&$INPUT_FD || return 1
  else
    read -r -p "$prompt" value || return 1
  fi
  printf '%s' "$value"
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
  init_input_fd
  while true; do
    if ! MODE="$(read_interactive_line "Deploy frps(server) or frpc(client)? [frps/frpc]: ")"; then
      echo "Input aborted." >&2
      exit 1
    fi
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
