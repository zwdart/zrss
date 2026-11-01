#!/bin/sh
# Shared helpers for the release scripts. POSIX sh: works in Git Bash, WSL,
# macOS and Linux. ROOT must be set by the caller.

die() {
    printf '%s\n' "$*" >&2
    exit 1
}

# config_value <key> reads a top-level string value from mygo.json.
config_value() {
    sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$ROOT/mygo.json" | head -n 1
}

# require_key exports MYGO_UPDATER_PRIVATE_KEY from the local key when the
# variable is not set.
require_key() {
    if [ -n "${MYGO_UPDATER_PRIVATE_KEY:-}" ]; then
        return 0
    fi
    key="$ROOT/.update-keys/mygo-update.key"
    [ -f "$key" ] || die "No signing key: run scripts/keygen.sh or set MYGO_UPDATER_PRIVATE_KEY."
    MYGO_UPDATER_PRIVATE_KEY=$(tr -d '\015\012' < "$key")
    export MYGO_UPDATER_PRIVATE_KEY
}

# native_path <path> converts a Git Bash path for a native Windows program.
native_path() {
    if command -v cygpath >/dev/null 2>&1; then
        cygpath -w "$1"
    else
        printf '%s\n' "$1"
    fi
}
