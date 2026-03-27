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
- `client/mix_test.go` and `client/service_test.go`
- `server/service.go` and `server/mix.go`
- `pkg/config/v1/*`
- `pkg/config/v1/validation/*`
- `pkg/msg/msg.go`
- `conf/frpc_mix_example.toml`
- `doc/mix.md`
- `.github/workflows/*`

Those files carry most of the mix transport, observability, and release automation changes.

## Recommended Conflict Policy

- Prefer taking upstream structural refactors first, then re-apply the mix behavior.
- Treat the client-side mix candidate order as a flattened priority queue of `endpoint × protocol`. Re-verify this invariant after any upstream reconnect or connector refactor.
- Re-check `mixFallbackHosts` parsing and alias loading if upstream changes client config loading, strict validation, or TOML field names.
- Re-run `./hack/run-mix-bench.sh` after resolving conflicts in client/server mix code.
- Re-run `go test ./pkg/config/... ./client/...` after resolving config or client merge conflicts so host fallback ordering and failback tests still pass.
- Re-check `Release.md`, `doc/mix.md`, `doc/mix_benchmark_results.md`, and workflow files after sync if upstream changed packaging or release behavior.

## Long-Term Maintainer Checklist

- If upstream adds new transport protocols, decide explicitly whether they stay single-protocol only or become valid `mixToken` entries.
- If upstream changes connector lifecycle or control replacement behavior, re-verify that mix still uses the intended "close current control, recreate next control" model.
- If upstream changes config tags or alias handling, update both camelCase and snake_case fields for `mixBindPort`, `mixToken`, and `mixFallbackHosts`.
- Keep example configs aligned with real parser behavior. Example hostnames and IPs should remain illustrative only, never treated as defaults in code.
