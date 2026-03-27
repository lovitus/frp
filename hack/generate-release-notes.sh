#!/bin/sh

set -eu

if [ $# -ne 2 ]; then
    echo "usage: $0 <tag> <output-file>" >&2
    exit 1
fi

tag="$1"
out="$2"

previous_tag="$(git tag --sort=-version:refname | grep -Fxv "$tag" | head -n 1 || true)"

{
    echo "# frp ${tag}"
    echo
    cat Release.md
    echo
    echo "## Automated Changelog"
    echo
    if [ -n "$previous_tag" ]; then
        echo "Changes since \`$previous_tag\`."
        echo
        git log --no-merges --pretty='* %h %s' "${previous_tag}..${tag}"
    else
        echo "Initial tagged release in this repository."
        echo
        git log --no-merges --pretty='* %h %s' "$tag"
    fi
} > "$out"
