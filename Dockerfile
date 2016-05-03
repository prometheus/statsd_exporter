FROM        quay.io/prometheus/busybox:latest
MAINTAINER  The Prometheus Authors <prometheus-developers@googlegroups.com>

COPY statsd_exporter /bin/statsd_exporter

EXPOSE      9102 9125/udp
ENTRYPOINT  [ "/bin/statsd_exporter" ]
