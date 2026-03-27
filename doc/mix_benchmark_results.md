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

- `mix config tests`: `0.18s` wall time, `39 MB` max RSS
- `mix client tests`: `0.16s` wall time, `48 MB` max RSS
- `mix server tests`: `31.94s` wall time, `340 MB` max RSS
- `mix benchmark scenarios`: `104.47s` wall time, `469 MB` max RSS

## Benchmark Summary

Notes:

- Fallback / failback timing in the benchmark harness is intentionally scaled to `100ms / 200ms / 3` so the suite stays runnable in CI-like environments.
- Functional correctness on the default `10s / 60s / 3` policy is covered by unit and integration tests.
- `mixFallbackHosts` ordering, default-port expansion, wraparound, and higher-priority host failback are currently covered by unit tests rather than the benchmark harness.
- `log bytes` is the final size of `tmp/mix-bench/mix-bench.log` for the whole run.

| scenario | protocol | concurrency | attempts | success rate | avg connect ms | p95 connect ms | fallback ms | failback ms | hold s | peak online | probe count | log bytes | notes |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| mix-failback | tcp | 100 | 100 | 100.00% | 3237.37 | 3238.34 | 0.00 | 207.46 | 0.00 | 0 | 0 | 7774349 | all clients first converged on ss=100 before restore |
| mix-failback-probe-overhead | ss | 100 | 100 | 100.00% | 3037.47 | 3033.05 | 0.00 | 0.00 | 3.00 | 0 | 1500 | 7774349 | approx probe attempts=1500 with scaled 200ms failback interval |
| mix-fallback | ss | 100 | 100 | 100.00% | 3042.51 | 3035.17 | 4020.61 | 0.00 | 0.00 | 0 | 0 | 7774349 | client token tcp://wrong,ss://..., fallback timing scaled to 100ms/200ms/3 |
| mix-intermittent-fallback | ss | 100 | 100 | 100.00% | 3066.09 | 3068.06 | 3039.25 | 0.00 | 0.00 | 0 | 0 | 7774349 | clients first stabilized on tcp with peak online=100 before forced reconnect |
| mix-primary-soak | tcp | 100 | 100 | 100.00% | 22.77 | 29.28 | 0.00 | 0.00 | 15.00 | 100 | 0 | 7774349 | peak online=100, final online after soak=100 |
| mix-primary-stable | tcp | 100 | 100 | 100.00% | 19.65 | 28.92 | 0.00 | 0.00 | 1.00 | 100 | 0 | 7774349 | peak online=100 |
| mix-primary-stable | tcp | 500 | 500 | 100.00% | 338.14 | 1053.74 | 0.00 | 0.00 | 1.00 | 500 | 0 | 7774349 | peak online=500 |
| mix-primary-stable | tcp | 1000 | 1000 | 100.00% | 507.69 | 1102.30 | 0.00 | 0.00 | 1.00 | 1000 | 0 | 7774349 | peak online=1000 |
| mix-third-to-first-failback | tcp | 100 | 100 | 100.00% | 7242.00 | 7303.25 | 0.00 | 206.71 | 0.00 | 0 | 0 | 7774349 | all clients first converged on ss=100 before restoring tcp |
| single-protocol-long | kcp | 100 | 100 | 100.00% | 78.95 | 130.97 | 0.00 | 0.00 | 2.00 | 100 | 0 | 7774349 | steady online=100, final online=100 |
| single-protocol-long | quic | 100 | 100 | 100.00% | 23.88 | 48.86 | 0.00 | 0.00 | 2.00 | 100 | 0 | 7774349 | steady online=100, final online=100 |
| single-protocol-long | ss | 100 | 100 | 100.00% | 30.54 | 29.34 | 0.00 | 0.00 | 2.00 | 100 | 0 | 7774349 | steady online=100, final online=100 |
| single-protocol-long | ssh | 100 | 100 | 100.00% | 26.73 | 28.34 | 0.00 | 0.00 | 2.00 | 100 | 0 | 7774349 | steady online=100, final online=100 |
| single-protocol-long | tcp | 100 | 100 | 100.00% | 20.04 | 29.39 | 0.00 | 0.00 | 2.00 | 100 | 0 | 7774349 | steady online=100, final online=100 |
| single-protocol-long | wss | 100 | 100 | 100.00% | 27.55 | 29.29 | 0.00 | 0.00 | 2.00 | 100 | 0 | 7774349 | steady online=100, final online=100 |
| single-protocol-short-burst | kcp | 100 | 300 | 100.00% | 142.90 | 250.83 | 0.00 | 0.00 | 0.00 | 0 | 0 | 7774349 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | quic | 100 | 300 | 100.00% | 32.82 | 46.09 | 0.00 | 0.00 | 0.00 | 0 | 0 | 7774349 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | ss | 100 | 300 | 100.00% | 40.25 | 33.25 | 0.00 | 0.00 | 0.00 | 0 | 0 | 7774349 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | ssh | 100 | 300 | 100.00% | 32.03 | 55.39 | 0.00 | 0.00 | 0.00 | 0 | 0 | 7774349 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | tcp | 100 | 300 | 100.00% | 22.67 | 30.28 | 0.00 | 0.00 | 0.00 | 0 | 0 | 7774349 | 3 cycles of 100 short-lived control connections |
| single-protocol-short-burst | wss | 100 | 300 | 100.00% | 30.70 | 52.79 | 0.00 | 0.00 | 0.00 | 0 | 0 | 7774349 | 3 cycles of 100 short-lived control connections |

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
