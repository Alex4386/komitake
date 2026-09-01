package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/Alex4386/komitake/internal/logging"
	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/spf13/cobra"
)

// version is overridable at link time:
//
//	go build -ldflags "-X main.version=v1.2.3"
var version = "dev"

type options struct {
	configPath string
	// address is the client-side admin API address override: "unix:/path" or
	// "host:port". Empty means discover it from config or use the default.
	address string
	// listen is the daemon-side admin API address override.
	listen      string
	jsonOutput  bool
	noColor     bool
	interfaceIF string
	pairingFile string

	// verbose counts -v occurrences: 1 is debug, 2 or more is trace.
	verbose int
	// logLevel and logFormat are explicit overrides for -v.
	logLevel  string
	logFormat string

	ui  *ui
	log *logging.Logger
}

// resolveLevel maps the verbosity flags to a level. An explicit --log-level
// always wins over -v so scripted invocations are unambiguous.
func (o *options) resolveLevel() (slog.Level, error) {
	if o.logLevel != "" {
		return logging.ParseLevel(o.logLevel)
	}
	switch {
	case o.verbose >= 2:
		return logging.LevelTrace, nil
	case o.verbose == 1:
		return slog.LevelDebug, nil
	default:
		return slog.LevelInfo, nil
	}
}

type adminCloser interface {
	Close() error
}

var adminDial = dialAdmin

const (
	pairingCommandTimeout = 2 * time.Minute
	defaultCallTimeout    = 10 * time.Second
)

// Command groups keep `--help` readable as the command count grows.
const (
	groupDaemon = "daemon"
	groupDevice = "device"
	groupConfig = "config"
)

// withClient centralizes dialing, teardown, timeouts, and error translation so
// each command body only contains its own RPC and rendering logic.
func (o *options) withClient(cmd *cobra.Command, timeout time.Duration, fn func(context.Context, adminv1.AdminServiceClient) error) error {
	ep, err := resolveEndpoint(o)
	if err != nil {
		return err
	}
	log := o.logger().With("command", cmd.Name())

	log.Debug("connecting to daemon", "endpoint", ep.String(), "timeout", timeout)
	started := time.Now()

	client, conn, err := adminDial(cmd.Context(), ep)
	if err != nil {
		log.Debug("dial failed", "endpoint", ep.String(), "error", err)
		return daemonError(ep, err)
	}
	defer func() { _ = conn.Close() }()

	ctx := cmd.Context()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if err := fn(ctx, client); err != nil {
		log.Debug("rpc failed", "elapsed", time.Since(started), "error", err)
		return daemonError(ep, err)
	}
	log.Debug("rpc completed", "elapsed", time.Since(started))
	return nil
}

// logger returns the resolved logger, falling back to the default when
// PersistentPreRunE has not run, such as in unit tests.
func (o *options) logger() *logging.Logger {
	if o.log == nil {
		return logging.New(nil)
	}
	return o.log
}

func New() *cobra.Command {
	opts := &options{}

	cmd := &cobra.Command{
		Use:   "komitake",
		Short: "Connect a Fuji kart to your PC over Wi-Fi",
		Long: "Brings up the LP2P access point the kart joins, pairs it, and lets you " +
			"drive, read telemetry, and view the live camera from the PC.",
		Example: `  komitake daemon
  komitake set --wireless-interface=wlan0
  komitake pair
  komitake status
  komitake devices
  komitake video
  komitake web`,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Resolve output styling and logging once, before any subcommand runs.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			opts.ui = newUI(cmd.OutOrStdout(), cmd.ErrOrStderr())
			if opts.noColor {
				opts.ui.color = false
				opts.ui.c = plain
			}

			level, err := opts.resolveLevel()
			if err != nil {
				return err
			}
			format, err := logging.ParseFormat(opts.logFormat)
			if err != nil {
				return err
			}

			// Logs go to stderr so they never mix into stdout, which may be
			// piped or parsed as JSON.
			opts.log = logging.NewLogger(cmd.ErrOrStderr(), logging.Options{
				Level:  level,
				Format: format,
			})
			slog.SetDefault(opts.log.Logger)
			return nil
		},
	}

	cmd.SetVersionTemplate("komitake {{.Version}}\n")

	flags := cmd.PersistentFlags()
	flags.StringVar(&opts.address, "address", "", "daemon admin API address: unix:/path or host:port (default unix:"+config.DefaultSocketPath+")")
	flags.BoolVar(&opts.jsonOutput, "json", false, "emit JSON instead of formatted text")
	flags.BoolVar(&opts.noColor, "no-color", false, "disable colored output")
	flags.CountVarP(&opts.verbose, "verbose", "v", "increase log verbosity (-v debug, -vv trace)")
	flags.StringVar(&opts.logLevel, "log-level", "", "explicit log level (trace, debug, info, warn, error); overrides -v")
	flags.StringVar(&opts.logFormat, "log-format", "text", "log output format (text, json)")

	_ = cmd.RegisterFlagCompletionFunc("log-level",
		cobra.FixedCompletions(logging.LevelNames(), cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("log-format",
		cobra.FixedCompletions([]string{"text", "json"}, cobra.ShellCompDirectiveNoFileComp))

	cmd.AddGroup(
		&cobra.Group{ID: groupDaemon, Title: "Daemon commands:"},
		&cobra.Group{ID: groupDevice, Title: "Device commands:"},
		&cobra.Group{ID: groupConfig, Title: "Configuration commands:"},
	)

	cmd.AddCommand(
		newDaemonCommand(opts),
		newStatusCommand(opts),
		newSetCommand(opts),
		newPairCommand(opts),
		newDevicesCommand(opts),
		newVideoCommand(opts),
		newWebCommand(opts),
		newCompletionCommand(),
	)

	return cmd
}
