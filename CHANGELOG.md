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
