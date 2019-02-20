# Logging Verbosity

Honeydipper uses stdout and stderr for logging. The stdout is used for all levels of logs, while stderr is used for reporting warning or more critical messages. The daemon and each driver can be configured individually on logging verbosity. Just put the verbosity level in `drivers.<driver name>.loglevel`. Use `daemon` as driver name for daemon logging.

For example:

```yaml
---
drivers:
  daemon:
    loglevel: INFO
  web:
    loglevel: DEBUG
  webhook:
    loglevel: WARNING
```

The supported levels are, from most critical to least:

 * `CRITICAL`
 * `ERROR`
 * `WARNING`
 * `NOTICE`
 * `INFO`
 * `DEBUG`
