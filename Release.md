## Release Focus

This release adds an offline local-binary wizard for quick deploy. Operators who
already have `frps` or `frpc` can now generate, verify, and smoke-test quick
deploy TOML configs directly from the binary without downloading or piping the
online bootstrap scripts.

## What Changed In This Release

### Offline quick-deploy wizard

* Added `frps --wizard` for local server bootstrap.
* Added `frpc --wizard` for local client bootstrap.
* The wizard writes TOML configs in the current working directory by default:
  * `frps.toml` for `frps --wizard`
  * `frpc.toml` for `frpc --wizard`
* `-c/--config` can be used to select an explicit output path.
* Existing target config files are never overwritten; reruns require deleting or
  renaming the previous output first.

### Server-to-client bootstrap

* The server wizard prints local `frpc --wizard` bootstrap commands for
  Unix-like shells and Windows PowerShell.
* The printed client command carries `mixBindPort` and a base64-encoded
  `mixToken`, so the client wizard only asks for:
  * `serverAddr`
  * `clientID`
* The server wizard still prints online quick-deploy commands for operators who
  want to install `frpc` on another machine.

### Validation and smoke checks

* Generated configs are verified with the running binary before the wizard
  reports success.
* `frps --wizard` performs a short start check and fails if the server exits
  immediately.
* `frpc --wizard` performs a short start check and warns, rather than failing,
  when the client exits quickly because the server is not reachable yet.
* `frpc --wizard --config_dir ...` is rejected because the wizard creates one
  local config file.

### Operator-facing hardening

* Connection passwords used to generate the default `mixToken` reject commas,
  because `mixToken` is comma-delimited.
* Shell and PowerShell command output is quoted for paths and token payloads.
* Preset `--mix-token-b64` input is decoded before TOML rendering so shell
  transport does not corrupt token values.

## Compatibility Notes

* Existing `frps.toml` and `frpc.toml` files remain compatible.
* Existing online quick-deploy scripts are unchanged and remain the default path
  when a matching release binary needs to be downloaded.
* The offline wizard is additive and only runs when `--wizard` is supplied.
* `frpc --wizard` intentionally ignores the legacy implicit `./frpc.ini`
  default and writes `./frpc.toml` unless `-c/--config` is explicitly provided.
* Generated client configs keep Gateway-friendly defaults:
  * `allowGatewayTunnels = true`
  * `mixAllowGateway = true`
  * `loginFailExit = false`

## Operator Notes

* Use `./frps --wizard` on a server that already has the `frps` binary.
* Copy one of the printed local `frpc --wizard` commands to a client host that
  already has the `frpc` binary.
* Use the printed online quick-deploy commands when the client host still needs
  to download a release binary.
* If the wizard refuses to overwrite an existing config, move or delete that
  file before rerunning.

## Validation

Validated for this release with:

* `go test -tags ",noweb" -v ./cmd/internal/wizard`
* `go test -tags ",noweb" ./cmd/...`
* `make wizard-acceptance`
* `go test ./pkg/config/... ./pkg/transport/... ./pkg/metrics/... ./client/... ./server/...`
* `./hack/run-mix-bench.sh`
