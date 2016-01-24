# statsd exporter [![Build Status](https://travis-ci.org/prometheus/statsd_exporter.svg)][travis]

[![CircleCI](https://circleci.com/gh/prometheus/statsd_exporter/tree/master.svg?style=shield)][circleci]
[![Docker Repository on Quay](https://quay.io/repository/prometheus/statsd-exporter/status)][quay]
[![Docker Pulls](https://img.shields.io/docker/pulls/prom/statsd-exporter.svg)][hub]

`statsd_exporter` receives StatsD-style metrics and exports them as Prometheus metrics.

## Overview

### With StatsD

To pipe metrics from an existing StatsD environment into Prometheus, configure
StatsD's repeater backend to repeat all received packets to a `statsd_exporter`
process. This exporter translates StatsD metrics to Prometheus metrics via
configured mapping rules.

    +----------+                     +-------------------+                        +--------------+
    |  StatsD  |---(UDP repeater)--->|  statsd_exporter  |<---(scrape /metrics)---|  Prometheus  |
    +----------+                     +-------------------+                        +--------------+

### Without StatsD

Since the StatsD exporter uses the same UDP protocol as StatsD itself, you can
also configure your applications to send StatsD metrics directly to the exporter.
In that case, you don't need to run a StatsD server anymore.

We recommend this only as an intermediate solution and recommend switching to
[native Prometheus instrumentation](http://prometheus.io/docs/instrumenting/clientlibs/)
in the long term.

### DogStatsD extensions

The exporter will convert DogStatsD-style tags to prometheus labels. See
[Tags](http://docs.datadoghq.com/guides/dogstatsd/#tags) in the DogStatsD
documentation for the concept description and
[Datagram Format](http://docs.datadoghq.com/guides/dogstatsd/#datagram-format)
for specifics. It boils down to appending
`|#tag:value,another_tag:another_value` to the normal StatsD format.  Tags
without values (`#some_tag`) are not supported.

## Building and Running

    $ go build
    $ ./statsd_exporter --help
    Usage of ./statsd_exporter:
      -statsd.listen-address=":9125": The UDP address on which to receive statsd metric lines.
      -statsd.mapping-config="": Metric mapping configuration file name.
      -web.listen-address=":9102": The address on which to expose the web interface and generated Prometheus metrics.
      -web.telemetry-path="/metrics": Path under which to expose metrics.

## Tests

    $ go test

## Metric Mapping and Configuration

The `statsd_exporter` can be configured to translate specific dot-separated StatsD
metrics into labeled Prometheus metrics via a simple mapping language. A
mapping definition starts with a line matching the StatsD metric in question,
with `*`s acting as wildcards for each dot-separated metric component. The
lines following the matching expression must contain one `label="value"` pair
each, and at least define the metric name (label name `name`). The Prometheus
metric is then constructed from these labels. `$n`-style references in the
label value are replaced by the n-th wildcard match in the matching line,
starting at 1. Multiple matching definitions are separated by one or more empty
lines. The first mapping rule that matches a StatsD metric wins.

Metrics that don't match any mapping in the configuration file are translated
into Prometheus metrics without any labels and with certain characters escaped
(`_` -> `__`; `-` -> `__`; `.` -> `_`).

In general, the different metric types are translated as follows:

    StatsD gauge   -> Prometheus gauge

    StatsD counter -> Prometheus counter

    StatsD timer   -> Prometheus summary                    <-- indicates timer quantiles
                   -> Prometheus counter (suffix `_total`)  <-- indicates total time spent
                   -> Prometheus counter (suffix `_count`)  <-- indicates total number of timer events

If `-statsd.add-suffix` is set, the exporter appends the metric type (`_gauge`,
`_counter`, `_timer`) to the resulting metrics. This is enabled by default for
backward compatibility but discouraged to use. Instead, it is better to
explicitly define the full metric name in your mapping and run the exporter
with `-statsd.add-suffix=false`.

An example mapping configuration with `-statsd.add-suffix=false`:

    test.dispatcher.*.*.*
    name="dispatcher_events_total"
    processor="$1"
    action="$2"
    outcome="$3"
    job="test_dispatcher"

    *.signup.*.*
    name="signup_events_total"
    provider="$2"
    outcome="$3"
    job="${1}_server"

This would transform these example StatsD metrics into Prometheus metrics as
follows:

    test.dispatcher.FooProcessor.send.success
     => dispatcher_events_total{processor="FooProcessor", action="send", outcome="success", job="test_dispatcher"}

    foo_product.signup.facebook.failure
     => signup_events_total{provider="facebook", outcome="failure", job="foo_product_server"}

    test.web-server.foo.bar
     => test_web__server_foo_bar{}

## Using Docker

You can deploy this exporter using the [prom/statsd-exporter](https://registry.hub.docker.com/u/prom/statsd-exporter/) Docker image.

For example:

```bash
docker pull prom/statsd-exporter

docker run -d -p 9102:9102 -p 9125:9125/udp \
        -v $PWD/statsd_mapping.conf:/tmp/statsd_mapping.conf \
        prom/statsd-exporter -statsd.mapping-config=/tmp/statsd_mapping.conf
```


[travis]: https://travis-ci.org/prometheus/statsd_exporter
[circleci]: https://circleci.com/gh/prometheus/statsd_exporter
[quay]: https://quay.io/repository/prometheus/statsd-exporter
[hub]: https://hub.docker.com/r/prom/statsd-exporter/
