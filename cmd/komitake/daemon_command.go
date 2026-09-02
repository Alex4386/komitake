package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/Alex4386/komitake/internal/daemon"
	"github.com/Alex4386/komitake/internal/ipc"
	"github.com/Alex4386/komitake/internal/logging"
	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
)

// errDaemonRestart signals the daemon exited to be restarted by its supervisor.
// It surfaces as a non-zero exit so systemd (Restart=always) starts a fresh
// instance.
var errDaemonRestart = errors.New("daemon restart requested")

func newDaemonCommand(opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run the Komitake daemon in the foreground",
		Long: "Runs the daemon in the foreground, serving the admin socket that the other\n" +
			"commands use. Requires root for wireless and DHCP setup.\n\n" +
			"Use -v for per-session detail, or -vv to trace every protocol message.",
		GroupID: groupDaemon,
		Args:    cobra.NoArgs,
		Example: `  komitake daemon
  komitake daemon -v
  komitake daemon -vv --log-format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// The logger is already built by PersistentPreRunE from -v and the
			// --log-* flags.
			logger := opts.log

			rt, err := config.Load(opts.configPath, config.Options{
				ConfigPath:  opts.configPath,
				Interface:   opts.interfaceIF,
				Listen:      opts.listen,
				PairingFile: opts.pairingFile,
			})
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			listener, cleanup, err := listenAdmin(rt.Listen, rt.SocketPerm, rt.HasSocketPerm, logger)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			manager := daemon.NewManager(rt)
			if err := manager.Start(ctx); err != nil {
				logger.Error("daemon failed to start", "error", err)
				_ = manager.Close()
				return err
			}
			defer manager.Close()

			server := grpc.NewServer()
			adminv1.RegisterAdminServiceServer(server, ipc.NewDaemonService(manager))

			errCh := make(chan error, 1)
			go func() {
				errCh <- server.Serve(listener)
			}()

			select {
			case <-ctx.Done():
				logger.Info("received shutdown signal")
				server.GracefulStop()
				return nil
			case <-manager.RestartRequested():
				// A restart was requested over the admin API. Exit non-zero so a
				// process supervisor (systemd Restart=always) starts a fresh
				// instance with the updated config.
				logger.Info("restart requested; exiting for supervisor to restart")
				server.GracefulStop()
				return errDaemonRestart
			case err := <-errCh:
				server.Stop()
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
				logger.Error("grpc server stopped with error", "error", err)
				return err
			}
		},
	}

	cmd.Flags().StringVar(&opts.configPath, "config", "", "path to config.json")
	cmd.Flags().StringVar(&opts.interfaceIF, "interface", "", "wireless interface name override")
	cmd.Flags().StringVar(&opts.pairingFile, "pairing-file", "", "override pairing.json path")
	cmd.Flags().StringVar(&opts.listen, "listen", "", "admin API address: unix:/path or host:port; TCP is unauthenticated, use only on a trusted network")
	_ = cmd.MarkFlagFilename("config", "json")

	return cmd
}

// listenAdmin opens the admin-API listener described by the parsed IPC address.
// It returns a cleanup func to run on shutdown. A unix address gets the
// socket-specific handling (stale-socket check, owner-only mode); a tcp address
// is warned about because the API is unauthenticated.
func listenAdmin(
	addr config.IPCAddress,
	configuredPerm os.FileMode,
	hasConfiguredPerm bool,
	logger *logging.Logger,
) (net.Listener, func(), error) {
	if addr.Network == "unix" {
		path := addr.Address
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, err
		}
		// Refuse to steal a socket that another daemon is serving. The old
		// unconditional RemoveAll let a second instance silently take over.
		if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
			_ = conn.Close()
			return nil, nil, fmt.Errorf("another komitake daemon is already listening on %s", path)
		}
		if err := os.RemoveAll(path); err != nil {
			return nil, nil, err
		}

		ln, err := net.Listen("unix", path)
		if err != nil {
			if errors.Is(err, os.ErrPermission) {
				return nil, nil, fmt.Errorf("cannot create the admin socket at %s: %w\n"+
					"  the daemon needs root to configure wireless and DHCP", path, err)
			}
			return nil, nil, err
		}

		mode := config.DefaultSocketPerm
		if hasConfiguredPerm {
			mode = configuredPerm & 0o777
		}
		if err := os.Chmod(path, mode); err != nil {
			_ = ln.Close()
			return nil, nil, fmt.Errorf("failed to set socket permissions: %w", err)
		}
		logger.Info("daemon socket ready", "path", path, "mode", fmt.Sprintf("%04o", mode))
		return ln, func() { _ = os.Remove(path) }, nil
	}

	ln, err := net.Listen("tcp", addr.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot listen on tcp %s: %w", addr.Address, err)
	}
	// A TCP endpoint is unauthenticated and can reveal the pairing seed
	// (equivalent to the network key), so it is a real exposure. Warn always.
	logger.Warn("admin API exposed over TCP without authentication; restrict to a trusted network",
		"address", ln.Addr().String())
	return ln, func() {}, nil
}
