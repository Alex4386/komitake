package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Alex4386/komitake/internal/wireless"
	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/skip2/go-qrcode"
	"github.com/spf13/cobra"
)

func newPairCommand(opts *options) *cobra.Command {
	var (
		qrPath      string
		keepQRFile  bool
		noQR        bool
		showSecrets bool
	)

	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Pair a new kart using a scannable QR code",
		Long: "Puts the daemon into pairing mode and renders the pairing QR code in the\n" +
			"terminal. Scan it with the kart to join. Returns to normal mode when\n" +
			"pairing completes, is canceled, or times out.",
		GroupID: groupDevice,
		Args:    cobra.NoArgs,
		Example: `  komitake pair
  komitake pair --qr-file /tmp/pair.png
  komitake pair --qr-file /tmp/pair.png --keep-qr-file
  komitake pair --no-qr`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), pairingCommandTimeout)
			defer cancel()

			ep, err := resolveEndpoint(opts)
			if err != nil {
				return err
			}
			client, conn, err := adminDial(ctx, ep)
			if err != nil {
				return daemonError(ep, err)
			}
			defer func() { _ = conn.Close() }()

			if _, err := client.SetState(ctx, &adminv1.SetStateRequest{State: adminv1.State_STATE_PAIRING}); err != nil {
				return daemonError(ep, err)
			}
			defer bestEffortRestoreRunning(client)

			resp, err := client.GetPairingInfo(ctx, &adminv1.GetPairingInfoRequest{})
			if err != nil {
				return daemonError(ep, err)
			}
			if resp.Pairing == nil {
				return errors.New("daemon entered pairing mode but returned no pairing info")
			}
			if opts.jsonOutput {
				return writeJSON(cmd, resp)
			}

			seed, err := hex.DecodeString(resp.Pairing.SeedHex)
			if err != nil {
				return fmt.Errorf("daemon returned an invalid pairing seed: %w", err)
			}
			// switchbrew: pairing QR channel is "Usually 0" (lp2p default).
			// The live AP still uses resp.Pairing.Channel (1/6/11); do not
			// pin the kart to that channel via the QR.
			payload, err := wireless.BuildPairingQRCodePayload(seed, resp.Pairing.Ssid, 0)
			if err != nil {
				return err
			}
			qr, err := qrcode.NewWithForcedVersion(string(payload), 4, qrcode.Medium)
			if err != nil {
				return err
			}

			u := opts.ui

			// Render in the terminal by default. Previously the QR only reached a
			// temp file and required a keypress to open in an image viewer.
			if !noQR && u.tty {
				u.Println()
				u.Println(qr.ToSmallString(false))
			}

			u.Field(7, "ssid", "%s", resp.Pairing.Ssid)
			u.Field(7, "channel", "%d", resp.Pairing.Channel)
			if showSecrets {
				u.Field(7, "seed", "%s", resp.Pairing.SeedHex)
			}

			if qrPath != "" {
				if err := writeQRFile(qr, qrPath); err != nil {
					return err
				}
				u.Field(7, "qr file", "%s", qrPath)
				if !keepQRFile {
					// The PNG is equivalent to the pairing key; remove it when the
					// session ends unless the operator asked to keep it.
					defer removeQRFile(qrPath)
				}
			}

			u.Println()
			u.Printf("%s\n", u.paint(u.c.dim, "scan the code with your kart, Ctrl-C to cancel"))

			return awaitPairing(ctx, u, client, ep)
		},
	}

	cmd.Flags().StringVar(&qrPath, "qr-file", "", "also write the QR code to a PNG file")
	cmd.Flags().BoolVar(&keepQRFile, "keep-qr-file", false, "keep --qr-file after pairing ends (default: delete it)")
	cmd.Flags().BoolVar(&noQR, "no-qr", false, "do not render the QR code in the terminal")
	cmd.Flags().BoolVar(&showSecrets, "show-secrets", false, "print the pairing seed, which is equivalent to the network key")
	return cmd
}

// awaitPairing polls daemon state until pairing resolves. The daemon has no
// event stream, so polling is the only option available.
func awaitPairing(ctx context.Context, u *ui, client adminv1.AdminServiceClient, ep endpoint) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	sp := u.Spinner("waiting for the kart to join")
	defer sp.Stop()

	for {
		select {
		case <-ctx.Done():
			sp.StopFail()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return errors.New("pairing timed out after 2m")
			}
			return errors.New("pairing canceled")
		case <-ticker.C:
		}

		// Derive from ctx so Ctrl-C interrupts the in-flight poll too.
		pollCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		state, err := client.GetState(pollCtx, &adminv1.GetStateRequest{})
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				continue
			}
			sp.StopFail()
			return daemonError(ep, err)
		}

		switch state.State {
		case adminv1.State_STATE_PAIRING:
			// Keep spinning.
		case adminv1.State_STATE_RUNNING:
			sp.StopSuccess("pairing complete")
			return nil
		default:
			sp.StopFail()
			return fmt.Errorf("pairing ended with daemon in state %s", stateToMode(state.State))
		}
	}
}
