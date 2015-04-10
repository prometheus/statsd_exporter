StatsD-Bridge
=============

StatsD-Bridge receives StatsD-style metrics and exports them as Prometheus metrics.

## Overview

To pipe metrics from an existing StatsD environment into Prometheus, configure
StatsD's repeater backend to repeat all received packets to a StatsD-Bridge
process. This bridge translates StatsD metrics to Prometheus metrics via
configured mapping rules.

    +----------+                     +-----------------+                        +--------------+
    |  StatsD  |---(UDP repeater)--->|  StatsD-Bridge  |<---(scrape /metrics)---|  Prometheus  |
    +----------+                     +-----------------+                        +--------------+

## Building and Running

    $ go build
    $ ./statsd_bridge --help
    Usage of ./statsd_bridge:
      -statsd.listen-address=":9125": The UDP address on which to receive statsd metric lines.
      -statsd.mapping-config="": Metric mapping configuration file name.
      -web.listen-address=":9102": The address on which to expose the web interface and generated Prometheus metrics.
      -web.telemetry-path="/metrics": Path under which to expose metrics.

## Tests

    $ go test

## Metric Mapping and Configuration

The StatsD-Bridge can be configured to translate specific dot-separated StatsD
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

In general, the different metric types are translated as follows, with certain
suffixes appended to the Prometheus metric names:

    StatsD gauge   -> Prometheus gauge (suffix `_gauge`)
    
    StatsD counter -> Prometheus counter (suffix `_counter`)
    
    StatsD timer   -> Prometheus summary (suffix `_timer`)        <-- indicates timer quantiles
                   -> Prometheus counter (suffix `_timer_total`)  <-- indicates total time spent
                   -> Prometheus counter (suffix `_timer_count`)  <-- indicates total number of timer events

An example mapping configuration:

    test.dispatcher.*.*.*
    name="dispatcher_events"
    processor="$1"
    action="$2"
    outcome="$3"
    job="test_dispatcher"
    
    *.signup.*.*
    name="signup_events"
    provider="$2"
    outcome="$3"
    job="$1_server"
    
This would transform these example StatsD metrics into Prometheus metrics as
follows:

    test.dispatcher.FooProcessor.send.success (counter)
     => dispatcher_events_counter{processor="FooProcessor", action="send", outcome="success", job="test_dispatcher"}
    
    foo_product.signup.facebook.failure (counter)
     => signup_events_counter{provider="facebook", outcome="failure", job="foo_product_server"}
     
    test.web-server.foo.bar (gauge)
     => test_web__server_foo_bar_gauge{}

## Docker

```
docker run -d --name="statsd_bridge" \
  -p 9102:9102 \
  -p 9125:9125 \
  -v /path/on/host/to/mapping.conf:/mapping.conf \
  statsd_bridge -statsd.mapping-config="/mapping.conf"
```
