#!/bin/sh
# Creates the Ed25519 key pair that signs zrss updates.
#
# The public key belongs in mygo.json (updates.publicKey); the private key
# stays out of git and becomes the MYGO_UPDATER_PRIVATE_KEY secret of the
# GitHub repository. Losing the private key means installed apps can never
# receive another update.
#
# Usage: sh scripts/keygen.sh [--out DIR]
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
. "$SCRIPT_DIR/common.sh"

OUT="$ROOT/.update-keys"
while [ $# -gt 0 ]; do
    case "$1" in
        --out)
            [ $# -ge 2 ] || die "--out needs a directory"
            OUT=$2
            shift 2
            ;;
        -h|--help)
            sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            die "unknown option: $1"
            ;;
    esac
done

case "$OUT" in
    /*) ;;
    *) OUT="$ROOT/$OUT" ;;
esac

cd "$ROOT"
key="$OUT/mygo-update.key"
pub="$OUT/mygo-update.pub"

if [ -f "$key" ]; then
    echo "Keys already exist in $OUT"
else
    go tool mygo keygen -o "$(native_path "$OUT")"
fi

public_key=$(tr -d '\015\012' < "$pub")
configured_key=$(config_value publicKey)
repo=$(config_value github)

printf '\nPublic key: %s\n' "$public_key"
if [ "$configured_key" = "$public_key" ]; then
    echo "mygo.json already has this key."
else
    printf 'WARNING: mygo.json updates.publicKey does not match this key pair.\n' >&2
fi

printf '\nNext:\n'
printf '  1. Store %s in a password manager.\n' "$key"
if command -v gh >/dev/null 2>&1; then
    printf '  2. Add it as the GitHub secret of %s:\n' "$repo"
    printf '       cat .update-keys/mygo-update.key | gh secret set MYGO_UPDATER_PRIVATE_KEY --repo %s\n' "$repo"
else
    printf '  2. Add it as the repository secret MYGO_UPDATER_PRIVATE_KEY of %s:\n' "$repo"
    printf '       https://github.com/%s/settings/secrets/actions\n' "$repo"
    printf '       Or install GitHub CLI: https://cli.github.com/\n'
fi
printf '  3. Never commit it: .update-keys/ is ignored by git.\n'
