#!/bin/sh
# Verifies zrss for a release, then optionally tags it, pushes the tag, and
# publishes the draft release with the update feed check.
#
# Usage:
#   sh scripts/release.sh                 # format, vet, test, signed build
#   sh scripts/release.sh --tag           # also tag and push (starts CI)
#   sh scripts/release.sh --publish       # publish the draft as latest
#
# Publish after CI has finished: the release workflow builds and uploads the
# signed archives and update manifests.
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
. "$SCRIPT_DIR/common.sh"

SKIP_TESTS=0
TAG=0
PUBLISH=0
ALLOW_DIRTY=0
REMOTE=github

while [ $# -gt 0 ]; do
    case "$1" in
        --skip-tests) SKIP_TESTS=1; shift ;;
        --tag) TAG=1; shift ;;
        --publish) PUBLISH=1; shift ;;
        --allow-dirty) ALLOW_DIRTY=1; shift ;;
        --remote)
            [ $# -ge 2 ] || die "--remote needs a name"
            REMOTE=$2
            shift 2
            ;;
        -h|--help)
            sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            die "unknown option: $1"
            ;;
    esac
done

VERSION=$(config_value version)
REPO=$(config_value github)
TAG_NAME="v$VERSION"

[ -n "$REPO" ] || die "mygo.json has no updates.github"
[ -n "$VERSION" ] || die "mygo.json has no version"
grep -qFx "## $VERSION" "$ROOT/CHANGELOG.md" 2>/dev/null ||
    die "CHANGELOG.md has no '## $VERSION' section"

if [ "$PUBLISH" -eq 0 ]; then
    if [ "$ALLOW_DIRTY" -eq 0 ] && [ -n "$(git status --porcelain)" ]; then
        die "The working tree has uncommitted changes. Commit them first, or pass --allow-dirty."
    fi

    if [ "$SKIP_TESTS" -eq 0 ]; then
        unformatted=$(gofmt -l .)
        [ -z "$unformatted" ] || die "gofmt wants to change: $unformatted"

        go vet ./...
        go test ./... -count=1
    fi

    sh "$SCRIPT_DIR/build.sh"

    printf '\nVerified zrss %s.\n' "$VERSION"
fi

if [ "$TAG" -eq 1 ]; then
    if git rev-parse -q --verify "refs/tags/$TAG_NAME" >/dev/null 2>&1; then
        die "Tag $TAG_NAME already exists"
    fi
    git tag "$TAG_NAME"
    git push "$REMOTE" "$TAG_NAME"
    printf 'Pushed %s to %s; CI is building the draft release.\n' "$TAG_NAME" "$REMOTE"
    printf 'When it finishes, run: sh scripts/release.sh --publish\n'
fi

if [ "$PUBLISH" -eq 1 ]; then
    command -v gh >/dev/null 2>&1 ||
        die "GitHub CLI (gh) is required to publish: https://cli.github.com/ — or publish the draft at https://github.com/$REPO/releases"

    gh release edit "$TAG_NAME" --repo "$REPO" --draft=false --latest

    printf '\nUpdate feeds:\n'
    for platform in windows-amd64 windows-arm64 darwin-arm64 darwin-amd64 linux-amd64 linux-arm64; do
        url="https://github.com/$REPO/releases/latest/download/update-$platform.json"
        if json=$(curl -fsSL --max-time 30 "$url" 2>/dev/null); then
            version=$(printf '%s' "$json" |
                sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
                head -n 1)
            printf '  %-14s %s\n' "$platform" "$version"
        else
            printf '  %-14s %s (not available)\n' "$platform" "$url" >&2
        fi
    done
fi
