#!/usr/bin/env bash
set -euo pipefail

REPO="${FRP_REPO:-lovitus/frp}"
RELEASE_TAG="${FRP_RELEASE_TAG:-}"
RAW_BASE="${FRP_RAW_BASE:-}"

BINARY_NAME="frpc"
CFG_NAME="frpc.toml"
INPUT_FD=0
HAS_TTY_FD=0

SERVER_ADDR="${FRP_SERVER_ADDR:-}"
MIX_BIND_PORT="${FRP_MIX_BIND_PORT:-7001}"
MIX_TOKEN="${FRP_MIX_TOKEN:-}"
MIX_TOKEN_B64="${FRP_MIX_TOKEN_B64:-}"
CLIENT_ID="${FRP_CLIENT_ID:-}"

usage() {
  cat <<'EOF'
Usage:
  install-frpc.sh [options]

Options:
  --repo owner/repo
  --release-tag vX.Y.Z
  --raw-base URL
  --server-addr HOST_OR_IP
  --mix-bind-port PORT
  --mix-token VALUE
  --mix-token-b64 BASE64_VALUE
  --client-id VALUE

Notes:
  - If --mix-token/--mix-token-b64 is provided, script will only ask for missing
    serverAddr and clientID.
  - Without mix token parameters, script enters standalone mode and asks all
    required fields.
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

download_file() {
  local url="$1"
  local out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fL "$url" -o "$out"
    return 0
  fi
  if command -v wget >/dev/null 2>&1; then
    wget -O "$out" "$url"
    return 0
  fi
  echo "Error: curl or wget is required." >&2
  return 1
}

trim() {
  local s="$1"
  s="$(printf '%s' "$s" | tr -d '\r\n')"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}

read_interactive_line() {
  local prompt="$1"
  local secret="${2:-false}"
  local value
  if [[ "$HAS_TTY_FD" -eq 1 ]]; then
    if [[ "$secret" == "true" ]]; then
      read -r -s -p "$prompt" value <&$INPUT_FD || return 1
      printf '\n' >&$INPUT_FD
    else
      read -r -p "$prompt" value <&$INPUT_FD || return 1
    fi
  else
    if [[ "$secret" == "true" ]]; then
      read -r -s -p "$prompt" value || return 1
      echo >&2
    else
      read -r -p "$prompt" value || return 1
    fi
  fi
  printf '%s' "$value"
}

prompt_line() {
  local label="$1"
  local default_value="${2:-}"
  local value
  if [[ -n "$default_value" ]]; then
    if ! value="$(read_interactive_line "${label} [${default_value}]: ")"; then
      return 1
    fi
    value="$(trim "$value")"
    if [[ -z "$value" ]]; then
      value="$default_value"
    fi
  else
    while true; do
      if ! value="$(read_interactive_line "${label}: ")"; then
        return 1
      fi
      value="$(trim "$value")"
      if [[ -n "$value" ]]; then
        break
      fi
      echo "This value is required." >&2
    done
  fi
  printf '%s' "$value"
}

prompt_port() {
  local label="$1"
  local default_value="$2"
  local value
  while true; do
    if ! value="$(prompt_line "$label" "$default_value")"; then
      return 1
    fi
    if [[ "$value" =~ ^[0-9]+$ ]] && ((value >= 1 && value <= 65535)); then
      printf '%s' "$value"
      return 0
    fi
    echo "Invalid port '${value}'. Expected 1..65535." >&2
  done
}

prompt_password() {
  local label="$1"
  local value
  while true; do
    if ! value="$(read_interactive_line "${label}: ")"; then
      return 1
    fi
    value="$(trim "$value")"
    if [[ -n "$value" ]]; then
      printf '%s' "$value"
      return 0
    fi
    echo "Password cannot be empty." >&2
  done
}

toml_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

decode_b64() {
  local raw="$1"
  if [[ -z "$raw" ]]; then
    return 0
  fi
  if command -v base64 >/dev/null 2>&1; then
    if base64 --help 2>/dev/null | grep -q '\-d'; then
      printf '%s' "$raw" | base64 -d 2>/dev/null && return 0
      printf '%s' "$raw" | base64 --decode 2>/dev/null && return 0
    fi
    printf '%s' "$raw" | base64 --decode 2>/dev/null && return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import base64,sys;print(base64.b64decode(sys.argv[1]).decode("utf-8"), end="")' "$raw"
    return 0
  fi
  return 1
}

detect_platform() {
  local os_name arch_name
  os_name="$(uname -s)"
  arch_name="$(uname -m)"

  case "$os_name" in
    Linux)
      if [[ -n "${ANDROID_ROOT:-}" ]] || uname -o 2>/dev/null | grep -qi 'android'; then
        DETECTED_OS="android"
      else
        DETECTED_OS="linux"
      fi
      ;;
    Darwin) DETECTED_OS="darwin" ;;
    FreeBSD) DETECTED_OS="freebsd" ;;
    OpenBSD) DETECTED_OS="openbsd" ;;
    *)
      echo "Unsupported OS: ${os_name}" >&2
      exit 1
      ;;
  esac

  case "$arch_name" in
    x86_64|amd64) DETECTED_ARCH="amd64" ;;
    i386|i686) DETECTED_ARCH="386" ;;
    aarch64|arm64) DETECTED_ARCH="arm64" ;;
    armv7l|armv7|armhf) DETECTED_ARCH="arm_hf" ;;
    armv6l|arm) DETECTED_ARCH="arm" ;;
    mips64) DETECTED_ARCH="mips64" ;;
    mips64el) DETECTED_ARCH="mips64le" ;;
    mips) DETECTED_ARCH="mips" ;;
    mipsel) DETECTED_ARCH="mipsle" ;;
    riscv64) DETECTED_ARCH="riscv64" ;;
    loongarch64) DETECTED_ARCH="loong64" ;;
    *)
      echo "Unsupported architecture: ${arch_name}" >&2
      exit 1
      ;;
  esac

  if [[ "$DETECTED_OS" == "android" && "$DETECTED_ARCH" != "arm64" ]]; then
    echo "Unsupported Android architecture: ${arch_name}. Current releases provide android_arm64." >&2
    exit 1
  fi
}

resolve_release_json() {
  if [[ -n "$RELEASE_TAG" ]]; then
    RELEASE_JSON="$(http_get "https://api.github.com/repos/${REPO}/releases/tags/${RELEASE_TAG}")"
  else
    RELEASE_JSON="$(http_get "https://api.github.com/repos/${REPO}/releases/latest")"
  fi
  RELEASE_TAG_RESOLVED="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [[ -z "$RELEASE_TAG_RESOLVED" ]]; then
    echo "Unable to resolve release tag from GitHub API." >&2
    exit 1
  fi
  RELEASE_VERSION="${RELEASE_TAG_RESOLVED#v}"
}

find_asset_url() {
	local suffix="$1"
	local archive="frp_${RELEASE_VERSION}_${suffix}.tar.gz"
	local asset_url
	asset_url="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/^[[:space:]]*"browser_download_url":[[:space:]]*"\([^"]*\)".*/\1/p' | grep "/${archive}$" | head -n 1 || true)"
	if [[ -z "$asset_url" ]]; then
		asset_url="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/^[[:space:]]*"browser_download_url":[[:space:]]*"\([^"]*\)".*/\1/p' | grep -E "/frp_[^/]+_${suffix}\.tar\.gz$" | head -n 1 || true)"
		if [[ -n "$asset_url" ]]; then
			echo "Warning: no exact asset for tag ${RELEASE_TAG_RESOLVED}, using ${asset_url##*/}" >&2
		fi
	fi
	if [[ -z "$asset_url" ]]; then
		echo "No matching asset found for ${suffix} in release ${RELEASE_TAG_RESOLVED}." >&2
		exit 1
  fi
  printf '%s' "$asset_url"
}

build_default_mix_token() {
  local pass_escaped
  pass_escaped="$(toml_escape "$1")"
  printf 'ss://chacha20-ietf-poly1305:%s,kcp://%s,ssh://frp:%s' "$pass_escaped" "$pass_escaped" "$pass_escaped"
}

generate_config() {
  local server_addr_esc client_id_esc mix_token_esc
  server_addr_esc="$(toml_escape "$SERVER_ADDR")"
  client_id_esc="$(toml_escape "$CLIENT_ID")"
  mix_token_esc="$(toml_escape "$MIX_TOKEN")"

  cat > "./${CFG_NAME}" <<EOF
serverAddr = "${server_addr_esc}"
mixBindPort = ${MIX_BIND_PORT}
mixToken = "${mix_token_esc}"
# mixFallbackHosts = "backup-a.example.com,backup-b.example.com:7002"

clientID = "${client_id_esc}"
allowGatewayTunnels = true
mixAllowGateway = true
loginFailExit = false
EOF
}

verify_and_smoke_run() {
  local verify_log run_log pid
  verify_log="$(mktemp)"
  run_log="$(mktemp)"

  if ! "./${BINARY_NAME}" verify -c "./${CFG_NAME}" >"${verify_log}" 2>&1; then
    echo "Config verification failed:"
    cat "${verify_log}" >&2
    rm -f "${verify_log}" "${run_log}"
    exit 1
  fi

  "./${BINARY_NAME}" -c "./${CFG_NAME}" >"${run_log}" 2>&1 &
  pid=$!
  sleep 3

  if kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" 2>/dev/null || true
    echo "Smoke start check passed."
  else
    echo "Smoke start check warning: frpc exited quickly. Recent log:"
    tail -n 60 "${run_log}" || true
    echo "You can still run manually after checking server reachability."
  fi
  rm -f "${verify_log}" "${run_log}"
}

while (($# > 0)); do
  case "$1" in
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
    --server-addr)
      SERVER_ADDR="${2:-}"
      shift 2
      ;;
    --mix-bind-port)
      MIX_BIND_PORT="${2:-}"
      shift 2
      ;;
    --mix-token)
      MIX_TOKEN="${2:-}"
      shift 2
      ;;
    --mix-token-b64)
      MIX_TOKEN_B64="${2:-}"
      shift 2
      ;;
    --client-id)
      CLIENT_ID="${2:-}"
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

if [[ -z "$RAW_BASE" ]]; then
  RAW_BASE="https://raw.githubusercontent.com/${REPO}/dev/hack/quick-deploy"
fi

if [[ -n "$MIX_TOKEN_B64" ]]; then
  if decoded="$(decode_b64 "$MIX_TOKEN_B64")"; then
    MIX_TOKEN="$decoded"
  else
    echo "Failed to decode --mix-token-b64 value." >&2
    exit 1
  fi
fi

echo "Repo: ${REPO}"
init_input_fd
detect_platform
echo "Detected platform: ${DETECTED_OS}_${DETECTED_ARCH}"
resolve_release_json
echo "Using release: ${RELEASE_TAG_RESOLVED}"

asset_suffix="${DETECTED_OS}_${DETECTED_ARCH}"
asset_url="$(find_asset_url "${asset_suffix}")"
echo "Downloading asset: ${asset_url}"

tmp_archive="$(mktemp)"
tmp_extract="$(mktemp -d)"
trap 'rm -f "${tmp_archive}"; rm -rf "${tmp_extract}"' EXIT

download_file "${asset_url}" "${tmp_archive}"
LC_ALL=C tar -xzf "${tmp_archive}" -C "${tmp_extract}"

pkg_dir="$(find "${tmp_extract}" -maxdepth 1 -type d -name "frp_${RELEASE_VERSION}_${asset_suffix}" | head -n 1 || true)"
if [[ -z "$pkg_dir" ]]; then
  echo "Extracted package directory not found." >&2
  exit 1
fi
if [[ ! -f "${pkg_dir}/${BINARY_NAME}" ]]; then
  echo "Binary ${BINARY_NAME} not found in package." >&2
  exit 1
fi

cp "${pkg_dir}/${BINARY_NAME}" "./${BINARY_NAME}"
chmod +x "./${BINARY_NAME}"

if [[ -z "$MIX_TOKEN" ]]; then
  echo
  echo "Configure frpc (standalone mode, press Enter to accept defaults)"
  if ! MIX_BIND_PORT="$(prompt_port "mixBindPort" "${MIX_BIND_PORT}")"; then
    echo "Input aborted." >&2
    exit 1
  fi
  if ! setup_password="$(prompt_password "Connection password (used for ss/kcp/ssh)")"; then
    echo "Input aborted." >&2
    exit 1
  fi
  MIX_TOKEN="$(build_default_mix_token "${setup_password}")"
else
  echo
  echo "Configure frpc (preset mode from frps command)"
  if ! MIX_BIND_PORT="$(prompt_port "mixBindPort" "${MIX_BIND_PORT}")"; then
    echo "Input aborted." >&2
    exit 1
  fi
fi

if ! SERVER_ADDR="$(prompt_line "serverAddr (frps IP/domain)" "${SERVER_ADDR}")"; then
  echo "Input aborted." >&2
  exit 1
fi
if ! CLIENT_ID="$(prompt_line "clientID" "${CLIENT_ID}")"; then
  echo "Input aborted." >&2
  exit 1
fi

generate_config
verify_and_smoke_run

cat <<EOF

Done. Generated files in current directory:
  - ./${BINARY_NAME}
  - ./${CFG_NAME}

One-click start command:
  ./${BINARY_NAME} -c ./${CFG_NAME}
EOF
