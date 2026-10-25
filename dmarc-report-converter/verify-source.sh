#!/bin/sh
# Fetch the release tag, but never compile it unless it matches the reviewed pin.
set -eu

upstream=$1
version=$2
expected=$3
destination=$4

case "$expected" in
    *[!0-9a-f]*|'') echo 'sourceCommit must be a full lowercase Git commit ID' >&2; exit 1 ;;
esac
if [ "${#expected}" -ne 40 ]; then
    echo 'sourceCommit must be a full lowercase Git commit ID' >&2
    exit 1
fi

git init --quiet "$destination"
git -C "$destination" fetch --quiet --depth=1 "$upstream" "refs/tags/v$version"
actual=$(git -C "$destination" rev-parse 'FETCH_HEAD^{commit}')
if [ "$actual" != "$expected" ]; then
    printf 'Source commit mismatch: expected %s, got %s\n' "$expected" "$actual" >&2
    exit 1
fi
git -C "$destination" checkout --quiet --detach "$actual"
printf '%s\n' "$actual" > "$destination/source-revision"
printf 'Verified upstream source commit: %s\n' "$actual"
