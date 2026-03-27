# Mix Transport

`mix` adds a high-level transport selector on top of frp's existing single-protocol client/server transports.

## Configuration

Enable mix on both `frps` and `frpc` with the same two fields:

```toml
mixBindPort = 7000
mixToken = "kcp://kcppass,quic://quicpass,ss://aes-256-gcm:sspass,wss://wsspass,ssh://user:sshpass,tcp://tcppass"
```

The server treats `mixToken` as an unordered set of enabled transports. The client treats the same list as an ordered priority list.

Supported token forms:

```text
kcp://PASSWORD
quic://PASSWORD
ss://METHOD:PASSWORD
wss://PASSWORD
ssh://USERNAME:PASSWORD
tcp://PASSWORD
```

## Runtime Behavior

- The client tries transports in the order listed in `mixToken`.
- The server listens on `mixBindPort/tcp` and `mixBindPort/udp`.
- Selected transport is exported in login metadata, logs, dashboard client status, and Prometheus server metrics.
- Fallback waits for three consecutive failures before advancing to the next transport.
- When the last configured transport also keeps failing, fallback wraps to the first configured transport and continues cycling instead of exiting.
- Failback probes higher-priority transports periodically when the active transport is not the first one in the list.
- When `mix` is enabled, the initial `loginFailExit` behavior is ignored so startup can keep retrying across protocols.

## Logging

The client and server emit mix-specific logs for:

- `mix init`
- `mix protocol dial start`
- `mix protocol dial fail`
- `mix fallback start`
- `mix fallback success`
- `mix failback probe start`
- `mix failback probe fail`
- `mix failback switch start`
- `mix failback switch success`
- `selected protocol`

## Examples

- Server example: `conf/frps_mix_example.toml`
- Client example: `conf/frpc_mix_example.toml`

## Known Limits

- `ss-udp` is not implemented.
- `mix` keeps the existing single-control-connection model. Fallback and failback allow a short interruption.
- TCP raw `mix` auth uses an explicit magic prefix, so raw TCP is matched before Shadowsocks decryption when that prefix is present.
- The server exposes `frp_server_client_selected_protocol_counts{selected_protocol="..."}` for online clients grouped by active transport.

## Related Docs

- `doc/mix_benchmark_results.md`
- `doc/upstream-sync.md`
- `doc/agents/release.md`
