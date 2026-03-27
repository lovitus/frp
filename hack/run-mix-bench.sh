#!/bin/sh

set -eu

SCRIPT=$(readlink -f "$0")
ROOT=$(unset CDPATH && cd "$(dirname "$SCRIPT")/.." && pwd)
OUT_DIR="${ROOT}/tmp/mix-bench"
OUT_FILE="${OUT_DIR}/results.txt"

mkdir -p "${OUT_DIR}"
: > "${OUT_FILE}"

run_case() {
    name="$1"
    shift
    echo "== ${name} ==" | tee -a "${OUT_FILE}"
    /usr/bin/time -l "$@" 2>&1 | tee -a "${OUT_FILE}"
    echo "" | tee -a "${OUT_FILE}"
}

run_case "mix config tests" \
    go test ./pkg/config/...

run_case "mix client tests" \
    go test ./client/...

run_case "mix server tests" \
    go test -run 'TestMix' ./server

run_case "mix benchmark scenarios" \
    env FRP_RUN_MIX_BENCH=1 FRP_MIX_BENCH_DIR="${OUT_DIR}" go test -run 'TestMixBench' -count=1 -v ./server

if [ -f "${OUT_DIR}/bench-summary.md" ]; then
    echo "== mix benchmark summary ==" | tee -a "${OUT_FILE}"
    cat "${OUT_DIR}/bench-summary.md" | tee -a "${OUT_FILE}"
    echo "" | tee -a "${OUT_FILE}"
fi

echo "mix benchmark results written to ${OUT_FILE}"
