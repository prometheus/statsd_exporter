# statsd exporter [![Build Status](https://travis-ci.org/prometheus/statsd_exporter.svg)][travis]

[![CircleCI](https://circleci.com/gh/prometheus/statsd_exporter/tree/master.svg?style=shield)][circleci]
[![Docker Repository on Quay](https://quay.io/repository/prometheus/statsd-exporter/status)][quay]
[![Docker Pulls](https://img.shields.io/docker/pulls/prom/statsd-exporter.svg)][hub]

`statsd_exporter` receives StatsD-style metrics and exports them as Prometheus metrics.

## Overview

### With StatsD

To pipe metrics from an existing StatsD environment into Prometheus, configure
StatsD's repeater backend to repeat all received metrics to a `statsd_exporter`
process. This exporter translates StatsD metrics to Prometheus metrics via
configured mapping rules.

    +----------+                         +-------------------+                        +--------------+
    |  StatsD  |---(UDP/TCP repeater)--->|  statsd_exporter  |<---(scrape /metrics)---|  Prometheus  |
    +----------+                         +-------------------+                        +--------------+

### Without StatsD

Since the StatsD exporter uses the same line protocol as StatsD itself, you can
also configure your applications to send StatsD metrics directly to the exporter.
In that case, you don't need to run a StatsD server anymore.

We recommend this only as an intermediate solution and recommend switching to
[native Prometheus instrumentation](http://prometheus.io/docs/instrumenting/clientlibs/)
in the long term.

### Tagging Extensions

The exporter supports Librato, InfluxDB, and DogStatsD-style tags,
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

Be aware: If you mix tag styles (e.g., Librato/InfluxDB with DogStatsD), the
exporter will consider this an error and the sample will be discarded. Also,
tags without values (`#some_tag`) are not supported and will be ignored.

## Building and Running

NOTE: Version 0.7.0 switched to the [kingpin](https://github.com/alecthomas/kingpin) flags library. With this change, flag behaviour is POSIX-ish:

* long flags start with two dashes (`--version`)
* multiple short flags can be combined (but there currently is only one)
* flag processing stops at the first `--`

    ```
    $ go build
    $ ./statsd_exporter --help
    usage: statsd_exporter [<flags>]

    Flags:
      -h, --help                    Show context-sensitive help (also try --help-long and --help-man).
          --web.listen-address=":9102"
                                    The address on which to expose the web interface and generated Prometheus     metrics.
          --web.telemetry-path="/metrics"
                                    Path under which to expose metrics.
          --statsd.listen-udp=":9125"
                                    The UDP address on which to receive statsd metric lines. "" disables it.
          --statsd.listen-tcp=":9125"
                                    The TCP address on which to receive statsd metric lines. "" disables it.
          --statsd.listen-unixgram=""
                                    The Unixgram socket path to receive statsd metric lines in datagram. ""     disables it.
          --statsd.unixsocket-mode="755"
                                    The permission mode of the unix socket.
          --statsd.mapping-config=STATSD.MAPPING-CONFIG
                                    Metric mapping configuration file name.
          --statsd.read-buffer=STATSD.READ-BUFFER
                                    Size (in bytes) of the operating system's transmit read buffer associated     with the UDP or Unixgram connection. Please make sure the kernel     parameters net.core.rmem_max is set to
                                    a value greater than the value specified.
          --statsd.cache-size=1000  Maximum size of your metric mapping cache. Relies on least recently used     replacement policy if max size is reached.
          --statsd.event-queue-size=10000
                                    Size of internal queue for processing events
          --statsd.event-flush-threshold=1000
                                    Number of events to hold in queue before flushing
          --statsd.event-flush-interval=200ms
                                    Number of events to hold in queue before flushing
          --debug.dump-fsm=""       The path to dump internal FSM generated for glob matching as Dot file.
          --log.level="info"        Only log messages with the given severity or above. Valid levels: [debug,     info, warn, error, fatal]
          --log.format="logger:stderr"
                                    Set the log target and format. Example: "logger:syslog?appname=bob&    local=7" or "logger:stdout?json=true"
          --version                 Show application version.

    ```

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

    StatsD timer   -> Prometheus summary                    <-- indicates timer quantiles
                   -> Prometheus counter (suffix `_total`)  <-- indicates total time spent
                   -> Prometheus counter (suffix `_count`)  <-- indicates total number of timer events

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

The metric name can also contain references to regex matches. The mapping above
could be written as:

```yaml
mappings:
- match: "test\\.(\\w+)\\.(\\w+)\\.counter"
  match_type: regex
  name: "${2}_total"
  labels:
    provider: "$1"
```

Be aware about yaml escape rules as a mapping like the following one will not work. 
```yaml 
mappings:
- match: "test\.(\w+)\.(\w+)\.counter"
  match_type: regex
  name: "${2}_total"
  labels:
    provider: "$1"
```

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

### StatsD timers

By default, statsd timers are represented as a Prometheus summary with
quantiles. You may optionally configure the [quantiles and acceptable
error](https://prometheus.io/docs/practices/histograms/#quantiles), as
well as adjusting how the summary metric is aggregated:

```yaml
mappings:
- match: "test.timing.*.*.*"
  timer_type: summary
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
    max_summary_age: 30s
    summary_age_buckets: 3
    stream_buffer_size: 1000
```

The default quantiles are 0.99, 0.9, and 0.5.

The default summary age is 10 minutes, the default number of buckets
is 5 and the default buffer size is 500.  See also the
[`golang_client` docs](https://godoc.org/github.com/prometheus/client_golang/prometheus#SummaryOpts).
The `max_summary_age` corresponds to `SummaryOptions.MaxAge`, `summary_age_buckets`
to `SummaryOptions.AgeBuckets` and `stream_buffer_size` to `SummaryOptions.BufCap`.

In the configuration, one may also set the timer type to "histogram". The
default is "summary" as in the plain text configuration format.  For example,
to set the timer type for a single metric:

```yaml
mappings:
- match: "test.timing.*.*.*"
  timer_type: histogram
  histogram_options:
    buckets: [ 0.01, 0.025, 0.05, 0.1 ]
  name: "my_timer"
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server"
```

Note that timers will be accepted with the `ms`, `h`, and `d` statsd types.  The first two are timers and histograms and the `d` type is for DataDog's "distribution" type.  The distribution type is treated identically to timers and histograms.

It should be noted that whereas timers in statsd expects the unit of timing data to be in milliseconds,
prometheus expects the unit to be seconds. Hence, the exporter converts all timers to seconds
before exporting them.

### DogStatsD Client Behavior

#### `timed()` decorator

If you are using the DogStatsD client's [timed](https://datadogpy.readthedocs.io/en/latest/#datadog.threadstats.base.ThreadStats.timed) decorator,
it emits the metric in seconds, set [use_ms](https://datadogpy.readthedocs.io/en/latest/index.html?highlight=use_ms) to `True` to fix this.

### Regular expression matching

Another capability when using YAML configuration is the ability to define matches
using raw regular expressions as opposed to the default globbing style of match.
This may allow for pulling structured data from otherwise poorly named statsd
metrics AND allow for more precise targetting of match rules. When no `match_type`
paramter is specified the default value of `glob` will be assumed:

```yaml
mappings:
- match: "(.*)\.(.*)--(.*)\.status\.(.*)\.count"
  match_type: regex
  name: "request_total"
  labels:
    hostname: "$1"
    exec: "$2"
    protocol: "$3"
    code: "$4"
```

Note, that one may also set the histogram buckets.  If not set, then the default
[Prometheus client values](https://godoc.org/github.com/prometheus/client_golang/prometheus#pkg-variables) are used: `[.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10]`. `+Inf` is added
automatically.

`timer_type` is only used when the statsd metric type is a timer. `buckets` is
only used when the statsd metric type is a timerand the `timer_type` is set to
"histogram."

### Global defaults

One may also set defaults for the timer type, buckets or quantiles, and match_type. These will be used
by all mappings that do not define these.

An option that can only be configured in `defaults` is `glob_disable_ordering`, which is `false` if omitted. By setting this to `true`, `glob` match type will not honor the occurance of rules in the mapping rules file and always treat `*` as lower priority than a general string.

```yaml
defaults:
  timer_type: histogram
  buckets: [.005, .01, .025, .05, .1, .25, .5, 1, 2.5 ]
  match_type: glob
  glob_disable_ordering: false
  ttl: 0 # metrics do not expire
mappings:
# This will be a histogram using the buckets set in `defaults`.
- match: "test.timing.*.*.*"
  name: "my_timer"
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server"
# This will be a summary timer.
- match: "other.timing.*.*.*"
  timer_type: summary
  name: "other_timer"
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server_other"
```

### Choosing between glob or regex match type

Despite from the missing flexibility of using regular expression in mapping and
formatting labels, `glob` matching is optimized to have better performance than
`regex` in certain use cases. In short, glob will have best performance if the
rules amount is not so less and captures (using of `*`) is not to much in a
single rule. Whether disabling ordering in glob or not won't have a noticable
effect on performance in general use cases. In edge cases like the below however,
disabling ordering will be beneficial:

    a.*.*.*.*
    a.b.*.*.*
    a.b.c.*.*
    a.b.c.d.*

The reason is that the list assignment of captures (using of `*`) is the most
expensive operation in glob. Honoring ordering will result in up to 10 list
assignments, while without ordering it will need only 4 at most.

For details, see [pkg/mapper/fsm/README.md](pkg/mapper/fsm/README.md).
Running `go test -bench .` in **pkg/mapper** directory will produce
a detailed comparison between the two match type.

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

Possible values for `match_metric_type` are `gauge`, `counter` and `timer`.

### Mapping cache size and cache replacement policy

There is a cache used to improve the performance of the metric mapping, that can greatly improvement performance.
The cache has a default maximum of 1000 unique statsd metric names -> prometheus metrics mappings that it can store.
This maximum can be adjust using the `statsd.cache-size` flag.

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


[travis]: https://travis-ci.org/prometheus/statsd_exporter
[circleci]: https://circleci.com/gh/prometheus/statsd_exporter
[quay]: https://quay.io/repository/prometheus/statsd-exporter
[hub]: https://hub.docker.com/r/prom/statsd-exporter/
