#!/bin/sh
set -e

BINPATH="${BINPATH:-.}"
SHELL="${SHELL:-/bin/sh}"

prog="$1"
if [ -n "$prog" ] && command -v "$prog" >/dev/null 2>&1; then
    shift
else
    prog="$BINPATH/honeydipper"
fi

while IFS='=' read -r -d '' k v; do
    case "$v" in
        "hd-secret-file://"*)
            f="/var/hd-secrets/${v:17}"
            f="$(realpath "$f")"
            ;;
        "docker-secret-file://"*)
            f="/run/secrets/${v:21}"
            f="$(realpath "$f")"
            ;;
        *)
            f=''
            ;;
    esac

    case "$f" in
        /var/hd-secrets/* | /run/secrets/*)
            if [ ! -f "$f" ]; then
                echo "secret file not found: $f" >&2
                exit 1
            fi
            eval "export $k=\"\$(cat '$f')\""
            ;;
        '')
            ;;
        *)
            echo "secret file path out of scope: $f" >&2
            exit 1
            ;;
    esac
done </proc/$$/environ

if [ -n "$HD_SECURE_LOADER" ]; then
    if command -v "$HD_SECURE_LOADER" >/dev/null 2>&1; then
        exec "$HD_SECURE_LOADER" exec -- "$prog" "$@"
    else
        echo "secure loader not found: $HD_SECURE_LOADER"
        exit 1
    fi
fi

exec "$prog" "$@"