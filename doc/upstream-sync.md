# Upstream Sync Guide

This fork is easiest to maintain with:

- `origin` -> your fork
- `upstream` -> `https://github.com/fatedier/frp.git`

## One-Time Remote Setup

If your local clone still points `origin` at upstream:

```bash
git remote rename origin upstream
git remote add origin https://github.com/lovitus/frp.git
git remote -v
```

## Sync Development Branch

Use this when upstream `dev` has moved and you want to bring the fork forward:

```bash
git fetch upstream --tags
git checkout dev
git pull origin dev
git merge upstream/dev
go test ./pkg/config/... ./pkg/transport/... ./pkg/metrics/... ./client/... ./server/...
git push origin dev
```

If you keep feature branches, rebase or merge them on top of the updated local `dev` after this step.

## Sync Stable Branch

If you also keep a stable `master` branch aligned with upstream:

```bash
git fetch upstream --tags
git checkout master
git pull origin master
git merge upstream/master
go test ./pkg/config/... ./pkg/transport/... ./pkg/metrics/... ./client/... ./server/...
git push origin master
```

## Mix-Specific Conflict Hotspots

When upstream changes, check these areas first:

- `client/service.go` and `client/mix.go`
- `client/gateway.go`
- `client/mix_test.go` and `client/service_test.go`
- `server/service.go` and `server/mix.go`
- `server/gateway.go`
- `server/proxy/tcp.go` and `server/proxy/udp.go`
- `server/http/controller.go`
- `pkg/config/v1/*`
- `pkg/config/v1/validation/*`
- `pkg/config/source/*`
- `pkg/gateway/*`
- `pkg/msg/msg.go`
- `README.md` and `README_zh.md`
- `conf/frpc_full_example.toml` and `conf/frps_full_example.toml`
- `conf/frpc_mix_example.toml`
- `doc/mix.md`
- `doc/gateway_tunnels.md`
- `.github/workflows/*`
- `web/frps/src/*`

Those files carry most of the mix transport, runtime gateway tunnel, observability, dashboard, and release automation changes.

## Recommended Conflict Policy

- Prefer taking upstream structural refactors first, then re-apply the mix behavior.
- Treat the client-side mix candidate order as a flattened priority queue of `endpoint × protocol`. Re-verify this invariant after any upstream reconnect or connector refactor.
- Re-check `mixFallbackHosts` parsing and alias loading if upstream changes client config loading, strict validation, or TOML field names.
- Re-run `./hack/run-mix-bench.sh` after resolving conflicts in client/server mix code.
- Re-run `go test ./pkg/config/... ./client/...` after resolving config or client merge conflicts so host fallback ordering and failback tests still pass.
- Re-run `go test -run 'TestMix' ./server` after changing mix listeners, protocol demux, or endpoint fallback behavior.
- Re-run `go test ./pkg/gateway ./client ./server` after changing runtime gateway synchronization, status refresh, or bind-address handling.
- Re-run `npm --prefix web/frps run build` after touching gateway dashboard views or API wiring.
- Re-check `Release.md`, `doc/mix.md`, `doc/mix_benchmark_results.md`, and workflow files after sync if upstream changed packaging or release behavior.
- Re-check `README.md`, `README_zh.md`, `doc/gateway_tunnels.md`, and the full example TOML files so user-facing docs still match the actual mix parser, retry model, and runtime gateway behavior.

## Long-Term Maintainer Checklist

- If upstream adds new transport protocols, decide explicitly whether they stay single-protocol only or become valid `mixToken` entries.
- If upstream changes connector lifecycle or control replacement behavior, re-verify that mix still uses the intended "close current control, recreate next control" model.
- If upstream changes config tags or alias handling, update both camelCase and snake_case fields for `mixBindPort`, `mixToken`, and `mixFallbackHosts`.
- Keep `allowGatewayTunnels` as the canonical client field and preserve `mixAllowGateway` / `mix_allow_gateway` only as compatibility aliases unless there is a deliberate migration plan.
- If upstream changes runtime config source merging, re-check that gateway-managed proxies still survive `start` filtering and still override the static source in the intended order.
- If upstream changes server proxy listener code, re-check that gateway tunnel `bindAddr` remains scoped only to runtime gateway-managed listeners.
- Keep example configs aligned with real parser behavior. Example hostnames and IPs should remain illustrative only, never treated as defaults in code.
