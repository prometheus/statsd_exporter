## 0.22.1 / 2021-09-01

* [ENHANCEMENT] Accept incoming metrics with multiple dashes (with mapping) ([#381](https://github.com/prometheus/statsd_exporter//pull/381))
* [ENHANCEMENT] Allow forwarding messages to statsd for easier transition ([#388](https://github.com/prometheus/statsd_exporter/pull/388))
* [BUGFIX] Actually expose pprof endpoints ([#386](https://github.com/prometheus/statsd_exporter/pull/386))
* [BUGFIX] Fix performance regression on metric ingestion ([#390](https://github.com/prometheus/statsd_exporter/pull/390))

## 0.21.0 / 2021-06-10

* [ENHANCEMENT] Update dependencies & switch to go-kit/log ([#379](https://github.com/prometheus/statsd_exporter/pull/379))

This release changes the log format to be more structured, in line with other Prometheus projects.

## 0.20.3 / 2021-06-04

* [ENHANCEMENT] Use extracted go-kit/log to reduce transitive dependencies ([#378](https://github.com/prometheus/statsd_exporter/pull/378))

Once again there is no functional change.
For library users, the dependency tree shrinks considerably.
See [prometheus/common#255](https://github.com/prometheus/common/issues/255) for more details.

## 0.20.2 / 2021-05-03

* [BUGFIX] Remove copyleft licensed dependency ([#375](https://github.com/prometheus/statsd_exporter/pull/375))

There is no functional change for exporter users.
Removing this dependency reduces uncertainty for anyone reusing the mapping code.

## 0.20.1 / 2021-03-26

* [CHANGE] [library] Split mapper caches out from mapper ([#363](https://github.com/prometheus/statsd_exporter/pull/363))
* [BUGFIX] Accept metric segments that start with numbers ([#365](https://github.com/prometheus/statsd_exporter/pull/365))

## 0.20.0 / 2021-02-05

* [ENHANCEMENT] Support full defaults for summaries and histograms ([#361](https://github.com/prometheus/statsd_exporter/pull/361))

This completes support for `summary_options` and `histogram_options`.
Change the legacy configuration attributes throughout the mapping configuration as follows:

* `quantiles: …` to `summary_options: { quantiles: … }`
* `buckets: …` to `histogram_options: { buckets: … }`
* `timer_type` to `observer_type`.

Support for the deprecated attributes will be removed in a future release.

## 0.19.1 / 2021-01-29

* [BUGFIX] Don't return empty responses to lifecycle api requests ([#360](https://github.com/prometheus/statsd_exporter/pull/360))

## 0.19.0 / 2021-01-22

* [CHANGE] [library] Require explicit Registerer ([#347](https://github.com/prometheus/statsd_exporter/pull/347))
* [ENHANCEMENT] Add /-/healthy and /-/ready endpoints ([#339](https://github.com/prometheus/statsd_exporter/pull/339))
* [BUGFIX] Do not open network ports when only checking config ([#357](https://github.com/prometheus/statsd_exporter/pull/357))

## 0.18.0 / 2020-08-21

* [ENHANCEMENT] Allow turning off tagging extensions ([#325](https://github.com/prometheus/statsd_exporter/pull/325))
* [ENHANCEMENT] Add a lifecycle API for configuration reloads and restarts ([#329](https://github.com/prometheus/statsd_exporter/pull/329))

This release changes the interface for the [`github.com/prometheus/statsd_exporter/pkg/line` library package](https://pkg.go.dev/github.com/prometheus/statsd_exporter@v0.18.0/pkg/line?tab=doc) to support the new configurability.

## 0.17.0 / 2020-06-26

* [CHANGE] Support non-timer distributions without unit conversion ([#314](https://github.com/prometheus/statsd_exporter/pull/314))
* [ENHANCEMENT] Offline configuration check ([#312](https://github.com/prometheus/statsd_exporter/pull/312))
* [ENHANCEMENT] Support the SignalFX tagging extension ([#315](https://github.com/prometheus/statsd_exporter/pull/315))
* [BUGFIX] Allow matching single-letter metric name components ([#309](https://github.com/prometheus/statsd_exporter/pull/309))

Distribution and histogram events (type `d`, `h`) are now treated as distinct from timer events (type `ms`).
Their values are observed as they are, while timer events are converted from milliseconds to seconds.

To reflect this generalization, the `observer_type` mapping option replaces `timer_type`.
Similary, change `match_metric_type: timer` to `match_metric_type: observer`.
The old name remains available for compatibility.

For users of the mapper library, the `ObserverEvent` replaces `TimerEvent`.
For timer metrics, it is emitted by the mapper already converted to seconds.

## 0.16.0 / 2020-05-29

* [CHANGE] Break out much of the exporter into reusable packages ([#298](https://github.com/prometheus/statsd_exporter/pull/298))
* [ENHANCEMENT] Log ingested lines at debug level ([#305](https://github.com/prometheus/statsd_exporter/pull/305))

This release mainly consists of an internal reorganization of the exporter.
This should not have any impact on users of the binary, if it does, please file
an issue.

For users of the existing library packages, nothing changes.

There are now multiple new packages available, exposing functionality that had
been locked away in the main package. Consider the interfaces of these
libraries preliminary; we will change them as we gain experience in how they
are used.

## 0.15.0 / 2020-03-05

* [ENHANCEMENT] Allow setting granularity for summary metrics ([#290](https://github.com/prometheus/statsd_exporter/pull/290))
* [ENHANCEMENT] Support a random-replacement cache invalidation strategy ([#281](https://github.com/prometheus/statsd_exporter/pull/281)

To facilitate the expanded settings for summaries, the configuration format changes from

```yaml
mappings:
- match: …
  timer_type: summary
  quantiles:
    - quantile: 0.99
      error: 0.001
    - quantile: 0.95
      error: 0.01
  …
```

to

```yaml
mappings:
- match: …
  timer_type: summary
  summary_options:
    quantiles:
      - quantile: 0.99
        error: 0.001
      - quantile: 0.95
        error: 0.01
      …
    max_summary_age: 30s
    summary_age_buckets: 3
    stream_buffer_size: 1000
  …
```

For consistency, the format for histogram buckets also changes from

```yaml
mappings:
- match: …
  timer_type: histogram
  buckets: [ 0.01, 0.025, 0.05, 0.1 ]
```

to

```yaml
mappings:
- match: …
  timer_type: histogram
  histogram_options:
    buckets: [ 0.01, 0.025, 0.05, 0.1 ]
```

Transitionally, the old format will still work but is *deprecated*. The new
settings are optional.

For users of the [mapper](https://pkg.go.dev/github.com/prometheus/statsd_exporter/pkg/mapper?tab=doc)
as a library, this is a breaking change. To adjust your code, replace
`mapping.Buckets` with `mapping.HistogramOptions.Buckets` and
`mapping.Quantiles` with `mapping.SummaryOptions.Quantiles`.

## 0.14.1 / 2020-01-13

* [BUGFIX] Mapper cache poisoning when name is variable ([#286](https://github.com/prometheus/statsd_exporter/pull/286))
* [BUGFIX] nil pointer dereference in UDP listener ([#287](https://github.com/prometheus/statsd_exporter/pull/287))

Thank you to everyone who reported these, and @bakins for the mapper cache fix!

## 0.14.0 / 2020-01-10

* [CHANGE] Switch logging to go-kit ([#283](https://github.com/prometheus/statsd_exporter/pull/283))
* [CHANGE] Rename existing metric for mapping cache size ([#284](https://github.com/prometheus/statsd_exporter/pull/284))
* [ENHANCEMENT] Add metrics for mapping cache hits ([#280](https://github.com/prometheus/statsd_exporter/pull/280))

Logs are more structured now. The `fatal` log level no longer exists; use `--log.level=error` instead. The valid log formats are `logfmt` and `json`.

The metric `statsd_exporter_cache_length` is now called `statsd_metric_mapper_cache_length`.

## 0.13.0 / 2019-12-06

* [ENHANCEMENT] Support sampling factors for all statsd metric types ([#264](https://github.com/prometheus/statsd_exporter/issues/250))
* [ENHANCEMENT] Support Librato and InfluxDB labeling formats ([#267](https://github.com/prometheus/statsd_exporter/pull/267))

## 0.12.2 / 2019-07-25

* [BUGFIX] Fix Unix socket handler ([#252](https://github.com/prometheus/statsd_exporter/pull/252))
* [BUGFIX] Fix panic under high load ([#253](https://github.com/prometheus/statsd_exporter/pull/253))

Thank you to everyone who reported and helped debug these issues!

## 0.12.1 / 2019-07-08

* [BUGFIX] Renew TTL when a metric receives updates ([#246](https://github.com/prometheus/statsd_exporter/pull/246))
* [CHANGE] Reload on SIGHUP instead of watching the file ([#243](https://github.com/prometheus/statsd_exporter/pull/243))

## 0.11.2 / 2019-06-14

* [BUGFIX] Fix TCP handler ([#235](https://github.com/prometheus/statsd_exporter/pull/235))

## 0.11.1 / 2019-06-14

* [ENHANCEMENT] Batch event processing for improved ingestion performance ([#227](https://github.com/prometheus/statsd_exporter/pull/227))
* [ENHANCEMENT] Switch Prometheus client to promhttp, freeing the standard HTTP metrics ([#233](https://github.com/prometheus/statsd_exporter/pull/233))

With #233, the exporter no longer exports metrics about its own HTTP status. These were not helpful since you could not get them when scraping fails. This allows mapping to metric names like `http_requests_total` that are useful as application metrics.

## 0.10.6 / 2019-06-07

* [BUGFIX] Fix mapping collision for metrics with different types, but the same name ([#229](https://github.com/prometheus/statsd_exporter/pull/229))

## 0.10.5 / 2019-05-27

* [BUGFIX] Fix "Error: inconsistent label cardinality: expected 0 label values but got N in prometheus.Labels" ([#224](https://github.com/prometheus/statsd_exporter/pull/224))

## 0.10.4 / 2019-05-20

* [BUGFIX] Revert #218 due to a race condition ([#221](https://github.com/prometheus/statsd_exporter/pull/221))

## 0.10.3 / 2019-05-17

* [ENHANCEMENT] Reduce allocations when escaping metric names ([#217](https://github.com/prometheus/statsd_exporter/pull/217))
* [ENHANCEMENT] Reduce allocations when handling packets ([#218](https://github.com/prometheus/statsd_exporter/pull/218))
* [ENHANCEMENT] Optimize label sorting ([#219](https://github.com/prometheus/statsd_exporter/pull/219))

This release is entirely powered by @claytono. Kudos!

## 0.10.2 / 2019-05-17

* [CHANGE] Do not run as root in the Docker container by default ([#202](https://github.com/prometheus/statsd_exporter/pull/202))
* [FEATURE] Add metric for count of events by action ([#193](https://github.com/prometheus/statsd_exporter/pull/193))
* [FEATURE] Add metric for count of distinct metric names ([#200](https://github.com/prometheus/statsd_exporter/pull/200))
* [FEATURE] Add UNIX socket listener support ([#199](https://github.com/prometheus/statsd_exporter/pull/199))
* [FEATURE] Accept Datadog [distributions](https://docs.datadoghq.com/graphing/metrics/distributions/) ([#211](https://github.com/prometheus/statsd_exporter/pull/211))
* [ENHANCEMENT] Add a health check to the Docker container ([#182](https://github.com/prometheus/statsd_exporter/pull/182))
* [ENHANCEMENT] Allow inconsistent label sets ([#194](https://github.com/prometheus/statsd_exporter/pull/194))
* [ENHANCEMENT] Speed up sanitization of metric names ([#197](https://github.com/prometheus/statsd_exporter/pull/197))
* [ENHANCEMENT] Enable pprof endpoints ([#205](https://github.com/prometheus/statsd_exporter/pull/205))
* [ENHANCEMENT] DogStatsD tag parsing is faster ([#210](https://github.com/prometheus/statsd_exporter/pull/210))
* [ENHANCEMENT] Cache mapped metrics ([#198](https://github.com/prometheus/statsd_exporter/pull/198))
* [BUGFIX] Fix panic if a mapping resulted in an empty name ([#192](https://github.com/prometheus/statsd_exporter/pull/192))
* [BUGFIX] Ensure that there are always default quantiles if using summaries ([#212](https://github.com/prometheus/statsd_exporter/pull/212))
* [BUGFIX] Prevent ingesting conflicting metric types that would make scraping fail ([#213](https://github.com/prometheus/statsd_exporter/pull/213))

With #192, the count of events rejected because of negative counter increments has moved into the `statsd_exporter_events_error_total` metric, instead of being lumped in with the different kinds of successful events.

## 0.9.0 / 2019-03-11

* [ENHANCEMENT] Update the Prometheus client library to 0.9.2 ([#171](https://github.com/prometheus/statsd_exporter/pull/171))
* [FEATURE] Metrics can now be expired with a per-mapping TTL ([#164](https://github.com/prometheus/statsd_exporter/pull/164))
* [CHANGE] Timers that mapped to a summary are scaled to seconds, just like histograms ([#178](https://github.com/prometheus/statsd_exporter/pull/178))

If you are using summaries, all your quantiles and `_total` will change by a factor of 1000.
Adjust your queries and dashboards, or consider switching to histograms altogether.

## 0.8.1 / 2018-12-05

* [BUGFIX] Expose the counter for unmapped matches ([#161](https://github.com/prometheus/statsd_exporter/pull/161))
* [BUGFIX] Unsuccessful backtracking does not clobber captures ([#169](https://github.com/prometheus/statsd_exporter/pull/169), fixes [#168](https://github.com/prometheus/statsd_exporter/issues/168))

## 0.8.0 / 2018-10-12

* [ENHANCEMENT] Speed up glob matching ([#157](https://github.com/prometheus/statsd_exporter/pull/157))

This release replaces the implementation of the glob matching mechanism,
speeding it up significantly. In certain sub-optimal configurations, a warning
is logged.

This major enhancement was contributed by [Wangchong Zhou](https://github.com/fffonion).

## 0.7.0 / 2018-08-22

This is a breaking release, but the migration is easy: command line flags now
require two dashes (`--help` instead of `-help`). The previous flag library
already accepts this form, so if necessary you can migrate the flags first
before upgrading.

The deprecated `--statsd.listen-address` flag has been removed, use
`--statsd.listen-udp` instead.

* [CHANGE] Switch to Kingpin for flags, fixes setting log level ([#141](https://github.com/prometheus/statsd_exporter/pull/141))
* [ENHANCEMENT] Allow matching on specific metric types ([#136](https://github.com/prometheus/statsd_exporter/pulls/136))
* [ENHANCEMENT] Summary quantiles can be configured ([#135](https://github.com/prometheus/statsd_exporter/pulls/135))
* [BUGFIX] Fix panic if an invalid regular expression is supplied ([#126](https://github.com/prometheus/statsd_exporter/pulls/126))

## 0.6.0 / 2018-01-17

* [ENHANCEMENT] Add a drop action ([#115](https://github.com/prometheus/statsd_exporter/pulls/115))
* [ENHANCEMENT] Allow templating metric names ([#117](https://github.com/prometheus/statsd_exporter/pulls/117))

## 0.5.0 / 2017-11-16

NOTE: This release breaks backward compatibility. `statsd_exporter` now uses
a YAML configuration file. You must convert your mappings configuration to
the new format when you upgrade. For example, the configuration

```
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
```

now has the format

```yaml
mappings:
- match: test.dispatcher.*.*.*
  help: "The total number of events handled by the dispatcher."
  name: "dispatcher_events_total"
  labels:
    processor: "$1"
    action: "$2"
    outcome: "$3"
    job: "test_dispatcher"
- match: *.signup.*.*
  name: "signup_events_total"
  help: "The total number of signup events."
  labels:
    provider: "$2"
    outcome: "$3"
    job: "${1}_server"
```

The help field is optional.

There is a [tool](https://github.com/bakins/statsd-exporter-convert) available to help with this conversion.

* [CHANGE] Replace the overloaded "packets" metric ([#106](https://github.com/prometheus/statsd_exporter/pulls/106))
* [CHANGE] Removed `-statsd.add-suffix` option flag [#99](https://github.com/prometheus/statsd_exporter/pulls/99). Users should remove
  this flag when upgrading.  Metrics will no longer automatically include the
  suffixes `_timer`  or `counter`. You may need to adjust any graphs that used
  metrics with these suffixes.
* [CHANGE] Reduce log levels [#92](https://github.com/prometheus/statsd_exporter/pulls/92). Many log events have been changed from error
  to debug log level.
* [CHANGE] Use YAML for configuration file [#66](https://github.com/prometheus/statsd_exporter/pulls/66). See note above about file format
  conversion.
* [ENHANCEMENT] Allow help text to be customized [#87](https://github.com/prometheus/statsd_exporter/pulls/87)
* [ENHANCEMENT] Add support for regex mappers [#85](https://github.com/prometheus/statsd_exporter/pulls/85)
* [ENHANCEMENT] Add TCP listener support [#71](https://github.com/prometheus/statsd_exporter/pulls/71)
* [ENHANCEMENT] Allow histograms for timer metrics [#66](https://github.com/prometheus/statsd_exporter/pulls/66)
* [ENHANCEMENT] Added support for sampling factor on timing events [#28](https://github.com/prometheus/statsd_exporter/pulls/28)
* [BUGFIX] Conflicting label sets no longer crash the exporter and will be
  ignored. Restart to clear the remembered label set. [#72](https://github.com/prometheus/statsd_exporter/pulls/72)

## 0.4.0 / 2017-05-12

* [ENHANCEMENT] Improve mapping configuration parser [#61](https://github.com/prometheus/statsd_exporter/pulls/61)
* [ENHANCEMENT] Add increment/decrement support to Gauges [#65](https://github.com/prometheus/statsd_exporter/pulls/65)
* [BUGFIX] Tolerate more forms of broken lines from StatsD [#48](https://github.com/prometheus/statsd_exporter/pulls/48)
* [BUGFIX] Skip metrics with invalid utf8 [#50](https://github.com/prometheus/statsd_exporter/pulls/50)
* [BUGFIX] ListenAndServe now fails on exit [#58](https://github.com/prometheus/statsd_exporter/pulls/58)

## 0.3.0 / 2016-05-05

* [CHANGE] Drop `_count` suffix for `loaded_mappings` metric ([#41](https://github.com/prometheus/statsd_exporter/pulls/41))
* [ENHANCEMENT] Use common's log and version packages, and add -version flag ([#44](https://github.com/prometheus/statsd_exporter/pulls/44))
* [ENHANCEMENT] Add flag to disable metric type suffixes ([#37](https://github.com/prometheus/statsd_exporter/pulls/37))
* [BUGFIX] Increase receivable UDP datagram size to 65535 bytes ([#36](https://github.com/prometheus/statsd_exporter/pulls/36))
* [BUGFIX] Warn, not panic when negative number counter is submitted ([#33](https://github.com/prometheus/statsd_exporter/pulls/33))

## 0.2.0 / 2016-03-19

NOTE: This release renames `statsd_bridge` to `statsd_exporter`

* [CHANGE] New Dockerfile using alpine-golang-make-onbuild base image ([#17](https://github.com/prometheus/statsd_exporter/pulls/17))
* [ENHANCEMENT] Allow configuration of UDP read buffer ([#22](https://github.com/prometheus/statsd_exporter/pulls/22))
* [BUGFIX] allow metrics with dashes when mapping ([#24](https://github.com/prometheus/statsd_exporter/pulls/24))
* [ENHANCEMENT] add root endpoint with redirect ([#25](https://github.com/prometheus/statsd_exporter/pulls/25))
* [CHANGE] rename bridge to exporter ([#26](https://github.com/prometheus/statsd_exporter/pulls/26))


## 0.1.0 / 2015-04-17

* Initial release
