#!/bin/sh
# Builds zrss locally with signed update archives and manifests.
#
# The signing key comes from MYGO_UPDATER_PRIVATE_KEY, or from
# .update-keys/mygo-update.key when the variable is not set. The platform
# defaults to the current one; pass a comma-separated list to cross-compile.
#
# Usage: sh scripts/build.sh [--platform OS/ARCH[,OS/ARCH...]] [--upload]
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
. "$SCRIPT_DIR/common.sh"

PLATFORM="$(go env GOOS)/$(go env GOARCH)"
UPLOAD=0
while [ $# -gt 0 ]; do
    case "$1" in
        --platform)
            [ $# -ge 2 ] || die "--platform needs a value such as windows/amd64"
            PLATFORM=$2
            shift 2
            ;;
        --upload)
            UPLOAD=1
            shift
            ;;
        -h|--help)
            sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            die "unknown option: $1"
            ;;
    esac
done

require_key
cd "$ROOT"

if [ "$UPLOAD" -eq 1 ] && [ -z "${GH_TOKEN:-}" ]; then
    die "--upload needs GH_TOKEN, for example: export GH_TOKEN=\$(gh auth token)"
fi

set -- tool mygo build -platform "$PLATFORM"
if [ "$UPLOAD" -eq 1 ]; then
    set -- "$@" -upload
fi
go "$@"

manifests=$(find "$ROOT/build" -type f -name 'update-*.json' 2>/dev/null || true)
[ -n "$manifests" ] || die "No update-*.json was written: check MYGO_UPDATER_PRIVATE_KEY and updates.publicKey."

printf '\nUpdate manifests:\n'
printf '%s\n' "$manifests"
