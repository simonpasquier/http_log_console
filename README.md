Run it
======

```
$ go run main.go -f /var/log/apache2/access.log -i 10
```

To-Do
=====

* Split the log processor into 2 components: one for tailing the log file and
  another one for parsing the payload.
* Implement a generic pipeline system to have an arbitrary number of workers
  consuming the same input stream.
* Have workers return structures instead of plain strings.

License
=======

This work is released under the Apache 2.0 license.
