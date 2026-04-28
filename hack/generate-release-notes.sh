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
if ! git rev-parse --verify "${tag}^{commit}" >/dev/null 2>&1; then
    range="HEAD"
    if [ -n "$previous_tag" ]; then
        range="${previous_tag}..HEAD"
    fi
fi

{
    echo "# frp ${tag}"
    echo
    if [ -f Release.md ]; then
        cat Release.md
        echo
    else
        echo "## Release Focus"
        echo
        echo "See the commit range below for changes in this release."
        echo
    fi

    echo "## Commit Range"
    echo
    if [ -n "$previous_tag" ]; then
        echo "Changes since \`${previous_tag}\`."
    else
        echo "Initial tagged release in this repository."
    fi
    echo
    git log --no-merges --pretty='* %h %s' "$range"
} >"$out"
