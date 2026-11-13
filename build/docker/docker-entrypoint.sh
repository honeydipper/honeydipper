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

while IFS='=' read -r k v; do
    optional=
    case "$v" in
        "hd-secret-file://"*)
            name="$(echo "$v" | cut -c 18-)"
            case "$name" in
                '?'*)
                    optional=true
                    name="$(echo "$name" | cut -c 2-)"
                    ;;
            esac
            f="/var/hd-secrets/$name"
            f="$(realpath "$f")"
            ;;
        "docker-secret-file://"*)
            name="$(echo "$v" | cut -c 22-)"
            case "$name" in
                '?'*)
                    optional=true
                    name="$(echo "$name" | cut -c 2-)"
                    ;;
            esac
            f="/run/secrets/$name"
            f="$(realpath "$f")"
            ;;
        *)
            f=''
            ;;
    esac

    case "$f" in
        /var/hd-secrets/* | /run/secrets/*)
            if [ -f "$f" ]; then
                eval "export $k=\"\$(cat '$f')\""
            elif [ "$optional" != true ]; then
                echo "secret file not found: $f" >&2
                exit 1
            fi
            ;;
        '')
            ;;
        *)
            echo "secret file path out of scope: $f" >&2
            exit 1
            ;;
    esac
done <<EOF
$(env)
EOF

if [ -n "$HD_SECURE_LOADER" ]; then
    if command -v "$HD_SECURE_LOADER" >/dev/null 2>&1; then
        exec "$HD_SECURE_LOADER" exec -- "$prog" "$@"
    else
        echo "secure loader not found: $HD_SECURE_LOADER"
        exit 1
    fi
fi

exec "$prog" "$@"