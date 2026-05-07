# Gateway Tunnels

This fork adds a server-managed runtime gateway tunnel feature to the frps dashboard.

It reuses the existing frpc <-> frps control connection. That means it works with either a normal single transport or the active `mix` candidate. There is no separate data plane or extra SOCKS layer for this feature.

## What It Does

The dashboard can create a runtime TCP or UDP listener on frps and attach it to one opted-in frpc client.

Each tunnel defines:

- a listener on frps: `bindAddr:listenPort`
- the selected gateway client
- the final destination on that client side: `targetHost:targetPort`
- or an embedded client-side proxy target such as `ss_proxy`, `sing_ss_proxy`, or `socks5_proxy`

frps keeps the desired runtime definition in memory and synchronizes it to the selected client over the existing control channel. frpc then materializes it as a normal runtime proxy.

## Client Requirements

The selected frpc must:

- set `clientID`
- opt in to server-managed gateway tunnels

Recommended config:

```toml
clientID = "edge-kr-01"
allowGatewayTunnels = true
```

Backward-compatible aliases are also accepted:

```toml
mixAllowGateway = true
mix_allow_gateway = true
```

`allowGatewayTunnels` is the canonical field. The `mix*` names are kept only as compatibility aliases.

## Dashboard Behavior

The frps dashboard exposes a `Gateway` page with runtime CRUD operations.

Each tunnel includes:

- `name`
- `remark`
- `protocol`: `tcp` or `udp`
- `bindAddr`: defaults to `0.0.0.0`
- `listenPort`
- `gatewayClient`
- `targetHost`: defaults to `127.0.0.1`
- `targetPort`
- `targetType`: `direct`, `ss_proxy`, `sing_ss_proxy`, or `socks5_proxy`
- `ssMethod` / `ssPassword` for Shadowsocks targets
- `uotEnabled` / `uotVersion` for `sing_ss_proxy` TCP UDP-over-TCP
- `status`

The page is optimized for dense operation:

- the header shows a memory-only warning because tunnels are not persisted by frps
- YAML import/export is available from the compact overflow menu
- status tabs filter all, online, pending, and attention-needed tunnels
- each tunnel card keeps `listen`, `gateway`, `target`, and `status` visible together on desktop
- the create/edit dialog uses segmented controls for protocol and target type

If there are no eligible gateway clients and no tunnels yet, the page may show a local preview dataset to demonstrate the layout. These preview rows are not sent to frps and cannot be edited or deleted.

The page refreshes tunnel state on load and on create, update, or delete. It does not run a continuous background poll by default.

The page also includes YAML import/export:

- `Export YAML` returns the current runtime gateway tunnel set as YAML text.
- `Import YAML` accepts pasted YAML text or a browser-selected local file.
- Exported YAML includes proxy secrets such as `ssPassword` and `socks5Pass` in plaintext.
- Import performs upsert by `clientKey + name` (update if existing, create if missing).
- Import is idempotent for replay: importing the same YAML again updates existing items instead of failing on duplicates.
- Import accepts unknown/offline `clientKey` entries so restore can happen before clients reconnect; those tunnels stay pending until a matching client appears.
- The dashboard and API only process YAML content. There is no server-side file path read/write.

The page also captures a one-time gateway client snapshot when the page is first loaded:

- `online / registered` gateway client count
- an expandable gateway node list with online/offline status and client details

This snapshot is captured at page entry and can be refreshed manually from the page refresh action. It is not continuously polled in the background.

In `Proxies`, gateway-managed runtime proxies keep their internal proxy name for identity and metrics, and show an additional friendly summary line:

- gateway tunnel `name`
- gateway tunnel `remark`
- gateway target address `targetHost:targetPort`

## Status Model

The current runtime status can be one of:

- `pending`
- `online`
- `client-offline`
- `disabled`
- `client-unsupported`
- `invalid-config`
- `apply-failed`
- `register-failed`
- `target-invalid`
- `target-unreachable`

`online` means the runtime proxy has been registered successfully on frps and the client-side target check also passed.

`client-unsupported` means the selected client is online and allows gateway tunnels, but its login metadata does not advertise the capability needed by the tunnel target type.

For UDP, status is still best-effort. The feature reuses the existing frp UDP proxy path and does not add a special lifecycle manager beyond normal runtime proxy handling.

## Embedded Proxy Targets

`ss_proxy` keeps the existing go-shadowsocks2 implementation unchanged.

`sing_ss_proxy` is a separate Shadowsocks target implemented with `github.com/sagernet/sing-shadowsocks`. It is intended for Mihomo-style Shadowsocks configuration:

- Mihomo `cipher` maps to `ssMethod`.
- Mihomo `password` maps to `ssPassword`.
- `protocol = tcp` exposes a TCP Shadowsocks service on the gateway client.
- `protocol = udp` exposes a UDP Shadowsocks service on the gateway client.
- `protocol = tcp` plus `uotEnabled = true` enables UDP-over-TCP. `uotVersion` accepts `1` or `2` and defaults to `2`.
- `2022-*` methods require a base64 PSK with the length expected by sing-shadowsocks. The server validates this as a PSK and does not convert normal passwords automatically.

`sing_ss_proxy` requires a new enough frpc that logs in with `gateway_sing_ss_proxy=true`. frps rejects create/update requests for online clients without this capability and does not sync existing `sing_ss_proxy` tunnels to clients that reconnect without it.

## Persistence Model

Gateway tunnels are runtime-only right now.

- They live in frps process memory.
- They are synchronized again when the selected client reconnects.
- They are lost if frps restarts.

There is currently no server-side persistent store for gateway tunnels.

## Binding Rules

- `bindAddr` must be a literal IP address.
- Default is `0.0.0.0`.
- Loopback binds such as `127.0.0.1` are supported.
- IPv6 literal binds are supported.

Gateway tunnel `bindAddr` is applied only to runtime gateway-managed TCP/UDP listeners. It does not change the global `proxyBindAddr` behavior for normal static proxies.

## Security Notes

- This feature is opt-in on the client.
- frps can only target clients that explicitly allow gateway tunnels and expose a stable `clientID`.
- The server operator can ask the client to connect to `targetHost:targetPort`, so this feature should be enabled only on trusted clients.
- The frps dashboard now applies a simple auth lockout: 10 failed logins within 1 minute pause authentication for 10 seconds. API requests receive `429`, and browser requests get an unlock countdown page.
- The lockout is only a brute-force speed bump. It does not protect plaintext HTTP from credential or session capture. Use HTTPS or place the dashboard behind a secure tunnel when it is reachable outside localhost.

## Examples

Client:

```toml
serverAddr = "frps.example.com"
serverPort = 7000
clientID = "sg-app-01"
allowGatewayTunnels = true
```

Or with `mix`:

```toml
serverAddr = "frps.example.com"
mixBindPort = 7001
mixToken = "kcp://kcppass,ss://aes-256-gcm:sspass,ssh://user:sshpass"
clientID = "sg-app-01"
allowGatewayTunnels = true
```

Then create the runtime listener from the frps dashboard:

- `bindAddr = 0.0.0.0`
- `listenPort = 6000`
- `gatewayClient = sg-app-01`
- `targetHost = 127.0.0.1`
- `targetPort = 22`

## Known Limits

- Runtime only, no persistence across frps restart.
- Import is additive/upsert only in this phase. It does not delete tunnels that are absent in the imported YAML.
- No ACL or target allowlist yet.
- No batch operations.
- `bindAddr` is limited to IP literals.
- Status refresh is on-demand, not continuously streamed.
