ARG ARCH="amd64"
ARG OS="linux"
FROM quay.io/prometheus/busybox-${OS}-${ARCH}:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

ARG ARCH="amd64"
ARG OS="linux"
COPY .build/${OS}-${ARCH}/statsd_exporter /bin/statsd_exporter
COPY docker-entrypoint.sh /bin/docker-entrypoint.sh
RUN chmod +x /bin/docker-entrypoint.sh

ENV TELEMETRY_PATH=/metrics \
    LISTEN_PORT=9102

USER        65534
EXPOSE      9102 9125 9125/udp
HEALTHCHECK CMD if [ -r /tmp/statsd_exporter_healthcheck.env ]; then LISTEN_PORT="$(sed -n 's/^LISTEN_PORT=//p' /tmp/statsd_exporter_healthcheck.env | tail -n 1)"; TELEMETRY_PATH="$(sed -n 's/^TELEMETRY_PATH=//p' /tmp/statsd_exporter_healthcheck.env | tail -n 1)"; fi; wget --spider -S "http://localhost:${LISTEN_PORT:-9102}${TELEMETRY_PATH:-/metrics}" -T 60 2>&1 || exit 1
ENTRYPOINT  [ "/bin/docker-entrypoint.sh" ]
