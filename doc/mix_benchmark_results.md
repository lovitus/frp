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

Validated on this workspace on 2026-03-27 with:

```bash
./hack/run-mix-bench.sh
```

Script-level timing and memory snapshots from `/usr/bin/time -l`:

- `mix config tests`: `0.19s` wall time, `42 MB` max RSS
- `mix client tests`: `0.37s` wall time, `275 MB` max RSS
- `mix server tests`: `35.37s` wall time, `336 MB` max RSS
- `mix benchmark scenarios`: `102.41s` wall time, `477 MB` max RSS

## Benchmark Summary

Notes:

- Fallback / failback timing in the benchmark harness is intentionally scaled to `100ms / 200ms / 3` so the suite stays runnable in CI-like environments.
- Functional correctness on the default `10s / 60s / 3` policy is covered by unit and integration tests.
- `mixFallbackHosts` ordering, default-port expansion, wraparound, and higher-priority host failback are currently covered by unit tests rather than the benchmark harness.
- Endpoint fallback and failback across multiple frps addresses are covered by integration tests; the benchmark harness still focuses on control-path performance rather than multi-endpoint churn.
- `log bytes` is the final size of `tmp/mix-bench/mix-bench.log` for the whole run.

| scenario | protocol | concurrency | attempts | success rate | avg connect ms | p95 connect ms | fallback ms | failback ms | hold s | peak online | probe count | log bytes | notes |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| mix-failback | tcp | 100 | 100 | 100.00% | 3235.80 | 3244.38 | 0.00 | 207.64 | 0.00 | 0 | 0 | 24450244 | all clients first converged on ss=100 before restore |
| mix-failback-probe-overhead | ss | 100 | 100 | 100.00% | 3059.80 | 3035.07 | 0.00 | 0.00 | 3.00 | 0 | 1500 | 24450244 | approx probe attempts=1500 with scaled 200ms failback interval |
| mix-fallback | ss | 100 | 100 | 100.00% | 3029.46 | 3037.19 | 3037.40 | 0.00 | 0.00 | 0 | 0 | 24450244 | client token tcp://wrong,ss://..., fallback timing scaled to 100ms/200ms/3 |
| mix-intermittent-fallback | ss | 100 | 100 | 100.00% | 3083.53 | 3065.54 | 4026.96 | 0.00 | 0.00 | 0 | 0 | 24450244 | clients first stabilized on tcp with peak online=100 before forced reconnect |
| mix-primary-soak | tcp | 100 | 100 | 100.00% | 20.95 | 29.26 | 0.00 | 0.00 | 15.00 | 100 | 0 | 24450244 | peak online=100, final online after soak=100 |
| mix-primary-stable | tcp | 100 | 100 | 100.00% | 23.18 | 27.71 | 0.00 | 0.00 | 1.00 | 100 | 0 | 24450244 | peak online=100 |
| mix-primary-stable | tcp | 500 | 500 | 100.00% | 423.69 | 1055.10 | 0.00 | 0.00 | 1.00 | 500 | 0 | 24450244 | peak online=500 |
| mix-primary-stable | tcp | 1000 | 1000 | 100.00% | 650.97 | 2089.12 | 0.00 | 0.00 | 1.00 | 1000 | 0 | 24450244 | peak online=1000 |
| mix-third-to-first-failback | tcp | 100 | 100 | 100.00% | 7269.75 | 7288.96 | 0.00 | 206.66 | 0.00 | 0 | 0 | 24450244 | all clients first converged on ss=100 before restoring tcp |
| single-protocol-long | kcp | 100 | 100 | 100.00% | 80.54 | 128.77 | 0.00 | 0.00 | 2.00 | 100 | 0 | 24450244 | steady online=100, final online=100 |
| single-protocol-long | quic | 100 | 100 | 100.00% | 29.81 | 31.83 | 0.00 | 0.00 | 2.00 | 100 | 0 | 24450244 | steady online=100, final online=100 |
| single-protocol-long | ss | 100 | 100 | 100.00% | 53.06 | 28.66 | 0.00 | 0.00 | 2.00 | 100 | 0 | 24450244 | steady online=100, final online=100 |
| single-protocol-long | ssh | 100 | 100 | 100.00% | 28.33 | 35.60 | 0.00 | 0.00 | 2.00 | 100 | 0 | 24450244 | steady online=100, final online=100 |
| single-protocol-long | tcp | 100 | 100 | 100.00% | 28.74 | 33.20 | 0.00 | 0.00 | 2.00 | 100 | 0 | 24450244 | steady online=100, final online=100 |
| single-protocol-long | wss | 100 | 100 | 100.00% | 32.96 | 54.83 | 0.00 | 0.00 | 2.00 | 100 | 0 | 24450244 | steady online=100, final online=100 |
| single-protocol-short-burst | kcp | 100 | 300 | 100.00% | 99.96 | 171.41 | 0.00 | 0.00 | 0.00 | 0 | 0 | 24450244 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | quic | 100 | 300 | 100.00% | 29.64 | 41.91 | 0.00 | 0.00 | 0.00 | 0 | 0 | 24450244 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | ss | 100 | 300 | 100.00% | 22.99 | 31.98 | 0.00 | 0.00 | 0.00 | 0 | 0 | 24450244 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | ssh | 100 | 300 | 100.00% | 35.08 | 56.98 | 0.00 | 0.00 | 0.00 | 0 | 0 | 24450244 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | tcp | 100 | 300 | 100.00% | 23.61 | 32.38 | 0.00 | 0.00 | 0.00 | 0 | 0 | 24450244 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | wss | 100 | 300 | 100.00% | 29.35 | 51.95 | 0.00 | 0.00 | 0.00 | 0 | 0 | 24450244 | 3 cycles of 100 short-lived control connections |

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
