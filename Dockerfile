FROM  quay.io/prometheus/busybox:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY statsd_exporter /bin/statsd_exporter

USER        nobody
EXPOSE      9102 9125 9125/udp
HEALTHCHECK CMD wget --spider -S "http://localhost:9102/metrics" -T 60 2>&1 || exit 1
ENTRYPOINT  [ "/bin/statsd_exporter" ]
