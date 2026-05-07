## Release Focus

This release focuses on expanding Gateway embedded proxy modes with Mihomo-compatible Shadowsocks over `sing-shadowsocks`, while keeping the existing `ss_proxy` path untouched. Compared with the previous tagged release, it adds a new `sing_ss_proxy` target type with TCP, UDP, and UDP-over-TCP support, plus the capability gating and dashboard changes needed to operate it safely.

## What Changed In This Release

### Gateway embedded sing Shadowsocks

* Added a new Gateway target type `sing_ss_proxy` alongside the existing `direct`, `ss_proxy`, and `socks5_proxy` modes.
* Added embedded `sing-shadowsocks` handling for:
  * pure Shadowsocks TCP
  * pure Shadowsocks UDP
  * Shadowsocks TCP carrying UDP-over-TCP (UOT)
* Added UOT configuration fields to Gateway tunnels:
  * `uotEnabled`
  * `uotVersion`
* Implemented UOT compatibility for Mihomo-style `udp-over-tcp-version: 1` and `2`.
* Reused existing `ssMethod` and `ssPassword` fields instead of introducing duplicate sing-specific secret fields.
* Added method validation through `sing-shadowsocks`, including AEAD 2022 password/PSK checks.

### Capability gating and status flow

* Added explicit client capability advertisement via login meta `gateway_sing_ss_proxy=true`.
* Prevented `frps` from assigning `sing_ss_proxy` tunnels to clients that do not advertise support.
* Added a stable Gateway status `client-unsupported` so unsupported clients are surfaced explicitly instead of oscillating between pending and apply failures.
* Updated status refresh ordering so expired tunnels remain `expired` even when the client is offline, disabled, or lacks sing-ss capability.
* Cleared stale `client-unsupported` state when a capable client reconnects and can accept the tunnel again.

### Gateway dashboard and API

* Extended Gateway HTTP API, YAML import/export, and wire messages with `sing_ss_proxy`, `uotEnabled`, and `uotVersion`.
* Added `Sing Shadowsocks` to the Gateway tunnel form and list views.
* Added UOT controls to the dashboard for `sing_ss_proxy + tcp`.
* Surfaced client sing-ss capability in the dashboard so unsupported client selections are blocked before submission.
* Updated tunnel summaries to distinguish normal sing-ss TCP, UDP, and `tcp+uot` modes.

## Compatibility Notes

* Existing `frps.toml` and `frpc.toml` files remain compatible.
* Existing `ss_proxy` and `socks5_proxy` behavior is preserved; `sing_ss_proxy` is additive.
* `sing_ss_proxy` requires a client that advertises `gateway_sing_ss_proxy=true` during login. Older clients will not receive those tunnels.
* Gateway tunnels remain runtime-only. No new server-side persistence or filesystem storage is introduced in this release.
* The release packaging workflow still injects the tag version at build time and verifies archive names, archive root directories, and binary `--version` output against the tag.

## Operator Notes

* For Mihomo `type: ss` clients using `udp-over-tcp: true`, configure `udp-over-tcp-version` to match the tunnel's `uotVersion`.
* AEAD 2022 methods continue to use PSK-style passwords rather than arbitrary strings. Operators should provision those values exactly as generated.
* Pure UDP sing-ss tunnels and UOT tunnels now share the same Gateway UI and import/export workflow, but they remain distinct runtime modes.

## Validation

Validated for this release with:

* `go test ./pkg/config/... ./pkg/transport/... ./pkg/metrics/... ./client/... ./server/...`
* `go test ./pkg/gateway -count=1`
* `go test ./client -count=1`
* `go test ./server -count=1`
