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
- `server/service.go` and `server/mix.go`
- `pkg/config/v1/*`
- `pkg/msg/msg.go`
- `.github/workflows/*`

Those files carry most of the mix transport, observability, and release automation changes.

## Recommended Conflict Policy

- Prefer taking upstream structural refactors first, then re-apply the mix behavior.
- Re-run `./hack/run-mix-bench.sh` after resolving conflicts in client/server mix code.
- Re-check `Release.md`, `doc/mix.md`, and workflow files after sync if upstream changed packaging or release behavior.
