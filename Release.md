## Overview

This release introduces the new `mix` transport selector for frpc/frps, adds runtime gateway tunnel management on the frps dashboard, and ships the supporting test, benchmark, and release automation needed to operate it as a maintained fork.

`mix` keeps the existing single-control-connection architecture, but adds ordered client-side transport selection and shared-port server-side protocol demultiplexing so the same frpc/frps configuration can negotiate between multiple transports.

## Highlights

* Added `mixBindPort` and `mixToken` on both frpc and frps so one shared configuration can enable `kcp`, `quic`, `ss`, `wss`, `ssh`, and `tcp`.
* Added a connector-based client state machine that performs ordered dial attempts, waits for 3 consecutive failures before fallback, and probes higher-priority transports for failback.
* Added server-side mix listeners that share one numeric TCP/UDP port and route accepted traffic to the correct sub-protocol implementation.
* Added `selectedProtocol` reporting to client status, server client registry, dashboard APIs, logs, and Prometheus server metrics.
* Added example configs, transport documentation, integration tests, and repeatable benchmark scripts for mix.
* Fixed mix startup retry behavior so initial authentication failures no longer exit the client, and an all-failing candidate list keeps cycling instead of stalling on the last protocol.
* Added ordered client-side host fallback via `mixFallbackHosts`, expanding the retry / failback priority space from `protocols[]` to `endpoint × protocol` candidates.
* Hardened mix release validation by fixing UDP demux shutdown races and making the TCP/UDP shared-port test port reservation deterministic in automated runs.
* Improved Android / Termux DNS compatibility by discovering resolver addresses from resolv.conf-style files and falling back to public DNS when the environment has no usable local `:53` resolver.
* Removed the probe-domain DNS workaround and switched restricted DNS fallback to the same runtime-detected strategy used in `flyssh`, so Termux-like environments now install the custom resolver directly across both Android and non-Android builds.
* Added runtime gateway tunnel management on the frps dashboard so operators can create TCP or UDP listeners that forward through an opted-in frpc client to a fixed target host and port.
* Added `allowGatewayTunnels` as the canonical frpc opt-in field, while preserving `mixAllowGateway` and `mix_allow_gateway` as compatibility aliases.
* Added runtime gateway synchronization over the existing control channel, plus status refresh and bind-address support scoped only to gateway-managed listeners.
* Added a simple dashboard authentication lockout: 10 failed logins within 1 minute pause authentication for 10 seconds, return `429` on API paths, and show a browser countdown page before automatic unlock.
* Fixed legacy `[common]` INI compatibility for `mix` and gateway-related client/server fields, and restored camelCase aliases such as `bindPort`, `serverAddr`, `mixBindPort`, and `mixToken` on that legacy load path.
* Improved the Gateway dashboard page so eligible clients are shown with richer runtime details including hostname, client IP, version, selected transport, online state, and internal key.

## Protocol and Runtime Notes

* `mix` is a high-level transport selector. Existing single-protocol flows for `tcp`, `kcp`, `quic`, `websocket`, and `wss` remain intact.
* Fallback and failback allow a short interruption by design. They reuse frp's existing "close old control, create new control" behavior instead of introducing dual-control migration.
* Raw `tcp` mix auth uses an explicit magic prefix. When that prefix is present, raw `tcp` is matched before Shadowsocks decryption so `tcp` and `ss` can safely coexist on the same mix port.
* Gateway tunnels reuse whichever control transport is currently active, including `mix`, and do not add a separate SOCKS layer or extra connection model.
* Gateway tunnels are runtime-only in this phase. They live in frps memory and are lost on frps restart.
* `ss-udp` is still out of scope for this phase.
* Dashboard lockout slows brute-force attempts, but it does not make plaintext HTTP safe. Operators should still use HTTPS or an outer secure tunnel when the dashboard leaves localhost.

## Validation and Benchmarking

* Added automated parser, client state-machine, server protocol-routing, and end-to-end mix tests.
* Added automated tests for gateway alias folding, runtime source precedence, gateway sync/status handling, and gateway bind-address annotation resolution.
* Added automated tests for dashboard auth success, failure delay, temporary lockout, API `429` responses, and automatic unlock behavior.
* Added automated benchmark coverage for single-protocol stability, short-burst churn, stable mix-primary operation, intermittent fallback, failback, probe overhead, and a repeatable soak scenario.
* The latest local benchmark summary is documented in `doc/mix_benchmark_results.md`.

## Release Automation

* GitHub Actions now validate the codebase, run the mix benchmark harness for release tags, cross-build all packaged binaries on GitHub runners, and publish GitHub Releases directly from Actions.
* Release assets are uploaded from GitHub-hosted runners, not through a local workstation.

## Maintainer Notes

* Use `doc/mix.md` for feature behavior and configuration guidance.
* Use `doc/gateway_tunnels.md` for runtime gateway tunnel behavior, dashboard semantics, and limits.
* Use `doc/upstream-sync.md` for keeping this fork in sync with upstream `fatedier/frp`.
* Use `doc/agents/release.md` for the maintainer release flow and tag-to-release pipeline.
