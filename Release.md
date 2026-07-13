## Release Focus

This release bounds UDP peer and session resources across mix transport, UDP/SUDP
proxies, and embedded Gateway UDP services. It prevents source-address floods
from permanently expanding Go maps or creating unbounded sockets, goroutines,
and packet buffers while preserving established sessions and TCP transports.

## What Changed In This Release

### Mix UDP admission control

* Split unauthenticated and authenticated KCP/QUIC peer routes.
* Added independent pending-peer quotas for KCP and QUIC so flooding one
  protocol does not consume the other's admission capacity.
* Pending peers expire after 10 seconds without extending their lifetime on
  additional packets. Authenticated routes retain the existing idle expiry.
* Token verification promotes a pending route atomically. Invalid or expired
  routes are rejected without evicting established peers.
* Stale established-route maps are rebuilt after substantial shrinkage so Go
  map buckets from earlier traffic peaks can be reclaimed.

### frpc UDP resource limits

* Added a shared per-proxy session limiter for UDP and SUDP forwarding.
* Added the same bounded-session behavior to embedded Shadowsocks and sing-box
  Gateway UDP services.
* Unknown UDP peers are dropped at capacity; existing peers continue to use
  their sockets and sessions.
* Closing or replacing a work connection now closes its old UDP sessions and
  immediately returns limiter capacity to the replacement connection.
* XTCP and NAT hole-punching sockets are intentionally outside these admission
  limits.

### Quick-deploy compatibility

* Migrated Unix quick-deploy scripts to portable `/bin/sh` syntax for BusyBox
  `ash`, `dash`, and other POSIX shells.
* Restored terminal input behavior for pipe-to-shell deployment.
* Corrected OpenWrt/ImmortalWrt MIPS endianness detection so `mipsel` devices
  download `linux_mipsle` binaries.

## Configuration

New settings are optional. Zero or omitted values use the defaults shown below.

```toml
# frps: pending limit is per UDP mix protocol; route limit is global.
transport.maxUDPPendingPeers = 4096
transport.maxUDPPeerRoutes = 65536

# frpc: shared by each UDP/SUDP proxy or embedded Gateway UDP service.
transport.maxUDPSessions = 1024
```

For memory-constrained OpenWrt/ImmortalWrt devices, start with
`transport.maxUDPSessions = 128` or `256` and increase only when required by
measured concurrent UDP usage.

## Compatibility Notes

* Existing configurations remain valid and receive the new safe defaults.
* The limits affect new UDP peers only; established peers are not evicted to
  admit unknown traffic.
* mix TCP, Shadowsocks TCP, SSH, WSS, and ordinary TCP proxies use independent
  TCP listeners and are not subject to UDP admission limits.
* Network-level UDP floods can still exhaust bandwidth, conntrack, CPU, or
  interrupt capacity. These limits bound application resources; they do not
  replace host or upstream DDoS protection.
* Common frpc transport settings, including `maxUDPSessions`, require an frpc
  restart to take effect.
* New frpc dials follow the current operating-system route. Mobile VPN hosts
  should protect sockets from VPN loopback and restart the frpc service after a
  debounced effective interface, address, or default-route change. Healthy
  same-address Wi-Fi roaming and unchanged DHCP renewals do not require restart.
* Leave `transport.connectServerLocalIP` empty when the client address can change.
  Embedded Gateway helper listeners use kernel-assigned loopback ports; fixed
  public, visitor, proxy, and web ports fail clearly on conflict rather than
  silently changing the advertised endpoint.

## Validation

Validated for this release with:

* `go test ./pkg/config/... ./pkg/transport/... ./pkg/metrics/... ./client/... ./server/...`
* `go test -race ./pkg/proto/udp ./pkg/transport/mix ./client`
* `go vet ./pkg/transport/mix ./pkg/proto/udp ./client ./server`
* `./hack/run-mix-bench.sh`
* Real-machine recovery checks cover UDP source-address flooding, concurrent TCP
  availability, process memory/FD stability, and non-disruptive route-loss and
  recovery simulation on a disposable interface.
