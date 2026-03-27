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

- `mix config tests`: `0.25s` wall time, `37 MB` max RSS
- `mix client tests`: `0.17s` wall time, `50 MB` max RSS
- `mix server tests`: `33.18s` wall time, `341 MB` max RSS
- `mix benchmark scenarios`: `101.54s` wall time, `451 MB` max RSS

## Benchmark Summary

Notes:

- Fallback / failback timing in the benchmark harness is intentionally scaled to `100ms / 200ms / 3` so the suite stays runnable in CI-like environments.
- Functional correctness on the default `10s / 60s / 3` policy is covered by unit and integration tests.
- `log bytes` is the final size of `tmp/mix-bench/mix-bench.log` for the whole run.

| scenario | protocol | concurrency | success rate | avg connect ms | p95 connect ms | fallback ms | failback ms | hold s | probe count | log bytes |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| mix-primary-stable | tcp | 100 | 100.00% | 22.97 | 29.07 | 0.00 | 0.00 | 1.00 | 0 | 24011031 |
| mix-primary-stable | tcp | 500 | 100.00% | 369.72 | 1052.95 | 0.00 | 0.00 | 1.00 | 0 | 24011031 |
| mix-primary-stable | tcp | 1000 | 100.00% | 534.10 | 2062.22 | 0.00 | 0.00 | 1.00 | 0 | 24011031 |
| mix-primary-soak | tcp | 100 | 100.00% | 21.16 | 29.24 | 0.00 | 0.00 | 15.00 | 0 | 24011031 |
| mix-fallback | ss | 100 | 100.00% | 3034.42 | 3014.70 | 4032.21 | 0.00 | 0.00 | 0 | 24011031 |
| mix-intermittent-fallback | ss | 100 | 100.00% | 3056.99 | 3067.98 | 3039.25 | 0.00 | 0.00 | 0 | 24011031 |
| mix-failback | tcp | 100 | 100.00% | 4233.69 | 4239.94 | 0.00 | 207.71 | 0.00 | 0 | 24011031 |
| mix-third-to-first-failback | tcp | 100 | 100.00% | 6255.33 | 6263.07 | 0.00 | 205.78 | 0.00 | 0 | 24011031 |
| mix-failback-probe-overhead | ss | 100 | 100.00% | 3016.66 | 3039.85 | 0.00 | 0.00 | 3.00 | 1500 | 24011031 |
| single-protocol-long | tcp | 100 | 100.00% | 24.90 | 28.36 | 0.00 | 0.00 | 2.00 | 0 | 24011031 |
| single-protocol-long | kcp | 100 | 100.00% | 72.83 | 128.85 | 0.00 | 0.00 | 2.00 | 0 | 24011031 |
| single-protocol-long | quic | 100 | 100.00% | 35.27 | 45.90 | 0.00 | 0.00 | 2.00 | 0 | 24011031 |
| single-protocol-long | ss | 100 | 100.00% | 20.13 | 28.99 | 0.00 | 0.00 | 2.00 | 0 | 24011031 |
| single-protocol-long | wss | 100 | 100.00% | 27.85 | 29.75 | 0.00 | 0.00 | 2.00 | 0 | 24011031 |
| single-protocol-long | ssh | 100 | 100.00% | 27.88 | 29.56 | 0.00 | 0.00 | 2.00 | 0 | 24011031 |
| single-protocol-short-burst | tcp | 100 | 100.00% | 21.42 | 31.58 | 0.00 | 0.00 | 0.00 | 0 | 24011031 |
| single-protocol-short-burst | kcp | 100 | 100.00% | 93.96 | 143.96 | 0.00 | 0.00 | 0.00 | 0 | 24011031 |
| single-protocol-short-burst | quic | 100 | 100.00% | 31.74 | 48.45 | 0.00 | 0.00 | 0.00 | 0 | 24011031 |
| single-protocol-short-burst | ss | 100 | 100.00% | 34.32 | 30.09 | 0.00 | 0.00 | 0.00 | 0 | 24011031 |
| single-protocol-short-burst | wss | 100 | 100.00% | 29.20 | 51.47 | 0.00 | 0.00 | 0.00 | 0 | 24011031 |
| single-protocol-short-burst | ssh | 100 | 100.00% | 35.80 | 53.73 | 0.00 | 0.00 | 0.00 | 0 | 24011031 |

## Interpretation

- All single-protocol and mix scenarios in this harness converged with `100%` control-plane login success.
- `tcp` remains the fastest stable control transport in this local run; `kcp` shows the highest latency among the currently supported protocols.
- Fallback and failback both worked under the scaled timing policy, including the intermittent-failure and third-to-first failback scenarios.
- The soak case held `100` mixed clients on the first protocol for `15s` without triggering unnecessary probes or losing steady-state online count.
- The failback probe-overhead scenario generated roughly `1500` higher-priority probe attempts over a `3s` hold window with the scaled `200ms` probe interval.

## Remaining Limits

- This harness exercises the control connection path. It is not yet a full proxy data-plane load generator for sustained tunneled traffic.
- Concurrency `500 / 1000` is currently covered for the stable mixed-primary case, not for every single protocol variant.
- The soak scenario is automated and repeatable, but it is still a short soak intended for regression coverage rather than a multi-minute endurance run.
- CPU and memory numbers are process-level measurements from `/usr/bin/time -l`, not per-protocol profiled attribution.
