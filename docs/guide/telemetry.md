# Telemetry

- `--local` gets spans for model requests, tool and MCP calls and hooks, per agent. They name the
  commands claude runs, and go to the local endpoint alone.
- For the JetBrains OpenTelemetry plugin, turn on "Use fixed OTLP server port" in Settings ›
  OpenTelemetry › Common.
- The plugin may set `OTEL_*` variables in a GoLand terminal. Whether a claude started there follows
  them or cld's settings is not checked yet.
- `--remote` keeps getting metrics while the IDE is closed; the local endpoint's data is dropped
  after 30 s.
- The collector keeps its port across runs, so that running sessions keep reaching it.
- cld writes the settings once the new collector takes connections. Where the collector stops
  first, its container stays for `docker logs cld-telemetry`.
- Docker starts the collector again after a reboot. To turn it off, run
  `docker rm -f cld-telemetry`, and remove the keys cld set from the `env` of claude's settings.
- cld sets `CLAUDE_CODE_ENABLE_TELEMETRY`, `OTEL_METRICS_EXPORTER`, `OTEL_EXPORTER_OTLP_PROTOCOL`
  and `OTEL_EXPORTER_OTLP_ENDPOINT`. With `--local` it also sets `OTEL_TRACES_EXPORTER`,
  `OTEL_LOGS_EXPORTER`, `CLAUDE_CODE_ENHANCED_TELEMETRY_BETA` and `OTEL_LOG_TOOL_DETAILS`. It
  removes the per-signal `OTEL_EXPORTER_OTLP_*_ENDPOINT` and `_PROTOCOL` keys.

## Your collector config

`--collector-config FILE` merges maps and replaces lists: to add an exporter to a pipeline, repeat
its whole `exporters` list. cld names the receiver `otlp`, the exporters `otlp_grpc/local` and
`otlp_grpc/remote`, and the pipelines `traces`, `metrics` and `logs`. An auth header, say:

```yaml
exporters:
  otlp_grpc/remote:
    headers:
      authorization: Bearer <token>
    compression: gzip
```

Edits apply at the next run. The container gets a copy, which `docker inspect` shows, secrets
included.
