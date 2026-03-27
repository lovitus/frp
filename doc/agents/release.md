# Release Process

This repository now uses GitHub Actions to validate, package, and publish release artifacts directly from GitHub-hosted runners.

## Expected Git Remotes

Use this remote layout so upstream sync and fork publishing stay separate:

- `origin`: your fork, for example `https://github.com/lovitus/frp.git`
- `upstream`: the source project, `https://github.com/fatedier/frp.git`

## Regular Development Flow

1. Sync your local branches from upstream.
2. Land new work on `dev`.
3. Keep curated release notes up to date in `Release.md`.
4. Run the local validation and mix benchmark commands before tagging:

```bash
go test ./pkg/config/... ./pkg/transport/... ./pkg/metrics/... ./client/... ./server/...
./hack/run-mix-bench.sh
```

## Stable Release Flow

1. Merge the desired `dev` state into `master`.
2. Update `pkg/util/version/version.go` if the binary version needs to change.
3. Update `Release.md` with curated highlights, compatibility notes, and operational guidance.
4. Create and push an annotated tag:

```bash
git checkout master
git pull origin master
git tag -a vX.Y.Z -m "release vX.Y.Z"
git push origin vX.Y.Z
```

## What GitHub Actions Do On Tag Push

When a `v*` tag is pushed, `.github/workflows/ci-release.yml` will:

1. Build the web assets.
2. Run the Go validation suite.
3. Run `./hack/run-mix-bench.sh`.
4. Cross-build packaged binaries for all configured OS/arch targets via `./package.sh`.
5. Generate SHA256 checksums for the packaged artifacts.
6. Generate release notes by combining `Release.md` with an automated git changelog.
7. Create or update the GitHub Release and upload all packages directly from Actions.

The workflow never depends on a local workstation to upload release binaries.

## Release Notes Sources

- Curated intro and operator-facing notes: `Release.md`
- Automated changelog assembly: `hack/generate-release-notes.sh`
- Benchmark evidence: `doc/mix_benchmark_results.md` and workflow artifacts from `tmp/mix-bench/`

## Release Assets

The release job publishes:

- all packaged `frpc` / `frps` archives from `release/packages/`
- `frp_sha256_checksums.txt`

## Related Docs

- `doc/mix.md`
- `doc/mix_benchmark_results.md`
- `doc/upstream-sync.md`
