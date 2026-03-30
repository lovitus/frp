#!/bin/sh

set -eu

if [ $# -ne 2 ]; then
    echo "usage: $0 <tag> <output-file>" >&2
    exit 1
fi

tag="$1"
out="$2"

previous_tag="$(git tag --sort=-version:refname | grep -Fxv "$tag" | head -n 1 || true)"
range="$tag"
if [ -n "$previous_tag" ]; then
    range="${previous_tag}..${tag}"
fi

cat >"$out" <<EOF
# frp ${tag}

## Release Focus

This release focuses on the Gateway runtime layer. Compared with \`${previous_tag:-the previous untagged state}\`, it adds embedded proxy targets on gateway clients, time-limited tunnel leases, richer gateway inspection, and a large batch of stability fixes around reload rollback, expiry handling, secret redaction, and dashboard editing flows.

## What Changed In This Release

### Gateway embedded proxy modes

* Added two new gateway tunnel target modes on top of the existing \`direct\` mode:
  * \`ss_proxy\`
  * \`socks5_proxy\`
* These modes run an embedded proxy service inside \`frpc\` and expose it through the existing gateway tunnel path on \`frps\`.
* \`ss_proxy\` supports TCP and UDP where the selected method supports UDP packet mode.
* \`socks5_proxy\` currently supports TCP only.

### Tunnel validity and expiration

* Added runtime tunnel validity controls:
  * \`permanent\`
  * \`Nh\`
  * \`Nd\`
* Expiration now removes expired tunnels from the client runtime instead of only changing dashboard state.
* Expiry handling was hardened so that:
  * already-expired tunnels are processed immediately
  * the next expiry is scheduled by the nearest deadline instead of a coarse fixed ticker
  * clients are synced once when expiry is applied, not repeatedly every cycle
  * editing an already-expired temporary tunnel renews it even when the validity unit/value stay the same

### Dashboard and API

* Gateway create/edit form now supports:
  * target type selection
  * SS method/password
  * SOCKS5 auth toggle and credentials
  * validity selection
* Gateway tunnel list/detail/create/update API responses now redact stored proxy secrets.
* YAML export/import keeps explicit expiry timestamps so restore/import does not silently extend temporary tunnels.
* Edit flows now allow leaving secret fields blank to keep the existing stored secret, instead of forcing re-entry on every update.

### Gateway observability

* Added on-demand gateway system inspection from the dashboard "More" dialog.
* Added richer gateway snapshot cards and tunnel cards with:
  * formatted validity
  * system/resource details
  * top memory processes
  * network interfaces
  * gateway runtime summary
* Proxy list backfill now preserves embedded gateway target type so gateway-managed proxies still show a readable target summary even without full proxy config hydration.

### Compatibility and safety fixes

* Added \`mixClientID\` and \`mix_client_id\` as aliases for \`clientID\`.
* Fixed gateway apply rollback behavior so a failed reload restores:
  * runtime config source state
  * live client config state
* Fixed embedded listener reuse so invalid updates do not tear down the currently running embedded proxy.
* Tightened Shadowsocks validation:
  * invalid UDP methods are rejected at config time
  * malformed Shadowsocks TCP headers are closed immediately instead of pinning goroutines
* Clearing or switching target type now removes stale secrets and stale inactive target fields from stored tunnel state.

## Compatibility Notes

* Existing \`direct\` gateway tunnels remain supported.
* Existing runtime gateway model is preserved: tunnels are still runtime-managed on \`frps\`.
* Existing mix transport behavior is preserved; this release builds on top of the previously shipped mix transport and gateway foundation.
* Secrets are still included in YAML export by design, because export is currently treated as a full restore/backup path.

## Validation

Validated in this release with:

* \`go test ./...\`
* \`npm run type-check\`
* \`npm run build-only\`

## Commit Range

EOF

if [ -n "$previous_tag" ]; then
    {
        echo "Changes since \`$previous_tag\`."
        echo
    } >>"$out"
else
    {
        echo "Initial tagged release in this repository."
        echo
    } >>"$out"
fi

git log --no-merges --pretty='* %h %s' "$range" >>"$out"
