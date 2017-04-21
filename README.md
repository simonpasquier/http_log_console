Monitor HTTP access logs in your terminal.

Build
=====

```
$ go get github.com/tools/godep
$ godep go test
$ godep go build
```

Run
===

```
$ ./http_log_console  -h
Usage of ./http_log_console:
  -f string
        HTTP log file to monitor
  -i int
        Interval at which statistics should be emitted (default 10)
  -t int
        Alarm threshold (default 100)
  -w int
        Alarm evaluation period (default 120)
$ ./http_log_console -f /var/log/nginx/access.log -i 10 -t 1000 -w 120
```

To-Do
=====

* The log processor relies on a fixed regexp pattern for parsing the logs. For
  greater flexibility, it should be split into 2 components: one for tailing
  the log file and another one for parsing the payload.
* The log processor should expose metrics about how many logs have been
  processed, broken down by success/failure.
* The pipeline is assembled "manually" in the main() function. We should
  implement a generic pipeline system that would allow an arbitrary number of
  workers consuming the same input stream (a typical example would be to run
  multiple alarm workers) and even chaining workers one after the other.
* The statistic and alarm workers return respectively slices of strings and
  strings. Instead they should return structures implementing the String() interface.
* The implementation of the alarm testing is quite crude and should be revised
  to be more deterministic (eg not rely on arbitrary timeouts).

License
=======

This work is released under the Apache 2.0 license.
