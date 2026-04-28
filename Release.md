## Release Focus

This release focuses on the operator-facing quick-deploy path and the Gateway dashboard experience. Compared with the previous tagged release, it improves one-command bootstrap on Unix/OpenWrt/Windows, keeps generated client install commands pinned to the same release, and makes the Gateway page denser and easier to operate.

## What Changed In This Release

### Quick deploy

* Added Windows PowerShell quick-deploy entry scripts for both `frps` and `frpc`.
* The `frps` installer now prints pinned `frpc` onboarding commands for Unix/macOS/Linux and Windows, so clients install the same release as the server.
* The top-level Unix installer now normalizes the deployment mode consistently for both interactive input and `--type` arguments:
  * `frpc`
  * `FRPC`
  * ` frpc `
* Removed the top-level installer's dependency on `xargs` for parsing `frps/frpc`, which improves OpenWrt/BusyBox compatibility while preserving the one-command interactive flow.
* Documented the quick-start path directly in `README.md` so new operators do not need to read the full technical documentation first.

### Gateway dashboard

* Updated the Gateway page layout for denser scanning:
  * compact status tabs
  * search-first filtering
  * inline gateway-node summary
  * compact tunnel cards with `listen`, `gateway`, `target`, and `status`
* Moved YAML import/export into a compact overflow menu and added a visible memory-only warning chip.
* Improved the create/edit dialog layout with segmented controls for protocol and target type.
* Added a safe empty-state preview so the page remains understandable before any eligible gateway client or tunnel exists.
* Preserved the existing runtime-only Gateway model. No server-side persistence or filesystem storage is added in this release.

## Compatibility Notes

* Existing `frps.toml` and `frpc.toml` files remain compatible.
* Existing Gateway tunnel APIs and runtime tunnel definitions remain compatible.
* `allowGatewayTunnels`, `mixAllowGateway`, `clientID`, and `mixClientID` behavior is unchanged.
* The release packaging workflow still injects the tag version at build time and verifies archive names, archive root directories, and binary `--version` output against the tag.

## Operator Notes

* On OpenWrt-like shells, the top-level installer should now accept the first interactive `frps/frpc` answer directly. `--type frpc` and `--type frps` remain supported for scripted installs.
* If Windows security software flags the binary because of embedded proxy capabilities, this release does not remove those capabilities. Operators should use allowlisting or controlled distribution rather than relying on reduced functionality.
* Gateway tunnels are still runtime-only. Use Dashboard YAML export/import if you need a manual backup/restore path.

## Validation

Validated for this release with:

* `bash -n hack/quick-deploy/install.sh`
* quick-deploy mode parsing smoke tests for uppercase, whitespace, interactive, and invalid values
* `go test ./pkg/config/... ./pkg/transport/... ./pkg/metrics/... ./client/... ./server/...`
* `npm run type-check`
* `npm run build-only`
