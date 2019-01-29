FROM  quay.io/prometheus/busybox:latest
LABEL maintainer="The Prometheus Authors <prometheus-developers@googlegroups.com>"

COPY statsd_exporter /bin/statsd_exporter

EXPOSE      9102 9125 9125/udp
HEALTHCHECK CMD curl --fail http://localhost:9102/metrics || exit 1
ENTRYPOINT  [ "/bin/statsd_exporter" ]
