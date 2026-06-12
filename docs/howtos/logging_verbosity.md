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

 * `CRITICAL` - Critical errors that may cause the daemon to crash
 * `ERROR` - Error messages indicating something went wrong
 * `WARNING` - Warning messages for potential issues
 * `NOTICE` - Normal but significant events
 * `INFO` - Informational messages about normal operation
 * `DEBUG` - Detailed debug information for troubleshooting

## Environment variable filtering

You can also control logging verbosity using the `DEBUG` environment variable. This provides a quick way to enable debug logging without modifying your configuration files.

To enable debug logging for all components:

```bash
DEBUG="*" honeydipper
```

To enable debug logging for a specific component:

```bash
DEBUG="daemon" honeydipper
```

To enable debug logging for multiple components:

```bash
DEBUG="daemon,web,webhook" honeydipper
```

The `DEBUG` environment variable supports wildcards. For example, to enable debug logging for all drivers starting with "web":

```bash
DEBUG="web*" honeydipper
```

Note: The `DEBUG` environment variable overrides the `loglevel` setting in the configuration file. If you specify `DEBUG="*"`, all components will log at DEBUG level regardless of their configured loglevel.
