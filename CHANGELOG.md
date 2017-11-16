## v0.5.0 / 2017-11-16

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

* [CHANGE] Replace the overloaded "packets" metric (#106)
* [CHANGE] Removed `-statsd.add-suffix` option flag #99. Users should remove
  this flag when upgrading.  Metrics will no longer automatically include the
  suffixes `_timer`  or `counter`. You may need to adjust any graphs that used
  metrics with these suffixes.
* [CHANGE] Reduce log levels #92. Many log events have been changed from error
  to debug log level.
* [CHANGE] Use YAML for configuration file #66. See note above about file format
  conversion.
* [IMPROVEMENT] Allow help text to be customized #87
* [IMPROVEMENT] Add support for regex mappers #85
* [IMPROVEMENT] Add TCP listener support #71
* [IMPROVEMENT] Allow histograms for timer metrics #66
* [IMPROVEMENT] Added support for sampling factor on timing events #28
* [BUGFIX] Conflicting label sets no longer crash the exporter and will be
  ignored. Restart to clear the remembered label set. #72

## v0.4.0 / 2017-05-12

* [IMPROVEMENT] Improve mapping configuration parser #61
* [IMPROVEMENT] Add increment/decrement support to Gauges #65
* [BUGFIX] Tolerate more forms of broken lines from StatsD #48
* [BUGFIX] Skip metrics with invalid utf8 #50
* [BUGFIX] ListenAndServe now fails on exit #58

## 0.3.0 / 2016-05-05

* [CHANGE] Drop `_count` suffix for `loaded_mappings` metric (#41)
* [IMPROVEMENT] Use common's log and version packages, and add -version flag (#44)
* [IMPROVEMENT] Add flag to disable metric type suffixes (#37)
* [BUGFIX] Increase receivable UDP datagram size to 65535 bytes (#36)
* [BUGFIX] Warn, not panic when negative number counter is submitted (#33)

## 0.2.0 / 2016-03-19

NOTE: This release renames `statsd_bridge` to `statsd_exporter`

* [CHANGE] New Dockerfile using alpine-golang-make-onbuild base image (#17)
* [IMPROVEMENT] Allow configuration of UDP read buffer (#22)
* [BUGFIX] allow metrics with dashes when mapping (#24)
* [IMPROVEMENT] add root endpoint with redirect (#25)
* [CHANGE] rename bridge to exporter (#26)


## 0.1.0 / 2015-04-17

* Initial release
