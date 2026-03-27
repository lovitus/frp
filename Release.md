## Overview

This release introduces the new `mix` transport selector for frpc/frps and ships the supporting test, benchmark, and release automation needed to operate it as a maintained fork.

`mix` keeps the existing single-control-connection architecture, but adds ordered client-side transport selection and shared-port server-side protocol demultiplexing so the same frpc/frps configuration can negotiate between multiple transports.

## Highlights

* Added `mixBindPort` and `mixToken` on both frpc and frps so one shared configuration can enable `kcp`, `quic`, `ss`, `wss`, `ssh`, and `tcp`.
* Added a connector-based client state machine that performs ordered dial attempts, waits for 3 consecutive failures before fallback, and probes higher-priority transports for failback.
* Added server-side mix listeners that share one numeric TCP/UDP port and route accepted traffic to the correct sub-protocol implementation.
* Added `selectedProtocol` reporting to client status, server client registry, dashboard APIs, logs, and Prometheus server metrics.
* Added example configs, transport documentation, integration tests, and repeatable benchmark scripts for mix.
* Fixed mix startup retry behavior so initial authentication failures no longer exit the client, and an all-failing candidate list keeps cycling instead of stalling on the last protocol.

## Protocol and Runtime Notes

* `mix` is a high-level transport selector. Existing single-protocol flows for `tcp`, `kcp`, `quic`, `websocket`, and `wss` remain intact.
* Fallback and failback allow a short interruption by design. They reuse frp's existing "close old control, create new control" behavior instead of introducing dual-control migration.
* Raw `tcp` mix auth uses an explicit magic prefix. When that prefix is present, raw `tcp` is matched before Shadowsocks decryption so `tcp` and `ss` can safely coexist on the same mix port.
* `ss-udp` is still out of scope for this phase.

## Validation and Benchmarking

* Added automated parser, client state-machine, server protocol-routing, and end-to-end mix tests.
* Added automated benchmark coverage for single-protocol stability, short-burst churn, stable mix-primary operation, intermittent fallback, failback, probe overhead, and a repeatable soak scenario.
* The latest local benchmark summary is documented in `doc/mix_benchmark_results.md`.

## Release Automation

* GitHub Actions now validate the codebase, run the mix benchmark harness for release tags, cross-build all packaged binaries on GitHub runners, and publish GitHub Releases directly from Actions.
* Release assets are uploaded from GitHub-hosted runners, not through a local workstation.

## Maintainer Notes

* Use `doc/mix.md` for feature behavior and configuration guidance.
* Use `doc/upstream-sync.md` for keeping this fork in sync with upstream `fatedier/frp`.
* Use `doc/agents/release.md` for the maintainer release flow and tag-to-release pipeline.
