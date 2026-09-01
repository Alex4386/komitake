package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/Alex4386/komitake/internal/config"
	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// endpoint is the parsed admin-API address a client dials.
type endpoint = config.IPCAddress

// resolveEndpoint finds where the daemon's admin API is. Clients do not read
// the full config; they only need to locate the IPC address. Precedence:
// --address flag, then socket.bind (or legacy listen) from --config,
// then a discovered default config, then the built-in default.
func resolveEndpoint(opts *options) (endpoint, error) {
	if opts.address != "" {
		return config.ParseIPCAddress(opts.address)
	}
	if opts.configPath != "" {
		return config.ParseIPCAddress(config.ResolveListenAddress("", opts.configPath))
	}
	for _, candidate := range config.DefaultConfigCandidates {
		if _, err := os.Stat(candidate); err == nil {
			return config.ParseIPCAddress(config.ResolveListenAddress("", candidate))
		}
	}
	return config.ParseIPCAddress("")
}

func dialAdmin(ctx context.Context, ep endpoint) (adminv1.AdminServiceClient, adminCloser, error) {
	// grpc.NewClient replaces the deprecated DialContext/WithBlock pair. It does
	// no I/O, so connection failures surface on the first RPC instead of here.
	network := ep.Network
	conn, err := grpc.NewClient(
		"passthrough:///"+ep.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		}),
	)
	if err != nil {
		return nil, nil, err
	}
	return adminv1.NewAdminServiceClient(conn), conn, nil
}

// daemonError converts a transport failure into actionable guidance. The old
// implementation labelled every RPC error "daemon not running", which hid real
// server-side failures such as an invalid state transition.
func daemonError(ep endpoint, err error) error {
	if err == nil {
		return nil
	}

	st, ok := status.FromError(err)
	if !ok {
		return err
	}

	switch st.Code() {
	case codes.Unavailable:
		if ep.Network == "tcp" {
			return fmt.Errorf("cannot reach the daemon at %s: %s\n"+
				"  check the host is up, the port is right, and the daemon is listening on tcp",
				ep, st.Message())
		}
		if _, statErr := os.Stat(ep.Address); errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("daemon is not running (no socket at %s)", ep.Address)
		}
		return fmt.Errorf("cannot reach the daemon at %s: %s\n"+
			"  check that it is running and that you have permission to use the socket",
			ep.Address, st.Message())
	case codes.PermissionDenied:
		return fmt.Errorf("permission denied on %s\n"+
			"  your user needs access to the socket (check its owner/group and mode)", ep)
	case codes.DeadlineExceeded:
		return fmt.Errorf("the daemon did not respond in time")
	case codes.NotFound:
		return errors.New(st.Message())
	case codes.Canceled:
		return errors.New("canceled")
	default:
		return errors.New(st.Message())
	}
}

func writeJSON(cmd *cobra.Command, value any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
