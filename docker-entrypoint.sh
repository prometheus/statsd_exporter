#!/bin/sh

set -eu

HEALTHCHECK_ENV_FILE=/tmp/statsd_exporter_healthcheck.env
STATSD_EXPORTER_BIN=${STATSD_EXPORTER_BIN:-/bin/statsd_exporter}

telemetry_path="/metrics"
listen_port="9102"
listen_address=""
pending_flag=""

# Parse both formats --flag=value and --flag value forms without altering original args.
for arg in "$@"; do
    case "$pending_flag" in
        telemetry_path)
            telemetry_path="$arg"
            pending_flag=""
            continue
            ;;
        listen_address)
            listen_address="$arg"
            pending_flag=""
            continue
            ;;
    esac

    case "$arg" in
        --web.telemetry-path=*)
            telemetry_path=${arg#*=}
            ;;
        --web.telemetry-path)
            pending_flag="telemetry_path"
            ;;
        --web.listen-address=*)
            listen_address=${arg#*=}
            ;;
        --web.listen-address)
            pending_flag="listen_address"
            ;;
    esac
done

# Ensure telemetry path is non-empty and absolute for URL construction.
if [ -z "$telemetry_path" ]; then
    telemetry_path="/metrics"
elif [ "${telemetry_path#/}" = "$telemetry_path" ]; then
    telemetry_path="/$telemetry_path"
fi

# Derive port from the final :PORT segment (works for host:port and [::]:port).
if [ -n "$listen_address" ]; then
    port_candidate=${listen_address##*:}
    case "$port_candidate" in
        ''|*[!0-9]*)
            ;;
        *)
            if [ "$port_candidate" -ge 1 ] 2>/dev/null && [ "$port_candidate" -le 65535 ] 2>/dev/null; then
                listen_port="$port_candidate"
            fi
            ;;
    esac
fi

export TELEMETRY_PATH="$telemetry_path"
export LISTEN_PORT="$listen_port"

# Persist parsed values so Docker healthcheck invocations can read them later.
tmp_env_file="${HEALTHCHECK_ENV_FILE}.tmp"
{
    printf 'TELEMETRY_PATH=%s\n' "$TELEMETRY_PATH"
    printf 'LISTEN_PORT=%s\n' "$LISTEN_PORT"
} >"$tmp_env_file"
mv "$tmp_env_file" "$HEALTHCHECK_ENV_FILE"

exec "$STATSD_EXPORTER_BIN" "$@"
