# 18. Telemetry

Status: Accepted (#30). Amended by [19](0019-project-settings.md),
[22](0022-setting-completion-up.md) and [48](0048-restore-after-reboot.md), which add commands to
`setup`.

## Context

The JetBrains OpenTelemetry plugin (2.1.5 in GoLand 2026.2.3) shows claude's spans per agent: where
a long multi-agent run spends its time. It takes OTLP over gRPC only. A team's collector wants
claude's metrics too. claude exports each signal to one endpoint, read as it starts (claude 2.1.282;
[claude's findings](../findings/claude.md)). The collector was probed with Docker 29.6.0 and its
image 0.161.0 ([the environment's findings](../findings/environment.md)).

## Decision

`cld setup telemetry` runs an OpenTelemetry Collector in Docker and points claude's user settings at
it. The collector sends traces, metrics and logs to `--local`, and metrics only to `--remote`. Each
exporter queues on its own, so the remote one keeps getting metrics while the IDE is closed. The
local one drops data after 30 s rather than retrying for 5 min. A mistake in a URL is a usage error.

### 18.1 Linux only

Host networking makes `127.0.0.1` in a URL the host itself. Elsewhere cld refuses. Not taken:
rewriting loopback URLs on macOS, which waits for a probe on Docker Desktop.

### 18.2 The port

The port is `--port`, or else the running or paused collector's, so that running claude sessions
keep sending to it. Failing those, the stopped collector's port if nothing holds it, or one the
kernel picks. Not 4317, which another collector or Jaeger is likely to hold. Never a port that a
`--local` or `--remote` URL reaches the receiver on: the collector would send to itself without end.
Not taken: the first free port from 14317 upward.

### 18.3 Extra configuration

Extra configuration is `--collector-config FILE` only, merged over cld's config, whose parts have
fixed names to refer to. Not taken: a `--remote-header` option.

### 18.4 The config in variables

Both configs reach the container as environment variables. They go to `docker` in its environment,
not on a command line every user of the host can list. The container needs no file of the host, and
the collector checks exactly what will run. A file longer than Linux takes a variable (131051 bytes
with 4 KiB pages) is refused. Not taken: a bind-mounted file.

### 18.5 The order of the steps

Everything is read and checked before the container is replaced, so a mistake changes nothing.
`--restart unless-stopped` brings the collector back after a reboot, where claude still sends to it,
and keeps a `docker stop` stopped. cld waits up to 10 s for the receiver's port to take connections.
A `--collector-config` may silence the ready line or move the receiver, so the port tells what the
log cannot. Not taken: forcing the log level with `--set`, which would override a `debug`. A
collector that fails leaves the settings alone, and cld shows its log. It turns off the restarts of
one that stopped, which Docker would repeat at every boot. The settings are read again once the
collector is ready, keeping what was written meanwhile.

### 18.6 claude's settings

cld sets the keys in `env` that send claude's signals to the collector. Traces, logs and tool
details go on only with `--local`, since tool details carry Bash commands and MCP names. cld removes
each signal's own endpoint, which would bypass the collector, and leaves every other key alone.

### 18.7 Where the config strays from #30

The exporters are `otlp_grpc`, since `otlp` is a deprecated alias in 0.161.0. The collector's own
metrics are off: with host networking they take port 8888, and a second collector there exits.

### 18.8 `setup` has commands of its own

cld checks the argument after `setup` before cobra, which would run `telemetry` for
`setup --local URL telemetry`. `help setup telemetry` works as cobra's help does
([decision 12.2](0012-help-from-cobra.md)). `project` ([decision 19](0019-project-settings.md)),
`completion` ([decision 22](0022-setting-completion-up.md)) and `restore`
([decision 48](0048-restore-after-reboot.md)) joined it later.

### 18.9 Completion

The checks for Linux and `docker` stay in the command itself, so completion runs no `docker`
([decision 17.4](0017-shell-completion.md)). Nothing completes `FILE`, the one argument of cld's
that is a file ([decision 17.3](0017-shell-completion.md)), a gap left for later.

### 18.10 The image is pinned

`otel/opentelemetry-collector:0.161.0` is pinned and bumped deliberately, as JediTerm is
([decision 7](0007-jediterm-pin.md)).

## Consequences

- Running setup again gives the same result, and keeps the port that running sessions send to.
- Out of scope: other systems, a header option, turning telemetry off, another image, and the
  plugin's port, which the IDE sets.
