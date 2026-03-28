#!/usr/bin/env bash
set -euo pipefail

REPO="${FRP_REPO:-lovitus/frp}"
RELEASE_TAG="${FRP_RELEASE_TAG:-}"
RAW_BASE="${FRP_RAW_BASE:-}"

BINARY_NAME="frps"
CFG_NAME="frps.toml"
INPUT_FD=0
HAS_TTY_FD=0

BIND_PORT="7000"
MIX_BIND_PORT="7001"
CONNECT_PASSWORD=""
DASHBOARD_ADDR="0.0.0.0"
DASHBOARD_PORT=""
DASHBOARD_USER="admin"
DASHBOARD_PASSWORD=""

usage() {
  cat <<'EOF'
Usage:
  install-frps.sh [--repo owner/repo] [--release-tag vX.Y.Z] [--raw-base URL]

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

prompt_password() {
  local label="$1"
  local default_value="${2:-}"
  local value
  while true; do
    if [[ -n "$default_value" ]]; then
      if ! value="$(read_interactive_line "${label} [press Enter to use default]: ")"; then
        return 1
      fi
      value="$(trim "$value")"
      if [[ -z "$value" ]]; then
        value="$default_value"
      fi
    else
      if ! value="$(read_interactive_line "${label}: ")"; then
        return 1
      fi
      value="$(trim "$value")"
    fi
    if [[ -n "$value" ]]; then
      printf '%s' "$value"
      return 0
    fi
    echo "Password cannot be empty." >&2
  done
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

toml_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
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

encode_b64() {
  local raw="$1"
  if command -v base64 >/dev/null 2>&1; then
    printf '%s' "$raw" | base64 | tr -d '\n'
    return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    python3 -c 'import base64,sys;print(base64.b64encode(sys.stdin.buffer.read()).decode(), end="")' <<<"$raw"
    return 0
  fi
  return 1
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

generate_config() {
  local mix_token dashboard_password_esc connect_password_esc dashboard_addr_esc
  connect_password_esc="$(toml_escape "$CONNECT_PASSWORD")"
  dashboard_password_esc="$(toml_escape "$DASHBOARD_PASSWORD")"
  dashboard_addr_esc="$(toml_escape "$DASHBOARD_ADDR")"
  mix_token="ss://chacha20-ietf-poly1305:${connect_password_esc},kcp://${connect_password_esc},ssh://frp:${connect_password_esc}"

  cat > "./${CFG_NAME}" <<EOF
bindPort = ${BIND_PORT}
mixBindPort = ${MIX_BIND_PORT}
mixToken = "${mix_token}"

webServer.addr = "${dashboard_addr_esc}"
webServer.port = ${DASHBOARD_PORT}
webServer.user = "${DASHBOARD_USER}"
webServer.password = "${dashboard_password_esc}"
EOF

  MIX_TOKEN_VALUE="${mix_token}"
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
  sleep 2

  if kill -0 "${pid}" >/dev/null 2>&1; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" 2>/dev/null || true
    echo "Smoke start check passed."
  else
    echo "Smoke start check failed. Recent log:"
    tail -n 60 "${run_log}" >&2
    rm -f "${verify_log}" "${run_log}"
    exit 1
  fi
  rm -f "${verify_log}" "${run_log}"
}

print_next_steps() {
  local token_b64 frpc_url
  frpc_url="${RAW_BASE}/install-frpc.sh"
  if ! token_b64="$(encode_b64 "${MIX_TOKEN_VALUE}")"; then
    echo "Warning: base64 encoder not found; cannot generate preset frpc one-liner." >&2
    token_b64=""
  fi
  if [[ -n "$token_b64" ]]; then
  cat <<EOF

Done. Generated files in current directory:
  - ./${BINARY_NAME}
  - ./${CFG_NAME}

One-click start command:
  ./${BINARY_NAME} -c ./${CFG_NAME}

One-click frpc deploy command (preloaded mix settings):
  wget -O- ${frpc_url} | bash -s -- --repo ${REPO} --mix-bind-port ${MIX_BIND_PORT} --mix-token-b64 '${token_b64}'

Curl alternative:
  curl -fsSL ${frpc_url} | bash -s -- --repo ${REPO} --mix-bind-port ${MIX_BIND_PORT} --mix-token-b64 '${token_b64}'

When frpc script runs with this command, it will ask only:
  1) serverAddr (your frps public IP/domain)
  2) clientID
EOF
  else
    cat <<EOF

Done. Generated files in current directory:
  - ./${BINARY_NAME}
  - ./${CFG_NAME}

One-click start command:
  ./${BINARY_NAME} -c ./${CFG_NAME}

Manual frpc installer:
  wget -O- ${frpc_url} | bash -
  # then input serverAddr / mixBindPort / password / clientID interactively
EOF
  fi
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
  pkg_dir="$(find "${tmp_extract}" -maxdepth 1 -type d -name "frp_*_${asset_suffix}" | head -n 1 || true)"
fi
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

echo
echo "Configure frps (press Enter to accept defaults)"
if ! BIND_PORT="$(prompt_port "bindPort" "${BIND_PORT}")"; then
  echo "Input aborted." >&2
  exit 1
fi
if ! MIX_BIND_PORT="$(prompt_port "mixBindPort" "${MIX_BIND_PORT}")"; then
  echo "Input aborted." >&2
  exit 1
fi
if ! CONNECT_PASSWORD="$(prompt_password "Connection password (used for ss/kcp/ssh)")"; then
  echo "Input aborted." >&2
  exit 1
fi
if ! DASHBOARD_ADDR="$(prompt_line "dashboard/webServer addr" "${DASHBOARD_ADDR}")"; then
  echo "Input aborted." >&2
  exit 1
fi

default_dashboard_port=$((MIX_BIND_PORT + 1))
if ((default_dashboard_port > 65535)); then
  default_dashboard_port=7501
fi
if ! DASHBOARD_PORT="$(prompt_port "dashboard/webServer port" "${default_dashboard_port}")"; then
  echo "Input aborted." >&2
  exit 1
fi
if ! DASHBOARD_PASSWORD="$(prompt_password "dashboard password" "${CONNECT_PASSWORD}")"; then
  echo "Input aborted." >&2
  exit 1
fi

generate_config
verify_and_smoke_run
print_next_steps
