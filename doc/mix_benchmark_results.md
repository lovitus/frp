# Mix Benchmark Notes

This file summarizes the automated mix-focused validation and benchmark entrypoints in this branch.

## Automated Entry Points

- Parser and config validation: `go test ./pkg/config/...`
- Client state machine and connector logic: `go test ./client/...`
- Server protocol identification and integration: `go test -run 'TestMix' ./server`
- Full benchmark harness: `./hack/run-mix-bench.sh`

The benchmark script writes raw output and generated summaries to:

- `tmp/mix-bench/results.txt`
- `tmp/mix-bench/bench-summary.md`
- `tmp/mix-bench/bench-summary.json`
- `tmp/mix-bench/mix-bench.log`

## Latest Local Run

Validated on this workspace on 2026-03-28 with:

```bash
./hack/run-mix-bench.sh
```

Script-level timing and memory snapshots from `/usr/bin/time -l`:

- `mix config tests`: `0.14s` wall time, `42 MB` max RSS
- `mix client tests`: `0.17s` wall time, `50 MB` max RSS
- `mix server tests`: `35.40s` wall time, `343 MB` max RSS
- `mix benchmark scenarios`: `101.75s` wall time, `464 MB` max RSS

## Benchmark Summary

Notes:

- Fallback / failback timing in the benchmark harness is intentionally scaled to `100ms / 200ms / 3` so the suite stays runnable in CI-like environments.
- Functional correctness on the default `10s / 60s / 3` policy is covered by unit and integration tests.
- `mixFallbackHosts` ordering, default-port expansion, wraparound, and higher-priority host failback are currently covered by unit tests rather than the benchmark harness.
- Endpoint fallback and failback across multiple frps addresses are covered by integration tests; the benchmark harness still focuses on control-path performance rather than multi-endpoint churn.
- `log bytes` is the final size of `tmp/mix-bench/mix-bench.log` for the whole run.

| scenario | protocol | concurrency | attempts | success rate | avg connect ms | p95 connect ms | fallback ms | failback ms | hold s | peak online | probe count | log bytes | notes |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| mix-failback | tcp | 100 | 100 | 100.00% | 4229.81 | 4230.93 | 0.00 | 207.73 | 0.00 | 0 | 0 | 32307430 | all clients first converged on ss=100 before restore |
| mix-failback-probe-overhead | ss | 100 | 100 | 100.00% | 3051.47 | 3039.50 | 0.00 | 0.00 | 3.00 | 0 | 1500 | 32307430 | approx probe attempts=1500 with scaled 200ms failback interval |
| mix-fallback | ss | 100 | 100 | 100.00% | 3066.86 | 3037.83 | 4048.45 | 0.00 | 0.00 | 0 | 0 | 32307430 | client token tcp://wrong,ss://..., fallback timing scaled to 100ms/200ms/3 |
| mix-intermittent-fallback | ss | 100 | 100 | 100.00% | 3082.47 | 3064.52 | 4019.84 | 0.00 | 0.00 | 0 | 0 | 32307430 | clients first stabilized on tcp with peak online=100 before forced reconnect |
| mix-primary-soak | tcp | 100 | 100 | 100.00% | 23.29 | 29.06 | 0.00 | 0.00 | 15.00 | 100 | 0 | 32307430 | peak online=100, final online after soak=100 |
| mix-primary-stable | tcp | 100 | 100 | 100.00% | 22.70 | 28.37 | 0.00 | 0.00 | 1.00 | 100 | 0 | 32307430 | peak online=100 |
| mix-primary-stable | tcp | 500 | 500 | 100.00% | 362.36 | 1056.75 | 0.00 | 0.00 | 1.00 | 500 | 0 | 32307430 | peak online=500 |
| mix-primary-stable | tcp | 1000 | 1000 | 100.00% | 703.36 | 2073.83 | 0.00 | 0.00 | 1.00 | 1000 | 0 | 32307430 | peak online=1000 |
| mix-third-to-first-failback | tcp | 100 | 100 | 100.00% | 6263.95 | 6277.78 | 0.00 | 206.65 | 0.00 | 0 | 0 | 32307430 | all clients first converged on ss=100 before restoring tcp |
| single-protocol-long | kcp | 100 | 100 | 100.00% | 78.29 | 130.58 | 0.00 | 0.00 | 2.00 | 100 | 0 | 32307430 | steady online=100, final online=100 |
| single-protocol-long | quic | 100 | 100 | 100.00% | 27.14 | 39.63 | 0.00 | 0.00 | 2.00 | 100 | 0 | 32307430 | steady online=100, final online=100 |
| single-protocol-long | ss | 100 | 100 | 100.00% | 22.67 | 28.59 | 0.00 | 0.00 | 2.00 | 100 | 0 | 32307430 | steady online=100, final online=100 |
| single-protocol-long | ssh | 100 | 100 | 100.00% | 27.43 | 33.51 | 0.00 | 0.00 | 2.00 | 100 | 0 | 32307430 | steady online=100, final online=100 |
| single-protocol-long | tcp | 100 | 100 | 100.00% | 22.67 | 29.47 | 0.00 | 0.00 | 2.00 | 100 | 0 | 32307430 | steady online=100, final online=100 |
| single-protocol-long | wss | 100 | 100 | 100.00% | 27.46 | 28.63 | 0.00 | 0.00 | 2.00 | 100 | 0 | 32307430 | steady online=100, final online=100 |
| single-protocol-short-burst | kcp | 100 | 300 | 100.00% | 211.08 | 469.12 | 0.00 | 0.00 | 0.00 | 0 | 0 | 32307430 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | quic | 100 | 300 | 100.00% | 29.56 | 51.12 | 0.00 | 0.00 | 0.00 | 0 | 0 | 32307430 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | ss | 100 | 300 | 100.00% | 21.76 | 37.10 | 0.00 | 0.00 | 0.00 | 0 | 0 | 32307430 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | ssh | 100 | 300 | 100.00% | 36.01 | 54.94 | 0.00 | 0.00 | 0.00 | 0 | 0 | 32307430 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | tcp | 100 | 300 | 100.00% | 24.13 | 28.64 | 0.00 | 0.00 | 0.00 | 0 | 0 | 32307430 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | wss | 100 | 300 | 100.00% | 29.19 | 52.09 | 0.00 | 0.00 | 0.00 | 0 | 0 | 32307430 | 3 cycles of 100 short-lived control connections |

## Interpretation

- All single-protocol and mix scenarios in this harness converged with `100%` control-plane login success.
- `tcp` remains the fastest stable control transport in this local run; `kcp` still shows the highest latency among the currently supported protocols.
- Fallback and failback both worked under the scaled timing policy, including the intermittent-failure and third-to-first failback scenarios.
- The soak case held `100` mixed clients on the first protocol for `15s` without triggering unnecessary probes or losing steady-state online count.
- The failback probe-overhead scenario generated roughly `1500` higher-priority probe attempts over a `3s` hold window with the scaled `200ms` probe interval.

## Remaining Limits

- This harness exercises the control connection path. It is not yet a full proxy data-plane load generator for sustained tunneled traffic.
- Concurrency `500 / 1000` is currently covered for the stable mixed-primary case, not for every single protocol variant.
- The soak scenario is automated and repeatable, but it is still a short soak intended for regression coverage rather than a multi-minute endurance run.
- CPU and memory numbers are process-level measurements from `/usr/bin/time -l`, not per-protocol profiled attribution.
