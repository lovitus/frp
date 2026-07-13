# Mix Transport

`mix` adds a high-level transport selector on top of frp's existing single-protocol client/server transports.

## Configuration

Enable mix on both `frps` and `frpc` with the same two fields:

```toml
mixBindPort = 7000
mixToken = "kcp://kcppass,quic://quicpass,ss://aes-256-gcm:sspass,wss://wsspass,ssh://user:sshpass,tcp://tcppass"
```

The server treats `mixToken` as an unordered set of enabled transports. The client treats the same list as an ordered priority list.

Optional client-only host fallback can extend the priority list across multiple frps endpoints:

```toml
serverAddr = "10.20.0.64"
mixBindPort = 7001
mixFallbackHosts = "10.20.0.65,kr.goodfood.com:7002,[2401:c080:1c02:aaf:5400:8ff:fe88:d88f]:7007"
mixToken = "kcp://kcppass,ss://aes-256-gcm:sspass,ssh://user:sshpass"
```

`mixFallbackHosts` is ordered. Each entry is `HOST` or `HOST:PORT`. If `PORT` is omitted, `mixBindPort` is used.
The host values in the example above are illustrative only. There are no built-in fallback endpoints in the client.

Supported token forms:

```text
kcp://PASSWORD
quic://PASSWORD
ss://METHOD:PASSWORD
wss://PASSWORD
ssh://USERNAME:PASSWORD
tcp://PASSWORD
```

## Candidate Order

The client expands its retry priority into a single flattened candidate queue:

1. `serverAddr:mixBindPort` with `mixToken[0]`
2. `serverAddr:mixBindPort` with `mixToken[1]`
3. `...`
4. `mixFallbackHosts[0]` with `mixToken[0]`
5. `mixFallbackHosts[0]` with `mixToken[1]`
6. `...`

If the last configured candidate also keeps failing, the client wraps to the first candidate and continues retrying. It does not exit just because every candidate has failed once.

Failback walks the same flattened queue from the beginning up to the candidate just ahead of the current active one. As soon as an earlier candidate becomes healthy again, the client closes the current control connection and recreates it on that earlier candidate.

## Runtime Behavior

- The client tries transports in the order listed in `mixToken`.
- If `mixFallbackHosts` is configured, the client expands the dial order as `primary host × protocols`, then `fallback host #1 × protocols`, then `fallback host #2 × protocols`, and so on.
- The server listens on `mixBindPort/tcp` and `mixBindPort/udp`.
- Selected transport is exported in login metadata, logs, dashboard client status, and Prometheus server metrics.
- Client logs include both the selected protocol and the selected endpoint when host fallback is in use.
- Fallback waits for three consecutive failures before advancing to the next transport.
- When the last configured candidate also keeps failing, fallback wraps to the first configured candidate and continues cycling instead of exiting.
- Failback probes higher-priority transports periodically when the active transport is not the first one in the list.
- When `mix` is enabled, the initial `loginFailExit` behavior is ignored so startup can keep retrying across protocols.

## UDP Resource Limits

`frps` keeps unauthenticated and authenticated UDP peers in separate bounded
route tables. KCP and QUIC have independent pending quotas, so packets that fill
one protocol's pending table do not consume the other protocol's quota.

```toml
# Defaults shown explicitly; both fields are optional.
transport.maxUDPPendingPeers = 4096
transport.maxUDPPeerRoutes = 65536
```

`maxUDPPendingPeers` applies independently to each mix UDP protocol. Pending
peers have a 10-second absolute lifetime and are promoted only after token
verification. `maxUDPPeerRoutes` applies to the combined authenticated KCP and
QUIC route table. At capacity, frps drops packets from unknown peers rather than
evicting established routes.

`frpc` separately bounds sockets and goroutines created for UDP/SUDP and
embedded Gateway UDP forwarding:

```toml
# Default shown explicitly; this field is optional.
transport.maxUDPSessions = 1024
```

The frpc limit is shared by each UDP/SUDP proxy or embedded Gateway service.
Existing remote addresses continue to work at capacity; packets for unknown
addresses are dropped until a slot is released. On memory-constrained embedded
devices, `128` or `256` is a more conservative starting point. This common
transport setting requires an frpc restart to take effect.

These settings do not apply to XTCP, NAT hole-punching probes, or direct tunnels
created by `pkg/nathole`. TCP transports use independent listeners and do not
consult the UDP admission tables. A network-level UDP flood can still affect
TCP by exhausting bandwidth, CPU, conntrack, or interrupt capacity, so upstream
DDoS controls remain necessary.

## Testing Coverage

Automated coverage for mix is split across:

- parser and validation tests in `pkg/config/...`
- client state-machine tests in `client/...`
- server protocol-routing and end-to-end tests in `server/...`
- repeatable benchmark and soak coverage in `./hack/run-mix-bench.sh`

Host fallback is covered both at the config/state-machine level and in server-side integration tests that verify fallback to a backup endpoint and failback to the primary endpoint.

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
- Host-level fallback is currently a client-side extension. The server still only needs the same `mixBindPort` and `mixToken` transport configuration.

## Related Docs

- `doc/mix_benchmark_results.md`
- `doc/gateway_tunnels.md`
- `doc/upstream-sync.md`
- `doc/agents/release.md`
