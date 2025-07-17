# statsd exporter [![Build Status](https://travis-ci.org/prometheus/statsd_exporter.svg)][travis]

[![CircleCI](https://circleci.com/gh/prometheus/statsd_exporter/tree/master.svg?style=shield)][circleci]
[![Docker Repository on Quay](https://quay.io/repository/prometheus/statsd-exporter/status)][quay]
[![Docker Pulls](https://img.shields.io/docker/pulls/prom/statsd-exporter.svg)][hub]

`statsd_exporter` receives StatsD-style metrics and exports them as Prometheus metrics.

## Overview

The StatsD exporter is a drop-in replacement for StatsD.
This exporter translates StatsD metrics to Prometheus metrics via configured mapping rules.

We recommend using the exporter only as an intermediate solution, and switching to [native Prometheus instrumentation](http://prometheus.io/docs/instrumenting/clientlibs/) in the long term.
While it is common to run centralized StatsD servers, the exporter works best as a [sidecar](https://docs.microsoft.com/en-us/azure/architecture/patterns/sidecar).

### Transitioning from an existing StatsD setup

The relay feature allows for a gradual transition.

Introduce the exporter by adding it as a sidecar alongside the application instances.
In Kubernetes, this means adding it to the [pod](https://kubernetes.io/docs/concepts/workloads/pods/).
Use the `--statsd.relay.address` to forward metrics to your existing StatsD UDP endpoint.
Relaying forwards statsd events unmodified, preserving the original metric name and tags in any format.

    +-------------+    +----------+                  +------------+
    | Application +--->| Exporter +----------------->|  StatsD    |
    +-------------+    +----------+                  +------------+
                              ^
                              |                      +------------+
                              +----------------------+ Prometheus |
                                                     +------------+

### Relaying from StatsD

To pipe metrics from an existing StatsD environment into Prometheus, configure StatsD's repeater backend to repeat all received metrics to a `statsd_exporter` process.

    +----------+                         +-------------------+                        +--------------+
    |  StatsD  |---(UDP/TCP repeater)--->|  statsd_exporter  |<---(scrape /metrics)---|  Prometheus  |
    +----------+                         +-------------------+                        +--------------+

This allows trying out the exporter with minimal effort, but does not provide the per-instance metrics of the sidecar pattern.

### Tagging Extensions

The exporter supports Librato, InfluxDB, DogStatsD, and SignalFX-style tags,
which will be converted into Prometheus labels.

For Librato-style tags, they must be appended to the metric name with a
delimiting `#`, as so:

```
metric.name#tagName=val,tag2Name=val2:0|c
```

See the [statsd-librato-backend README](https://github.com/librato/statsd-librato-backend#tags)
for a more complete description.

For InfluxDB-style tags, they must be appended to the metric name with a
delimiting comma, as so:

```
metric.name,tagName=val,tag2Name=val2:0|c
```

See [this InfluxDB blog post](https://www.influxdata.com/blog/getting-started-with-sending-statsd-metrics-to-telegraf-influxdb/#introducing-influx-statsd)
for a larger overview.


For DogStatsD-style tags, they're appended as a `|#` delimited section at the
end of the metric, as so:

```
metric.name:0|c|#tagName:val,tag2Name:val2
```

See [Tags](https://docs.datadoghq.com/developers/dogstatsd/data_types/#tagging)
in the DogStatsD documentation for the concept description and
[Datagram Format](https://docs.datadoghq.com/developers/dogstatsd/datagram_shell/).
If you encounter problems, note that this tagging style is incompatible with
the original `statsd` implementation.
The exporter also supports [DogStatD extended aggregations](https://github.com/prometheus/statsd_exporter/pull/558) in combination with DogStatsD tags, but not other tagging styles.

For [SignalFX dimension](https://github.com/signalfx/signalfx-agent/blob/main/docs/monitors/collectd-statsd.md#adding-dimensions-to-statsd-metrics), add the tags to the metric name in square brackets, as so:

```
metric.name[tagName=val,tag2Name=val2]:0|c
```

Be aware: If you mix tag styles (e.g., Librato/InfluxDB with DogStatsD), the exporter will consider this an error and the behavior is undefined.
Also, tags without values (`#some_tag`) are not supported and will be ignored.

The exporter parses all tagging formats by default, but individual tagging formats can be disabled with command line flags:
```
--no-statsd.parse-dogstatsd-tags
--no-statsd.parse-influxdb-tags
--no-statsd.parse-librato-tags
--no-statsd.parse-signalfx-tags
```

By default, labels explicitly specified in configuration take precedence over labels from tags.
To set the label from the statsd event tag, use [`honor_labels`](#honor-labels).

## Building and Running

NOTE: Version 0.7.0 switched to the [kingpin](https://github.com/alecthomas/kingpin) flags library. With this change, flag behaviour is POSIX-ish:

* long flags start with two dashes (`--version`)
* boolean long flags are disabled by prefixing with no (`--flag-name` is true, `--no-flag-name` is false)
* multiple short flags can be combined (but there currently is only one)
* flag processing stops at the first `--`
* see `--help` for a full list of flags

## Lifecycle API

The `statsd_exporter` has an optional lifecycle API (disabled by default) that can be used to reload or quit the exporter 
by sending a `PUT` or `POST` request to the `/-/reload` or `/-/quit` endpoints.

## Relay

The `statsd_exporter` has an optional mode that will buffer and relay incoming statsd lines to a remote server. This is useful to "tee" the data when migrating to using the exporter. The relay will flush the buffer at least once per second to avoid delaying delivery of metrics.

## Tests

    $ go test

## Metric Mapping and Configuration

The `statsd_exporter` can be configured to translate specific dot-separated StatsD
metrics into labeled Prometheus metrics via a simple mapping language. The config
file is reloaded on SIGHUP.

A mapping definition starts with a line matching the StatsD metric in question,
with `*`s acting as wildcards for each dot-separated metric component. The
lines following the matching expression must contain one `label="value"` pair
each, and at least define the metric name (label name `name`). The Prometheus
metric is then constructed from these labels. `$n`-style references in the
label value are replaced by the n-th wildcard match in the matching line,
starting at 1. Multiple matching definitions are separated by one or more empty
lines. The first mapping rule that matches a StatsD metric wins.

Metrics that don't match any mapping in the configuration file are translated
into Prometheus metrics without any labels and with any non-alphanumeric
characters, including periods, translated into underscores.

In general, the different metric types are translated as follows:

    StatsD gauge   -> Prometheus gauge

    StatsD counter -> Prometheus counter

    StatsD timer, histogram, distribution   -> Prometheus summary or histogram

### Glob matching

The default (and fastest) `glob` mapping style uses `*` to denote parts of the statsd metric name that may vary.
These varying parts can then be referenced in the construction of the Prometheus metric name and labels.

An example mapping configuration:

```yaml
mappings:
- match: "test.dispatcher.*.*.*"
  name: "dispatcher_events_total"
  labels:
    processor: "$1"
    action: "$2"
    outcome: "$3"
    job: "test_dispatcher"
- match: "*.signup.*.*"
  name: "signup_events_total"
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server"
```

This would transform these example StatsD metrics into Prometheus metrics as
follows:

    test.dispatcher.FooProcessor.send.success
     => dispatcher_events_total{processor="FooProcessor", action="send", outcome="success", job="test_dispatcher"}

    foo_product.signup.facebook.failure
     => signup_events_total{provider="facebook", outcome="failure", job="foo_product_server"}

    test.web-server.foo.bar
     => test_web_server_foo_bar{}

Each mapping in the configuration file must define a `name` for the metric. The
metric's name can contain `$n`-style references to be replaced by the n-th
wildcard match in the matching line. That allows for dynamic rewrites, such as:

```yaml
mappings:
- match: "test.*.*.counter"
  name: "${2}_total"
  labels:
    provider: "$1"
```

Glob matching offers the best performance for common mappings.

#### Ordering glob rules

List more specific matches before wildcards, from left to right:

    a.b.c
    a.b.*
    a.*.d
    a.*.*

This avoids unexpected shadowing of later rules, and performance impact from backtracking.

Alternatively, you can disable mapping ordering altogether.
With unordered mapping, at each hierarchy level the most specific match wins.
This has the same effect as using the recommended ordering.

### Regular expression matching

The `regex` mapping style uses regular expressions to match the full statsd metric name.
Use it if the glob mapping is not flexible enough to pull structured data from the available statsd metric names.

Regular expression matching is significantly slower than glob mapping as all mappings must be tested in order.
Because of this, **regex mappings are only executed after all glob mappings**.
In other words, glob mappings take preference over regex matches, irrespective of the order in which they are specified.
Regular expression matches are always evaluated in order, and the first match wins.

The metric name can also contain references to regex matches. The mapping above
could be written as:

```yaml
mappings:
- match: "test\\.(\\w+)\\.(\\w+)\\.counter"
  match_type: regex
  name: "${2}_total"
  labels:
    provider: "$1"
- match: "(.*)\\.(.*)--(.*)\\.status\.(.*)\\.count"
  match_type: regex
  name: "request_total"
  labels:
    hostname: "$1"
    exec: "$2"
    protocol: "$3"
    code: "$4"
```

Be aware about yaml escape rules as a mapping like the following one will not work.
```yaml
mappings:
- match: "test\\.(\w+)\\.(\w+)\\.counter"
  match_type: regex
  name: "${2}_total"
  labels:
    provider: "$1"
```

#### Special match groups

When using regex, the match group `0` is the full match and can be used to attach labels to the metric.
Example:

```yaml
mappings:
- match: ".+"
  match_type: regex
  name: "$0"
  labels:
    statsd_metric_name: "$0"
```

If a metric `my.statsd_counter` is received, the metric name will **still** be mapped to `my_statsd_counter` (Prometheus compatible name).
But the metric will also have the label `statsd_metric_name` with the value `my.statsd_counter` (unchanged value).

Note: If you use the `match` like the example (i.e. `.+`), be aware that it will be a "catch-all" block. So it should come at the very end of the mapping list.

The same is not achievable with glob matching, for more details check [this issue](https://github.com/prometheus/statsd_exporter/issues/444).

### Naming, labels, and help

Please note that metrics with the same name must also have the same set of
label names.

If the default metric help text is insufficient for your needs you may use the YAML
configuration to specify a custom help text for each mapping:

```yaml
mappings:
- match: "http.request.*"
  help: "Total number of http requests"
  name: "http_requests_total"
  labels:
    code: "$1"
```

### Honor labels

By default, labels specified in the mapping configuration take precedence over tags in the statsd event.

To set the label value to the original tag value, if present, specify `honor_labels: true` in the mapping configuration.
In this case, the label specified in the mapping acts as a default.

### StatsD timers and distributions

By default, statsd timers and distributions (collectively "observers") are
represented as a Prometheus summary with quantiles. You may optionally
configure the [quantiles and acceptable
error](https://prometheus.io/docs/practices/histograms/#quantiles), as well
as adjusting how the summary metric is aggregated:

```yaml
mappings:
- match: "test.timing.*.*.*"
  observer_type: summary
  name: "my_timer"
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server"
  summary_options:
    quantiles:
      - quantile: 0.99
        error: 0.001
      - quantile: 0.95
        error: 0.01
      - quantile: 0.9
        error: 0.05
      - quantile: 0.5
        error: 0.005
    max_age: 30s
    age_buckets: 3
    buf_cap: 1000
```

The default quantiles are 0.99, 0.9, and 0.5.

The default summary age is 10 minutes, the default number of buckets
is 5 and the default buffer size is 500.
See also the [`golang_client` docs](https://godoc.org/github.com/prometheus/client_golang/prometheus#SummaryOpts).
The `max_summary_age` corresponds to `SummaryOptions.MaxAge`, `summary_age_buckets` to `SummaryOptions.AgeBuckets` and `stream_buffer_size` to `SummaryOptions.BufCap`.

In the configuration, one may also set the observer type to "histogram". For example,
to set the observer type for a single timer metric:

```yaml
mappings:
- match: "test.timing.*.*.*"
  observer_type: histogram
  histogram_options:
    buckets: [ 0.01, 0.025, 0.05, 0.1 ]
    native_histogram_bucket_factor: 1.1
    native_histogram_max_buckets: 256
  name: "my_timer"
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server"
```

If not set, then the default
[Prometheus client 
values](https://godoc.org/github.com/prometheus/client_golang/prometheus#pkg-variables) 
are used for the histogram buckets:
`[.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10]`.
`+Inf` is added automatically.
If your Prometheus server is enabled to scrape native histograms (v2.40.0+), 
then you can set the `native_histogram_bucket_factor` to configure precision of the
buckets in the sparse histogram. More about this in the original [client_golang docs](https://github.com/prometheus/client_golang/blob/449b46435075e6e069e05af920fe028b941033cf/prometheus/histogram.go#L399-L430).
Also, a configuration of the maximum number of buckets can be set with `native_histogram_max_buckets`, this
avoids the histograms to grow too large in memory. More about this in the original [client_golang docs](https://github.com/prometheus/client_golang/blob/449b46435075e6e069e05af920fe028b941033cf/prometheus/histogram.go#L443-L467).

`observer_type` is only used when the statsd metric type is a timer, histogram, or distribution.
`buckets` is only used when the statsd metric type is one of these, and the `observer_type` is set to `histogram`.

Timers will be accepted with the `ms` statsd type.
Statsd timer data is transmitted in milliseconds, while Prometheus expects the unit to be seconds.
The exporter converts all timer observations to seconds.

Histogram and distribution events (`h` and `d` metric type) are not subject to unit conversion.

### DogStatsD Client Behavior

#### `timed()` decorator

The DogStatsD client's [timed](https://datadogpy.readthedocs.io/en/latest/#datadog.threadstats.base.ThreadStats.timed) decorator emits the metric in seconds but uses the `ms` type.
Set [`use_ms=True`](https://datadogpy.readthedocs.io/en/latest/index.html?highlight=use_ms) to send the correct units.

### Regular expression matching

Another capability when using YAML configuration is the ability to define matches
using raw regular expressions as opposed to the default globbing style of match.
This may allow for pulling structured data from otherwise poorly named statsd
metrics AND allow for more precise targetting of match rules. When no `match_type`
parameter is specified the default value of `glob` will be assumed:

```yaml
mappings:
- match: "(.*)\\.(.*)--(.*)\\.status\\.(.*)\\.count"
  match_type: regex
  name: "request_total"
  labels:
    hostname: "$1"
    exec: "$2"
    protocol: "$3"
    code: "$4"
```

### Global defaults

One may also set defaults for the observer type, histogram options, summary options, and match type.
These will be used by all mappings that do not define them.

An option that can only be configured in `defaults` is `glob_disable_ordering`, which is `false` if omitted.
By setting this to `true`, `glob` match type will not honor the occurance of rules in the mapping rules file and always treat `*` as lower priority than a concrete string.

Setting `buckets` or `quantiles` in the defaults is deprecated in favor of `histogram_options` and `summary_options`, which will override the deprecated values.

If `summary_options` is present in a mapping config, it will only override the fields set in the mapping. Unset fields in the mapping will take the values from the defaults. 

See [`config.exmple.yml`](config.example.yml) for an annotated example configuration.

### `drop` action

You may also drop metrics by specifying a "drop" action on a match. For
example:

```yaml
mappings:
# This metric would match as normal.
- match: "test.timing.*.*.*"
  name: "my_timer"
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server"
# Any metric not matched will be dropped because "." matches all metrics.
- match: "."
  match_type: regex
  action: drop
  name: "dropped"
```

You can drop any metric using the normal match syntax.
The default action is "map" which does the normal metrics mapping.

### Explicit metric type mapping

StatsD allows emitting of different metric types under the same metric name,
but the Prometheus client library can't merge those. For this use-case the
mapping definition allows you to specify which metric type to match:

```
mappings:
- match: "test.foo.*"
  name: "test_foo"
  match_metric_type: counter
  labels:
    provider: "$1"
```

Possible values for `match_metric_type` are `gauge`, `counter` and `observer`.

### Mapping cache size and cache replacement policy

There is a cache used to improve the performance of the metric mapping, that can greatly improvement performance.
The cache has a default maximum of 1000 unique statsd metric names -> prometheus metrics mappings that it can store.
This maximum can be adjusted using the `statsd.cache-size` flag.

If the maximum is reached, entries are by default rotated using the [least recently used replacement policy](https://en.wikipedia.org/wiki/Cache_replacement_policies#Least_recently_used_(LRU)). This strategy is optimal when memory is constrained as only the most recent entries are retained.

Alternatively, you can choose a [random-replacement cache strategy](https://en.wikipedia.org/wiki/Cache_replacement_policies#Random_replacement_(RR)). This is less optimal if the cache is smaller than the cacheable set, but requires less locking. Use this for very high throughput, but make sure to allow for a cache that holds all metrics.

The optimal cache size is determined by the cardinality of the _incoming_ metrics.

### Time series expiration

The `ttl` parameter can be used to define the expiration time for stale metrics.
The value is a time duration with valid time units: "ns", "us" (or "µs"),
"ms", "s", "m", "h". For example, `ttl: 1m20s`. `0` value is used to indicate
metrics that do not expire.

 TTL configuration is stored for each mapped metric name/labels combination
 whenever new samples are received. This means that you cannot immediately
 expire a metric only by changing the mapping configuration. At least one
 sample must be received for updated mappings to take effect.

### Unit conversions

The `scale` parameter can be used to define unit conversions for metric values. The value is a floating point number to scale metric values by. This can be useful for converting non-base units (e.g. milliseconds, kilobytes) to base units (e.g. seconds, bytes) as recommended in [prometheus best practices](https://prometheus.io/docs/practices/naming/).

```yaml
mappings:
- match: foo.latency_ms
  name: foo_latency_seconds
  scale: 0.001
- match: bar.processed_kb
  name: bar_processed_bytes
  scale: 1024
- match: baz.latency_us
  name: baz_latency_seconds
  scale: 1e-6
```

### Event flushing configuration

 Internally `statsd_exporter` runs a goroutine for each network listener (UDP, TCP & Unix Socket).  These each receive and parse metrics received into an event.  For performance purposes, these events are queued internally and flushed to the main exporter goroutine periodically in batches.  The size of this queue and the flush criteria can be tuned with the `--statsd.event-queue-size`, `--statsd.event-flush-threshold` and `--statsd.event-flush-interval`.  However, the defaults should perform well even for very high traffic environments.

## Using Docker

You can deploy this exporter using the [prom/statsd-exporter](https://registry.hub.docker.com/r/prom/statsd-exporter) Docker image.

For example:

```bash
docker pull prom/statsd-exporter

docker run -d -p 9102:9102 -p 9125:9125 -p 9125:9125/udp \
        -v $PWD/statsd_mapping.yml:/tmp/statsd_mapping.yml \
        prom/statsd-exporter --statsd.mapping-config=/tmp/statsd_mapping.yml
```

## Library packages

Parts of the implementation of this exporter are available as separate packages.
See the [documentation](https://pkg.go.dev/github.com/prometheus/statsd_exporter/pkg) for details.

For the time being, there are *no stability guarantees* for library interfaces.
We will try to call out any significant changes in the [changelog](https://github.com/prometheus/statsd_exporter/blob/master/CHANGELOG.md).
Semantic versioning of the exporter is based on the impact on users of the exporter, not users of the library.

We encourage re-use of these packages and welcome [issues](https://github.com/prometheus/statsd_exporter/issues?q=is%3Aopen+is%3Aissue+label%3Alibrary) related to their usability as a library.

[circleci]: https://circleci.com/gh/prometheus/statsd_exporter
[quay]: https://quay.io/repository/prometheus/statsd-exporter
[hub]: https://hub.docker.com/r/prom/statsd-exporter/
