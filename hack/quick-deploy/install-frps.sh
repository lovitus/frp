#!/bin/sh
set -eu

REPO="${FRP_REPO:-lovitus/frp}"
RELEASE_TAG="${FRP_RELEASE_TAG:-}"
RAW_BASE="${FRP_RAW_BASE:-}"

BINARY_NAME="frps"
CFG_NAME="frps.toml"
OS_RELEASE_FILE="${FRP_OS_RELEASE_FILE:-/etc/os-release}"

BIND_PORT="7000"
MIX_BIND_PORT="7001"
CONNECT_PASSWORD=""
DASHBOARD_ADDR="0.0.0.0"
DASHBOARD_PORT=""
DASHBOARD_USER="admin"
DASHBOARD_PASSWORD=""

INPUT_SRC=0
if (true < /dev/tty) 2>/dev/null; then
  INPUT_SRC="/dev/tty"
fi

usage() {
  cat <<'EOF'
Usage:
  install-frps.sh [--repo owner/repo] [--release-tag vX.Y.Z] [--raw-base URL]

Environment overrides:
  FRP_REPO, FRP_RELEASE_TAG, FRP_RAW_BASE
EOF
}

http_get() {
  local url
  url="$1"
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
  local url out
  url="$1"
  out="$2"
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
  local s
  s="$(printf '%s' "$1" | tr -d '\r\n')"
  s="${s#"${s%%[![:space:]]*}"}"
  s="${s%"${s##*[![:space:]]}"}"
  printf '%s' "$s"
}

read_interactive_line() {
  local prompt value
  prompt="$1"
  printf "%s" "$prompt" >&2
  if [ "$INPUT_SRC" = "/dev/tty" ]; then
    read -r value < /dev/tty || return 1
  else
    read -r value || return 1
  fi
  printf '%s' "$value"
}

prompt_line() {
  local label default_value value
  label="$1"
  default_value="${2:-}"
  if [ -n "$default_value" ]; then
    if ! value="$(read_interactive_line "${label} [${default_value}]: ")"; then
      return 1
    fi
    value="$(trim "$value")"
    if [ -z "$value" ]; then
      value="$default_value"
    fi
  else
    while true; do
      if ! value="$(read_interactive_line "${label}: ")"; then
        return 1
      fi
      value="$(trim "$value")"
      if [ -n "$value" ]; then
        break
      fi
      echo "This value is required." >&2
    done
  fi
  printf '%s' "$value"
}

prompt_password() {
  local label default_value value
  label="$1"
  default_value="${2:-}"
  while true; do
    if [ -n "$default_value" ]; then
      if ! value="$(read_interactive_line "${label} [press Enter to use default]: ")"; then
        return 1
      fi
      value="$(trim "$value")"
      if [ -z "$value" ]; then
        value="$default_value"
      fi
    else
      if ! value="$(read_interactive_line "${label}: ")"; then
        return 1
      fi
      value="$(trim "$value")"
    fi
    if [ -n "$value" ]; then
      printf '%s' "$value"
      return 0
    fi
    echo "Password cannot be empty." >&2
  done
}

prompt_port() {
  local label default_value value
  label="$1"
  default_value="$2"
  while true; do
    if ! value="$(prompt_line "$label" "$default_value")"; then
      return 1
    fi
    case "$value" in
      *[!0-9]*)
        ;;
      ?*)
        if [ "$value" -ge 1 ] && [ "$value" -le 65535 ]; then
          printf '%s' "$value"
          return 0
        fi
        ;;
    esac
    echo "Invalid port '${value}'. Expected 1..65535." >&2
  done
}

toml_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

read_os_release_var() {
  local key
  key="$1"
  [ -r "$OS_RELEASE_FILE" ] || return 1
  awk -F= -v wanted="$key" '
    $1 == wanted {
      value = substr($0, index($0, "=") + 1)
      sub(/\r$/, "", value)
      if (value ~ /^".*"$/) {
        sub(/^"/, "", value)
        sub(/"$/, "", value)
      }
      print value
      exit
    }
  ' "$OS_RELEASE_FILE"
}

detect_linux_arch_hint() {
  local arch_hint
  arch_hint="$(read_os_release_var OPENWRT_ARCH 2>/dev/null || true)"
  if [ -z "$arch_hint" ] && command -v opkg >/dev/null 2>&1; then
    arch_hint="$(opkg print-architecture 2>/dev/null | awk '$1 == "arch" && $2 != "all" { print $2; exit }' || true)"
  fi
  printf '%s' "$arch_hint"
}

normalize_arch_name() {
  local raw normalized
  raw="$(trim "$1")"
  normalized="$(printf '%s' "$raw" | tr '[:upper:]' '[:lower:]')"

  case "$normalized" in
    x86_64|amd64) printf '%s' "amd64" ;;
    i386|i486|i586|i686) printf '%s' "386" ;;
    aarch64|aarch64_*|arm64|arm64_*) printf '%s' "arm64" ;;
    armv7l|armv7|armhf|armv7_*|arm_cortex-a5*|arm_cortex-a7*|arm_cortex-a8*|arm_cortex-a9*|arm_cortex-a12*|arm_cortex-a15*|arm_cortex-a17*|arm_cortex-a53*) printf '%s' "arm_hf" ;;
    armv6l|armv6|armv6_*|arm) printf '%s' "arm" ;;
    mips64el|mips64le|mips64el_*|mips64le_*) printf '%s' "mips64le" ;;
    mips64|mips64_*) printf '%s' "mips64" ;;
    mipsel|mipsle|mipsel_*|mipsle_*) printf '%s' "mipsle" ;;
    mips|mips_*) printf '%s' "mips" ;;
    riscv64|riscv64_*) printf '%s' "riscv64" ;;
    loongarch64|loongarch64_*|loong64) printf '%s' "loong64" ;;
    *) return 1 ;;
  esac
}

detect_platform() {
  local os_name arch_name arch_hint normalized_arch
  os_name="$(uname -s)"
  arch_name="$(uname -m)"

  case "$os_name" in
    Linux)
      if [ -n "${ANDROID_ROOT:-}" ] || (uname -o 2>/dev/null | grep -i 'android' >/dev/null); then
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

  normalized_arch="$(normalize_arch_name "$arch_name" || true)"

  if [ "$DETECTED_OS" = "linux" ]; then
    arch_hint="$(detect_linux_arch_hint)"
    if [ -n "$arch_hint" ]; then
      case "$arch_name" in
        arm|mips|mips64)
          normalized_arch="$(normalize_arch_name "$arch_hint" || true)"
          ;;
        *)
          if [ -z "$normalized_arch" ]; then
            normalized_arch="$(normalize_arch_name "$arch_hint" || true)"
          fi
          ;;
      esac
    fi
  fi

  if [ -z "$normalized_arch" ]; then
    echo "Unsupported architecture: ${arch_name}" >&2
    exit 1
  fi
  DETECTED_ARCH="$normalized_arch"

  if [ "$DETECTED_OS" = "android" ] && [ "$DETECTED_ARCH" != "arm64" ]; then
    echo "Unsupported Android architecture: ${arch_name}. Current releases provide android_arm64." >&2
    exit 1
  fi
}

resolve_release_json() {
  if [ -n "$RELEASE_TAG" ]; then
    RELEASE_JSON="$(http_get "https://api.github.com/repos/${REPO}/releases/tags/${RELEASE_TAG}")"
  else
    RELEASE_JSON="$(http_get "https://api.github.com/repos/${REPO}/releases/latest")"
  fi
  RELEASE_TAG_RESOLVED="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [ -z "$RELEASE_TAG_RESOLVED" ]; then
    echo "Unable to resolve release tag from GitHub API." >&2
    exit 1
  fi
  RELEASE_VERSION="${RELEASE_TAG_RESOLVED#v}"
}

encode_b64() {
  local raw
  raw="$1"
  if command -v base64 >/dev/null 2>&1; then
    printf '%s' "$raw" | base64 | tr -d '\r\n'
    return 0
  fi
  if command -v openssl >/dev/null 2>&1; then
    printf '%s' "$raw" | openssl enc -a -A 2>/dev/null | tr -d '\r\n' && return 0
  fi
  if command -v python3 >/dev/null 2>&1; then
    printf '%s' "$raw" | python3 -c 'import base64,sys;print(base64.b64encode(sys.stdin.buffer.read()).decode(), end="")' 2>/dev/null
    return 0
  fi
  return 1
}

find_asset_url() {
  local suffix archive asset_url
  suffix="$1"
  archive="frp_${RELEASE_VERSION}_${suffix}.tar.gz"
  asset_url="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/^[[:space:]]*"browser_download_url":[[:space:]]*"\([^"]*\)".*/\1/p' | grep "/${archive}$" | head -n 1 || true)"
  if [ -z "$asset_url" ]; then
    asset_url="$(printf '%s\n' "$RELEASE_JSON" | sed -n 's/^[[:space:]]*"browser_download_url":[[:space:]]*"\([^"]*\)".*/\1/p' | grep -E "/frp_[^/]+_${suffix}\.tar\.gz$" | head -n 1 || true)"
    if [ -n "$asset_url" ]; then
      echo "Warning: no exact asset for tag ${RELEASE_TAG_RESOLVED}, using ${asset_url##*/}" >&2
    fi
  fi
  if [ -z "$asset_url" ]; then
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
  local token_b64 frpc_url frpc_ps_url
  frpc_url="${RAW_BASE}/install-frpc.sh"
  frpc_ps_url="${RAW_BASE}/install-frpc.ps1"
  if ! token_b64="$(encode_b64 "${MIX_TOKEN_VALUE}")"; then
    echo "Warning: base64 encoder not found; cannot generate preset frpc one-liner." >&2
    token_b64=""
  fi
  if [ -n "$token_b64" ]; then
  cat <<EOF

Done. Generated files in current directory:
  - ./${BINARY_NAME}
  - ./${CFG_NAME}

One-click start command:
  ./${BINARY_NAME} -c ./${CFG_NAME}

Unix/macOS/Linux frpc quick-deploy command (preloaded mix settings):
  wget -O- ${frpc_url} | sh -s -- --repo ${REPO} --release-tag ${RELEASE_TAG_RESOLVED} --mix-bind-port ${MIX_BIND_PORT} --mix-token-b64 '${token_b64}'

Curl alternative:
  curl -fsSL ${frpc_url} | sh -s -- --repo ${REPO} --release-tag ${RELEASE_TAG_RESOLVED} --mix-bind-port ${MIX_BIND_PORT} --mix-token-b64 '${token_b64}'

Windows PowerShell frpc quick-deploy command (preloaded mix settings):
  \$env:FRP_REPO='${REPO}'; \$env:FRP_RELEASE_TAG='${RELEASE_TAG_RESOLVED}'; \$env:FRP_MIX_BIND_PORT='${MIX_BIND_PORT}'; \$env:FRP_MIX_TOKEN_B64='${token_b64}'; powershell -ExecutionPolicy Bypass -Command "iwr -UseBasicParsing ${frpc_ps_url} | iex"

When either quick-deploy command runs, it will ask only:
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

Unix/macOS/Linux frpc installer:
  wget -O- ${frpc_url} | sh -
  # then input serverAddr / mixBindPort / password / clientID interactively

Windows PowerShell frpc installer:
  powershell -ExecutionPolicy Bypass -Command "iwr -UseBasicParsing ${frpc_ps_url} | iex"
  # then input serverAddr / mixBindPort / password / clientID interactively
EOF
  fi
}

while [ "$#" -gt 0 ]; do
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

if [ -z "$RAW_BASE" ]; then
  RAW_BASE="https://raw.githubusercontent.com/${REPO}/codex/mix-transport-release/hack/quick-deploy"
fi

echo "Repo: ${REPO}"
detect_platform
echo "Detected platform: ${DETECTED_OS}_${DETECTED_ARCH}"
resolve_release_json
echo "Using release: ${RELEASE_TAG_RESOLVED}"

CANDIDATES="${DETECTED_ARCH}"
if [ "${DETECTED_OS}" = "linux" ]; then
  case "${DETECTED_ARCH}" in
    arm64) CANDIDATES="arm64 arm_hf arm" ;;
    arm_hf) CANDIDATES="arm_hf arm" ;;
    mips64le) CANDIDATES="mips64le mipsle" ;;
    mips64) CANDIDATES="mips64 mips" ;;
  esac
fi

success=0
for cand in $CANDIDATES; do
  asset_suffix="${DETECTED_OS}_${cand}"
  echo "Trying candidate suffix: ${asset_suffix}"

  asset_url=""
  if ! asset_url="$(find_asset_url "${asset_suffix}" 2>/dev/null)"; then
    echo "No matching asset for ${asset_suffix} in release." >&2
    continue
  fi

  echo "Downloading asset: ${asset_url}"
  tmp_archive="$(mktemp)"
  tmp_extract="$(mktemp -d)"

  if ! download_file "${asset_url}" "${tmp_archive}"; then
    echo "Download failed for ${asset_suffix}." >&2
    rm -f "${tmp_archive}"
    rm -rf "${tmp_extract}"
    continue
  fi

  if ! (LC_ALL=C tar -xzf "${tmp_archive}" -C "${tmp_extract}" 2>/dev/null); then
    echo "Extraction failed for ${asset_suffix}." >&2
    rm -f "${tmp_archive}"
    rm -rf "${tmp_extract}"
    continue
  fi

  pkg_dir=""
  for d in "${tmp_extract}"/frp_"${RELEASE_VERSION}"_"${asset_suffix}" \
           "${tmp_extract}"/frp_*_"${asset_suffix}"; do
    if [ -d "$d" ]; then
      pkg_dir="$d"
      break
    fi
  done

  if [ -z "$pkg_dir" ] || [ ! -f "${pkg_dir}/${BINARY_NAME}" ]; then
    echo "Binary not found in extracted package for ${asset_suffix}." >&2
    rm -f "${tmp_archive}"
    rm -rf "${tmp_extract}"
    continue
  fi

  cp "${pkg_dir}/${BINARY_NAME}" "./${BINARY_NAME}"
  chmod +x "./${BINARY_NAME}"
  rm -f "${tmp_archive}"
  rm -rf "${tmp_extract}"

  version_log="$(mktemp)"
  if "./${BINARY_NAME}" --version >"${version_log}" 2>&1; then
    echo "Successfully verified execution of downloaded binary (${asset_suffix})."
    rm -f "${version_log}"
    success=1
    break
  else
    echo "Downloaded binary for ${asset_suffix} failed to execute on this host." >&2
    cat "${version_log}" >&2
    rm -f "${version_log}"
    rm -f "./${BINARY_NAME}"
    echo "Trying next fallback candidate..."
  fi
done

if [ "$success" -ne 1 ]; then
  echo "Error: Failed to download a working ${BINARY_NAME} binary for this system (tried candidates: ${CANDIDATES})." >&2
  exit 1
fi

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
if [ "$default_dashboard_port" -gt 65535 ]; then
  default_dashboard_port=7501
fi
if ! DASHBOARD_PORT="$(prompt_port "dashboard/webServer port" "$default_dashboard_port")"; then
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
