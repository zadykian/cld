package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/zadykian/cld/internal/fail"
	"github.com/zadykian/cld/internal/telemetry"
)

// telemetryLong is setup telemetry's help.
const telemetryLong = `send claude's telemetry through a local OpenTelemetry collector: cld runs the
collector in Docker, as the container cld-telemetry listening on 127.0.0.1, and
points claude's user settings at it ($CLAUDE_CONFIG_DIR/settings.json, by
default ~/.claude/settings.json). The collector sends traces, metrics and logs
to --local, such as the JetBrains OpenTelemetry plugin in the IDE, and metrics
only to --remote, such as a team's collector; give one of them, or both.

Running it again replaces the collector and rewrites the settings; the env keys
cld does not manage stay as they are. claude reads its settings as a session
starts: sessions running then keep theirs. Needs Docker; Linux only.`

// telemetryCommand is cld setup telemetry, named in its messages as typed, and its options as
// cobra reads them.
type telemetryCommand struct {
	typed                                string
	local, remote, port, collectorConfig string
}

// setupTelemetry is cld setup telemetry (decision 18), which off Linux refuses to run (see
// linuxOnly).
func setupTelemetry(typed string) *cobra.Command {
	t := &telemetryCommand{typed: typed}
	only := linuxOnly{typed: typed, supported: telemetry.Supported}
	command := &cobra.Command{
		Use:   "telemetry [--local URL] [--remote URL] [flags]",
		Short: "send claude's telemetry through a local OpenTelemetry collector",
		Long:  telemetryLong,
		Args:  only.arguments,
		RunE:  t.run,
	}
	command.SetFlagErrorFunc(only.flagError)
	flags := command.Flags()
	flags.StringVar(&t.local, "local", "", "where traces, metrics and logs go, such as\n"+
		"http://127.0.0.1:4319 (the IDE's plugin on a\n"+
		"fixed port); `URL` is http://HOST:PORT for\n"+
		"plaintext gRPC or https://HOST:PORT for TLS")
	flags.StringVar(&t.remote, "remote", "", "where metrics go too (`URL` as for --local), such\n"+
		"as https://otel.example.com:4317")
	flags.StringVar(&t.port, "port", "", "the `PORT` the collector listens on, on 127.0.0.1\n"+
		"(default: the one the collector has, or one\n"+
		"the kernel picks)")
	flags.StringVar(&t.collectorConfig, "collector-config", "",
		"`FILE` holds YAML that the collector merges over\n"+
			"cld's config, such as headers or TLS for the\n"+
			"exporters otlp_grpc/local and otlp_grpc/remote")
	return command
}

// run is setup telemetry's RunE, which refuses a mistake in the options before anything runs.
func (t *telemetryCommand) run(c *cobra.Command, _ []string) error {
	var options telemetry.Options
	if err := t.endpoints(c, &options); err != nil {
		return err
	}
	if c.Flags().Changed("port") {
		var ok bool
		if options.Port, ok = telemetry.ParsePort(t.port); !ok {
			return fail.Usage(fmt.Sprintf("invalid port '%s': a number from 1 to 65535 "+
				"(see cld help)", t.port))
		}
	}
	if options.Local == nil && options.Remote == nil {
		return fail.Usage(t.typed + ": --local URL, --remote URL or both are needed " +
			"(see cld help)")
	}
	if option := options.Collector(options.Port); options.Port != 0 && option != "" {
		return fail.Usage(fmt.Sprintf("%s is where the collector would listen (--port %d): "+
			"give the receiver's port, or another --port (see cld help)", option, options.Port))
	}
	if c.Flags().Changed("collector-config") {
		options.CollectorConfig = t.collectorConfig
		if options.CollectorConfig == "" {
			return fail.Usage("option '--collector-config' needs a value (see cld help)")
		}
	}
	return telemetry.Setup(options)
}

// endpoints sets the endpoints of options from --local and --remote, where given.
func (t *telemetryCommand) endpoints(c *cobra.Command, options *telemetry.Options) error {
	for _, endpoint := range []struct {
		name, url string
		into      **telemetry.Endpoint
	}{{"local", t.local, &options.Local}, {"remote", t.remote, &options.Remote}} {
		if !c.Flags().Changed(endpoint.name) {
			continue
		}
		parsed, ok := telemetry.ParseEndpoint(endpoint.url)
		if !ok {
			return fail.Usage(fmt.Sprintf("invalid URL '%s' for --%s: http://HOST:PORT or "+
				"https://HOST:PORT (see cld help)", endpoint.url, endpoint.name))
		}
		*endpoint.into = &parsed
	}
	return nil
}
