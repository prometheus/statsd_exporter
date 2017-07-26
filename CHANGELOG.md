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
