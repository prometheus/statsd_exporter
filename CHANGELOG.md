## v 0.5.0 / 2017-11-14

NOTE: This release breaks backward compatibility. `statsd_exporter` now uses
a YAML configuration file. You must convert your mappings configuration to
the new format when you upgrade.

* [CHANGE] Use YAML for configuration file
* [BUGFIX] Fix matching empty statsd metric components #105
* [CHANGE] Reduce log levels #92
* [CHANGE] Removed `-statsd.add-suffix` option flag #99
* [IMPROVEMENT] Allow help text to be customized #87
* [IMPROVEMENT] Add support for regex mappers #85
* [BUGFIX] Fixes for crashes with conflicting metric values #72
* [IMPROVEMENT] Add TCP listener support #71
* [IMPROVEMENT] Allow histograms for timer metrics #66
* [IMPROVEMENT] Added support for sampling factor on timing events #28

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
